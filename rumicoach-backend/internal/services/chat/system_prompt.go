package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/chat/session"
	"go.uber.org/zap"
)

// LanguageInstructions covers exactly the languages the app supports. Keys are the
// BCP-47 codes clients store in preferred_language; an unknown code falls back to the
// generic "respond in the customer preferred language" directive at the call sites.
var LanguageInstructions = map[string]string{
	"en-US": "IMPORTANT: RESPOND IN US English (en-US). YOU MUST RESPOND UNMISTAKABLY IN US English (en-US).",
	"en-GB": "IMPORTANT: RESPOND IN British English (en-GB). YOU MUST RESPOND UNMISTAKABLY IN British English (en-GB). Use British grammar and vocabulary, and SPEAK WITH A BRITISH ACCENT (Received Pronunciation) — never an American accent or American pronunciation.",
	"pt-PT": "IMPORTANT: RESPOND IN European Portuguese (pt-PT). YOU MUST RESPOND UNMISTAKABLY IN European Portuguese (pt-PT). Use the grammar construction 'estar a + infinitive' (such as 'estás a sentir' or 'estamos a construir') instead of the Brazilian gerund. Use European Portuguese terms (such as 'ecrã', 'equipa', 'facto', and 'planeamento'). Address the user as 'Tu' for a warm and respectful informality, avoiding 'Você'. For a coaching insight, use the borrowed word 'insight' (\"qual é o insight que levas...\") — NEVER 'introspeção', which reads as jargon and confuses users (QA).",
	"pt-BR": "IMPORTANT: RESPOND IN Portuguese (Brazil) (pt-BR). YOU MUST RESPOND UNMISTAKABLY IN Portuguese (Brazil) (pt-BR). For a coaching insight, use the borrowed word 'insight' — never 'introspecção'.",
	"es-ES": "IMPORTANT: RESPOND IN European Spanish (Español de España) (es-ES). YOU MUST RESPOND UNMISTAKABLY IN European Spanish (Español de España) (es-ES). Use the grammar and vocabulary typical of Spain, and SPEAK WITH A EUROPEAN SPANISH (CASTILIAN) ACCENT — never a Latin American accent or pronunciation.",
	"de-DE": "IMPORTANT: RESPOND IN German (Germany) (de-DE). YOU MUST RESPOND UNMISTAKABLY IN German (Germany) (de-DE).",
	"fr-FR": "IMPORTANT: RESPOND IN French (France) (fr-FR). YOU MUST RESPOND UNMISTAKABLY IN French (France) (fr-FR).",
	"it-IT": "IMPORTANT: RESPOND IN Italian (Italy) (it-IT). YOU MUST RESPOND UNMISTAKABLY IN Italian (Italy) (it-IT).",
	"ja-JP": "IMPORTANT: RESPOND IN Japanese (Japan) (ja-JP). YOU MUST RESPOND UNMISTAKABLY IN Japanese (Japan) (ja-JP).",
	"zh-CN": "IMPORTANT: RESPOND IN Chinese (Simplified, China) (zh-CN). YOU MUST RESPOND UNMISTAKABLY IN Chinese (Simplified, China) (zh-CN).",
	"ko-KR": "IMPORTANT: RESPOND IN Korean (Korea) (ko-KR). YOU MUST RESPOND UNMISTAKABLY IN Korean (Korea) (ko-KR).",
	"hi-IN": "IMPORTANT: RESPOND IN Hindi (India) (hi-IN). YOU MUST RESPOND UNMISTAKABLY IN Hindi (India) (hi-IN).",
	"pl-PL": "IMPORTANT: RESPOND IN Polish (Poland) (pl-PL). YOU MUST RESPOND UNMISTAKABLY IN Polish (Poland) (pl-PL).",
	"tr-TR": "IMPORTANT: RESPOND IN Turkish (Turkey) (tr-TR). YOU MUST RESPOND UNMISTAKABLY IN Turkish (Turkey) (tr-TR).",
	"uk-UA": "IMPORTANT: RESPOND IN Ukrainian (Ukraine) (uk-UA). YOU MUST RESPOND UNMISTAKABLY IN Ukrainian (Ukraine) (uk-UA).",
	"sv-SE": "IMPORTANT: RESPOND IN Swedish (Sweden) (sv-SE). YOU MUST RESPOND UNMISTAKABLY IN Swedish (Sweden) (sv-SE).",
	"nl-NL": "IMPORTANT: RESPOND IN Dutch (Netherlands) (nl-NL). YOU MUST RESPOND UNMISTAKABLY IN Dutch (Netherlands) (nl-NL).",
	"nb-NO": "IMPORTANT: RESPOND IN Norwegian (Bokmål) (nb-NO). YOU MUST RESPOND UNMISTAKABLY IN Norwegian (Bokmål) (nb-NO).",
	"da-DK": "IMPORTANT: RESPOND IN Danish (Denmark) (da-DK). YOU MUST RESPOND UNMISTAKABLY IN Danish (Denmark) (da-DK).",
	"fi-FI": "IMPORTANT: RESPOND IN Finnish (Finland) (fi-FI). YOU MUST RESPOND UNMISTAKABLY IN Finnish (Finland) (fi-FI).",
}

