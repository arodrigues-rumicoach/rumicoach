package chat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/aiusage"
	"github.com/rumi/rumi-be/internal/services/badge"
	"github.com/rumi/rumi-be/internal/services/balance"
	"github.com/rumi/rumi-be/internal/services/chat/providers"
	"github.com/rumi/rumi-be/internal/services/chat/session/vision"
	"go.uber.org/zap"
)

// Control markers the model emits inline to drive screen reveals and pauses.
//
// The model emits a two-glyph SYMBOL marker (command glyph + parameter glyph) rather than the
// old word-like bracket tags ([SS=memories], [P=2]), because the native-audio model occasionally
// vocalized the readable tags. The glyphs are common Geometric-Shapes/Block-Elements characters:
// reliably reproduced by the model, but not spoken by the audio model, and the adjacency pair
// makes false positives effectively impossible.
//
//	SCREEN: ◆ + parameter glyph — ◆▣ memories, ◆▤ session
//	PAUSE:  ● + a lower-block glyph whose height encodes the seconds — ●▁=1s … ●█=8s
//
// Screen reveals are limited to data-less screens (memories, session). Data-bearing screens
// like the Wheel of Life are NEVER opened with a screen marker — they are shown by the tool
// that carries their data (set_wheel_of_life_categories → wheel_of_life_update), otherwise the
// screen would open empty.
//
// The legacy [SS=...] / [P=N] tags are still parsed as a fallback (the audio for those can't be
// fixed after the fact, but we still strip them and run the same logic).
var (
	pauseRegex        = regexp.MustCompile(`(?i)\[P=(\d+)\]`)
	screenRegex       = regexp.MustCompile(`(?i)\[SS=([a-zA-Z0-9_-]+)\]`)
	screenSymbolRegex = regexp.MustCompile("◆([▣▤▥▦▧▨])") // ◆ + ▣|▤|▥|▦|▧|▨
	pauseSymbolRegex  = regexp.MustCompile("●([▁-█])")    // ● + ▁..█
)

// screenSymbolToName maps a screen parameter glyph to its screen name. Only data-less screens
// belong here (see the note above). Add a glyph + entry to support a new such screen.
var screenSymbolToName = map[string]string{
	"▣": "memories",                     // ▣
	"▤": "session",                      // ▤
	"▥": "journey",                      // ▥
	"▦": sessionSummaryMarkerScreen,     // ▦
	"▧": sessionSummaryNextMarkerScreen, // ▧
	"▨": "profile",                      // ▨ — the intro tour's profile walk-through
}

// sessionSummaryMarkerScreen and sessionSummaryNextMarkerScreen are the pseudo
// screen-names the ◆▦ / ◆▧ markers parse to. Neither is a real navigable screen —
// deliberately absent from showScreenAllowed, so the show_screen TOOL can never produce
// either — only these markers can, keeping the session-summary reveal audio-synced like
// any other marker instead of a tool call the model could fire at an arbitrary moment
// (see scheduleScreenReveal). ▦ reveals the panel without its next-session card, at the
// start of the Vision closing's synthesis; ▧ later adds that card, at the goodbye's
// bridge sentence — see emitAndPersistSessionSummary.
const (
	sessionSummaryMarkerScreen     = "session_summary"
	sessionSummaryNextMarkerScreen = "session_summary_next"
)

// pauseSymbolToSeconds maps a lower-block glyph to its pause duration (block height == seconds).
var pauseSymbolToSeconds = map[string]int{
	"▁": 1, "▂": 2, "▃": 3, "▄": 4,
	"▅": 5, "▆": 6, "▇": 7, "█": 8,
}

var (
	genericBraceRegex = regexp.MustCompile(`(?i)\b(?:call\s*:\s*)?[a-zA-Z0-9_]+\s*\{[^{}]*\}`)
	specificToolRegex = regexp.MustCompile(`(?i)\b(?:call\s*:\s*)?(save_memory|update_wheel_of_life|save_focus|complete_current_task|terminate_session|save_session_insight|save_ideal_life_vision|set_wheel_of_life_categories|init_eisenhower_matrix|update_eisenhower_matrix|update_commitment_plan)\s*(?:\([^)]*\)|\[[^\]]*\]|\{[^}]*\})`)
	openBraceRegex    = regexp.MustCompile(`(?i)\b[a-zA-Z0-9_]+\s*\{[^}]*$`)
	openToolRegex     = regexp.MustCompile(`(?i)\b(?:call\s*:\s*)?(save_memory|update_wheel_of_life|save_focus|complete_current_task|terminate_session|save_session_insight|save_ideal_life_vision|set_wheel_of_life_categories|init_eisenhower_matrix|update_eisenhower_matrix|update_commitment_plan)\s*[\(\{\[][^)\}\]]*$`)
	// leakedToolEchoRegex strips a leaked tool-call/response echo the model occasionally
	// speaks, e.g. "response:save_memory{message:...,status:success" or "save_focus{area:...}".
	// The brace content is frequently left UNCLOSED and runs straight into the real speech, so
	// we anchor the end on the tool-response terminator (status:<word>) when present, otherwise
	// the first closing brace — preventing the match from swallowing the genuine coaching text.
	leakedToolEchoRegex = regexp.MustCompile(`(?i)(?:\b(?:response|result|output|call)\s*:\s*)?(?:save_memory|update_wheel_of_life|save_focus|complete_current_task|terminate_session|save_session_insight|save_ideal_life_vision|set_wheel_of_life_categories|init_eisenhower_matrix|update_eisenhower_matrix|update_commitment_plan|save_commitments|schedule_notifications)\s*\{[^}]*?(?:status\s*:\s*(?:success|error|ignored)|\})`)
	// unterminatedToolCallRegex is the last-resort cleaner for a tool call written out as text
	// whose brace is NEVER closed and has no status terminator (QA: "call:save_ideal_life_vision
	// {vision:A Filipa visualiza...voluntariadoFico feliz..."). The leaked arguments run straight
	// into whatever the model says next with no reliable boundary, so the whole tail is dropped.
	// Applied AFTER the terminated-call cleaners, so a properly closed call followed by genuine
	// speech keeps that speech.
	unterminatedToolCallRegex = regexp.MustCompile(`(?is)\b(?:call\s*:\s*)?(?:save_memory|update_wheel_of_life|save_focus|complete_current_task|terminate_session|save_session_insight|save_ideal_life_vision|set_wheel_of_life_categories|init_eisenhower_matrix|update_eisenhower_matrix|update_commitment_plan)\s*[\(\{\[].*$`)
	// leakedTransitionToolRegex detects a STATE-ADVANCING tool call written out as plain text in
	// the model's spoken output instead of being invoked natively. Nothing executes, the state
	// never advances, and the model loops on the same phase questions. Structural syntax only —
	// language-agnostic.
	leakedTransitionToolRegex = regexp.MustCompile(`(?i)\b(?:call\s*:\s*)?(save_ideal_life_vision|save_focus|complete_current_task|terminate_session|set_wheel_of_life_categories|update_wheel_of_life)\s*[\(\{\[]`)
	// leakedVisionArgRegex captures the vision argument text out of a leaked
	// save_ideal_life_vision call, so a repeat offense can be auto-saved server-side.
	leakedVisionArgRegex = regexp.MustCompile(`(?is)save_ideal_life_vision\s*[\(\{\[]\s*"?vision"?\s*[:=]\s*(.+)$`)
	// visionGlueCutRegex finds where leaked vision text glues onto the model's next spoken
	// sentence: a sentence terminator directly followed by an uppercase letter (".Pode"), or a
	// lowercase letter directly followed by an uppercase one ("voluntariadoFico").
	visionGlueCutRegex = regexp.MustCompile(`[.!?…]\p{Lu}|\p{Ll}\p{Lu}`)
)

// systemErrorEchoRegex strips Gemini's own error annotations (e.g. "[SYSTEM ERROR: Invalid
// function call. ...]") that the Live API occasionally emits into the output text stream when
// the model attempts a malformed/undeclared function call. They must never reach the user.
var systemErrorEchoRegex = regexp.MustCompile(`\[SYSTEM ERROR:[^\]]*\]`)

// scaffoldTokenRegex matches internal scaffold tokens the model occasionally leaks into
// its spoken text (QA: "RESPONSE_AFTER_TOOL_USE" spoken aloud mid-sentence). ALL-CAPS
// words joined by underscores never occur in genuine coaching speech, so stripping them
// from the user-facing transcript is safe. The audio itself cannot be unspoken — this
// keeps the on-screen transcript and history clean.
var scaffoldTokenRegex = regexp.MustCompile(`\b[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+\b`)

// knownScaffoldWordRegex catches lowercase scaffold tokens the ALL-CAPS pattern misses.
// Only verbatim-observed leaks belong here ("notranslation", QA 2026-07-29) — each entry
// must be a string that cannot occur in genuine speech in any supported language.
var knownScaffoldWordRegex = regexp.MustCompile(`(?i)\bnotranslation\b`)

// hallucinatedMarkerRegex matches the ◆ screen-marker command glyph followed by plain ASCII
// letters instead of one of the real glyphs (▣▤▥) — the model inventing a marker like
// "◆tasks" for a screen it has no real glyph for (QA 2026-07-29: spoken at the end of
// nearly every turn in the Vision closing, where show_screen was declared but no script
// used it). A real marker is always ◆ + a single non-ASCII glyph, so this can never match one.
var hallucinatedMarkerRegex = regexp.MustCompile(`◆[a-zA-Z_]+`)

// stripScaffoldTokens removes leaked scaffold tokens while preserving the server's own
// processed-marker annotations ([PROCESSED_PAUSE: N] / [PROCESSED_SCREEN]), which QA
// relies on in saved transcripts.
func stripScaffoldTokens(text string) string {
	text = scaffoldTokenRegex.ReplaceAllStringFunc(text, func(m string) string {
		if m == "PROCESSED_PAUSE" || m == "PROCESSED_SCREEN" {
			return m
		}
		return ""
	})
	text = knownScaffoldWordRegex.ReplaceAllString(text, "")
	return hallucinatedMarkerRegex.ReplaceAllString(text, "")
}

