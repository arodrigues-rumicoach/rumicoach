// Package companion implements the async messaging conversation with Rumi
// (WhatsApp today): a turn-based STT + LLM (+ TTS) pipeline over the Gemini
// generateContent REST API. It is deliberately separate from the live-audio
// engine in internal/services/chat, reusing only its transport-agnostic data
// helpers (context loading/formatting, language table, Gemini endpoint auth).
package companion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/balance"
	"github.com/rumi/rumi-be/internal/services/chat"
)

// Default models, overridable via env (same pattern as GEMINI_RECOMMENDATION_MODEL).
const (
	defaultChatModel       = "gemini-3.7-flash"
	defaultTranscribeModel = "gemini-3.7-flash"
	defaultTTSModel        = "gemini-3.1-flash-tts-preview"
)

func chatModel() string       { return envOr("GEMINI_COMPANION_MODEL", defaultChatModel) }
func transcribeModel() string { return envOr("GEMINI_TRANSCRIBE_MODEL", defaultTranscribeModel) }
func ttsModel() string        { return envOr("GEMINI_TTS_MODEL", defaultTTSModel) }

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// --- generateContent request/response types. Richer than the chat package's
// (roles, inline media, function calling, speech config), so declared here. ---

type Blob struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64
}

type FunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type FunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type Part struct {
	Text             string            `json:"text,omitempty"`
	InlineData       *Blob             `json:"inlineData,omitempty"`
	FunctionCall     *FunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *FunctionResponse `json:"functionResponse,omitempty"`
	// Thought/ThoughtSignature round-trip Gemini 3 reasoning metadata: the API
	// rejects a replayed functionCall part whose thoughtSignature was dropped.
	Thought          bool   `json:"thought,omitempty"`
	ThoughtSignature string `json:"thoughtSignature,omitempty"`
}

type Content struct {
	Role  string `json:"role,omitempty"` // "user" | "model"
	Parts []Part `json:"parts"`
}

type Tool struct {
	FunctionDeclarations []map[string]any `json:"functionDeclarations,omitempty"`
}

type PrebuiltVoiceConfig struct {
	VoiceName string `json:"voiceName"`
}

type VoiceConfig struct {
	PrebuiltVoiceConfig *PrebuiltVoiceConfig `json:"prebuiltVoiceConfig,omitempty"`
}

type SpeechConfig struct {
	VoiceConfig *VoiceConfig `json:"voiceConfig,omitempty"`
}

type GenerationConfig struct {
	ResponseModalities []string      `json:"responseModalities,omitempty"`
	SpeechConfig       *SpeechConfig `json:"speechConfig,omitempty"`
}

type Request struct {
	SystemInstruction *Content          `json:"systemInstruction,omitempty"`
	Contents          []Content         `json:"contents"`
	Tools             []Tool            `json:"tools,omitempty"`
	GenerationConfig  *GenerationConfig `json:"generationConfig,omitempty"`
}

type Response struct {
	Candidates []struct {
		Content Content `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		// TotalTokenCount also covers thinking tokens, which
		// candidatesTokenCount excludes on Gemini 3 — bill from this.
		TotalTokenCount int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

// Usage accumulates token spend across every Gemini call that serves one
// message turn — the chat tool loop makes up to maxToolIterations+1 calls, and
// audio turns add transcription and TTS calls on top. Summing at the source is
// what makes the per-message usage record complete; recording only the final
// response undercounted every turn that used a tool.
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// AddResponse folds one response's usageMetadata into the running total. Models
// that omit totalTokenCount fall back to input+output so TotalTokens is never
// the field that quietly reads zero while the others are populated.
func (u *Usage) AddResponse(resp *Response) {
	if resp == nil {
		return
	}
	m := resp.UsageMetadata
	u.InputTokens += m.PromptTokenCount
	u.OutputTokens += m.CandidatesTokenCount
	if m.TotalTokenCount > 0 {
		u.TotalTokens += m.TotalTokenCount
	} else {
		u.TotalTokens += m.PromptTokenCount + m.CandidatesTokenCount
	}
}

// Add folds another accumulated total (e.g. the transcription step's) into this one.
func (u *Usage) Add(other Usage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.TotalTokens += other.TotalTokens
}

// Spend is one billable event's token cost, kept apart by which model earned it.
//
// Answering a voice note runs three models at three prices — transcription, the chat
// turn, then speech synthesis — and they differ by more than a rounding error: TTS
// output is $20.00 per million tokens against $9.00 for chat. Summing them into a
// single Usage, which is what this replaced, left the cost ledger with one number and
// no way to know which rate applied to any of it, so the whole turn was priced as
// chat. Keeping them apart costs one struct and makes the ledger correct.
//
// Chat is the primary model; STT and TTS are empty on the many events that use
// neither, which is most of them.
type Spend struct {
	Chat Usage
	STT  Usage
	TTS  Usage

	// The billable length of each side of the exchange, when it was spoken rather
	// than typed. Distinct in nature from the token fields above — those are what the
	// exchange cost US, these are what it costs the USER — but they travel the same
	// path through deliverReply, and splitting them into a second struct would mean
	// threading two things through every call site to no benefit.
	//
	// Zero for a typed message, and zero when the length could not be measured.
	InboundAudioSeconds  int64
	OutboundAudioSeconds int64
}

// ChargeSeconds is what the user pays for this exchange, given how the reply went out.
//
// Each side is priced for what it actually was. Audio is charged by its length: a
// two-minute voice note is two minutes of the coach's attention however few tokens it
// took to transcribe. Text is charged the flat reply fee, once, on the outbound side —
// a typed question costs the user nothing to ask, only to answer.
//
// The four combinations all fall out of that, and the modes are the integration's
// ReplyMode: "auto" mirrors whatever the user sent, "text" and "audio" pin the reply
// regardless. So a voice note answered by voice pays for both recordings; a voice note
// answered in text pays what the user spoke plus the flat fee — which is the ordinary
// case for someone who set ReplyMode to text, not only a TTS failure falling back; a
// typed message answered by voice pays for the recording alone; and a typed exchange
// pays the flat fee.
func (s Spend) ChargeSeconds(replyType string) int64 {
	total := s.InboundAudioSeconds
	if replyType == models.ChannelMessageTypeAudio {
		return total + s.OutboundAudioSeconds
	}
	return total + balance.MessageCostSeconds
}

var geminiHTTPClient = &http.Client{Timeout: 60 * time.Second}

// callGemini POSTs a generateContent request to the given model using the
// provider/auth configuration shared with the rest of the backend.
func callGemini(ctx context.Context, model string, req Request) (*Response, error) {
	url, headers, err := chat.GeminiEndpoint(ctx, model)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, vv := range headers {
		for _, v := range vv {
			httpReq.Header.Add(k, v)
		}
	}
	resp, err := geminiHTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call gemini api: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini api returned status %d: %s", resp.StatusCode, truncate(string(body), 2048))
	}
	var parsed Response
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode gemini response: %w", err)
	}
	return &parsed, nil
}

// firstCandidate returns the first candidate's content, or an error when the
// response is empty (e.g. fully blocked).
func firstCandidate(resp *Response) (Content, error) {
	if len(resp.Candidates) == 0 {
		return Content{}, fmt.Errorf("gemini response contains no candidates")
	}
	return resp.Candidates[0].Content, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