// FormatBehaviorPlansContext renders the user's behavior commitments (Behavior Change
// Protocol) as a system-prompt section. Active plans carry the follow-up directive; parked
// plans are listed quietly so the model can reactivate them if the user brings them up.
// Empty when the user has no plans.
func FormatBehaviorPlansContext(plans []models.BehaviorPlan, lastCheckIns map[string]models.BehaviorCheckIn) string {
	if len(plans) == 0 {
		return ""
	}

	var active, parked []models.BehaviorPlan
	for _, p := range plans {
		if p.Status == models.BehaviorPlanStatusActive {
			active = append(active, p)
		} else if p.Status == models.BehaviorPlanStatusParked {
			parked = append(parked, p)
		}
	}
	if len(active) == 0 && len(parked) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### 1.3 ACTIVE BEHAVIOR COMMITMENTS (CROSS-SESSION)\n")
	if len(active) > 0 {
		sb.WriteString("The user holds these behavior commitments. EARLY in the session — after your opening, before the main topic — warmly ask how each one has been going and record what they report with 'log_behavior_checkin'. A miss is information to adjust the plan, never a failure. Celebrate identity, not just execution:\n")
		for _, p := range active {
			sb.WriteString(fmt.Sprintf("- **%s**", p.Behavior))
			if p.Trigger != "" {
				sb.WriteString(fmt.Sprintf(" (trigger: %s)", p.Trigger))
			}
			if p.Identity != "" {
				sb.WriteString(fmt.Sprintf(" — identity: \"%s\"", p.Identity))
			}
			if p.StartDate != nil && *p.StartDate != "" {
				sb.WriteString(fmt.Sprintf("; since %s", *p.StartDate))
			}
			if p.WinsCount > 0 {
				sb.WriteString(fmt.Sprintf("; wins so far: %d", p.WinsCount))
			}
			if ci, ok := lastCheckIns[p.ID]; ok {
				sb.WriteString(fmt.Sprintf("; last check-in %s: %s", ci.CreatedAt.Format("2006-01-02"), ci.Status))
				if ci.Note != "" {
					sb.WriteString(fmt.Sprintf(" (\"%s\")", ci.Note))
				}
			}
			if p.PlanB != "" {
				sb.WriteString(fmt.Sprintf("; plan B: %s", p.PlanB))
			}
			sb.WriteString("\n")
		}
	}
	if len(parked) > 0 {
		sb.WriteString("Parked intentions (do NOT bring these up on your own — only if the user mentions the topic or the moment clearly invites it):\n")
		for _, p := range parked {
			sb.WriteString(fmt.Sprintf("- %s (parked %s)\n", p.Behavior, p.UpdatedAt.Format("2006-01-02")))
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

// FormatIdentityReflectionContext renders the user's latest Identity Reflection (the
// structured synthesis of the Identity session) as a system-prompt section, so later
// sessions can speak to who the user chose to keep becoming — Filipa's session designs
// lean on this continuity ("who are you practicing being?"). Empty when the user has
// not done the Identity session yet.
func FormatIdentityReflectionContext(userID string) string {
	var reflections []models.IdentityReflection
	if err := database.DB.Where("user_id = ?", userID).Order("created_at desc").Limit(1).
		Find(&reflections).Error; err != nil || len(reflections) == 0 {
		return ""
	}
	r := reflections[0]

	var sb strings.Builder
	sb.WriteString("### 1.4 IDENTITY REFLECTION (FROM THE IDENTITY SESSION)\n")
	sb.WriteString(fmt.Sprintf("The user's own words (recorded %s). Reference this naturally when it deepens the conversation — especially the identity they chose to practice — never as a recitation:\n", r.CreatedAt.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("- Who they learned to be: %s\n", r.LearnedIdentity))
	if r.WhatItGave != "" {
		sb.WriteString(fmt.Sprintf("- What that gave them: %s\n", r.WhatItGave))
	}
	if r.WhatItCosts != "" {
		sb.WriteString(fmt.Sprintf("- What it sometimes costs them: %s\n", r.WhatItCosts))
	}
	sb.WriteString(fmt.Sprintf("- Who they want to keep becoming: %s\n", r.WhoBecoming))
	if len(r.Qualities) > 0 {
		sb.WriteString(fmt.Sprintf("- Qualities they chose to strengthen: %s\n", strings.Join(r.Qualities, ", ")))
	}
	if r.Evidence != "" {
		sb.WriteString(fmt.Sprintf("- The evidence they chose: %s\n", r.Evidence))
	}
	sb.WriteString("\n")
	return sb.String()
}

// FormatLastMonthContext formats all user memories, commitments, wheel of life, and Eisenhower matrix assessments in the last month into a readable markdown string.
func FormatLastMonthContext(
	memories []models.UserMemory,
	commitments []models.Commitment,
	wheels []models.WheelOfLifeExercise,
	eisenhowers []models.EisenhowerMatrixExercise,
) string {
	var sb strings.Builder

	// 1. Memories
	if len(memories) > 0 {
		sb.WriteString("### 1.1 PAST INSIGHTS AND MEMORIES (TOTAL RECALL)\n")
		sb.WriteString("Here are key facts, emotional drivers, and insights gathered from previous sessions in the last month. Use these to personalize your coaching profoundly:\n")
		for _, m := range memories {
			sb.WriteString(fmt.Sprintf("- [%s] %s (Recorded: %s)\n", m.Category, m.Content, m.CreatedAt.Format("2006-01-02")))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("### 1.1 PAST INSIGHTS AND MEMORIES (TOTAL RECALL)\nNo significant memories or insights recorded in the last month.\n\n")
	}

	// 2. Exercises
	sb.WriteString("### 1.2 COMPLETED EXERCISES AND PLANS (LAST 30 DAYS)\n")
	sb.WriteString("Here are the active exercises, commitment plans, and matrix tasks completed in the last 30 days:\n\n")

	hasExercises := false

	// Commitments — the user's commitments across sessions: the commitment plan (origin=plan),
	// self-added items (manual), and habit projections (behavior). Without this block the
	// "small step this week" from the Movement session would be invisible to the next session.
	if len(commitments) > 0 {
		hasExercises = true
		sb.WriteString("#### A. ACTIONS & COMMITMENTS:\n")
		sb.WriteString("Commitments the user committed to in past sessions or added themselves, each with the date it was recorded. The ones with the LATEST 'Recorded' date are from the user's most recent session — when a task instruction says to reference \"the commitment they made last session\", it means those, and ONLY those. If you cannot tell which commitment a session produced, ask the user to remind you rather than guessing — attributing a commitment they never made breaks trust. Follow up naturally when relevant (celebrate what got done; treat what did not as information, never as failure):\n")
		for _, t := range commitments {
			statusStr := "Pending"
			if t.Done {
				statusStr = "Completed"
			}

			details := ""
			if t.Type == "one_time" && t.Date != nil && *t.Date != "" {
				details = fmt.Sprintf(" (Target Date: %s)", *t.Date)
			} else if t.Type == "recurring" && len(t.Days) > 0 {
				var dayNames []string
				weekdayMap := map[int]string{
					1: "Monday",
					2: "Tuesday",
					3: "Wednesday",
					4: "Thursday",
					5: "Friday",
					6: "Saturday",
					7: "Sunday",
				}
				for _, d := range t.Days {
					if name, ok := weekdayMap[d]; ok {
						dayNames = append(dayNames, name)
					}
				}
				if len(dayNames) > 0 {
					details = fmt.Sprintf(" (Days: %s)", strings.Join(dayNames, ", "))
				}
			}

			sb.WriteString(fmt.Sprintf("- [%s | %s] %s%s - Status: %s (Recorded: %s)\n", t.Origin, t.Type, t.Title, details, statusStr, t.CreatedAt.Format("2006-01-02")))
		}
		sb.WriteString("\n")
	}

	// Wheel of Life
	if len(wheels) > 0 {
		hasExercises = true
		sb.WriteString("#### B. WHEEL OF LIFE ASSESSMENTS:\n")
		for _, w := range wheels {
			sb.WriteString(fmt.Sprintf("- **Wheel Assessment** (Completed: %s):\n", w.UpdatedAt.Format("2006-01-02")))
			var items []WheelOfLifeItem
			if err := json.Unmarshal([]byte(w.Data), &items); err == nil {
				for _, item := range items {
					sb.WriteString(fmt.Sprintf("  * %s: %.0f/10 - %s\n", item.Name, item.CurrentScore, item.Reasoning))
				}
			}
		}
		sb.WriteString("\n")
	}

	// Eisenhower Matrix
	if len(eisenhowers) > 0 {
		hasExercises = true
		sb.WriteString("#### C. EISENHOWER MATRIX (CHAOS SORTER) TASKS:\n")
		for _, e := range eisenhowers {
			sb.WriteString(fmt.Sprintf("- **Eisenhower Matrix** (Created: %s):\n", e.CreatedAt.Format("2006-01-02")))
			var data EisenhowerMatrixData
			if err := json.Unmarshal([]byte(e.Data), &data); err == nil {
				if len(data.UrgentImportant) > 0 {
					sb.WriteString("  * *Urgent & Important (Do first):*\n")
					for _, item := range data.UrgentImportant {
						sb.WriteString(fmt.Sprintf("    - %s (%s)\n", item.Task, item.Reasoning))
					}
				}
				if len(data.NotUrgentImportant) > 0 {
					sb.WriteString("  * *Important, Not Urgent (Schedule):*\n")
					for _, item := range data.NotUrgentImportant {
						sb.WriteString(fmt.Sprintf("    - %s (%s)\n", item.Task, item.Reasoning))
					}
				}
				if len(data.UrgentNotImportant) > 0 {
					sb.WriteString("  * *Urgent, Not Important (Delegate):*\n")
					for _, item := range data.UrgentNotImportant {
						sb.WriteString(fmt.Sprintf("    - %s (%s)\n", item.Task, item.Reasoning))
					}
				}
				if len(data.NotUrgentNotImportant) > 0 {
					sb.WriteString("  * *Not Urgent & Not Important (Eliminate):*\n")
					for _, item := range data.NotUrgentNotImportant {
						sb.WriteString(fmt.Sprintf("    - %s (%s)\n", item.Task, item.Reasoning))
					}
				}
			}
		}
		sb.WriteString("\n")
	}

	if !hasExercises {
		sb.WriteString("No coaching exercises completed in the last 30 days.\n\n")
	}

	return sb.String()
}

// BuildSystemInstruction builds the full system prompt. The user-data-driven sections
// (profile, recent context, language) and the universal safety/protocol sections are
// shared across every session type; the Identity & Persona / Interaction Style block is
// selected per session type (sessionType).
func BuildSystemInstruction(
	sessionType api.SessionType,
	user *models.User,
	memories []models.UserMemory,
	commitments []models.Commitment,
	wheels []models.WheelOfLifeExercise,
	eisenhowers []models.EisenhowerMatrixExercise,
	loc *time.Location,
) string {
	name := "Unknown"
	country := "Unknown"
	gender := "Unknown"
	lang := "en-US"
	coachGender := "male"

	if user != nil {
		if user.Name != nil && *user.Name != "" {
			names := strings.Fields(*user.Name)
			if len(names) > 0 {
				name = names[0]
			}
		}
		if user.Country != nil && *user.Country != "" {
			country = *user.Country
		}
		if user.Gender != nil && *user.Gender != "" {
			gender = *user.Gender
		}
		if user.PreferredLanguage != nil && *user.PreferredLanguage != "" {
			lang = *user.PreferredLanguage
		}
		if user.CoachGender != nil && *user.CoachGender != "" {
			coachGender = *user.CoachGender
		}
		if user.CoachVoice != nil && *user.CoachVoice != "" {
			switch *user.CoachVoice {
			case "despina", "vindemiatrix", "gacrux":
				coachGender = "female"
			case "charon", "orus", "enceladus", "algieba":
				coachGender = "male"
			}
		}
	}

	idealLifeVision := "Not defined yet"
	if user != nil && user.IdealLifeVision != nil && *user.IdealLifeVision != "" {
		idealLifeVision = *user.IdealLifeVision
	}

	// The focus area drives the deep-session narrative ("the priority area you chose").
	// Without it in the profile the model guesses from wheel scores and gets it wrong
	// (QA: it greeted the user with "Money" when their chosen focus was Health).
	focusArea := "Not chosen yet"
	if user != nil && user.FocusArea != nil && *user.FocusArea != "" {
		focusArea = *user.FocusArea
	}

	// The chosen top values (Values session outcome) are the user's compass — every
	// later session's script leans on them by name.
	topValues := "Not chosen yet"
	if user != nil && len(user.TopValues) > 0 {
		topValues = strings.Join(user.TopValues, ", ")
	}

	age := "Unknown"
	if user != nil && user.DateOfBirth != nil {
		now := time.Now()
		dob := *user.DateOfBirth
		a := now.Year() - dob.Year()
		if now.Month() < dob.Month() || (now.Month() == dob.Month() && now.Day() < dob.Day()) {
			a--
		}
		age = fmt.Sprintf("%d", a)
	}

	langInstruction, ok := LanguageInstructions[lang]
	if !ok {
		langInstruction = "IMPORTANT: RESPOND IN THE CUSTOMER PREFERRED LANGUAGE. YOU MUST RESPOND UNMISTAKABLY IN THE CUSTOMER PREFERRED LANGUAGE."
	}

	toneDesc := "Male, deep, warm, calm."
	mentorDesc := "fatherly"
	if coachGender == "female" {
		toneDesc = "Female, warm, soothing, calm."
		mentorDesc = "motherly"
	}

	dynamicContextSection := FormatLastMonthContext(memories, commitments, wheels, eisenhowers)

	// Behavior commitments are a cross-session skill for every session EXCEPT the opening
	// arc: the onboarding intro and the Vision session must stay on their scripted path,
	// so neither sees plans nor declares the behavior tools.
	if sessionType != api.SessionTypeOnboarding && sessionType != api.SessionTypeSessionVision && user != nil {
		plans, lastCheckIns := LoadBehaviorPlansContext(user.ID, zap.L())
		dynamicContextSection += FormatBehaviorPlansContext(plans, lastCheckIns)
		dynamicContextSection += FormatIdentityReflectionContext(user.ID)
	}

	// The user's clock, not the server's. Anything the model schedules — "tomorrow
	// morning", "the day of the exam" — is reasoned from the two lines below, so a
	// server-time clock silently shifts every scheduled message by the timezone offset.
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)

	// Shared header: user profile, recent context, language. Identical for every session.
	header := fmt.Sprintf(`### 1. DYNAMIC CONTEXT & PROFILE
# User Profile:
- Name: %s
- Country: %s
- Age: %s
- Gender: %s
- Ideal Life Vision: %s
- Focus Area (the priority area the user chose to work on): %s
- Top Values (the values the user chose as their compass, in their words): %s

# System Context:
- Current Date: %s (the user's own local date)
- Current Time: %s (the user's own local time — count from THIS when scheduling anything for them)
%s
### 2. PRIME DIRECTIVE: LANGUAGE & LOCALIZATION
%s
CRITICAL: When translating or localizing any suggested scripts or instructions to the user's preferred language, you MUST preserve the silent screen marker (◆ followed by a screen glyph, e.g. ◆▣) in its exact glyph and corresponding position. Never translate, alter, spell out, or speak it.
* Scripts Are Templates, Never Verbatim English: Every scripted line, greeting, question, or example sentence inside your task instructions (Section 9) is written in English ONLY as a template. You MUST translate it into the user's language stated above BEFORE speaking it. Wording like "deliver this EXACT script" or "exactly as written" means reproduce its exact meaning, tone, and structure IN THE USER'S LANGUAGE — it does NOT mean speak the literal English. You are STRICTLY FORBIDDEN from speaking English (or any language other than the one in this directive) merely because a script happens to be authored in English. The ONLY elements you keep byte-for-byte unchanged are the silent screen markers (◆).
* Country Is NOT Language: The user's Country in the profile above does NOT determine the language. The language is ONLY the one stated in this directive. A user may live in one country yet use the app in a different language (e.g. living in Portugal while using US English). Always follow this directive, never the country.
* Address the USER With Their Gender (CRITICAL): The user's gender is stated in the profile above. In gendered languages, every grammatical form that refers to the USER must agree with THEIR gender — for a female user in Portuguese: "bem-vinda", "estás preparada?", "sentes-te realizada?", "tu própria"; for a male user: "bem-vindo", "preparado", "realizado", "tu próprio". This is separate from your own persona gender: forms referring to YOURSELF follow the Persona Gender Consistency rule; forms referring to the USER follow the user's profile gender. Getting this wrong ("bem-vindo" to a woman) is a jarring, personal mistake — check the profile, never guess from the name or the sound of their voice. If the gender is 'Unknown', use gender-neutral phrasing.
* Tool Argument Language Constraint: You are STRICTLY REQUIRED to write ALL user-facing text you pass to tools in the user's preferred language as stated above (e.g. US English for "en-US"). This applies to every content-bearing tool — including 'save_memory', 'save_focus', 'save_session_insight', 'save_ideal_life_vision', 'set_wheel_of_life_categories' (the area names), 'update_wheel_of_life' (the reasoning), and every commitment title you pass to 'add_commitment', 'update_commitment_plan', or 'save_commitments' (titles are shown verbatim on the user's board — a title in the wrong language sits on their home screen). Never switch to another language (like Spanish or Portuguese) for tool arguments because of the user's country or because an earlier note happened to be written in another language. If the user is speaking US English, every tool argument must be in US English.`,
		name,
		country,
		age,
		gender,
		idealLifeVision,
		focusArea,
		topValues,
		now.Format("2006-01-02"),
		now.Format("15:04"),
		dynamicContextSection,
		langInstruction,
	)

	// Identity & Persona (Section 3) + Interaction Style (Section 4) are selected per
	// session type. A registered session provides its own persona; others use the shared default.
	voice := session.Voice{CoachGender: coachGender, MentorDesc: mentorDesc, ToneDesc: toneDesc}
	persona := session.DefaultPersona(voice)
	if sess, ok := sessions.Get(sessionType); ok {
		persona = sess.SystemPersona(voice)
	}

	// Shared tail: universal safety, audio-handling, and function-calling protocol.
	return header + "\n\n" + persona + "\n\n" + commonSystemTail
}

// commonSystemTail holds the universal safety/protocol sections (5-8) that apply to
// every session type regardless of persona.
const commonSystemTail = `### 5. SAFETY PROTOCOL (CRITICAL)
If you detect suicidal ideation, self-harm, or imminent danger:
1. Shift Tone: Be extremely compassionate but firm.
2. Validate Pain: "I am so sorry you are going through such deep darkness..."
3. Redirect: DO NOT try to fix it clinically. Redirect to human support (SOS Voz Amiga or 112 or the local equivalent).

### 6. AUDIO & TRANSCRIPTION HANDLING (CRITICAL)
If the user's transcript consists of vague sounds (e.g., "hmmm", "ah"), physical sounds (e.g., coughs, sighs), or is completely garbled and unintelligible due to poor audio quality:
1. Acknowledge and Clarify: Politely state that you couldn't hear them clearly and ask them to repeat.
2. Wait for Response: Do NOT guess their intent. Do NOT call any tools. Do NOT assume a "yes" or "no". Wait for them to answer clearly.

### 7. PREVENTING REPETITION LOOPS
If you have just asked a question, or given instructions, and the user responds with an empty message, silence, or unintelligible noise, DO NOT re-generate your previous question over and over. DO NOT summarize again. Wait patiently for them to speak clearly.

### 8. FUNCTION CALLING & TOOL SYNTAX (ABSOLUTE RULE)

* FATAL ERROR PREVENTION - NO RAW TEXT TOOL CALLS: You are STRICTLY FORBIDDEN from outputting raw JSON strings, pseudo-code, or strings like "call:update_wheel_of_life{...}" or "call:save_memory{...}" in your text response. Do not invent your own syntax. You MUST NEVER leak tool logic into the spoken transcript. If you need to use a tool, you MUST use your built-in, native function calling capability. Your text response should contain ONLY the words that Rumi is speaking aloud to the user.`