func CleanLeakedToolCalls(text string) string {
	// Strip leaked (possibly unclosed) tool-call/response echoes first, before the closed-brace
	// cleaners, so an unterminated "tool{...,status:success" prefix is removed cleanly.
	text = systemErrorEchoRegex.ReplaceAllString(text, "")
	text = leakedToolEchoRegex.ReplaceAllString(text, "")
	text = genericBraceRegex.ReplaceAllString(text, "")
	text = specificToolRegex.ReplaceAllString(text, "")
	text = unterminatedToolCallRegex.ReplaceAllString(text, "")
	text = stripScaffoldTokens(text)
	text = regexp.MustCompile(`[ \t]+`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(` \n|\n `).ReplaceAllString(text, "\n")
	text = regexp.MustCompile(`\n+`).ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

func CleanLeakedToolCallsRaw(text string) string {
	text = leakedToolEchoRegex.ReplaceAllString(text, "")
	text = genericBraceRegex.ReplaceAllString(text, "")
	text = specificToolRegex.ReplaceAllString(text, "")
	text = unterminatedToolCallRegex.ReplaceAllString(text, "")
	return text
}

// leakedTransitionToolName returns the name of the first state-advancing tool the model wrote
// out as text in the given raw turn text, or "" when none.
func leakedTransitionToolName(raw string) string {
	if m := leakedTransitionToolRegex.FindStringSubmatch(raw); len(m) > 1 {
		return strings.ToLower(m[1])
	}
	return ""
}

// extractLeakedVision recovers the vision summary from a leaked (usually unterminated)
// "save_ideal_life_vision{vision: ..." text so the server can save it itself. Cut points, in
// order: the closing brace when present, otherwise the glue point where the leaked argument
// runs into the model's next spoken sentence. Returns "" when nothing usable is found.
func extractLeakedVision(raw string) string {
	m := leakedVisionArgRegex.FindStringSubmatch(raw)
	if len(m) < 2 {
		return ""
	}
	arg := m[1]
	if i := strings.Index(arg, "}"); i != -1 {
		arg = arg[:i]
	} else if loc := visionGlueCutRegex.FindStringIndex(arg); loc != nil {
		// Keep the first rune of the two-rune glue match (the sentence terminator or the
		// final lowercase letter of the argument).
		_, size := utf8.DecodeRuneInString(arg[loc[0]:])
		arg = arg[:loc[0]+size]
	}
	arg = strings.TrimSpace(arg)
	if utf8.RuneCountInString(arg) < 40 {
		return "" // too short to be a real vision summary — do not auto-save garbage
	}
	if runes := []rune(arg); len(runes) > 1000 {
		arg = string(runes[:1000])
	}
	return arg
}

// processedPauseRegex matches the [PROCESSED_PAUSE: N] placeholders left in the transcript
// where a pause marker was parsed; N seconds of injected silence play at that point.
var processedPauseRegex = regexp.MustCompile(`\[PROCESSED_PAUSE:\s*(\d+)\]`)

// estimatedSpeechCharsPerSecond approximates how many transcript characters the native-audio
// voice speaks per second (measured ≈15–16 chars/s across pt-PT and English sessions). Used
// to estimate a marker's audio offset when timing screen reveals.
const estimatedSpeechCharsPerSecond = 15.0

// screenRevealLeadSeconds opens a marker-driven screen this many seconds ahead of the
// marker's estimated position in the speech, so the screen is already on display while
// the sentence announcing it is being spoken.
const screenRevealLeadSeconds = 2.0

// visionMarkerCorrective steers the model back when it emits a screen marker in the Ideal
// Life Vision phase instead of calling save_ideal_life_vision — it tends to then improvise
// the Wheel of Life intro and scoring questions aloud, off-state and off-script.
// dupSpeechCorrective interrupts a turn in which the model is re-delivering text it
// already spoke (QA: the full Wheel of Life intro spoken twice inside one 67-second
// turn). Injected prompts barge in on the generation, cutting the repeat before it is
// fully heard.
const dupSpeechCorrective = "[SYSTEM] You are REPEATING text you already spoke in this same turn — the user is hearing the same sentences twice. Stop the repetition NOW. Do not apologize and do not start the passage again. Continue exactly from where the FIRST delivery ended: if this step requires a tool call (such as set_wheel_of_life_categories), make that call silently now; otherwise finish the script's single next sentence and stop."

// dupSpeechWindowRunes is the length of the verbatim block that must recur within one
// turn before the repetition tripwire fires. Coaching scripts never legitimately repeat
// this much text inside a single turn, so the check is safe to run at parse time.
const dupSpeechWindowRunes = 80

// hasWithinTurnRepetition reports whether the last dupSpeechWindowRunes runes of the
// turn's accumulated transcript (whitespace-normalized) already occurred earlier in it.
func hasWithinTurnRepetition(text string) bool {
	norm := strings.Join(strings.Fields(text), " ")
	r := []rune(norm)
	if len(r) < dupSpeechWindowRunes*2 {
		return false
	}
	tail := string(r[len(r)-dupSpeechWindowRunes:])
	return strings.Contains(string(r[:len(r)-dupSpeechWindowRunes]), tail)
}

const visionMarkerCorrective = "[SYSTEM] You just output a screen marker, which does NOTHING in this phase, and you are NOT allowed to start the next exercise yourself — do NOT ask the user to score or rate any area of their life; the current-reality assessment is introduced automatically by the system and any scoring question you asked is void. NEVER acknowledge this correction aloud: do NOT apologize, and do NOT mention screens, markers, or anything technical — the user heard none of it, and an apology about 'screens I haven't shown you' is pure confusion to them (QA). What you say next depends ONLY on where the conversation stands: IF THE USER HAS NOT YET ANSWERED the question you last asked them (including the opening 'describe that life to me'), output NOTHING or at most one soft, brief re-invitation to take their time — do NOT ask any new question, their turn to speak is still open (QA: the model fired follow-up questions before the user had said a single word, and they had to protest to start over). If they HAVE been answering: unless they have already answered at least two or three genuine follow-up questions about their vision, the exploration is NOT finished — ask your next open follow-up (who is with them, how their days feel, what matters most in that life) and WAIT; a single answer, however rich, is never the whole picture. ONLY once the exploration is genuinely complete, the ONLY way to advance is calling the 'save_ideal_life_vision' tool (silently, no extra speech — you already gave your acknowledgment) with a concise summary of everything they described. Never output a screen marker again in this phase."

type ChatSession struct {
	UserID                    string
	User                      *models.User
	ClientWs                  *websocket.Conn
	GeminiWs                  *websocket.Conn
	SessionDB                 models.CommunicationSession
	Location                  *time.Location
	logger                    *zap.Logger
	provider                  providers.GeminiProvider
	setupDone                 chan struct{}
	setupOnce                 sync.Once
	geminiMutex               sync.RWMutex
	latestSessionHandle       string
	accumulatedText           string
	sentCleanText             string
	SessionType               api.SessionType
	historyMutex              sync.Mutex
	history                   []string
	accumulatedUserTranscript string
	currentPartialTranscript  string
	accumulatedPartsText      string
	conversationSummary       string
	pendingRestart            bool
	lastRestartAt             time.Time
	forcedRestartInjectedAt   time.Time
	restartInstructions       string
	stopChan                  chan struct{}
	notificationsDone         chan struct{}
	pendingShutdown           bool
	// tokens accumulates what this connection has spent on the Live model, per
	// modality. In memory rather than on SessionDB: token counts belong to the cost
	// ledger (ai_usage_records), which is the only place they can be summed against
	// messages and background generations. Reset by InitDB, so each half of a
	// handover accounts for itself.
	tokens sessionTokens

	// balanceExempt marks the session as free (no minutes debit at cleanup).
	// Onboarding is never billed. Captured once at session start from the
	// server-side user state — never from the client-supplied session_type.
	balanceExempt bool

	pendingTransitionPrompt      string
	NeedsDynamicTransitionPrompt bool
	// wheelSetupThisTurn marks that set_wheel_of_life_categories executed in the current
	// turn; combined with turnSpokeText it detects the model creating the wheel WITHOUT
	// speaking the introduction script. Reset in the TURN_COMPLETE handler.
	wheelSetupThisTurn bool
	// revealsScheduledThisTurn records screens whose reveal is already scheduled in the
	// current turn, deduplicating the show_screen tool against the ◆ marker for the same
	// screen. Cleared at TURN_COMPLETE and on interrupt.
	revealsScheduledThisTurn map[string]bool
	// visionCorrectiveSentThisTurn marks that the vision-phase marker corrective was already
	// injected mid-turn (barging in on the improvised generation), so the TURN_COMPLETE
	// fallback must not inject it a second time. Reset in the TURN_COMPLETE handler.
	visionCorrectiveSentThisTurn bool
	// visionCorrectiveIssued is the sticky (session-lifetime) record that a vision-marker
	// corrective fired at some point. The wheel-entry restart directive uses it to skip the
	// "Thank you for sharing that" opening bridge — the user already heard an acknowledgment
	// (possibly cut mid-sentence), and a second thank-you sounds like a stutter-restart.
	visionCorrectiveIssued bool
	// userAwaitingReplySince marks the last moment user speech was transcribed WITHOUT the
	// model having produced any content (audio, transcript, or tool call) since. Zero when
	// the model is responding or nothing is pending. Watched by monitorDeadAir: after an
	// interruption Gemini sometimes stops generating entirely — the user repeats "are you
	// there?" into total silence and eventually gives up (QA) — and no TURN_COMPLETE safety
	// net can fire because no turn ever starts.
	userAwaitingReplySince time.Time
	// deadAirNudges counts watchdog nudges for the current unanswered wait (capped so a
	// truly dead connection doesn't accumulate a backlog of prompts).
	deadAirNudges int
	// bareScoreRejectedArea records the wheel area whose bare-score save was already
	// rejected once, so the model's retry for that area is never blocked (a legitimately
	// terse user must not get stuck in a rejection loop).
	bareScoreRejectedArea string
	// userMsgsSinceWheelSave counts user utterances since the last successful
	// update_wheel_of_life. The bare-score guard uses it to tell reasoning-before-score
	// ("...it's been a challenge" → "a six") apart from a bare score whose preceding long
	// message belonged to the PREVIOUS area.
	userMsgsSinceWheelSave int
	// wheelIntroSpokenEarlier records that the wheel introduction was delivered in an
	// earlier turn of this connection (the spoke-but-no-tool corrective fired). The
	// first-area cue must then NOT use the missing-intro recovery variant — re-delivering
	// the intro a third time is exactly the repetition QA flagged.
	wheelIntroSpokenEarlier bool
	// dupSpeechCorrectiveSentThisTurn dedupes the within-turn repetition tripwire.
	dupSpeechCorrectiveSentThisTurn bool
	// saveCommitmentsConfirmRejected implements the reject-once confirmation gate on
	// save_commitments (language-agnostic; see handleSaveActions).
	saveCommitmentsConfirmRejected bool
	// visionCopyRejected implements the reject-once gate against saving a vision copied
	// verbatim from the profile placeholder (see handleSaveIdealLifeVision).
	visionCopyRejected bool
	// visionUserTurns counts the user's spoken turns while in VISION_IDEAL_LIFE. The
	// exploration phase mandates two-to-three follow-up questions, but the model saved the
	// vision right after the user's very first answer (QA: "não houve nenhum follow-up...
	// perdemos um follow-up muito rico"), so handleSaveIdealLifeVision rejects a save that
	// arrives before at least one follow-up has been answered.
	visionUserTurns int
	// visionEarlySaveRejected makes that rejection fire once only, so a resumed session
	// (whose counter restarted) can still advance after a single extra follow-up.
	visionEarlySaveRejected bool
	// pendingSummaryReveal defers the ◆▦ session-summary reveal to TURN_COMPLETE. The
	// marker opens the synthesis turn, so the estimated-position schedule fired the panel
	// the instant Rumi STARTED speaking — forcing the user to read and listen at once
	// (QA: "não dá para fazer a leitura enquanto ouvimos ao mesmo tempo"). TURN_COMPLETE
	// arrives when the synthesis has effectively been heard (Gemini paces audio at
	// ≈realtime), which is exactly when the panel should appear. ◆▧ keeps its estimated
	// timing — it lands mid-goodbye, tied to the "one question still lingers" sentence.
	pendingSummaryReveal bool
	// closingQuestionWaitRejected implements the reject-once gate against completing the
	// emotional closing in the same turn the model asked a question (see
	// handleCompleteCurrentTask).
	closingQuestionWaitRejected bool
	// sessionSummaryEmitted and sessionSummaryNextSessionSent each guard one stage of
	// emitAndPersistSessionSummary against firing twice — the panel reveals in two steps
	// for a staged session (Vision): first without the next-session card (as the AI starts
	// its synthesis), then with it added (once the AI actually reaches the goodbye's bridge
	// sentence). See emitAndPersistSessionSummary for the full picture.
	sessionSummaryEmitted         bool
	sessionSummaryNextSessionSent bool
	// leakedToolAttempts counts turns in which the model wrote a state-advancing tool call
	// as plain text instead of invoking it natively. The first offense gets a corrective;
	// in the vision phase a repeat offense auto-saves the vision server-side (extracted
	// from the leak itself) so the session can never loop forever in that phase.
	leakedToolAttempts int
	// Onboarding emotional-closing guard: after save_memory(insight) the model must still
	// deliver the permission question, the personalized synthesis, and the clarity check
	// (≥2 spoken turns) before complete_current_task is accepted; an early call is
	// rejected exactly once (closingEarlyCompleteRejected) so the session can always end.
	closingInsightSaved          bool
	closingTurnsAfterInsight     int
	closingEarlyCompleteRejected bool
	// Deaf-connection detection state (monitorGeminiLiveness), guarded by geminiMutex.
	// loudAudioSecsSinceGemini accumulates the seconds of AUDIBLE user audio forwarded to
	// Gemini since Gemini last sent us any message; lastClientAudioAt is when the last
	// audio chunk arrived (the client only streams while the user speaks, so a gap means
	// the user finished). A healthy Gemini transcribes within a couple of seconds of the
	// user FINISHING — it may stay legitimately silent DURING a long monologue (pt-PT
	// transcriptions arrive at turn end), which is why the detector must never count
	// in-progress speech alone (QA: it restarted mid-answer five times in one session).
	// livenessRestarts caps forced reconnects per session: past that, the detector is more
	// likely wrong than the connection.
	loudAudioSecsSinceGemini float64
	lastClientAudioAt        time.Time
	livenessRestarts         int
	// In-session (non-restart) state transition: used for every transition except
	// entering/leaving the Wheel of Life. The directive + the next task's full
	// instructions are injected into the live Gemini session instead of restarting it.
	pendingInSessionTransition   bool
	inSessionTransitionDirective string
	turnHasToolCall              bool
	// turnSpokeText records whether the model emitted ANY spoken text during the current
	// turn — including text already flushed to the client before a mid-turn tool call (which
	// resets accumulatedText). The silent-tool-call nudge checks this, not the leftover text,
	// so a turn that spoke and then called a tool is not mistaken for a silent one.
	turnSpokeText bool
	// pendingRestartSpoke captures whether the model had spoken in the turn that triggered a
	// pending restart (e.g. the vision outro before save_ideal_life_vision). It is read when the
	// post-restart directive is injected, to decide whether a 1s silence is needed so the
	// pre-restart audio and the new turn's audio don't sound glued together.
	pendingRestartSpoke  bool
	pendingToolResponses []map[string]interface{}

	// clientGone is set when the client WebSocket has disconnected and the end-of-session flow
	// (scheduled notifications, review) is running. The Gemini connection is still alive for those
	// closing tool calls, but there is no user anymore — so conversation-continuation nudges
	// must not fire.
	clientGone bool
	// transitionInjectedThisTurn is set when HandleToolCall injects an in-session transition
	// directive for the current model turn. The TURN_COMPLETE handler must then NOT stack the
	// generic silent-tool nudge on top of it (the two would give conflicting guidance).
	transitionInjectedThisTurn bool
	// suppressedMarkerThisTurn records that a screen-marker reveal was suppressed during the
	// current turn because the state forbids it (e.g. the model leaked ◆▣ in the Ideal Life
	// Vision conclusion instead of calling save_ideal_life_vision). The TURN_COMPLETE handler
	// uses it to inject a corrective prompt instead of leaving the user in dead air.
	suppressedMarkerThisTurn bool
	// restartFallbackPrompt stashes the transition instructions that were displaced by the
	// 7-minute restart flow (InjectTransitionPrompt's restart branch injects restartPrompt
	// INSTEAD of them). If the model fails to actually call restart_session_with_summary,
	// the next TURN_COMPLETE falls back to injecting these so the session continues instead
	// of dying in silence. Cleared once consumed or once a restart actually begins.
	restartFallbackPrompt string
	// Audio-sync tracking: used to delay screen transitions until the audio
	// playback reaches the point in the speech where the [SS=...] tag appeared.
	turnAudioDuration  float64
	turnAudioStartTime time.Time
	interruptCounter   int64
	// measuredCharsPerSecond is the observed speech rate for THIS session, measured at
	// TURN_COMPLETE from real turns (transcript length / received audio duration) and
	// blended across turns. The static estimatedSpeechCharsPerSecond default drifts with
	// language, voice, and pacing — over the intro tour's long four-marker turn the
	// accumulated error pushed later reveals seconds off their sentences (QA: the Talk
	// screen opened noticeably late). Zero until the first measurement lands.
	measuredCharsPerSecond float64
	// dobTodayRejected records that an age-derived date of birth was already bounced once
	// this session, so a user genuinely born on today's day and month is not stuck in a
	// loop — their second attempt is accepted (see handleSaveProfileDetails).
	dobTodayRejected bool
	// terminateNoSpeechBounced records that a model-initiated terminate_session with no
	// goodbye spoken in its turn was already rejected once this session, so the retry is
	// always accepted and a stubborn model can never soft-lock the shutdown (see
	// handleTerminateSession).
	terminateNoSpeechBounced bool
	// lastUserSpeechUnixNano is when the most recent inputTranscription event arrived —
	// the only trustworthy "the user is talking right now" signal for the delayed
	// silent-tool nudge. Client mic-audio frames CANNOT be used for this: the client
	// streams audio continuously even in silence (QA: every 10s window carried exactly
	// 117 frames / 319410 bytes regardless of speech), so a frame-arrival timestamp is
	// always fresh and cancelled every single nudge, leaving users in dead air after
	// each silent tool call. (Atomic: written by the Gemini read loop, read from the
	// nudge goroutine.)
	lastUserSpeechUnixNano atomic.Int64
	// silentNudgeSeq invalidates a scheduled silent-tool nudge once a newer turn
	// completes: incremented on every TURN_COMPLETE, captured at scheduling time, and
	// compared before firing (see scheduleSilentToolNudge).
	silentNudgeSeq int
	// pendingShutdownDelaySecs is estimateSpeechRemainingSecs's estimate for the turn
	// currently being dispatched, stashed so the pendingShutdown handler (checked at the
	// bottom of the read loop) can hold session_terminated until the goodbye has had time
	// to actually finish playing on the client — see the comment at the pendingShutdown
	// check for why this cannot simply send immediately.
	pendingShutdownDelaySecs float64

	geminiWriteMutex sync.Mutex
	clientWriteMutex sync.Mutex
}

func NewChatSession(userID string, sessionType api.SessionType, clientWs *websocket.Conn, loc *time.Location, logger *zap.Logger) *ChatSession {
	return &ChatSession{
		UserID:            userID,
		SessionType:       sessionType,
		ClientWs:          clientWs,
		Location:          loc,
		logger:            logger.With(zap.String("user_id", userID)),
		provider:          providers.NewGeminiProvider(),
		setupDone:         make(chan struct{}),
		stopChan:          make(chan struct{}),
		notificationsDone: make(chan struct{}),
	}
}

// activeSessions tracks the single active ChatSession per user. A user must never have two
// live sessions at once: duplicate client connections (e.g. a frontend double-mount) would
// otherwise create two Gemini sessions speaking over each other, and a half-closed client
// would leave a zombie session running after the user left.
var (
	activeSessionsMu sync.Mutex
	activeSessions   = make(map[string]*ChatSession)
)

// registerActiveSession makes s the user's active session, terminating any previous one.
func registerActiveSession(s *ChatSession) {
	activeSessionsMu.Lock()
	prev := activeSessions[s.UserID]
	activeSessions[s.UserID] = s
	activeSessionsMu.Unlock()

	if prev != nil && prev != s {
		s.logger.Warn("User already has an active chat session; terminating the previous one",
			zap.String("previous_session_id", prev.SessionDB.ID))
		// Closing the client socket unblocks the previous session's read loop, which runs its
		// normal end-of-session flow (as if the client disconnected).
		if prev.ClientWs != nil {
			prev.ClientWs.Close()
		}
	}
}

// unregisterActiveSession removes s from the registry, unless a newer session took over.
func unregisterActiveSession(s *ChatSession) {
	activeSessionsMu.Lock()
	if activeSessions[s.UserID] == s {
		delete(activeSessions, s.UserID)
	}
	activeSessionsMu.Unlock()
}

func (s *ChatSession) Run() {
	defer s.Cleanup()

	registerActiveSession(s)
	defer unregisterActiveSession(s)

	// 1. Load user from DB for session context
	var user models.User
	if err := database.DB.Where("id = ?", s.UserID).First(&user).Error; err != nil {
		s.logger.Warn("Could not load user for session initialization", zap.Error(err))
	}
	s.User = &user

	// Resolve and set session type on ChatSession
	s.resolveSessionType()

	// 2. Initialize the communication session in DB
	s.InitDB()

	// Send session ID to client
	// session_type is the SERVER-resolved type (resolveSessionType ran above), so the
	// client can label the conversation screen with the session actually being held —
	// the type it requested may have been resolved to something else.
	if err := s.writeClientJSON(map[string]interface{}{
		"type":         "session_created",
		"session_id":   s.SessionDB.ID,
		"session_type": string(s.SessionType),
	}); err != nil {
		s.logger.Error("Failed to send session_created message to client", zap.Error(err))
	}

	// Auto-advance transient closing states if the session is not a quick restart (> 10s).
	// This prevents starting a brand-new session with a goodbye speech. The threshold matches
	// the Gemini-handle resume window below so the two stay consistent.
	if user.State != nil {
		currentState := *user.State
		isClosingState := models.SessionState(currentState).IsEndingSession() || models.SessionState(currentState).IsEmotionalClosing()
		isStaleSession := user.LatestSessionHandleAt == nil || time.Since(*user.LatestSessionHandleAt) > 10*time.Second

		if isClosingState && isStaleSession {
			// A stale session left in a closing state advances to the CHECKIN resting state.
			newState := string(models.StateCheckin)
			user.State = &newState
			database.DB.Model(&models.User{}).Where("id = ?", s.UserID).Update("state", newState)
			s.logger.Info("Auto-advanced stale closing state",
				zap.String("previous_state", currentState),
				zap.String("new_state", newState))
		}
	}

	// Free only if this is one of the introductory sessions AND that pair has not yet
	// produced what it exists to produce. Anything else — a check-in, a deep session —
	// comes out of the balance whatever state the account is in.
	//
	// s.SessionType is safe to pass because resolveSessionType ran above (line 539) and
	// replaced whatever the client asked for with what the server decided. Passing the
	// client's value would let a client name a free session type at will.
	//
	// Judged once, here, and held for the life of the session: the intro writes the
	// profile details part-way through and Vision writes the vision at its end, so
	// re-asking mid-session would start billing a session that began free.
	//
	// A failed check bills the session. That is the safe direction here, unlike the
	// WebSocket pre-flight: the worst case is a debit against a user who was owed a free
	// session, which the ledger records and support can reverse, whereas exempting on
	// error would hand out unlimited free sessions to anyone who could make the query fail.
	free, err := balance.FreeSessionAvailable(context.Background(), s.UserID, string(s.SessionType))
	if err != nil {
		s.logger.Error("Free-session check failed, billing session", zap.Error(err),
			zap.String("userID", s.UserID))
	}
	s.balanceExempt = free

	// Check if we have a valid recent session handle in the DB to resume across browser reloads.
	// Only resume the same Gemini session if the user reconnected within 10 seconds; after that a
	// user-started session is treated as a brand-new Gemini session. (Kept short so a user reset
	// or an ended session doesn't get "ghost-resumed" into a stale Gemini context.)
	isResumed := false
	if user.LatestSessionHandle != nil && user.LatestSessionHandleAt != nil {
		if time.Since(*user.LatestSessionHandleAt) < 10*time.Second {
			s.latestSessionHandle = *user.LatestSessionHandle
			s.logger.Info("Found valid session handle in DB, resuming cross-device/reload", zap.String("handle", s.latestSessionHandle))
			isResumed = true
		}
	}

	// 3. Connect to Gemini Live API via the selected provider
	conn, err := s.provider.Connect(s.logger)
	if err != nil {
		s.logger.Error("Gemini Connection Error", zap.Error(err), zap.String("provider", s.provider.Name()))
		s.ClientWs.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(1011, "Gemini Connect Failed"))
		return
	}
	s.GeminiWs = conn

	// 4. Setup initial config (model, voice, system prompt, tools)
	s.SendInitialConfig(&user)

	// 5. Start the Gemini→Client proxy early so it can receive setupComplete
	errc := make(chan error, 2)
	go s.proxyGeminiToClient(errc)
	go s.monitorForcedRestart()
	go s.monitorDeadAir()
	go s.monitorGeminiLiveness()

	// 6. Wait for setupComplete before sending content
	s.logger.Info("Waiting for setupComplete from Gemini...")
	select {
	case <-s.setupDone:
		s.logger.Info("Setup complete, proceeding with initial prompt")
	case err := <-errc:
		s.logger.Error("Gemini connection failed during setup", zap.Error(err))
		s.ClientWs.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(1011, "Gemini Setup Failed"))
		return
	}

	if isResumed {
		// The Gemini session was resumed with its full context, so we must NOT re-greet or
		// re-deliver the task (that would duplicate the greeting and re-run visual side-effects).
		// But native audio never speaks on its own, so without a nudge the user reconnects to dead
		// air. Inject a light re-engagement directive that makes the model pick the conversation
		// back up in one sentence.
		s.logger.Info("Resumed session detected. Injecting a light re-engagement nudge.")
		s.InjectPrompt("The user has just reconnected to your ongoing session. In ONE short, warm sentence, gently pick the conversation back up — re-ask the question you last asked them, or invite them to continue where you left off. Do NOT greet them again, re-introduce yourself, repeat your previous message in full, restart the current exercise, or output any screen marker.")
	} else {
		// Brand-new session: the full active-task instructions are already in Section 9 of the
		// system prompt (built during setup), so we only inject a short trigger that starts
		// generation and asks the model to greet by name — re-sending the task here would
		// duplicate it and re-run its visual side-effects.
		displayName := "User"
		if user.Name != nil && *user.Name != "" {
			names := strings.Fields(*user.Name)
			if len(names) > 0 {
				displayName = names[0]
			}
		}
		initialTrigger := fmt.Sprintf("Greet the user warmly by name (%s), then begin the session now by delivering the first step of your CURRENT ACTIVE TASK INSTRUCTIONS (Section 9 of your system instructions) exactly as written. Do not wait for the user to speak.", displayName)

		// Reconnecting into a partially-completed Wheel of Life must NOT replay the task
		// from the top: "deliver the first step" makes the model re-deliver the whole wheel
		// introduction (and re-call set_wheel_of_life_categories) on every app restart. No
		// dialogue summary exists on client reconnects, so the wheel template's "IF
		// RESUMING" clause never applies — steer the model to the first pending area instead.
		if user.State != nil && models.SessionState(*user.State) == models.StateVisionWheelOfLife {
			if items := s.loadOnboardingWheelItems(&user); len(items) > 0 {
				initialTrigger = fmt.Sprintf("Greet the user warmly by name (%s) in ONE short welcome-back sentence, without re-introducing yourself. %s", displayName, vision.WheelReconnectPrompt(items))
			}
		}
		s.InjectPrompt(initialTrigger)
	}

	// 8. Start client→Gemini proxy (Gemini→Client is already running)
	go s.proxyClientToGemini(errc)

	select {
	case err := <-errc:
		// The client is gone from here on: the Gemini connection stays up only for the
		// closing housekeeping (scheduled notifications). Conversation nudges must not fire.
		s.geminiMutex.Lock()
		s.clientGone = true
		s.geminiMutex.Unlock()

		if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) ||
			strings.Contains(err.Error(), "use of closed network connection") || strings.Contains(err.Error(), "client disconnected") {
			s.logger.Info("Chat Session ended", zap.String("close_reason", err.Error()))
			// Clear session handle if ended intentionally
			s.latestSessionHandle = ""
			database.DB.Model(s.User).Updates(map[string]interface{}{
				"latest_session_handle":    nil,
				"latest_session_handle_at": nil,
			})
		} else {
			s.logger.Error("Chat Session Terminated due to error", zap.Error(err))
		}

		// Inject notifications prompt before final closure, but only if the session lasted > 10 seconds
		if time.Since(s.SessionDB.StartTime) > 10*time.Second {
			s.logger.Info("Requesting notifications from AI...")

			notificationsPrompt := `[SYSTEM COMMAND] The user has left the session. Please analyze the entire conversation we just had using your full native memory of the interaction. Call the 'schedule_notifications' tool to generate personalized notifications to be sent over the next few days. Generate only the notifications that make sense and will be genuinely useful and actionable for the user based on our specific discussion. In the SAME tool call, also set 'quote_category' (the category of daily quotes that will serve the user best in the coming days) and 'theme' (the visual theme for their Journey screen matching the emotional tone of the session) — choose both from the allowed values, based on the conversation and the user's current state.`

			// After the Vision session, add conversion-focused guidance so the notifications
			// gently bridge toward the paid Commitment Plan session. It is Vision — not the short
			// intro — that produces the vision, priority area and insight this copy leans on.
			if s.SessionType == api.SessionTypeSessionVision {
				notificationsPrompt += `

ADDITIONAL GUIDANCE FOR FIRST SESSION (IDEAL LIFE VISION):
This was the user's first real coaching session and they have not yet subscribed. Your notifications should:
1. Reference the user's specific vision, priority area, and key insight from today's session to make them feel seen and remembered.
2. Include at least one notification that creates a natural bridge toward their next step — the Commitment Plan session — where we will answer the question "what has been stopping you?" and build a concrete plan. Frame it as an invitation, not a sales pitch (e.g., "Your commitment plan is waiting — ready to take the next step?").
3. Remind them of the insight or breakthrough moment they had today so the emotional value of the session stays with them.
4. Keep all notifications warm, personal, and rooted in what they actually shared — never generic.`
			}

			s.InjectPrompt(notificationsPrompt)

			// Wait for the tool call
			waitCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			select {
			case <-s.notificationsDone:
				s.logger.Info("Notifications scheduled successfully before cleanup")
			case <-waitCtx.Done():
				s.logger.Warn("Timed out waiting for notifications tool call")
			case <-errc:
				// If Gemini connection fully drops while waiting
				s.logger.Warn("Gemini connection dropped while waiting for notifications")
			}
		} else {
			s.logger.Info("Session duration less than 1 minute, skipping notifications")
		}

		return
	}
}

func (s *ChatSession) InitDB() {
	sessionTypeStr := string(s.SessionType)
	s.SessionDB = models.CommunicationSession{
		UserID:      s.UserID,
		StartTime:   time.Now(),
		Language:    s.User.PreferredLanguage,
		SessionType: &sessionTypeStr,
	}
	// A handover opens a second row on the same connection, and each row has to
	// account for its own spend: the intro's tokens were already written to the cost
	// ledger by rolloverSessionDB, so carrying them forward would bill them twice.
	s.tokens = sessionTokens{}
	database.DB.Create(&s.SessionDB)
}

// rolloverSessionDB closes the current communication_sessions row and opens a fresh one
// for s.SessionType, which the caller has already switched to the session being handed
// over to. Called at the start_planned_session handover (intro → Vision, check-in → the
// planned deep session) so one connection leaves one row per session actually held.
// Without the split a single row spanned both halves under the FIRST type: the journey
// gates read typed, ended rows (SessionCountsAsDone), so the second half never registered
// as done — after intro→Vision the journey kept proposing Vision, and a deep session done
// through the check-in gateway stayed an overlong "checkin" forever.
func (s *ChatSession) rolloverSessionDB() {
	old := s.SessionDB
	if old.ID == "" {
		s.InitDB()
		return
	}

	// Finalize the ended half exactly as Cleanup finalizes the connection's final row:
	// transcript, end time, duration, and the in-memory token counters via Save.
	s.flushUserTranscript()
	historyLog := s.getHistoryLog()
	if historyLog != "" {
		old.Transcript = &historyLog
	}
	endTime := time.Now()
	old.EndTime = &endTime
	elapsed := int64(endTime.Sub(old.StartTime).Seconds())
	old.Duration = int(elapsed)
	if err := database.DB.Save(&old).Error; err != nil {
		s.logger.Error("Failed to finalize session row at planned-session handover",
			zap.Error(err), zap.String("session_id", old.ID))
	}

	// What the ended half cost us, for the same reason its minutes are debited below:
	// Cleanup only ever accounts for the row that is current at close, so the tokens
	// the intro spent before handing over to Vision would otherwise reach the session
	// row and never the cost ledger. InitDB below starts a fresh SessionDB, so these
	// counters belong to this half alone and are not double-counted at close.
	aiusage.Write(context.Background(), aiusage.Record{
		UserID:            s.UserID,
		Kind:              models.AIUsageSession,
		Model:             liveModelName(),
		RefType:           models.AIUsageRefSession,
		RefID:             old.ID,
		InputTokens:       s.tokens.Input,
		OutputTokens:      s.tokens.Output,
		TotalTokens:       s.tokens.Total,
		InputTextTokens:   s.tokens.InputText,
		OutputTextTokens:  s.tokens.OutputText,
		InputAudioTokens:  s.tokens.InputAudio,
		OutputAudioTokens: s.tokens.OutputAudio,
		InputVideoTokens:  s.tokens.InputVideo,
		OutputVideoTokens: s.tokens.OutputVideo,
	})

	// Debit the ended half now: Cleanup only debits the row that is current at close,
	// so a paid session handed over from (a check-in) would otherwise never be charged
	// for its pre-handover minutes. Same rules as Cleanup: log-and-continue, and the
	// unique session_id ledger index makes a double debit impossible.
	if s.balanceExempt {
		// Same as Cleanup: the handed-over half was free, and saying so explicitly is
		// what keeps it visible in the user's own usage history.
		if _, err := balance.RecordFreeSession(context.Background(), s.UserID, old.ID, old.SessionType); err != nil {
			s.logger.Error("Failed to record free handed-over session", zap.Error(err),
				zap.String("session_id", old.ID))
		}
	} else if _, err := balance.DebitSession(context.Background(), s.UserID, old.ID, old.SessionType, elapsed); err != nil {
		s.logger.Error("Failed to debit handed-over session usage", zap.Error(err),
			zap.String("session_id", old.ID), zap.Int64("elapsed_seconds", elapsed))
	}

	oldType := api.SessionType("")
	if old.SessionType != nil {
		oldType = api.SessionType(*old.SessionType)
	}
	if historyLog != "" && elapsed > 10 {
		s.runPostSessionAnalysis(old.ID, oldType, s.getCleanHistoryLog())
	}

	// The new row starts a new transcript; the Gemini connection restarts anyway, so no
	// in-memory reader needs the pre-handover entries.
	s.historyMutex.Lock()
	s.history = nil
	s.historyMutex.Unlock()

	// Each row is judged on its own, by the same two conditions as a fresh session.
	// Intro → Vision stays exempt: the type is right and the ideal-life vision is still
	// missing, which is exactly what the session about to start is for. A check-in
	// handing over to a deep session is billable on the type alone.
	//
	// s.SessionType is the planned session the caller already set (tasks.go), chosen
	// server-side from the journey — not a client request.
	//
	// Re-read rather than reusing the flag from session start: the intro's write lands
	// between the two checks, and this row is a different session that has to answer
	// for itself. Same failure direction as the initial check: an error bills rather
	// than exempts.
	free, err := balance.FreeSessionAvailable(context.Background(), s.UserID, string(s.SessionType))
	if err != nil {
		s.logger.Error("Free-session check failed at handover, billing session",
			zap.Error(err), zap.String("userID", s.UserID))
	}
	s.balanceExempt = free

	s.InitDB()
	s.logger.Info("Session row rolled over at planned-session handover",
		zap.String("ended_session_id", old.ID), zap.String("ended_type", string(oldType)),
		zap.String("new_session_id", s.SessionDB.ID), zap.String("new_type", string(s.SessionType)))
}

// liveModelName is the Live model this connection speaks to, tolerant of a config
// that was never loaded. Metering runs during session teardown and must not be able
// to panic there; an empty name records the tokens with a NULL cost, which is the
// honest reading for an environment that never said which model it was using.
func liveModelName() string {
	if config.AppConfig == nil {
		return ""
	}
	return config.AppConfig.GeminiLiveModel
}

func (s *ChatSession) resolveSessionType() {
	var sessionTypeStr string
	if s.User != nil {
		state := s.CurrentState()
		switch state {
		case models.StateOnboardingIntro, models.StateLegacyOnboarding:
			sessionTypeStr = string(api.SessionTypeOnboarding)
		case models.StateVisionIdealLife:
			// If the frontend explicitly asks for the onboarding intro, honor it,
			// because VISION_IDEAL_LIFE is the resting default state for new accounts.
			if s.SessionType == api.SessionTypeOnboarding {
				sessionTypeStr = string(api.SessionTypeOnboarding)
			} else {
				sessionTypeStr = string(api.SessionTypeSessionVision)
			}
		case models.StateVisionWheelOfLife,
			models.StateVisionMetaphor,
			models.StateVisionEmotionalClosing,
			models.StateVisionEndingSession:
			// These are all part of the Vision session. Even if the user just
			// finished the onboarding intro and is parked at VISION_IDEAL_LIFE,
			// they must be served by the vision session package, as the onboarding
			// package no longer contains instructions for this state.
			sessionTypeStr = string(api.SessionTypeSessionVision)
		case models.StateCheckin:
			// The resting state. The opening pair (onboarding intro, Vision) is done once
			// and never revisited, so a client still asking for either here is stale — QA:
			// the app opened session_type=session_vision at CHECKIN, the session fell back
			// to the check-in prompt but stayed TYPED session_vision, which (a) skipped the
			// PlannedSessionForToday lookup so the check-in never offered the Movement
			// session that was due, (b) would have recorded a >5-minute chat as a completed
			// Vision deep session, pushing the journey gates forward, and (c) keys
			// buildSessionSummary into emitting a Vision synthesis panel for a check-in.
			// A specific LATER deep session (movement, values, ...) launched directly by
			// the app is still honored — availability was already decided by the growth
			// screen that offered it.
			//
			// Unless the opening pair never actually finished, which CHECKIN does not
			// prove: terminate_session parks a Vision session the user cut short at
			// CHECKIN, leaving the ideal-life vision unwritten. Such a user is proposed
			// Vision by the journey (journey.ProposeSession asks the same question), so
			// resolving them to a check-in here made the app offer Vision and the server
			// serve something else — and, because the allowance follows the artifacts,
			// serve it for free. Route them back to the session that is actually owed.
			if s.User.NeedsProfileDetails() || s.User.IdealLifeVisionSetAt == nil {
				sessionTypeStr = string(api.SessionTypeSessionVision)
				break
			}
			switch s.SessionType {
			case api.SessionTypeOnboarding, api.SessionTypeSessionVision, "":
				sessionTypeStr = string(api.SessionTypeCheckin)
			default:
				if _, ok := sessions.Get(s.SessionType); ok {
					sessionTypeStr = string(s.SessionType)
				} else {
					sessionTypeStr = string(api.SessionTypeCheckin)
				}
			}
		}
	}

	if sessionTypeStr == "" {
		if s.SessionType != "" {
			sessionTypeStr = string(s.SessionType)
		} else {
			sessionTypeStr = string(api.SessionTypeSessionVision)
		}
	}
	s.SessionType = api.SessionType(sessionTypeStr)
}

// CurrentState returns the active session state. It applies an override for new users:
// since fresh accounts default to VISION_IDEAL_LIFE in the database, if the frontend
// explicitly requests the optional onboarding intro (s.SessionType == onboarding),
// this returns StateOnboardingIntro so the intro instructions run correctly.
func (s *ChatSession) CurrentState() models.SessionState {
	if s.User == nil || s.User.State == nil {
		if s.SessionType == api.SessionTypeOnboarding {
			return models.StateOnboardingIntro
		}
		return models.StateVisionIdealLife
	}

	state := models.SessionState(*s.User.State)
	if s.SessionType == api.SessionTypeOnboarding && state == models.StateVisionIdealLife {
		return models.StateOnboardingIntro
	}
	return state
}

func (s *ChatSession) LoadLastMonthContext() ([]models.UserMemory, []models.Commitment, []models.WheelOfLifeExercise, []models.EisenhowerMatrixExercise) {
	return LoadLastMonthContext(s.UserID, s.logger)
}

// LoadLastMonthContext loads the user's recent memories, commitments, wheel-of-life
// and Eisenhower exercises — the cross-session context rendered into system
// prompts. Shared by the live session and the companion (WhatsApp) channel.
func LoadLastMonthContext(userID string, logger *zap.Logger) ([]models.UserMemory, []models.Commitment, []models.WheelOfLifeExercise, []models.EisenhowerMatrixExercise) {
	oneMonthAgo := time.Now().AddDate(0, -1, 0)

	var memories []models.UserMemory
	if err := database.DB.Where("user_id = ? AND created_at >= ?", userID, oneMonthAgo).Order("created_at desc").Find(&memories).Error; err != nil {
		logger.Warn("Failed to load user memories", zap.Error(err))
	}

	var wheels []models.WheelOfLifeExercise
	if err := database.DB.Where("user_id = ? AND (created_at >= ? OR updated_at >= ?)", userID, oneMonthAgo, oneMonthAgo).Order("updated_at desc").Find(&wheels).Error; err != nil {
		logger.Warn("Failed to load wheel of life exercises", zap.Error(err))
	}

	var eisenhowers []models.EisenhowerMatrixExercise
	if err := database.DB.Where("user_id = ? AND created_at >= ?", userID, oneMonthAgo).Order("created_at desc").Find(&eisenhowers).Error; err != nil {
		logger.Warn("Failed to load eisenhower matrix exercises", zap.Error(err))
	}

	// Recent commitments (plan, manual, and behavior commitments) plus any still-open
	// one-time commitment. Without these, the "first step" and "today's proof" a session
	// like Movement captures would be invisible to the next session's context.
	var commitments []models.Commitment
	if err := database.DB.Where("user_id = ? AND (created_at >= ? OR (type = ? AND done = ?))",
		userID, oneMonthAgo, "one_time", false).
		Order("created_at asc").Limit(50).Find(&commitments).Error; err != nil {
		logger.Warn("Failed to load commitments", zap.Error(err))
	}

	return memories, commitments, wheels, eisenhowers
}

func (s *ChatSession) SendInitialConfig(user *models.User) {
	s.geminiMutex.Lock()
	defer s.geminiMutex.Unlock()
	s.sendInitialConfigLocked(user)
}

func (s *ChatSession) sendInitialConfigLocked(user *models.User) {
	// (Assumes caller holds s.geminiMutex if needed, but since it's called on Connect/Reconnect
	// it's either before threads start or inside the reconnect lock)
	memories, commitments, wheels, eisenhowers := s.LoadLastMonthContext()

	voiceName := "gacrux" // Default overall
	if user != nil {
		if user.CoachVoice != nil && *user.CoachVoice != "" {
			voiceName = *user.CoachVoice
		} else if user.CoachGender != nil && *user.CoachGender == "male" {
			voiceName = "charon"
		}
	}

	currentState := s.CurrentState()

	systemInstruction := BuildSystemInstruction(s.SessionType, user, memories, commitments, wheels, eisenhowers, s.Location)

	// Append active task instructions to system instructions
	taskInstructions := s.GetNextInstructions(user)
	if taskInstructions != "" {
		systemInstruction += fmt.Sprintf("\n\n### 9. CURRENT ACTIVE TASK INSTRUCTIONS\nYou MUST follow these instructions for the current stage of the coaching session:\n%s\n", taskInstructions)
	}

	summary := s.conversationSummary
	if summary != "" {
		// The wording here must not push the model backwards. An earlier version said
		// "Do NOT skip any steps", which — for a single-state session whose Section 9 is
		// the complete script — read as an order to re-walk phases the summary showed
		// were already done, and QA heard the session repeat itself from the middle.
		systemInstruction += fmt.Sprintf("\n\n### 8. SUMMARY OF RECENT DIALOGUE (PRE-RESTART CONTEXT)\nHere is a detailed summary of what was discussed earlier in this session before we took a brief pause. Treat this as the immediate context of our conversation and resume smoothly.\n%s\n\nCURRENT SESSION STATE: %s\nYou MUST continue the session in the state '%s'. This summary is the authoritative record of progress: every phase, step, or question it shows as already covered is DONE — you are STRICTLY FORBIDDEN from repeating it, re-asking its questions, or re-delivering its scripted lines, even though your task instructions (Section 9) contain the full script from the beginning. Resume from the first step NOT yet covered (the PROGRESS line in the summary tells you exactly where that is), and do not jump ahead of it either.\n", summary, string(currentState), string(currentState))
	}

	s.logger.Info("Sending initial configuration with system instruction", zap.String("system_instruction", systemInstruction))
	s.logHistory("System Instruction", systemInstruction)

	// The restart tool must ALWAYS be declared. The trigger prompt is only injected by
	// shouldTriggerRestart (7-minute rule), and handleRestartSessionWithSummary itself rejects
	// calls made too soon after a restart. Gating the DECLARATION here (as previously done for
	// 5 minutes after a restart) created a deadly mismatch: a connection created by the wheel
	// restart would never declare the tool, so when the 7-minute trigger later fired on that
	// same connection, the model was ordered to call a tool Gemini didn't know — producing an
	// "Invalid function call" error and leaving the session in dead air.
	allowRestart := true

	var statePtr *models.SessionState
	if s.User != nil {
		state := s.CurrentState()
		statePtr = &state
	}
	tools := ToolsForSession(s.SessionType, allowRestart, statePtr)

	// Optional STT language hint (experimental, off by default): native-audio models
	// document language detection as automatic, but QA sessions in pt-PT keep getting
	// user turns transcribed as Spanish/Italian/Korean, which then derails the model
	// ("the four" → language-constraint gibberish). The flag lets QA trial the hint
	// without risking every session on an unsupported setup field.
	languageCode := ""
	if config.AppConfig.GeminiSpeechLanguageHint && s.User != nil && s.User.PreferredLanguage != nil {
		languageCode = *s.User.PreferredLanguage
	}

	// How long the user may fall silent before Gemini decides their turn is over. The
	// provider default is tuned for quick back-and-forth and cuts people off mid-thought:
	// QA watched a user pause to search for words and be talked over by the very cue that
	// was meant to give him room ("take your time..."). The phases built around silent
	// reflection get visibly more room than a normal conversational beat. Setup is rebuilt
	// on every connection — including the Wheel of Life restart — so this tracks the phase.
	turn := providers.TurnDetection{SilenceDurationMs: config.AppConfig.GeminiTurnSilenceMs}
	if s.User != nil {
		switch s.CurrentState() {
		case models.StateVisionIdealLife, models.StateVisionWheelOfLife:
			turn.SilenceDurationMs = config.AppConfig.GeminiReflectiveTurnSilenceMs
		}
	}

	// Delegate setup message construction to the provider
	setupMsg := s.provider.BuildSetupMessage(voiceName, systemInstruction, tools, s.latestSessionHandle, languageCode, turn)

	s.writeGeminiJSONWithConn(s.GeminiWs, setupMsg)
}

// InjectPrompt sends a new system instruction to Gemini mid-session.
// This is used to progressively reveal the different stages of the onboarding process.
func (s *ChatSession) InjectPrompt(promptText string) {
	s.geminiMutex.RLock()
	defer s.geminiMutex.RUnlock()

	if s.GeminiWs == nil {
		s.logger.Warn("Cannot inject prompt: Gemini websocket is nil")
		return
	}

	wrappedPrompt := promptText
	if !strings.HasPrefix(promptText, "[SYSTEM DIRECTIVE") && !strings.HasPrefix(promptText, "CRITICAL:") {
		wrappedPrompt = fmt.Sprintf("[SYSTEM DIRECTIVE (CRITICAL): %s]", promptText)
	}

	s.logger.Info("Injecting progressive prompt into session", zap.String("prompt", wrappedPrompt))
	s.logHistory("Inject Prompt", wrappedPrompt)

	promptMsg := s.provider.BuildPromptMessage(wrappedPrompt)

	if err := s.writeGeminiJSONWithConn(s.GeminiWs, promptMsg); err != nil {
		s.logger.Error("Failed to inject progressive prompt", zap.Error(err))
	}
}

// InjectPromptNoTrigger sends a new system instruction to Gemini mid-session without triggering immediate response generation.
func (s *ChatSession) InjectPromptNoTrigger(promptText string) {
	s.geminiMutex.RLock()
	defer s.geminiMutex.RUnlock()

	if s.GeminiWs == nil {
		s.logger.Warn("Cannot inject prompt no-trigger: Gemini websocket is nil")
		return
	}

	wrappedPrompt := promptText
	if !strings.HasPrefix(promptText, "[SYSTEM DIRECTIVE") && !strings.HasPrefix(promptText, "CRITICAL:") {
		wrappedPrompt = fmt.Sprintf("[SYSTEM DIRECTIVE (CRITICAL): %s]", promptText)
	}

	s.logger.Info("Injecting progressive prompt no-trigger into session", zap.String("prompt", wrappedPrompt))
	s.logHistory("Inject Prompt No-Trigger", wrappedPrompt)

	promptMsg := s.provider.BuildPromptMessageNoTrigger(wrappedPrompt)

	if err := s.writeGeminiJSONWithConn(s.GeminiWs, promptMsg); err != nil {
		s.logger.Error("Failed to inject progressive prompt no-trigger", zap.Error(err))
	}
}

const restartPrompt = `IMPORTANT: We need to pause for a moment to optimize our session. You MUST call the 'restart_session_with_summary' tool now. You must pass a detailed summary of the entire conversation so far (including user goals, feelings, state, categories scored, and any progress made) to the 'summary' argument of the tool. The summary MUST end with a PROGRESS line stating precisely where you are in your task instructions: which phases/steps are already COMPLETED (list them by name or number), which phase you are in RIGHT NOW, and the exact last question you asked or were about to ask. Example: "PROGRESS: Phases 1-4 completed. Currently in Phase 5; last open question: how likely they are to follow through, zero to ten." This line is what prevents the session from repeating itself after the pause.

Crucially, in your spoken/text response, you must speak out loud to the user in their preferred language to buy some time. Say ONLY a single very natural and brief phrase like "Let me take just a quick moment to gather my thoughts," or "Give me just a second to organize this." You are STRICTLY FORBIDDEN from outputting the conversation summary or any technical terms (like restart, connection reset, or reconnection) in your spoken/text output. ONLY say the brief phrase, call the tool, and do nothing else.`

func (s *ChatSession) shouldTriggerRestart() bool {
	if s.SessionDB.StartTime.IsZero() || s.pendingShutdown {
		return false
	}
	if s.User != nil && s.CurrentState().IsEndingSession() {
		return false
	}

	s.geminiMutex.RLock()
	lastRestart := s.lastRestartAt
	s.geminiMutex.RUnlock()

	hasElapsedSevenMinutes := time.Since(s.SessionDB.StartTime) > 7*time.Minute
	hasRestartedRecently := !lastRestart.IsZero() && time.Since(lastRestart) < 7*time.Minute
	return hasElapsedSevenMinutes && !hasRestartedRecently
}

func (s *ChatSession) InjectTransitionPrompt(nextInstructions string) {
	if s.shouldTriggerRestart() {
		s.logger.Info("7 minutes elapsed, triggering connection restart flow on state transition")
		s.geminiMutex.Lock()
		s.restartInstructions = "We have just resumed from a brief pause. You MUST speak out loud right now to continue the exercise based on the active task instructions. Do NOT recap, repeat, summarize, or reference the pause or the conversation history. This conversation is already in progress: do NOT greet the user, do NOT say 'welcome back', and do NOT deliver your task's scripted OPENING (that is only for the start of a session). Simply deliver the next coaching step naturally based on the summary. Do not wait for the user to speak."
		// Keep the displaced transition instructions: if the model fails to call
		// restart_session_with_summary, the next TURN_COMPLETE injects these instead so the
		// conversation continues rather than stalling.
		s.restartFallbackPrompt = nextInstructions
		s.geminiMutex.Unlock()
		s.InjectPrompt(restartPrompt)
	} else {
		// nextInstructions already carries an ENDING_SESSION-specific directive (see the
		// override in handleCompleteCurrentTask) ahead of the task script itself — this used
		// to ALSO prepend a third, near-identical "deliver the goodbye then call
		// terminate_session" paragraph here, stacking the same instruction three times over
		// (QA: turns this bloated correlated with the model producing the full transcript but
		// no audio at all). Let the one directive plus the task script speak for themselves,
		// same as every other transition.
		s.InjectPrompt(nextInstructions)
	}
}

// deadAirTimeout is how long a user utterance may sit unanswered — no model audio, text,
// or tool call — before the watchdog nudges the model. Long enough to never race a normal
// response (which starts within 1-3s) or a thinking pause the model is deliberately holding.
const deadAirTimeout = 9 * time.Second

// maxDeadAirNudges caps escalation for a single unanswered wait.
const maxDeadAirNudges = 2

// bareScoreMinReasoningRunes is the minimum length of the user's last utterance for a
// first-fill update_wheel_of_life call to be trusted as carrying real reasoning. The
// original 25 caught terse bare scores ("um sete", "1 3 também") but let VERBOSE bare
// scores through — QA saw "Okay. Um I guess it's a six." (28 runes) and "say like this a
// seven on eight and maybe eight" (45 runes) both saved with fabricated reasoning, once as
// the priority area itself. Genuine "why" answers in the same logs run 60–150+ runes.
// A false positive here is cheap by design: the guard rejects once per area, and the
// forced retry question ("what sits behind that number?") is the exact question the
// script mandates anyway — so a genuinely terse user loses one beat, never their answer.
const bareScoreMinReasoningRunes = 60

// Client input audio is 16 kHz 16-bit mono PCM (see providers' BuildAudioInputMessage).
const inputAudioBytesPerSecond = 16000 * 2

// audibleAudioMeanAbs is the mean absolute 16-bit sample amplitude above which an inbound
// audio chunk is considered actual speech/sound rather than silence. Quiet-room silence
// sits near zero; speech runs well into the thousands.
const audibleAudioMeanAbs = 500

// geminiDeafMinSpeechSecs is the minimum audible speech that must have gone unanswered
// before the deaf detector may even consider firing — filters stray noise blips.
const geminiDeafMinSpeechSecs = 2.0

// geminiDeafSilenceAfterSpeech is how long after the user FINISHES speaking (client audio
// stops arriving) Gemini may stay completely silent before the connection is declared
// deaf. A healthy Gemini transcribes within a couple of seconds of speech ending; it is
// often legitimately silent DURING speech, so in-progress audio must never count.
const geminiDeafSilenceAfterSpeech = 10 * time.Second

// geminiLivenessCooldown spaces forced liveness reconnects; maxLivenessRestarts caps them
// per session — repeated firing means the detector is wrong, not the connection (QA: five
// back-to-back false restarts shredded a session, cutting the user off mid-answer).
const geminiLivenessCooldown = 90 * time.Second
const maxLivenessRestarts = 2

// isAudibleAudio reports whether a raw little-endian 16-bit PCM chunk carries actual sound.
// It samples every 4th frame, which is plenty for a mean-amplitude estimate.
func isAudibleAudio(pcm []byte) bool {
	if len(pcm) < 2 {
		return false
	}
	var sum, n int64
	for i := 0; i+1 < len(pcm); i += 8 {
		v := int64(int16(uint16(pcm[i]) | uint16(pcm[i+1])<<8))
		if v < 0 {
			v = -v
		}
		sum += v
		n++
	}
	return n > 0 && sum/n >= audibleAudioMeanAbs
}

const deadAirNudgePrompt = "[SYSTEM] The user spoke several seconds ago and you have NOT responded — they are sitting in complete silence, possibly asking whether you are still there. You MUST respond OUT LOUD right now, in the user's language. IF their last words were an UNFINISHED fragment (a sentence trailing off mid-thought, e.g. 'I'd say it's a...'), they are still thinking: give ONLY one brief, soft cue that you are here and listening, do NOT re-ask your question, and WAIT for them to finish (QA: re-asking made the user repeat an answer they were mid-way through giving). Otherwise: briefly reassure them you are here, address what they just said (if they asked you to repeat or finish a sentence, do so), and then continue your active task instructions from where you left off. Never mention any technical issue or pause."

// monitorDeadAir revives the model when the user has spoken and been left in dead air.
// After a barge-in interruption Gemini occasionally stops generating for every subsequent
// user turn (each one only yields an "interrupted" signal): the user repeats "are you
// there?" into silence until they abandon the session (QA). No TURN_COMPLETE-based safety
// net can catch this — no turn ever starts — so this watchdog runs on wall-clock time.
func (s *ChatSession) monitorDeadAir() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.geminiMutex.RLock()
			awaiting := s.userAwaitingReplySince
			nudges := s.deadAirNudges
			skip := s.clientGone || s.pendingRestart || s.pendingShutdown || s.GeminiWs == nil
			s.geminiMutex.RUnlock()

			// The final goodbye stage terminates on its own schedule; a nudge there could
			// make the model speak again after the farewell.
			if s.User != nil && s.CurrentState().IsEndingSession() {
				continue
			}
			if skip || awaiting.IsZero() || time.Since(awaiting) < deadAirTimeout || nudges >= maxDeadAirNudges {
				continue
			}

			s.geminiMutex.Lock()
			if s.userAwaitingReplySince.IsZero() || s.deadAirNudges >= maxDeadAirNudges {
				s.geminiMutex.Unlock()
				continue
			}
			s.deadAirNudges++
			// Restart the wait window so the next escalation (if any) waits a full timeout.
			s.userAwaitingReplySince = time.Now()
			nudge := s.deadAirNudges
			s.geminiMutex.Unlock()

			s.logger.Warn("Dead-air watchdog: user is waiting with no model response; injecting nudge", zap.Int("nudge", nudge))
			s.InjectPrompt(deadAirNudgePrompt)
		}
	}
}

// markModelResponding clears the dead-air watchdog: the model has produced content (audio,
// transcript, or a tool call), so the user is no longer waiting unanswered.
func (s *ChatSession) markModelResponding() {
	s.geminiMutex.Lock()
	s.userAwaitingReplySince = time.Time{}
	s.deadAirNudges = 0
	s.geminiMutex.Unlock()
}

// monitorGeminiLiveness rebuilds the Gemini connection when it goes deaf: the user spoke,
// FINISHED speaking, and Gemini produced nothing at all — no input transcription, no
// interrupt, no content (QA: the user explained their Health score and Rumi never spoke
// again). The dead-air watchdog cannot see this case because it arms on input
// transcriptions, which a deaf connection never produces. Crucially, the detector only
// looks at the silence AFTER speech ends — Gemini is often legitimately quiet during a
// long reflective monologue, and firing mid-answer cuts the user off (QA regression).
// Recovery reuses the pendingRestart machinery: the state-derived system prompt restores
// the task context and the resume directive has the model apologize briefly and ask the
// user to repeat themselves.
func (s *ChatSession) monitorGeminiLiveness() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.geminiMutex.RLock()
			loudSecs := s.loudAudioSecsSinceGemini
			lastAudio := s.lastClientAudioAt
			lastRestart := s.lastRestartAt
			restarts := s.livenessRestarts
			skip := s.clientGone || s.pendingRestart || s.pendingShutdown || s.GeminiWs == nil
			s.geminiMutex.RUnlock()

			if skip || restarts >= maxLivenessRestarts {
				continue
			}
			// Enough unanswered speech, AND the user has finished (no audio arriving),
			// AND Gemini stayed mute through the whole post-speech window.
			if loudSecs < geminiDeafMinSpeechSecs || lastAudio.IsZero() ||
				time.Since(lastAudio) < geminiDeafSilenceAfterSpeech {
				continue
			}
			// Cooldown after ANY restart (scheduled or liveness): right after a reconnect
			// the pipeline is still settling and stale counters must not trigger again.
			if !lastRestart.IsZero() && time.Since(lastRestart) < geminiLivenessCooldown {
				continue
			}

			s.logger.Error("Gemini connection appears deaf: user finished speaking and Gemini stayed silent; forcing reconnect",
				zap.Float64("loud_audio_secs", loudSecs),
				zap.Duration("silence_after_speech", time.Since(lastAudio)),
				zap.Int("liveness_restart", restarts+1))

			s.geminiMutex.Lock()
			s.loudAudioSecsSinceGemini = 0
			s.livenessRestarts++
			s.pendingRestart = true
			s.restartInstructions = "You are continuing an ongoing session after a brief technical pause. The user has been speaking, but you could not hear them. You MUST speak out loud right now, in the user's language: apologize briefly and warmly in ONE sentence and ask them to repeat what they were saying — vary the wording naturally if you have already apologized for losing them earlier in this session; never repeat the same apology twice. Then continue the exercise from your CURRENT ACTIVE TASK INSTRUCTIONS from the exact point where you left off. Do NOT greet them again, do NOT re-introduce yourself, do NOT restart the exercise, and do NOT mention any technical terms like connection or restart."
			if s.GeminiWs != nil {
				s.GeminiWs.Close()
			}
			s.geminiMutex.Unlock()
		}
	}
}

func (s *ChatSession) monitorForcedRestart() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.geminiMutex.RLock()
			lastRestart := s.lastRestartAt
			pendingRestart := s.pendingRestart
			s.geminiMutex.RUnlock()

			if !pendingRestart {
				// Single-prompt sessions (movement, values, ...) never leave their one state,
				// so the state checks below can't see that their closing is underway. The
				// saved session insight is the reliable signal: it is captured in the
				// integration/closing phase, and a forced restart after that point rips the
				// goodbye apart and re-delivers the script from the top.
				closingReached := s.SessionDB.UserSessionInsight != nil && *s.SessionDB.UserSessionInsight != ""
				isEnding := s.pendingShutdown || closingReached || (s.User != nil && (s.CurrentState().IsEndingSession() || s.CurrentState().IsEmotionalClosing()))
				if !isEnding {
					var elapsed time.Duration
					if lastRestart.IsZero() {
						if s.SessionDB.StartTime.IsZero() {
							continue
						}
						elapsed = time.Since(s.SessionDB.StartTime)
					} else {
						elapsed = time.Since(lastRestart)
					}
					if elapsed >= 9*time.Minute+30*time.Second {
						s.geminiMutex.RLock()
						injectedAt := s.forcedRestartInjectedAt
						s.geminiMutex.RUnlock()

						if injectedAt.IsZero() || time.Since(injectedAt) > 1*time.Minute {
							s.logger.Info("9:30 reached since start/last restart. Forcing connection restart...")

							s.geminiMutex.Lock()
							s.restartInstructions = "The brief pause is over. You must speak out loud right now to resume the conversation exactly where you left off based on the summary. This conversation is already in progress: do NOT greet the user, do NOT say 'welcome back', and do NOT deliver your task's scripted OPENING question (that is only for the start of a session). Pick up the exact thread from the summary — continue the point that was being discussed or re-ask the last open question. Do not wait for the user to speak."
							s.forcedRestartInjectedAt = time.Now()
							s.geminiMutex.Unlock()

							s.InjectPrompt(restartPrompt)
						}
					}
				}
			}
		}
	}
}

// writeClientJSON logs and sends a JSON message to the client.
func (s *ChatSession) writeClientJSON(v interface{}) error {
	// Nil-safe: tool handlers emit client events (e.g. tasks_updated) from paths that can
	// run after the client connection is gone, or under unit tests with no socket at all.
	if s.ClientWs == nil {
		return nil
	}
	var typeStr string
	if m, ok := v.(map[string]string); ok {
		typeStr = m["type"]
		s.logger.Info("Server → Client [JSON]", zap.String("type", typeStr), zap.Any("payload", v))
	} else if m, ok := v.(map[string]interface{}); ok {
		typeStr, _ = m["type"].(string)
		s.logger.Info("Server → Client [JSON]", zap.String("type", typeStr), zap.Any("payload", v))
	}
	if payloadJSON, err := json.Marshal(v); err == nil {
		// Do not record these in the conversation transcript. ai_transcript is the AI's spoken
		// text, which is already captured separately under the "AI" entries — logging the
		// server→client payload here would just duplicate it.
		if typeStr != "turn_complete" && typeStr != "ai_transcript" {
			if typeStr != "" {
				s.logHistory(fmt.Sprintf("Server to Client (%s)", typeStr), string(payloadJSON))
			} else {
				s.logHistory("Server to Client", string(payloadJSON))
			}
		}
	}
	s.clientWriteMutex.Lock()
	defer s.clientWriteMutex.Unlock()
	return s.ClientWs.WriteJSON(v)
}

// writeClientMessage sends a raw (binary/close) frame to the client, serialized by
// clientWriteMutex so it cannot race writeClientJSON (audio frames, queued show_screen
// messages, etc.) and trigger a "concurrent write to websocket connection" panic.
func (s *ChatSession) writeClientMessage(messageType int, data []byte) error {
	s.clientWriteMutex.Lock()
	defer s.clientWriteMutex.Unlock()
	return s.ClientWs.WriteMessage(messageType, data)
}

func (s *ChatSession) writeGeminiJSONWithConn(ws *websocket.Conn, v interface{}) error {
	if ws == nil {
		return fmt.Errorf("gemini websocket is nil")
	}
	s.geminiWriteMutex.Lock()
	defer s.geminiWriteMutex.Unlock()
	return ws.WriteJSON(v)
}

func (s *ChatSession) proxyClientToGemini(errc chan error) {
	// Inbound-audio instrumentation: one compact log line per window summarizing how much user
	// audio reached the backend and was forwarded to Gemini. When QA reports "Rumi stopped
	// responding", these lines tell us whether the user's audio (a) stopped arriving from the
	// client, (b) arrived but failed to forward to Gemini, or (c) flowed fine (pointing at
	// Gemini/VAD or the outbound path instead).
	var (
		audioWindowStart    = time.Now()
		audioMsgsIn         int
		audioBytesIn        int
		audioForwardErrs    int
		audioDroppedNoConn  int
		lastAudioForwardErr error
	)
	flushAudioStats := func() {
		if audioMsgsIn == 0 && audioForwardErrs == 0 && audioDroppedNoConn == 0 {
			audioWindowStart = time.Now()
			return
		}
		fields := []zap.Field{
			zap.Duration("window", time.Since(audioWindowStart).Round(time.Second)),
			zap.Int("audio_msgs_in", audioMsgsIn),
			zap.Int("audio_bytes_in", audioBytesIn),
			zap.Int("forward_errors", audioForwardErrs),
			zap.Int("dropped_no_conn", audioDroppedNoConn),
		}
		if lastAudioForwardErr != nil {
			fields = append(fields, zap.NamedError("last_forward_error", lastAudioForwardErr))
		}
		if audioForwardErrs > 0 || audioDroppedNoConn > 0 {
			s.logger.Warn("Client audio inbound window had forwarding problems", fields...)
		} else {
			s.logger.Info("Client audio inbound window", fields...)
		}
		audioWindowStart = time.Now()
		audioMsgsIn, audioBytesIn, audioForwardErrs, audioDroppedNoConn, lastAudioForwardErr = 0, 0, 0, 0, nil
	}

	for {
		msgType, msg, err := s.ClientWs.ReadMessage()
		if err != nil {
			// Flush the final partial audio window so the log shows how much (or how little)
			// user audio was still arriving right before the disconnect.
			flushAudioStats()
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				s.logger.Info("Client disconnected", zap.Error(err))
			} else if strings.Contains(err.Error(), "use of closed network connection") {
				s.logger.Debug("Client WebSocket closed intentionally")
			} else {
				s.logger.Error("Error reading from Client WebSocket", zap.Error(err))
			}
			errc <- err
			return
		}

		if msgType == websocket.TextMessage {
			// Check if message is a JSON control message
			var controlMsg map[string]interface{}
			if err := json.Unmarshal(msg, &controlMsg); err == nil {
				msgTypeStr, _ := controlMsg["type"].(string)
				if msgTypeStr != "turn_complete" {
					s.logHistory("Client Control Message", string(msg))
				}
				if msgTypeStr == "turn_complete" {
					s.logger.Debug("Client → Server [TURN_COMPLETE received] → Ignored (using Gemini server-side VAD)")
					continue
				}
			} else {
				// If it's just raw text meant for Gemini (e.g. initial prompt or typed message)
				textStr := strings.TrimSpace(string(msg))
				if textStr != "" {
					if s.User != nil {
						state := s.CurrentState()
						if state == models.StateCheckin {
							s.logger.Warn("Session is already terminated or in a future state. Ignoring client message.", zap.String("state", string(state)))
							continue
						}
					}
					s.logger.Info("Client → Gemini [TEXT FORWARD]", zap.String("text", textStr))
					s.flushUserTranscript()
					s.logHistory("User", textStr)

					s.geminiMutex.Lock()
					ws := s.GeminiWs
					setupChan := s.setupDone

					var payload map[string]interface{}
					hasDirective := false
					var directiveText string

					if s.User != nil {
						state := s.CurrentState()
						if state == models.StateVisionIdealLife {
							s.logger.Info("Prepending Ideal Life Vision completion instruction to user text response")
							directiveText = "Conclude the Ideal Life Vision phase now. 1. Acknowledge the user's vision warmly in exactly ONE concise sentence first. DO NOT repeat yourself or write multiple sentences of validation. 2. In the SAME turn, call the 'save_ideal_life_vision' tool with a beautifully written, concise summary of the ideal life they described to the 'vision' parameter. Do NOT propose categories, ask questions, or transition. Yield control immediately after calling the tool."
							hasDirective = true
						} else if state.IsEmotionalClosing() {
							lastAI := s.getLastAIMessage()
							if strings.Contains(strings.ToLower(lastAI), "most important insight") || strings.Contains(strings.ToLower(lastAI), "take away from this conversation") {
								s.logger.Info("Prepending EMOTIONAL_CLOSING insight instruction to user text response")
								directiveText = "The user has shared their key insight. You MUST, in ONE turn: 1. Output ONE short sentence acknowledging their insight warmly. 2. Call 'save_memory' (category=\"insight\", content=the user's insight) natively. 3. Ask, in the user's language, whether you may share something that caught your attention while listening to them, then STOP and wait. Do NOT call 'complete_current_task' yet — the personalized synthesis and clarity check still follow, per your task instructions."
								hasDirective = true
							}
						}
					}

					if hasDirective {
						wrappedDirective := directiveText
						if !strings.HasPrefix(directiveText, "[SYSTEM DIRECTIVE") && !strings.HasPrefix(directiveText, "CRITICAL:") {
							wrappedDirective = fmt.Sprintf("[SYSTEM DIRECTIVE (CRITICAL): %s]", directiveText)
						}
						forwardText := fmt.Sprintf("User's response: %s\n\n%s", textStr, wrappedDirective)
						payload = s.provider.BuildPromptMessage(forwardText)
					} else {
						payload = s.provider.BuildPromptMessage(textStr)
					}
					s.geminiMutex.Unlock()

					if ws != nil {
						select {
						case <-setupChan:
							s.writeGeminiJSONWithConn(ws, payload)
						case <-time.After(10 * time.Second):
							s.logger.Error("Timeout waiting for setupComplete to forward text message")
						}
					}
				}
			}
		} else if msgType == websocket.BinaryMessage {
			audioMsgsIn++
			audioBytesIn += len(msg)
			// NOTE: frame arrival says nothing about speech — the client streams mic
			// audio continuously even in silence, so nothing here may be used as a
			// "user is talking" signal (see lastUserSpeechUnixNano).

			// Delegate audio input format to the provider
			payload := s.provider.BuildAudioInputMessage(msg)

			s.geminiMutex.Lock()
			ws := s.GeminiWs
			setupChan := s.setupDone
			s.geminiMutex.Unlock()

			if ws != nil {
				select {
				case <-setupChan:
					// Route through writeGeminiJSONWithConn so the write is serialized by
					// geminiWriteMutex — otherwise this audio-forward races InjectPrompt
					// (e.g. during a restart) and panics with "concurrent write to websocket".
					if err := s.writeGeminiJSONWithConn(ws, payload); err != nil {
						audioForwardErrs++
						lastAudioForwardErr = err
					} else {
						s.geminiMutex.Lock()
						s.lastClientAudioAt = time.Now()
						if isAudibleAudio(msg) {
							// Audible speech successfully forwarded: feed the deaf-connection
							// watchdog. Any message back from Gemini resets the accumulator.
							s.loudAudioSecsSinceGemini += float64(len(msg)) / float64(inputAudioBytesPerSecond)
						}
						s.geminiMutex.Unlock()
					}
				case <-time.After(10 * time.Second):
					s.logger.Error("Timeout waiting for setupComplete to forward audio message")
					audioForwardErrs++
					lastAudioForwardErr = fmt.Errorf("timeout waiting for setupComplete")
				}
			} else {
				audioDroppedNoConn++
			}

			if time.Since(audioWindowStart) >= 10*time.Second {
				flushAudioStats()
			}
		}
	}
}

func (s *ChatSession) proxyGeminiToClient(errc chan error) {
	for {
		// Safely read from current GeminiWs
		s.geminiMutex.RLock()
		ws := s.GeminiWs
		s.geminiMutex.RUnlock()

		_, msg, err := ws.ReadMessage()
		if err != nil {
			isSessionExpiryError := strings.Contains(err.Error(), "1008") ||
				strings.Contains(err.Error(), "GoAway") ||
				strings.Contains(err.Error(), "policy violation")

			s.geminiMutex.Lock()
			isPendingRestart := s.pendingRestart
			s.geminiMutex.Unlock()

			if isPendingRestart {
				s.logger.Info("Initiating requested Gemini connection restart...")
				s.geminiMutex.Lock()
				s.pendingRestart = false
				s.turnHasToolCall = false
				// The restart is happening — drop any stashed fallback so it cannot fire on the
				// new connection (the post-restart instructions take over from here).
				s.restartFallbackPrompt = ""
				// Capture whether the model spoke in the turn that triggered this restart (e.g. the
				// vision outro before save_ideal_life_vision) BEFORE resetting the flag, so the
				// post-restart directive injection can add a 1s gap and avoid glued audio.
				s.pendingRestartSpoke = s.turnSpokeText
				s.turnSpokeText = false
				s.lastRestartAt = time.Now()
				s.forcedRestartInjectedAt = time.Time{}
				// A fresh connection starts with a clean liveness slate — audio that went
				// unanswered on the OLD connection must not count against the new one.
				s.loudAudioSecsSinceGemini = 0
				if s.GeminiWs != nil {
					s.GeminiWs.Close()
				}
				conn, dialErr := s.provider.Connect(s.logger)
				if dialErr == nil {
					s.logger.Info("Gemini restarted successfully after summary!")
					s.GeminiWs = conn
					s.setupDone = make(chan struct{})
					s.setupOnce = sync.Once{}
					s.sendInitialConfigLocked(s.User)
					s.geminiMutex.Unlock()
					continue
				} else {
					s.logger.Error("Failed to reconnect GeminiWS after summary restart", zap.Error(dialErr))
					s.geminiMutex.Unlock()
					// fallthrough to err handling
				}
			} else if s.latestSessionHandle != "" && isSessionExpiryError && !s.pendingShutdown {
				s.logger.Info("Gemini Connection closed, initiating transparent session resumption...", zap.Error(err))

				s.geminiMutex.Lock()
				s.turnHasToolCall = false
				s.turnSpokeText = false
				s.loudAudioSecsSinceGemini = 0
				// A GoAway lands disproportionately often right in the closing (Gemini's
				// connection lifetime ≈ the length of a full session), and after the
				// transparent resumption nothing used to tell the model to finish ending
				// the session — QA watched two sessions where the goodbye had been spoken
				// (or was mid-flight), the resumption swallowed the terminate, and the
				// session hung open with the synthesis card never shown. If the closing
				// was already underway (insight saved), the resumed connection's first
				// job is to close: one short goodbye and terminate_session, same turn.
				if s.SessionDB.UserSessionInsight != nil && *s.SessionDB.UserSessionInsight != "" {
					s.restartInstructions = "The conversation is in its CLOSING moments. If you have not yet delivered the session's closing (the next-session seed, the card announcement and the goodbye), continue it from where you left off. If you already said goodbye, do not repeat it — simply say one short, warm closing sentence (the equivalent of 'See you soon.'). In BOTH cases you MUST call 'terminate_session' in the SAME turn as your final sentence — the session cannot end, and the user's summary card cannot appear, until you do."
				}
				if s.GeminiWs != nil {
					s.GeminiWs.Close()
				}

				conn, dialErr := s.provider.Connect(s.logger)
				if dialErr == nil {
					s.logger.Info("Gemini Session Resumed successfully!")
					s.GeminiWs = conn
					s.setupDone = make(chan struct{})
					s.setupOnce = sync.Once{}
					s.sendInitialConfigLocked(s.User)
					s.geminiMutex.Unlock()
					continue
				} else {
					s.logger.Error("Failed to reconnect GeminiWS for resumption", zap.Error(dialErr))
					s.geminiMutex.Unlock()
					// fallthrough to err handling
				}
			}

			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				s.logger.Info("Gemini connection closed", zap.Error(err))
			} else if strings.Contains(err.Error(), "use of closed network connection") {
				s.logger.Debug("Gemini WebSocket closed (client disconnected)", zap.Error(err))
			} else {
				s.logger.Error("Error reading from Gemini WebSocket", zap.Error(err))
			}
			errc <- err
			return
		}

		// Gemini sent something — whatever it is, the connection is alive; reset the
		// deaf-connection accumulator (see monitorGeminiLiveness).
		s.geminiMutex.Lock()
		s.loudAudioSecsSinceGemini = 0
		s.geminiMutex.Unlock()

		var resp map[string]interface{}
		if err := json.Unmarshal(msg, &resp); err != nil {
			s.logger.Warn("Failed to parse Gemini message", zap.Error(err))
			continue
		}

		// Log raw response for debugging tool calls
		// s.logger.Info("Gemini → Server [RAW]", zap.Any("resp", resp))

		known := false

		// Handle setupComplete — signal that the session is ready
		if _, ok := resp["setupComplete"]; ok {
			s.logger.Debug("Gemini → Server [SETUP_COMPLETE]")
			s.logHistory("System", "Gemini connection setup complete")
			s.setupOnce.Do(func() { close(s.setupDone) })
			known = true

			s.geminiMutex.Lock()
			if s.restartInstructions != "" {
				s.logger.Info("Injecting instructions after restart setupComplete", zap.String("instructions", s.restartInstructions))

				langInstruction, ok := LanguageInstructions[*s.User.PreferredLanguage]
				if !ok {
					langInstruction = "IMPORTANT: RESPOND IN THE CUSTOMER PREFERRED LANGUAGE. YOU MUST RESPOND UNMISTAKABLY IN THE CUSTOMER PREFERRED LANGUAGE."
				}
				instr := s.restartInstructions + " " + langInstruction
				s.restartInstructions = ""
				// If the model spoke right before this restart (e.g. the vision outro before
				// save_ideal_life_vision), the upcoming turn's audio would butt straight up against
				// that pre-restart audio with no user turn between. Append 1s of silence to the
				// client stream first so the two utterances are cleanly separated, not glued.
				addGap := s.pendingRestartSpoke
				s.pendingRestartSpoke = false
				s.geminiMutex.Unlock()
				if addGap {
					s.injectSilence(1)
				}
				s.InjectPrompt(instr)
			} else {
				s.pendingRestartSpoke = false
				s.geminiMutex.Unlock()
			}
		}
		// Handle usage metadata (tokens)
		if meta, ok := resp["usageMetadata"].(map[string]interface{}); ok {
			//s.logger.Debug("Gemini → Server [USAGE_METADATA]")
			s.UpdateUsageMetadata(meta)
			known = true
		}

		// Handle tool calls from Gemini
		if toolCall, ok := resp["toolCall"].(map[string]interface{}); ok {
			s.markModelResponding()
			s.geminiMutex.Lock()
			s.turnHasToolCall = true
			s.geminiMutex.Unlock()

			// Flush accumulated text to client before processing the tool call
			s.flushUserTranscript()
			aiText := strings.TrimSpace(s.accumulatedText)
			if aiText == "" {
				aiText = strings.TrimSpace(s.accumulatedPartsText)
			}
			aiText = CleanLeakedToolCalls(aiText)
			if aiText != "" {
				s.turnSpokeText = true
				s.logHistory("AI", aiText)
				s.writeClientJSON(map[string]interface{}{
					"type": "ai_transcript",
					"text": aiText,
				})
			}
			// Stashed unconditionally, before dispatch, in case this call turns out to be
			// terminate_session: pendingShutdown is set from inside HandleToolCall below,
			// and on that turn's own TURN_COMPLETE (if it even arrives before this same
			// iteration's pendingShutdown check runs — see the check's own comment) the
			// audio for THIS text has typically not started arriving yet, so it cannot
			// supply its own estimate in time.
			s.pendingShutdownDelaySecs = s.estimateSpeechRemainingSecs(aiText)
			s.accumulatedText = ""
			s.accumulatedPartsText = ""
			s.sentCleanText = ""

			s.HandleToolCall(toolCall)
			known = true
		}

		if serverContent, ok := resp["serverContent"].(map[string]interface{}); ok {
			known = true
			contentKnown := false

			// Handle generation completion (silent)
			if gc, ok := serverContent["generationComplete"].(bool); ok && gc {
				contentKnown = true
			}

			// Handle interruption
			if interrupted, ok := serverContent["interrupted"].(bool); ok && interrupted {
				s.logger.Debug("Gemini → Server [INTERRUPT]")
				s.geminiMutex.Lock()
				s.interruptCounter++
				// Scheduled marker reveals belong to audio that was just discarded — their
				// timer goroutines observe the interruptCounter bump and abort. Clear the
				// dedup set so a re-emitted marker after the interrupt schedules anew.
				s.revealsScheduledThisTurn = nil
				// NOTE: do NOT reset the per-turn flags (turnHasToolCall / turnSpokeText) here.
				// An interrupt — whether from the user cutting in or from a server prompt
				// injection — is always followed by a TURN_COMPLETE for the turn being cut, and
				// that is where the flags are authoritatively read and reset. Clearing them on the
				// interrupt corrupts the very turn that is about to complete: e.g. a turn that
				// called complete_current_task into ENDING_SESSION would look tool-less and trip
				// the "ended ENDING_SESSION without terminate_session" auto-terminate, killing the
				// connection before the goodbye is delivered.
				s.geminiMutex.Unlock()
				s.flushUserTranscript()
				aiText := strings.TrimSpace(s.accumulatedText)
				if aiText == "" {
					aiText = strings.TrimSpace(s.accumulatedPartsText)
				}
				aiText = CleanLeakedToolCalls(aiText)
				if aiText != "" {
					s.logHistory("AI (Interrupted)", aiText)
					// The user heard (at least part of) this speech. Without this, an
					// interrupted delivery leaves turnSpokeText=false, and the wheel-setup
					// variant chooser then treats the turn as silent — injecting the
					// missing-intro recovery that re-delivers an introduction the user
					// already heard (QA: intro spoken a third time after the repetition
					// tripwire cut the turn).
					s.turnSpokeText = true
				}
				s.accumulatedText = ""
				s.accumulatedPartsText = ""
				s.sentCleanText = ""
				s.writeClientJSON(map[string]string{"type": "interrupt"})
				contentKnown = true
			}

			// 1. Process output transcription first to detect any pause or screen instructions
			pauseSeconds := 0
			if outputTranscription, ok := serverContent["outputTranscription"].(map[string]interface{}); ok {
				contentKnown = true
				if text, ok := outputTranscription["text"].(string); ok && text != "" {
					s.markModelResponding()
					s.flushUserTranscript()
					s.logger.Info("Gemini → Server [AI_TRANSCRIPT]", zap.String("text", text))
					s.accumulatedText += text

					// Pause: the symbol marker (●▂) is primary; the legacy [P=N] tag is a fallback.
					if m := pauseSymbolRegex.FindStringSubmatch(s.accumulatedText); len(m) > 1 {
						pauseSeconds = pauseSymbolToSeconds[m[1]]
						if pauseSeconds <= 0 {
							pauseSeconds = 2
						}
						s.accumulatedText = strings.Replace(s.accumulatedText, m[0], fmt.Sprintf("[PROCESSED_PAUSE: %d]", pauseSeconds), 1)
					} else if matches := pauseRegex.FindStringSubmatch(s.accumulatedText); len(matches) > 1 {
						_, err := fmt.Sscanf(matches[1], "%d", &pauseSeconds)
						if err != nil || pauseSeconds <= 0 {
							pauseSeconds = 5
						}
						s.accumulatedText = strings.Replace(s.accumulatedText, matches[0], fmt.Sprintf("[PROCESSED_PAUSE: %d]", pauseSeconds), 1)
					}

					// Screen: the symbol marker (◆▣) is primary; the legacy [SS=name] tag is a
					// fallback. Process EVERY marker currently in the buffer, one at a time,
					// each replaced individually: an earlier version scheduled only the FIRST
					// match and then ReplaceAllString'd the rest away, so when two markers
					// landed in the same accumulation window (the intro tour's one turn
					// carries four) the later one was silently destroyed and its screen never
					// opened (QA: the profile stop never appeared).
					for {
						m := screenSymbolRegex.FindStringSubmatch(s.accumulatedText)
						if len(m) < 2 {
							break
						}
						screenName := screenSymbolToName[m[1]]
						s.logger.Info("Parsed screen marker", zap.String("screen", screenName))
						textBefore := s.accumulatedText[:strings.Index(s.accumulatedText, m[0])]
						if screenName == sessionSummaryMarkerScreen || screenName == sessionSummaryNextMarkerScreen {
							// Not a real screen — bypass queueShowScreen's navigable-screen
							// allowlist (and its suppression rules, which don't apply here)
							// and go straight to the same audio-synced scheduling.
							s.scheduleScreenReveal(screenName, textBefore)
						} else {
							s.queueShowScreen(screenName, textBefore)
						}
						s.accumulatedText = strings.Replace(s.accumulatedText, m[0], "[PROCESSED_SCREEN]", 1)
					}
					for {
						screenMatches := screenRegex.FindStringSubmatch(s.accumulatedText)
						if len(screenMatches) < 2 {
							break
						}
						screenName := screenMatches[1]
						s.logger.Info("Parsed screen tag (legacy)", zap.String("screen", screenName))
						textBefore := s.accumulatedText[:strings.Index(s.accumulatedText, screenMatches[0])]
						s.queueShowScreen(screenName, textBefore)
						s.accumulatedText = strings.Replace(s.accumulatedText, screenMatches[0], "[PROCESSED_SCREEN]", 1)
					}

					// Vision-phase marker tripwire: ANY ◆ in this phase is illegal — well-formed
					// pairs were already consumed above, so a surviving ◆ means the model is
					// substituting a marker for save_ideal_life_vision and then improvising the
					// next exercise aloud (observed repeatedly in QA). The transcript streams
					// well ahead of the audio, so acting NOW — instead of at TURN_COMPLETE —
					// barges in on the generation (an injected prompt interrupts it) before the
					// improvised intro is ever heard. The TURN_COMPLETE corrective remains as
					// fallback for the suppressed well-formed-marker case.
					if strings.Contains(s.accumulatedText, "◆") && s.User != nil &&
						s.CurrentState() == models.StateVisionIdealLife {
						s.geminiMutex.Lock()
						shouldInject := !s.visionCorrectiveSentThisTurn && !s.clientGone && !s.pendingRestart
						if shouldInject {
							s.visionCorrectiveSentThisTurn = true
							s.visionCorrectiveIssued = true
						}
						s.geminiMutex.Unlock()
						if shouldInject {
							s.logger.Warn("Marker glyph streamed in vision phase; interrupting generation with save_ideal_life_vision corrective")
							s.InjectPrompt(visionMarkerCorrective)
						}
					}

					// Within-turn repetition tripwire: Gemini occasionally re-delivers a long
					// script verbatim inside the SAME turn. The transcript streams well ahead
					// of the audio, so acting now — instead of at TURN_COMPLETE — cuts the
					// second delivery before the user has heard much of it.
					if !s.dupSpeechCorrectiveSentThisTurn && hasWithinTurnRepetition(s.accumulatedText) {
						s.geminiMutex.Lock()
						shouldInject := !s.dupSpeechCorrectiveSentThisTurn && !s.clientGone && !s.pendingRestart
						if shouldInject {
							s.dupSpeechCorrectiveSentThisTurn = true
						}
						s.geminiMutex.Unlock()
						if shouldInject {
							s.logger.Warn("Within-turn speech repetition detected; interrupting generation with stop-repeating corrective")
							s.InjectPrompt(dupSpeechCorrective)
						}
					}

					// Hold back any partially-streamed tool-call syntax (e.g. "save_memory{" still
					// being formed) so we never emit half of a leaked tool call. This matches
					// structural tool syntax only — never natural-language phrases — so it is
					// language-agnostic.
					cleanText := s.accumulatedText
					braceLoc := openBraceRegex.FindStringIndex(s.accumulatedText)
					toolLoc := openToolRegex.FindStringIndex(s.accumulatedText)

					var toolStartIdx = -1
					if braceLoc != nil {
						toolStartIdx = braceLoc[0]
					}
					if toolLoc != nil {
						if toolStartIdx == -1 || toolLoc[0] < toolStartIdx {
							toolStartIdx = toolLoc[0]
						}
					}

					if toolStartIdx != -1 {
						cleanText = s.accumulatedText[:toolStartIdx]
					}

					// Clean any fully formed tool calls in the clean part
					cleanText = CleanLeakedToolCallsRaw(cleanText)

					// Determine if we have new clean text to stream to client.
					// Streaming partials are gated behind a config flag (disabled by default).
					if strings.HasPrefix(cleanText, s.sentCleanText) {
						newPart := cleanText[len(s.sentCleanText):]
						if newPart != "" {
							if config.AppConfig.SendAITranscriptPartial {
								s.writeClientJSON(map[string]interface{}{
									"type": "ai_transcript_partial",
									"text": newPart,
								})
							}
							s.sentCleanText = cleanText
						}
					} else if config.AppConfig.SendAITranscriptPartial {
						// Fallback if strings mismatch
						s.writeClientJSON(map[string]interface{}{
							"type": "ai_transcript_partial",
							"text": text,
						})
					}
				}
			}

			// 2. If a pause instruction was parsed in this frame, inject the silence immediately!
			if pauseSeconds > 0 {
				s.injectSilence(pauseSeconds)
			}

			// 3. Process modelTurn audio and text parts
			if modelTurn, ok := serverContent["modelTurn"].(map[string]interface{}); ok {
				contentKnown = true
				if parts, ok := modelTurn["parts"].([]interface{}); ok {
					for _, partIntf := range parts {
						part, ok := partIntf.(map[string]interface{})
						if !ok {
							continue
						}

						// Text response
						if text, ok := part["text"].(string); ok {
							s.logger.Info("Gemini → Server [TEXT]", zap.String("text", text))
							s.accumulatedPartsText += text
						}

						// Audio — send as raw binary bytes, NOT JSON
						if inline, ok := part["inlineData"].(map[string]interface{}); ok {
							if dataStr, ok := inline["data"].(string); ok {
								s.markModelResponding()
								// Gemini returns base64-encoded PCM; decode and send as raw binary
								audioBytes, err := base64.StdEncoding.DecodeString(dataStr)
								if err != nil {
									s.logger.Warn("Failed to decode Gemini audio base64", zap.Error(err))
									continue
								}

								// Track audio duration sent to calculate screen-transition delay.
								s.geminiMutex.Lock()
								if s.turnAudioStartTime.IsZero() {
									s.turnAudioStartTime = time.Now()
									s.turnAudioDuration = 0
									// Diagnostic: on a turn whose transcript arrives as one lump ahead
									// of its audio (QA — the goodbye + terminate_session shape), this
									// timestamp is the only way to tell from the logs whether Gemini
									// ever actually started streaming audio for it, and how long after
									// the transcript that took.
									s.logger.Info("First audio chunk for turn")
								}
								s.turnAudioDuration += float64(len(audioBytes)) / 48000.0 // 24kHz 16-bit PCM = 2 bytes/sample
								s.geminiMutex.Unlock()

								s.writeClientMessage(websocket.BinaryMessage, audioBytes)
							}
						}
					}
				}
			}

			// Handle input transcription (user's speech → text)
			if inputTranscription, ok := serverContent["inputTranscription"].(map[string]interface{}); ok {
				contentKnown = true
				if text, ok := inputTranscription["text"].(string); ok && text != "" {
					// The user is speaking: (re)arm the dead-air watchdog from this moment.
					// Any model content (audio/transcript/tool call) disarms it. This is
					// also the signal the delayed silent-tool nudge checks to stand down
					// when the user resumed mid-thought.
					s.lastUserSpeechUnixNano.Store(time.Now().UnixNano())
					s.geminiMutex.Lock()
					s.userAwaitingReplySince = time.Now()
					s.geminiMutex.Unlock()

					isPartial, _ := inputTranscription["isPartial"].(bool)
					if isPartial {
						s.logger.Debug("Gemini → Server [USER_TRANSCRIPT_PARTIAL]", zap.String("text", text))
					} else {
						s.logger.Info("Gemini → Server [USER_TRANSCRIPT]", zap.String("text", text))
					}
					s.historyMutex.Lock()
					if isPartial {
						s.currentPartialTranscript = text
					} else {
						// When final, append it. Add a space to separate multiple utterances.
						if s.accumulatedUserTranscript != "" && !strings.HasSuffix(s.accumulatedUserTranscript, " ") {
							s.accumulatedUserTranscript += " "
						}
						s.accumulatedUserTranscript += text
						s.currentPartialTranscript = ""
					}
					s.historyMutex.Unlock()
				}
			}

			// Handle turn complete
			if turnComplete, ok := serverContent["turnComplete"].(bool); ok && turnComplete {
				s.logger.Debug("Gemini → Server [TURN_COMPLETE]")
				s.flushUserTranscript()
				// rawTurnText is kept pre-cleaning: the leaked-tool safety net below needs to
				// see tool syntax the model wrote as text, which the cleaners strip from aiText.
				rawTurnText := strings.TrimSpace(s.accumulatedText)
				if rawTurnText == "" {
					rawTurnText = strings.TrimSpace(s.accumulatedPartsText)
				}
				aiText := CleanLeakedToolCalls(rawTurnText)
				if aiText != "" {
					s.turnSpokeText = true
					s.logHistory("AI", aiText)
					s.writeClientJSON(map[string]interface{}{
						"type": "ai_transcript",
						"text": aiText,
					})
				}
				// Diagnostic only: a valid marker is always ◆+glyph and gets replaced with
				// [PROCESSED_SCREEN] as soon as it's parsed (see screenSymbolRegex above) — a
				// bare ◆ surviving to here means the model wrote its OWN invented marker (e.g.
				// "◆tasks", QA) that never matched, so it was never stripped and — unlike text
				// leaks — was already spoken aloud by the audio model before this code ever ran;
				// nothing server-side can un-speak it. This just makes the failure visible.
				if strings.ContainsRune(aiText, '◆') {
					s.logger.Warn("Unrecognized ◆ marker survived to TURN_COMPLETE — likely spoken aloud", zap.String("text", aiText))
				}
				s.accumulatedText = ""
				s.accumulatedPartsText = ""
				s.sentCleanText = ""
				contentKnown = true
				s.geminiMutex.Lock()
				spokeThisTurn := s.turnSpokeText
				s.turnSpokeText = false // reset for the next turn
				if s.closingInsightSaved && spokeThisTurn {
					s.closingTurnsAfterInsight++
				}
				isPendingRestart := s.pendingRestart
				hadToolCall := s.turnHasToolCall
				s.turnHasToolCall = false // ALWAYS reset for the next turn
				s.silentNudgeSeq++        // any pending silent-tool nudge is now stale
				clientGone := s.clientGone
				transitionInjected := s.transitionInjectedThisTurn
				s.transitionInjectedThisTurn = false // per-turn flag
				suppressedMarker := s.suppressedMarkerThisTurn
				s.suppressedMarkerThisTurn = false // per-turn flag
				restartFallback := s.restartFallbackPrompt
				s.restartFallbackPrompt = ""
				prompt := s.pendingTransitionPrompt
				s.pendingTransitionPrompt = ""
				wheelSetup := s.wheelSetupThisTurn
				s.wheelSetupThisTurn = false     // per-turn flag
				s.revealsScheduledThisTurn = nil // per-turn dedup set
				summaryReveal := s.pendingSummaryReveal
				s.pendingSummaryReveal = false // deferred ◆▦ reveal fires below
				visionCorrectiveSent := s.visionCorrectiveSentThisTurn
				s.visionCorrectiveSentThisTurn = false    // per-turn flag
				s.dupSpeechCorrectiveSentThisTurn = false // per-turn flag
				if s.NeedsDynamicTransitionPrompt {
					s.NeedsDynamicTransitionPrompt = false
					if s.User != nil {
						// A wheel-setup turn with no speech means the model skipped the
						// introduction script — the first-area cue must recover it.
						introSkipped := wheelSetup && !spokeThisTurn && !s.wheelIntroSpokenEarlier
						if introSkipped {
							s.logger.Warn("Wheel created without the introduction being spoken; first-area cue will recover it")
						}
						prompt = s.GetDynamicTransitionPrompt(s.User, introSkipped)
					}
				}
				s.geminiMutex.Unlock()

				if summaryReveal {
					s.logger.Info("Revealing session summary at end of synthesis turn")
					s.emitAndPersistSessionSummary(false)
				}

				if isPendingRestart {
					s.logger.Info("Triggering connection restart on TURN_COMPLETE")
					s.geminiMutex.Lock()
					if s.GeminiWs != nil {
						s.GeminiWs.Close()
					}
					s.geminiMutex.Unlock()
					continue
				}

				// Reset audio-sync tracking for the next turn — logged first so a turn that
				// never received any audio (turnAudioDuration stays 0) is visible in the logs
				// rather than silently indistinguishable from one that did. Captured before the
				// reset so the ENDING_SESSION handling below can compute actual remaining
				// playback instead of a text-length guess — now that the goodbye is spoken in
				// its own clean turn (see the IsEndingSession block below), Gemini paces this
				// turn's audio close to realtime, so by TURN_COMPLETE the real elapsed/duration
				// numbers are a far better signal than estimateSpeechRemainingSecs's estimate.
				s.geminiMutex.Lock()
				completedTurnAudioDuration := s.turnAudioDuration
				completedTurnAudioStartTime := s.turnAudioStartTime
				s.turnAudioStartTime = time.Time{}
				s.turnAudioDuration = 0
				s.geminiMutex.Unlock()
				s.logger.Info("Turn audio duration forwarded to client", zap.Float64("seconds", completedTurnAudioDuration))
				s.recordSpeechRate(aiText, completedTurnAudioDuration)

				var missingToolCallPrompt string

				// Safety net: the 7-minute restart flow displaced a transition prompt and told
				// the model to call restart_session_with_summary. If this turn ended WITHOUT the
				// restart actually starting (tool not called / call failed), fall back to the
				// displaced transition instructions — otherwise the session dies in dead air
				// after the model's "give me a second" filler line.
				if restartFallback != "" && !isPendingRestart && !clientGone {
					s.logger.Warn("Restart flow did not start; falling back to the displaced transition prompt")
					missingToolCallPrompt = restartFallback
				}

				if s.User != nil {
					state := s.CurrentState()
					if state.IsEndingSession() && !hadToolCall {
						// This is now the PRIMARY way a session ends, not a rare fallback: the
						// ENDING_SESSION prompt deliberately asks the model to ONLY speak the
						// goodbye and call no tool at all, because asking it to also call
						// terminate_session in the same turn was intermittently producing a
						// goodbye with a full text transcript but no audio whatsoever (QA — the
						// same injected prompt sometimes yielded real audio, sometimes none, with
						// no reliable trigger found). Terminating here, from the backend, once the
						// speaking-only turn completes removes that "speak AND call a tool in the
						// same breath" pressure entirely. (If the model calls terminate_session
						// anyway, hadToolCall is true and the normal tool-call path already
						// handled it — this is skipped.)
						s.logger.Info("Ending Session turn completed; terminating session on backend")
						remainingAudioSecs := 0.0
						if !completedTurnAudioStartTime.IsZero() {
							// +1s margin: better to hold slightly too long than clip the tail.
							remainingAudioSecs = completedTurnAudioDuration - time.Since(completedTurnAudioStartTime).Seconds() + 1
							if remainingAudioSecs < 0 {
								remainingAudioSecs = 0
							}
						} else {
							// No audio was tracked for this turn at all — fall back to the
							// text-length estimate rather than disconnecting into a turn that
							// may still have audio in flight.
							remainingAudioSecs = s.estimateSpeechRemainingSecs(aiText)
						}
						s.pendingShutdownDelaySecs = remainingAudioSecs
						s.handleTerminateSession(nil)
					}

					// Safety net: the model sometimes writes a state-advancing tool call as PLAIN
					// TEXT ("call:save_ideal_life_vision{vision:...", usually with the brace never
					// closed) instead of invoking it natively. Nothing executes, the state never
					// advances, and the model loops on the same phase questions while the user
					// hears raw code read aloud (QA: five leaked attempts in one session; the user
					// redirected twice and gave up). The first offense gets a corrective ordering
					// a native, silent call. In the vision phase, a REPEAT offense auto-advances
					// server-side: the model has clearly decided the vision is complete and even
					// composed the summary — it just cannot express the call — so extract the
					// vision from the leak and save it ourselves.
					if leakedTool := leakedTransitionToolName(rawTurnText); leakedTool != "" &&
						!hadToolCall && !isPendingRestart && prompt == "" && !clientGone &&
						missingToolCallPrompt == "" {
						s.leakedToolAttempts++
						s.logger.Warn("Model wrote a tool call as plain text instead of a native call",
							zap.String("tool", leakedTool), zap.Int("attempt", s.leakedToolAttempts))

						autoAdvanced := false
						if leakedTool == "save_ideal_life_vision" &&
							state == models.StateVisionIdealLife && s.leakedToolAttempts >= 2 {
							if vision := extractLeakedVision(rawTurnText); vision != "" {
								s.logger.Warn("Repeated leaked save_ideal_life_vision; auto-saving the extracted vision and advancing",
									zap.String("vision", vision))
								s.geminiMutex.Lock()
								// The user already heard (garbled) acknowledgments — skip the
								// extra thank-you bridge on wheel entry.
								s.visionCorrectiveIssued = true
								s.geminiMutex.Unlock()
								if _, saveErr := s.handleSaveIdealLifeVision(map[string]interface{}{"vision": vision}); saveErr != nil {
									s.logger.Error("Auto-save of leaked vision failed", zap.Error(saveErr))
								} else {
									// handleSaveIdealLifeVision set pendingRestart + the wheel-entry
									// directive; close the connection to trigger the restart now
									// (mirrors the HandleToolCall restart path).
									s.geminiMutex.Lock()
									if s.GeminiWs != nil {
										s.GeminiWs.Close()
									}
									s.geminiMutex.Unlock()
									autoAdvanced = true
								}
							}
						}
						if autoAdvanced {
							continue
						}
						missingToolCallPrompt = fmt.Sprintf("[SYSTEM] Your last turn WROTE the tool call as plain text in your spoken output (\"call:%[1]s{...}\") — that does NOTHING: no tool was executed, the session did NOT advance, and the user heard raw code read aloud. Tools are invoked ONLY through your NATIVE function-calling ability, never as written text. Do it right now: call the '%[1]s' function natively and SILENTLY with the arguments you intended. Output NO spoken text in that turn — no apology, no repeated question — and never write 'call:' or braces in your speech again.", leakedTool)
					}

					// Safety net: in the Ideal Life Vision conclusion the model sometimes outputs
					// a screen marker INSTEAD of calling save_ideal_life_vision — it believes the
					// marker advances the session — and then improvises the next exercise itself
					// (asking 0–10 scoring questions while the state, tools, and wheel are not
					// ready). A stray ◆ is normally caught mid-turn by the parse-time tripwire
					// (visionCorrectiveSent); this fallback covers the suppressed well-formed
					// marker case and any turn the tripwire missed.
					strayMarker := strings.Contains(aiText, "◆")
					if (suppressedMarker || strayMarker) && !visionCorrectiveSent &&
						state == models.StateVisionIdealLife &&
						!isPendingRestart && prompt == "" && !clientGone && missingToolCallPrompt == "" {
						s.logger.Warn("Screen marker emitted in vision phase without a transition; injecting save_ideal_life_vision corrective",
							zap.Bool("suppressed", suppressedMarker), zap.Bool("stray", strayMarker))
						missingToolCallPrompt = visionMarkerCorrective
						s.geminiMutex.Lock()
						s.visionCorrectiveIssued = true
						s.geminiMutex.Unlock()
					}

					// Safety net: at the Wheel of Life SETUP step the model must deliver the
					// intro script AND call set_wheel_of_life_categories in the same turn. It
					// sometimes speaks the script and skips the tool — no areas appear on the
					// user's screen, the first-area cue never fires, and the user is left with
					// no question to answer (no CTA). Detect "spoke, no tool, wheel still
					// empty" and course-correct immediately.
					if state == models.StateVisionWheelOfLife && spokeThisTurn && !hadToolCall &&
						!isPendingRestart && prompt == "" && !clientGone && missingToolCallPrompt == "" {
						if len(s.loadOnboardingWheelItems(s.User)) == 0 {
							s.logger.Warn("Wheel setup turn ended without set_wheel_of_life_categories; injecting corrective")
							s.geminiMutex.Lock()
							s.wheelIntroSpokenEarlier = true
							s.geminiMutex.Unlock()
							missingToolCallPrompt = "[SYSTEM] You delivered the Wheel of Life introduction but did NOT call the 'set_wheel_of_life_categories' tool — no areas appeared on the user's screen and the session cannot continue; the user is waiting in silence with nothing to answer. Call 'set_wheel_of_life_categories' NOW with the five default areas translated into the user's language (Health & Energy, Relationships, Purpose & Career, Finances & Lifestyle, Wellbeing & Growth). Do NOT speak and do NOT repeat the introduction — just call the tool silently and yield; the system will then cue you to ask for the first area."
						}
					}
				}

				// Safety net: if the model ended the turn with a tool call but said nothing to
				// the user (e.g. it saved a memory silently), it has gone quiet while the user
				// waits. Nudge it to keep the conversation going. This does NOT apply to silent
				// transition tools (those set isPendingRestart / a transition prompt, or injected
				// an in-session transition directive already), to the wheel re-prompts above, or
				// once the client has disconnected (the closing housekeeping tools — e.g.
				// schedule_notifications — are legitimately silent: nobody is listening).
				var continuePrompt string
				if !spokeThisTurn && hadToolCall && !isPendingRestart && !transitionInjected && !clientGone && prompt == "" && missingToolCallPrompt == "" && !s.pendingShutdown {
					continuePrompt = "[SYSTEM] You just used a tool silently (for example saving a memory) but did not say anything, and the user is now waiting for your response. Continue the conversation immediately: warmly acknowledge what they just shared, then carry on naturally with your active task instructions — let those instructions decide whether to ask another question or to conclude this step and move on. If their last words were an unfinished sentence or they asked for a moment to think, do NOT ask a new question — just give one brief, soft cue that you are here and listening, and wait. Do NOT mention saving anything or that you used a tool."

					// Wheel-of-Life assessment variant: reaching here means the silent tool was
					// NOT update_wheel_of_life (that tool schedules a transition prompt, which is
					// checked above). The model tends to call save_memory with the user's
					// reasoning, believe the score is "saved", and skip to the next area — while
					// the wheel on the user's screen never updates. Redirect it explicitly.
					if s.User != nil &&
						s.CurrentState() == models.StateVisionWheelOfLife {
						continuePrompt = "[SYSTEM] You just used a tool silently but did not say anything, and the user is waiting. IMPORTANT: saving a memory does NOT save the Wheel of Life score — the wheel on the user's screen only updates when you call 'update_wheel_of_life'. If you do not yet have the user's reasoning for the CURRENT area, warmly acknowledge what they shared and ask what sits behind their score. If they have explained their score but you have NOT yet invited them to add more for this area, ask that ONE gentle draw-out follow-up now (the equivalent of 'And what else?' / 'Is there anything more behind that?', in the user's language) and WAIT — their second pass is usually the most valuable thing they say, and skipping it makes the wheel shallow (QA). ONLY once they have emptied the area (their answer repeats itself or they say that's it), you MUST call 'update_wheel_of_life' for that area NOW with EVERYTHING they shared as the reasoning (their real words) before anything else — do NOT move to the next area without it, and never claim the score was saved unless you called that tool. Do NOT mention tools or saving."
					}
				}

				if missingToolCallPrompt != "" {
					s.logger.Info("Skipping client turn_complete because missing tool call prompt is pending, will be injected below")
				} else if prompt != "" {
					s.logger.Info("Skipping client turn_complete because transition prompt is pending, will be injected below")
				} else if continuePrompt != "" {
					s.logger.Warn("Turn ended with a silent tool call and no speech; nudging the model to continue")
				} else {
					if !s.pendingShutdown {
						s.writeClientJSON(map[string]string{"type": "turn_complete"})
					}
				}

				// Session shutdown (pendingShutdown) is handled at the end of the message
				// loop, immediately after terminate_session: session_terminated is sent right
				// away and the FRONTEND owns the close — it keeps playing the buffered
				// goodbye audio and only tears down when playback ends. No hold happens here.

				if missingToolCallPrompt != "" {
					s.InjectPrompt(missingToolCallPrompt)
				} else if prompt != "" {
					// Once the client has disconnected, transition prompts (e.g. the first-area
					// wheel cue) must NOT fire — they would race the closing housekeeping and
					// confuse the model into resuming the exercise with nobody listening (QA:
					// it called update_wheel_of_life with a fabricated score-0 answer right
					// after the push-notifications prompt).
					if clientGone {
						s.logger.Info("Dropping transition prompt; client disconnected", zap.String("prompt", prompt))
					} else {
						// System-driven double-turn: the AI just spoke and we are about to make it
						// speak again with no user turn in between (e.g. Wheel intro -> first-area
						// question). Without a gap, the second turn's audio butts right up against the
						// first and sounds glued. If the AI actually spoke this turn, append 1s of
						// silence to the client stream so the two utterances are cleanly separated.
						if spokeThisTurn {
							s.injectSilence(1)
						}
						s.logger.Info("Injecting transition prompt on TURN_COMPLETE", zap.String("prompt", prompt))
						s.InjectTransitionPrompt(prompt)
					}
				} else if continuePrompt != "" {
					s.scheduleSilentToolNudge(continuePrompt)
				}
			}

			if !contentKnown {
				s.logger.Debug("Gemini → Server [SERVER_CONTENT_UNKNOWN]", zap.Any("content", serverContent))
			}
		}

		if !known {
			// Extract sessionResumptionUpdate
			if sru, ok := resp["sessionResumptionUpdate"].(map[string]interface{}); ok {
				if newHandle, ok := sru["newHandle"].(string); ok {
					s.geminiMutex.RLock()
					gone := s.clientGone
					s.geminiMutex.RUnlock()
					if gone {
						// The user has left and the shutdown flow already cleared the handle.
						// Persisting handles from the closing housekeeping turns (push
						// notifications) would let the NEXT session "ghost-resume" into this
						// dead conversation.
						known = true
					} else {
						s.latestSessionHandle = newHandle
						// Persist to DB so it survives browser refreshes
						now := time.Now()
						database.DB.Model(s.User).Updates(map[string]interface{}{
							"latest_session_handle":    newHandle,
							"latest_session_handle_at": now,
						})
						known = true
					}
				}
			}
		}

		if !known {
			s.logger.Debug("Gemini → Server [OTHER]", zap.Any("response", resp))
			if respJSON, err := json.Marshal(resp); err == nil {
				s.logHistory("Gemini Unprocessed Response", string(respJSON))
			}
		}

		if s.pendingShutdown {
			s.pendingShutdown = false
			// Sending session_terminated the instant terminate_session was processed raced
			// ahead of the goodbye's own audio: on this shape of turn Gemini streams the full
			// transcript well before the corresponding audio bytes start arriving at all
			// (confirmed via turnAudioStartTime still being zero at that point in QA logs),
			// so the client's playback-remaining estimate — built only from audio it has
			// actually received — was ~0 and it disconnected before the goodbye's tail
			// (including the bridge sentence to the next session) ever reached it. Hold for
			// our own text-length estimate of that turn's speech (estimateSpeechRemainingSecs,
			// stashed in pendingShutdownDelaySecs at dispatch time) before sending.
			delaySecs := s.pendingShutdownDelaySecs
			s.logger.Info("AI ended the session: sending session_terminated after remaining audio; the frontend closes once the outro audio finishes",
				zap.Float64("delaySecs", delaySecs))
			go func(session *ChatSession, delay float64) {
				if delay > 0 {
					time.Sleep(time.Duration(delay * float64(time.Second)))
				}
				session.writeClientJSON(map[string]interface{}{
					"type": "session_terminated",
					"data": map[string]string{
						"reason": "AI_REQUESTED",
					},
				})
				// The frontend owns the close: it keeps playing the already-buffered outro
				// audio and closes the socket itself when playback ends. We only force-close
				// as a long safety net, so a frontend that never closes (e.g. it crashed or
				// dropped) does not leak this connection. We must NOT cut the audio ourselves.
				time.Sleep(60 * time.Second)
				session.ClientWs.Close()
			}(s, delaySecs)
			continue
		}
	}
}

// queueShowScreen delays the show_screen message so the frontend opens the screen
// at the moment the audio playback reaches the [SS=...] tag's position in the speech,
// rather than the instant the tag is parsed (which arrives well ahead of the audio).
// markSuppressedMarker records that a screen-marker reveal was blocked in the current turn, so
// the TURN_COMPLETE handler can course-correct the model (it likely used the marker in place of
// the required transition tool, which would otherwise leave the user in dead air).
func (s *ChatSession) markSuppressedMarker() {
	s.geminiMutex.Lock()
	s.suppressedMarkerThisTurn = true
	s.geminiMutex.Unlock()
}

func (s *ChatSession) queueShowScreen(screenName string, textBeforeTag string) {
	// Central policy: screen reveals are only ever allowed for data-less screens (see
	// showScreenAllowed). A data-bearing screen like the Wheel of Life that slips through a
	// (legacy) tag is ignored here, so it can never be opened empty by a screen reveal.
	if !showScreenAllowed[screenName] {
		s.logger.Warn("Ignoring screen reveal for non-allowed screen", zap.String("screen", screenName))
		s.markSuppressedMarker()
		return
	}

	// The intro tour's screens (memories, Journey, Talk, profile) must NOT be revealed
	// during the Ideal Life Vision,
	// Metaphor, or Wheel of Life phases. The model sometimes leaks their markers there (it
	// remembered them from the intro tour); ignore the reveal in those states so the screen
	// does not pop up mid-exercise and pull the user out of the conversation.
	if (screenName == "memories" || screenName == "journey" || screenName == "profile" || screenName == "session") && s.User != nil {
		switch s.CurrentState() {
		case models.StateVisionIdealLife,
			models.StateVisionMetaphor,
			models.StateVisionWheelOfLife:
			s.logger.Warn("Ignoring screen reveal during onboarding exercise phase",
				zap.String("screen", screenName), zap.String("state", string(s.CurrentState())))
			s.markSuppressedMarker()
			return
		}
	}

	s.scheduleScreenReveal(screenName, textBeforeTag)
}

// silentToolNudgeDelay is how long the silent-tool nudge holds back before filling the
// silence. The nudge exists for the user who is genuinely waiting after a silent save —
// but when the model saved a memory off a HALF-FINISHED thought, firing instantly made
// Rumi speak right as the user drew breath to continue, colliding with them mid-sentence
// (QA: "Calma, deixa eu falar — estás a interromper-me"). A human waits a beat before
// filling a pause; so does this. If the user resumes speaking during the delay, the nudge
// stands down — Gemini will answer their actual words instead, and the 9s dead-air
// watchdog still guards the case where that answer never comes.
const silentToolNudgeDelay = 2500 * time.Millisecond

// scheduleSilentToolNudge delivers the silent-tool "keep the conversation going" prompt
// after silentToolNudgeDelay, unless it has become moot: the user resumed speaking
// (Gemini emitted a fresh inputTranscription — never client audio frames, which flow
// continuously even in silence), a newer turn completed (silentNudgeSeq moved on), or
// the session is closing/restarting.
func (s *ChatSession) scheduleSilentToolNudge(prompt string) {
	scheduledAt := time.Now().UnixNano()
	s.geminiMutex.RLock()
	seq := s.silentNudgeSeq
	s.geminiMutex.RUnlock()

	go func() {
		time.Sleep(silentToolNudgeDelay)
		if s.lastUserSpeechUnixNano.Load() > scheduledAt {
			s.logger.Info("Dropping silent-tool nudge; the user resumed speaking")
			return
		}
		s.geminiMutex.RLock()
		superseded := s.silentNudgeSeq != seq || s.clientGone || s.pendingShutdown || s.pendingRestart
		s.geminiMutex.RUnlock()
		if superseded {
			s.logger.Info("Dropping silent-tool nudge; superseded by a newer turn or shutdown")
			return
		}
		s.InjectPrompt(prompt)
	}()
}

// recordSpeechRate folds a completed turn's real transcript-length / audio-duration ratio
// into the session's measured speech rate. Short turns and turns with little or no audio
// are skipped (their ratio is noise), and implausible ratios are discarded rather than
// clamped — a truncated turn must not drag the estimate around. Blended 60/40 toward the
// newest sample so the rate adapts within a couple of turns of session start.
func (s *ChatSession) recordSpeechRate(turnText string, audioSeconds float64) {
	if audioSeconds < 5 {
		return
	}
	clean := strings.ReplaceAll(turnText, "[PROCESSED_SCREEN]", "")
	clean = processedPauseRegex.ReplaceAllString(clean, "")
	chars := len([]rune(strings.TrimSpace(clean)))
	if chars < 80 {
		return
	}
	rate := float64(chars) / audioSeconds
	if rate < 8 || rate > 30 {
		return
	}
	s.geminiMutex.Lock()
	if s.measuredCharsPerSecond == 0 {
		s.measuredCharsPerSecond = rate
	} else {
		s.measuredCharsPerSecond = 0.6*s.measuredCharsPerSecond + 0.4*rate
	}
	measured := s.measuredCharsPerSecond
	s.geminiMutex.Unlock()
	s.logger.Debug("Updated measured speech rate",
		zap.Float64("turn_rate", rate), zap.Float64("blended_rate", measured))
}

// speechRate returns the best current chars-per-second estimate for this session: the
// measured, per-session rate once at least one real turn has been observed, else the
// static default.
func (s *ChatSession) speechRate() float64 {
	s.geminiMutex.Lock()
	defer s.geminiMutex.Unlock()
	if s.measuredCharsPerSecond > 0 {
		return s.measuredCharsPerSecond
	}
	return estimatedSpeechCharsPerSecond
}

// estimateSpeechRemainingSecs estimates how many seconds of playback remain for a turn's
// spoken text, using the same text-length heuristic scheduleScreenReveal uses for markers
// (estimatedSpeechCharsPerSecond), minus whatever time has already elapsed if this turn's
// audio has started arriving. On a turn that ends in a tool call — notably the goodbye +
// terminate_session turn — the actual audio bytes for the turn often have not started
// arriving at all yet at the point the tool call is processed (the transcript streams well
// ahead of the audio here), so turnAudioDuration/turnAudioStartTime byte-accounting cannot
// be trusted to say how much playback is left; this text-length estimate is the only signal
// available in time to hold session_terminated back.
func (s *ChatSession) estimateSpeechRemainingSecs(text string) float64 {
	if text == "" {
		return 0
	}
	estimate := float64(len([]rune(text))) / s.speechRate()
	s.geminiMutex.Lock()
	audioStart := s.turnAudioStartTime
	s.geminiMutex.Unlock()
	if !audioStart.IsZero() {
		estimate -= time.Since(audioStart).Seconds()
	}
	if estimate < 0 {
		return 0
	}
	return estimate
}

// scheduleScreenReveal times a reveal parsed MID-stream and sends show_screen when the
// client's audio playback reaches the reveal's position in the speech.
//
// The transcript streams far ahead of the audio, and Gemini paces the audio stream at
// ≈ realtime — so by TURN_COMPLETE the speech has effectively finished playing. The
// previous design deferred marker reveals to TURN_COMPLETE for "exact" position math,
// which meant every reveal fired at the very END of the speech (QA: the model announced
// the memories screen, but it only opened when it was already talking about something
// else). Instead, estimate the marker's audio offset from the length of the text spoken
// before it (estimatedSpeechCharsPerSecond) plus any injected pauses, and fire relative
// to the turn's audio start.
func (s *ChatSession) scheduleScreenReveal(screenName string, textBeforeTag string) {
	s.geminiMutex.Lock()
	capturedInterrupts := s.interruptCounter
	audioStart := s.turnAudioStartTime
	if s.revealsScheduledThisTurn == nil {
		s.revealsScheduledThisTurn = map[string]bool{}
	}
	s.revealsScheduledThisTurn[screenName] = true
	if screenName == sessionSummaryMarkerScreen {
		// The ◆▦ marker opens the synthesis turn, so the estimated-position schedule
		// below would fire the panel the instant Rumi STARTS speaking — making the user
		// read and listen at once (QA: "não dá para fazer a leitura enquanto ouvimos ao
		// mesmo tempo"). Defer to TURN_COMPLETE instead: Gemini paces audio at
		// ≈realtime, so it arrives as the synthesis has effectively been heard — the
		// panel appears right as "does this make sense to you?" lands, when the user
		// can actually read it. (◆▧ keeps the estimated timing: it sits mid-goodbye.)
		s.pendingSummaryReveal = true
		s.geminiMutex.Unlock()
		return
	}
	s.geminiMutex.Unlock()

	// Injected pauses before the marker play as real seconds on top of the spoken text.
	totalPauseBefore := 0
	for _, m := range processedPauseRegex.FindAllStringSubmatch(textBeforeTag, -1) {
		var ps int
		fmt.Sscanf(m[1], "%d", &ps)
		totalPauseBefore += ps
	}
	cleanBefore := processedPauseRegex.ReplaceAllString(textBeforeTag, "")
	cleanBefore = strings.ReplaceAll(cleanBefore, "[PROCESSED_SCREEN]", "")

	// Fire slightly BEFORE the marker's estimated position: the screen should already be
	// visible as the sentence announcing it lands ("I've just opened your memories screen"),
	// not appear at its tail (QA tuning). Uses the session's measured speech rate when one
	// exists — the static default drifted seconds off on the intro tour's long turn (QA).
	offset := float64(len([]rune(cleanBefore)))/s.speechRate() + float64(totalPauseBefore) - screenRevealLeadSeconds
	if offset < 0 {
		offset = 0
	}
	delay := offset
	if !audioStart.IsZero() {
		delay = offset - time.Since(audioStart).Seconds()
	}

	fire := func() {
		switch screenName {
		case sessionSummaryMarkerScreen:
			s.emitAndPersistSessionSummary(false)
			return
		case sessionSummaryNextMarkerScreen:
			s.emitAndPersistSessionSummary(true)
			return
		}
		s.writeClientJSON(map[string]interface{}{
			"type": "show_screen",
			"data": map[string]string{
				"screen": screenName,
			},
		})
	}

	if delay > 0 {
		s.logger.Info("Queueing reveal with delay to sync with audio", zap.String("screen", screenName), zap.Float64("delaySeconds", delay))
		go func() {
			time.Sleep(time.Duration(delay * float64(time.Second)))

			s.geminiMutex.Lock()
			if s.interruptCounter != capturedInterrupts {
				s.logger.Info("Aborting queued reveal because of user interrupt", zap.String("screen", screenName))
				s.geminiMutex.Unlock()
				return
			}
			s.geminiMutex.Unlock()

			fire()
		}()
	} else {
		s.logger.Info("Executing reveal immediately (marker position already played)", zap.String("screen", screenName))
		fire()
	}
}

func (s *ChatSession) injectSilence(seconds int) {
	if seconds <= 0 {
		return
	}
	s.logger.Info("Injecting sample-accurate silence directly to client WebSocket", zap.Int("seconds", seconds))

	// 24kHz sample rate, 16-bit PCM (2 bytes per sample)
	sampleRate := 24000
	bytesPerSecond := sampleRate * 2
	silenceBytes := make([]byte, seconds*bytesPerSecond)

	if err := s.writeClientMessage(websocket.BinaryMessage, silenceBytes); err != nil {
		s.logger.Error("Failed to write silence bytes to client WS", zap.Error(err))
	} else {
		// Add silence duration to audio tracking to keep screen-transition sync accurate.
		s.geminiMutex.Lock()
		if s.turnAudioStartTime.IsZero() {
			s.turnAudioStartTime = time.Now()
			s.turnAudioDuration = 0
		}
		s.turnAudioDuration += float64(seconds)
		s.geminiMutex.Unlock()
	}
}

func (s *ChatSession) Cleanup() {
	if s.ClientWs != nil {
		s.writeClientMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		time.Sleep(100 * time.Millisecond)
		s.ClientWs.Close()
	}
	s.geminiMutex.Lock()
	if s.GeminiWs != nil {
		s.GeminiWs.Close()
	}
	s.geminiMutex.Unlock()

	if s.stopChan != nil {
		select {
		case <-s.stopChan:
		default:
			close(s.stopChan)
		}
	}

	s.flushUserTranscript()
	aiText := strings.TrimSpace(s.accumulatedText)
	if aiText == "" {
		aiText = strings.TrimSpace(s.accumulatedPartsText)
	}
	aiText = CleanLeakedToolCalls(aiText)
	if aiText != "" {
		s.logHistory("AI", aiText)
	}
	s.accumulatedText = ""
	s.accumulatedPartsText = ""
	s.sentCleanText = ""

	historyLog := s.getHistoryLog()
	if historyLog != "" {
		s.SessionDB.Transcript = &historyLog
	}

	endTime := time.Now()
	s.SessionDB.EndTime = &endTime
	elapsed := int64(endTime.Sub(s.SessionDB.StartTime).Seconds())
	s.SessionDB.Duration = int(elapsed)
	database.DB.Save(&s.SessionDB)

	// Safety net for the synthesis screen: if the session reached its closing (the
	// insight was saved) but terminate_session never fired — model omission, or the
	// client dropped during the goodbye — persist the summary now so the app can still
	// render the card for this session. Idempotent: a no-op when terminate_session
	// already emitted it, and buildSessionSummary returns nil for session types
	// without a card.
	if s.SessionDB.UserSessionInsight != nil && *s.SessionDB.UserSessionInsight != "" {
		s.emitAndPersistSessionSummary(true)
	}

	// What the session cost US, in the cost ledger — always, whether or not the user
	// was charged for it. The two are separate questions and the free introductory
	// sessions are exactly where they diverge: those cost real tokens and produce no
	// debit, so reading spend from balance_transactions alone understates it.
	//
	// This is the ONLY place the connection's tokens are persisted: they are counted in
	// memory during the session and land here, not on the session row, so that a voice
	// session can be summed against messages and background generations. RefID ties the
	// row back to the session it paid for.
	if s.SessionDB.ID != "" {
		aiusage.Write(context.Background(), aiusage.Record{
			UserID:            s.UserID,
			Kind:              models.AIUsageSession,
			Model:             liveModelName(),
			RefType:           models.AIUsageRefSession,
			RefID:             s.SessionDB.ID,
			InputTokens:       s.tokens.Input,
			OutputTokens:      s.tokens.Output,
			TotalTokens:       s.tokens.Total,
			InputTextTokens:   s.tokens.InputText,
			OutputTextTokens:  s.tokens.OutputText,
			InputAudioTokens:  s.tokens.InputAudio,
			OutputAudioTokens: s.tokens.OutputAudio,
			InputVideoTokens:  s.tokens.InputVideo,
			OutputVideoTokens: s.tokens.OutputVideo,
		})
	}

	// Debit the minutes balance for the elapsed session time (the introductory
	// sessions are free). Synchronous so a user who closes and immediately reconnects
	// sees the debited balance at the WS pre-flight. A debit failure must never break
	// session close: log and continue — the unique session_id ledger index
	// also guarantees a session can never be charged twice.
	if s.SessionDB.ID != "" {
		if s.balanceExempt {
			// A zero-amount row rather than nothing at all: it is what lets the usage
			// history show the session as free. Without it the session is simply absent,
			// which is indistinguishable from a debit that failed.
			if _, err := balance.RecordFreeSession(context.Background(), s.UserID, s.SessionDB.ID, s.SessionDB.SessionType); err != nil {
				s.logger.Error("Failed to record free session", zap.Error(err),
					zap.String("session_id", s.SessionDB.ID))
			}
		} else if _, err := balance.DebitSession(context.Background(), s.UserID, s.SessionDB.ID, s.SessionDB.SessionType, elapsed); err != nil {
			s.logger.Error("Failed to debit session usage", zap.Error(err),
				zap.String("session_id", s.SessionDB.ID), zap.Int64("elapsed_seconds", elapsed))
		}
	}

	// Award any badges this session just earned (first deep session, streaks, ...).
	// Here — at the moment of achievement — rather than only on the next Profile
	// visit, so earned_at is a truthful analytics timestamp. Runs after EndTime is
	// saved because the conditions count only ended sessions; in the background
	// because badge writes must never delay session close.
	if s.UserID != "" {
		go badge.EvaluateAndAward(s.UserID, s.Location, s.logger)
	}

	// Trigger LLM session review in the background if the session lasted > 10 seconds and transcript is not empty
	if historyLog != "" && time.Since(s.SessionDB.StartTime) > 10*time.Second {
		s.runPostSessionAnalysis(s.SessionDB.ID, s.SessionType, s.getCleanHistoryLog())
	}

	s.enqueueCompanionFollowUp()
}

// runPostSessionAnalysis kicks off the background recap + AI review for a finished
// communication_sessions row. Called from Cleanup for the connection's final row, and
// from rolloverSessionDB for the row closed mid-connection at a planned-session
// handover — sessionType must be the type of THAT row, not the session the connection
// moved on to.
func (s *ChatSession) runPostSessionAnalysis(sessionID string, sessionType api.SessionType, cleanTranscript string) {
	// Each session type provides its own review rubric; fall back to the generic one.
	reviewPrompt := ""
	if sess, ok := sessions.Get(sessionType); ok {
		reviewPrompt = sess.ReviewPrompt()
	}
	// The user-facing recap for the sessions list. Skipped for the onboarding intro:
	// it collects registration details and gives a tour — there is no coaching content
	// to summarize, and a row saying "you told me where you live" helps nobody.
	if sessionType != api.SessionTypeOnboarding {
		language := ""
		if s.User != nil && s.User.PreferredLanguage != nil {
			language = *s.User.PreferredLanguage
		}
		go func(userID, sessionID, transcript, language string, logger *zap.Logger) {
			title, recap, err := GenerateSessionRecap(context.Background(), transcript, language, userID, sessionID)
			if err != nil {
				// No fallback text: an empty recap makes the app fall back to the
				// session-type label, which is better than storing an error string
				// the user would read in their history.
				logger.Error("Failed to generate session recap", zap.Error(err), zap.String("session_id", sessionID))
				return
			}
			if recap == "" {
				return
			}
			updates := map[string]interface{}{"recap": recap}
			if title != "" {
				updates["recap_title"] = title
			}
			if err := database.DB.Model(&models.CommunicationSession{}).Where("id = ?", sessionID).
				Updates(updates).Error; err != nil {
				logger.Error("Failed to save session recap", zap.Error(err), zap.String("session_id", sessionID))
			} else {
				logger.Info("Session recap saved", zap.String("session_id", sessionID))
			}
		}(s.UserID, sessionID, cleanTranscript, language, s.logger)
	}

	s.logger.Info("Triggering background AI session review...", zap.String("session_id", sessionID))
	go func(userID, sessionID string, transcript string, reviewPrompt string, logger *zap.Logger) {
		notes, evaluation, err := GenerateSessionReview(context.Background(), transcript, reviewPrompt, userID, sessionID)
		if err != nil {
			logger.Error("Failed to generate AI session review", zap.Error(err))

			// Fallback to unblock the simulator loop on API timeouts
			fallbackUpdates := map[string]interface{}{
				"ai_notes":      fmt.Sprintf("API Timeout/Error: %v", err),
				"ai_evaluation": 1.0,
			}
			database.DB.Model(&models.CommunicationSession{}).Where("id = ?", sessionID).Updates(fallbackUpdates)
			return
		}

		updates := map[string]interface{}{
			"ai_notes":      notes,
			"ai_evaluation": evaluation,
		}

		if err := database.DB.Model(&models.CommunicationSession{}).Where("id = ?", sessionID).Updates(updates).Error; err != nil {
			logger.Error("Failed to save AI session notes and evaluation to database", zap.Error(err))
		} else {
			logger.Info("AI session notes and evaluation successfully saved to database")
		}
	}(s.UserID, sessionID, cleanTranscript, reviewPrompt, s.logger)
}

// enqueueCompanionFollowUp schedules a post-session WhatsApp check-in a few
// hours after a real (5+ minute) session, for users with a linked channel.
// The companion dispatcher drains it and handles the 24h-window rules.
func (s *ChatSession) enqueueCompanionFollowUp() {
	if !config.AppConfig.WhatsAppEnabled || time.Since(s.SessionDB.StartTime) < 5*time.Minute {
		return
	}
	var integration models.Integration
	if err := database.DB.Where("user_id = ? AND status = ?", s.UserID, models.IntegrationActive).First(&integration).Error; err != nil {
		return
	}
	var pending int64
	if err := database.DB.Model(&models.ChannelFollowUp{}).
		Where("binding_id = ? AND kind = ? AND sent_at IS NULL AND failed_at IS NULL", integration.ID, models.ChannelFollowUpPostSession).
		Count(&pending).Error; err != nil || pending > 0 {
		return
	}
	followUp := models.ChannelFollowUp{
		UserID:      s.UserID,
		BindingID:   integration.ID,
		Kind:        models.ChannelFollowUpPostSession,
		ScheduledAt: time.Now().Add(4 * time.Hour),
	}
	if err := database.DB.Create(&followUp).Error; err != nil {
		s.logger.Warn("Failed to enqueue post-session companion follow-up", zap.Error(err))
	}
}

// sessionTokens is one connection-half's spend on the Live model, kept per modality
// because audio and text are priced very differently ($12.00 against $4.50 per million
// output tokens) and a voice session is almost entirely audio.
type sessionTokens struct {
	Input, Output, Total                 int
	InputText, InputAudio, InputVideo    int
	OutputText, OutputAudio, OutputVideo int
}

func (s *ChatSession) UpdateUsageMetadata(meta map[string]interface{}) {
	s.tokens.Input += getInt(meta, "promptTokenCount")
	s.tokens.Output += getInt(meta, "candidatesTokenCount")
	s.tokens.Total += getInt(meta, "totalTokenCount")

	if pDetails, ok := meta["promptTokensDetails"].([]interface{}); ok {
		for _, d := range pDetails {
			if detail, ok := d.(map[string]interface{}); ok {
				modality, _ := detail["modality"].(string)
				count := getInt(detail, "tokenCount")
				switch strings.ToUpper(modality) {
				case "TEXT":
					s.tokens.InputText += count
				case "AUDIO":
					s.tokens.InputAudio += count
				case "VIDEO":
					s.tokens.InputVideo += count
				}
			}
		}
	}

	if cDetails, ok := meta["candidatesTokensDetails"].([]interface{}); ok {
		for _, d := range cDetails {
			if detail, ok := d.(map[string]interface{}); ok {
				modality, _ := detail["modality"].(string)
				count := getInt(detail, "tokenCount")
				switch strings.ToUpper(modality) {
				case "TEXT":
					s.tokens.OutputText += count
				case "AUDIO":
					s.tokens.OutputAudio += count
				case "VIDEO":
					s.tokens.OutputVideo += count
				}
			}
		}
	}
}

func getInt(m map[string]interface{}, key string) int {
	val, ok := m[key]
	if !ok {
		return 0
	}
	switch v := val.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return 0
	}
}

func (s *ChatSession) logHistory(category string, message string) {
	s.historyMutex.Lock()
	defer s.historyMutex.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("[%s] [%s] %s", timestamp, strings.ToUpper(category), message)
	s.history = append(s.history, entry)
}

func (s *ChatSession) flushUserTranscript() {
	s.historyMutex.Lock()
	defer s.historyMutex.Unlock()

	// Combine finalized accumulated text and the current partial (if any)
	fullText := s.accumulatedUserTranscript
	if s.currentPartialTranscript != "" {
		if fullText != "" && !strings.HasSuffix(fullText, " ") {
			fullText += " "
		}
		fullText += s.currentPartialTranscript
	}

	trimmed := strings.TrimSpace(fullText)
	if trimmed != "" {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		entry := fmt.Sprintf("[%s] [USER] %s", timestamp, trimmed)
		s.history = append(s.history, entry)
		s.accumulatedUserTranscript = ""
		s.currentPartialTranscript = ""
		s.userMsgsSinceWheelSave++
		if s.CurrentState() == models.StateVisionIdealLife {
			s.visionUserTurns++
		}
	}
}

func (s *ChatSession) flushPendingToolResponses() {
	s.geminiMutex.Lock()
	defer s.geminiMutex.Unlock()

	if len(s.pendingToolResponses) == 0 {
		return
	}

	s.logger.Info("Sending accumulated tool responses to Gemini",
		zap.Int("count", len(s.pendingToolResponses)),
	)

	response := map[string]interface{}{
		"toolResponse": map[string]interface{}{
			"functionResponses": s.pendingToolResponses,
		},
	}
	s.pendingToolResponses = nil

	if s.GeminiWs != nil {
		if err := s.writeGeminiJSONWithConn(s.GeminiWs, response); err != nil {
			s.logger.Error("Failed to send accumulated tool responses to Gemini", zap.Error(err))
		}
	}
}

func (s *ChatSession) getHistoryLog() string {
	s.historyMutex.Lock()
	defer s.historyMutex.Unlock()
	if len(s.history) == 0 {
		return ""
	}
	return strings.Join(s.history, "\n")
}

func (s *ChatSession) getCleanHistoryLog() string {
	s.historyMutex.Lock()
	defer s.historyMutex.Unlock()

	var clean []string
	for _, entry := range s.history {
		if strings.Contains(entry, "] [Inject Prompt]") ||
			strings.Contains(entry, "] [Inject Prompt No-Trigger]") ||
			strings.Contains(entry, "] [System Instruction]") ||
			strings.Contains(entry, "] [Gemini Unprocessed Response]") ||
			strings.Contains(entry, "] [Server to Client") ||
			strings.Contains(entry, "] [Client Control Message]") ||
			strings.Contains(entry, "] [System]") {
			continue
		}
		clean = append(clean, entry)
	}

	if len(clean) == 0 {
		return ""
	}
	return strings.Join(clean, "\n")
}

func (s *ChatSession) getLastAIMessage() string {
	s.historyMutex.Lock()
	defer s.historyMutex.Unlock()
	for i := len(s.history) - 1; i >= 0; i-- {
		if strings.Contains(s.history[i], "[AI]") {
			return s.history[i]
		}
	}
	return ""
}

// lastEntryIsAIQuestion reports whether the newest transcript entry is the model's own
// speech ending in a question mark — i.e. the model just asked the user something and no
// answer has arrived yet. Punctuation-based, so it works in every language.
func (s *ChatSession) lastEntryIsAIQuestion() bool {
	s.historyMutex.Lock()
	defer s.historyMutex.Unlock()
	if len(s.history) == 0 {
		return false
	}
	last := s.history[len(s.history)-1]
	const tag = "] [AI] "
	idx := strings.Index(last, tag)
	if idx == -1 {
		return false
	}
	return strings.HasSuffix(strings.TrimSpace(last[idx+len(tag):]), "?")
}

// getRecentUserMessages returns up to n most-recent [USER] transcript entries (newest
// first), with the timestamp/role prefix stripped.
func (s *ChatSession) getRecentUserMessages(n int) []string {
	s.historyMutex.Lock()
	defer s.historyMutex.Unlock()
	const tag = "] [USER] "
	var out []string
	for i := len(s.history) - 1; i >= 0 && len(out) < n; i-- {
		if idx := strings.Index(s.history[i], tag); idx != -1 {
			out = append(out, s.history[i][idx+len(tag):])
		}
	}
	return out
}

// getLastUserMessage returns the text of the most recent [USER] transcript entry, with the
// timestamp/role prefix stripped, or "" when the user has not spoken yet.
func (s *ChatSession) getLastUserMessage() string {
	s.historyMutex.Lock()
	defer s.historyMutex.Unlock()
	const tag = "] [USER] "
	for i := len(s.history) - 1; i >= 0; i-- {
		if idx := strings.Index(s.history[i], tag); idx != -1 {
			return s.history[i][idx+len(tag):]
		}
	}
	return ""
}
