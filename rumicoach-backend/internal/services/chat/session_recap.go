package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/aiusage"
)

// The session recap is the short, user-facing summary shown in the sessions list: a
// couple of sentences on what the session was about and what the user took from it.
// It is deliberately NOT the QA review (English, rubric-driven, admin-only) and NOT the
// synthesis screen (structured JSON built from stored fields). This one is prose, in the
// user's own language, and short enough to sit under a row in a list.

// recapMaxChars bounds what we store. The model is asked to stay well under this; the
// cap is the backstop that keeps a runaway generation from breaking the list layout.
const recapMaxChars = 400

// recapPromptTemplate receives the user's BCP-47 language tag and the transcript.
const recapPromptTemplate = `You are summarizing a coaching session between a user and Rumi, their AI life coach, for the user themselves to read later in a list of their past sessions.

Write the summary in the language identified by the BCP-47 tag "%s" — the user's own language. This is not negotiable: the user will read it, and a summary in the wrong language is useless to them. Note the regional variant (pt-PT is European Portuguese, not Brazilian; en-GB is British English).

Session transcript:
%s

Write a SHORT summary with two parts:
1. "title" — at most 6 words naming what this session was actually about, in the user's language. Concrete and specific to THIS conversation (e.g. "Setting boundaries at work"), never generic ("Coaching session", "Daily check-in") and never just the session type.
2. "recap" — 2 to 3 sentences, at most 300 characters. Say what was explored and what the user realised or decided. Lead with what matters to THEM.

Rules:
- Write about the user in the second person ("you explored...", "you realised..."), warm and plain, the way a thoughtful coach would jot a note for them.
- Include the user's own key insight if they named one, in their words where possible.
- Only state what actually happened in the transcript. Do NOT invent progress, feelings, or commitments that were not there.
- If the conversation was very short or the user barely engaged, say so plainly and briefly rather than inflating it.
- No markdown, no bullet points, no headings, no quotes around the text — just plain sentences.
- Never mention Rumi in the third person, the AI, the system, tools, screens, or anything technical.`

// GenerateSessionRecap produces the short user-facing summary for a session transcript,
// written in the user's preferred language (a BCP-47 tag such as "pt-PT"). Returns the
// title and the recap.
//
// userID and sessionID are carried only to attribute the token spend in the cost ledger.
func GenerateSessionRecap(ctx context.Context, transcript, language, userID, sessionID string) (string, string, error) {
	model := os.Getenv("GEMINI_REVIEW_MODEL")
	if model == "" {
		model = "gemini-3.7-flash"
	}

	url, reqHeaders, err := GeminiEndpoint(ctx, model)
	if err != nil {
		return "", "", err
	}

	responseSchema := map[string]interface{}{
		"type": "OBJECT",
		"properties": map[string]interface{}{
			"title": map[string]interface{}{
				"type":        "STRING",
				"description": "At most 6 words naming what this session was about, in the user's language.",
			},
			"recap": map[string]interface{}{
				"type":        "STRING",
				"description": "2-3 sentences, at most 300 characters, in the user's language, addressed to the user.",
			},
		},
		"required": []string{"title", "recap"},
	}

	if language == "" {
		language = "en-US"
	}
	payload := SessionReviewRequest{
		Contents: []Content{
			{Parts: []Part{{Text: fmt.Sprintf(recapPromptTemplate, language, transcript)}}},
		},
		GenerationConfig: GenerationConfig{
			ResponseMimeType: "application/json",
			ResponseSchema:   responseSchema,
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal recap payload: %w", err)
	}

	client := &http.Client{Timeout: 45 * time.Second}
	var resp *http.Response
	var reqErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
		if err != nil {
			return "", "", fmt.Errorf("failed to create recap request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		for k, vv := range reqHeaders {
			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}
		resp, reqErr = client.Do(req)
		if reqErr == nil && resp.StatusCode == http.StatusOK {
			break
		}
		if resp != nil {
			bodyBytes, _ := io.ReadAll(resp.Body)
			reqErr = fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyBytes))
			resp.Body.Close()
		}
		if attempt < 3 {
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}
	}
	if reqErr != nil {
		return "", "", fmt.Errorf("failed to call gemini for recap after retries: %w", reqErr)
	}
	defer resp.Body.Close()

	var geminiResp SessionReviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return "", "", fmt.Errorf("failed to decode recap response: %w", err)
	}

	// Before the content checks: the tokens were spent regardless of whether the
	// response is usable.
	aiusage.Write(ctx, geminiResp.usageRecord(models.AIUsageRecap, model, userID, sessionID))

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", "", fmt.Errorf("recap response contains no candidates or parts")
	}

	var result struct {
		Title string `json:"title"`
		Recap string `json:"recap"`
	}
	if err := json.Unmarshal([]byte(geminiResp.Candidates[0].Content.Parts[0].Text), &result); err != nil {
		return "", "", fmt.Errorf("failed to parse recap JSON: %w", err)
	}

	return strings.TrimSpace(result.Title), truncateRecap(result.Recap), nil
}

// truncateRecap trims a recap to recapMaxChars, cutting at the last sentence boundary or
// word break so the stored text never ends mid-word. Rune-safe: the recap is in the
// user's language, which is frequently not ASCII.
func truncateRecap(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= recapMaxChars {
		return s
	}
	cut := string(runes[:recapMaxChars])
	// Prefer ending on a completed sentence.
	if i := strings.LastIndexAny(cut, ".!?"); i > recapMaxChars/2 {
		return strings.TrimSpace(cut[:i+1])
	}
	if i := strings.LastIndex(cut, " "); i > 0 {
		return strings.TrimSpace(cut[:i]) + "…"
	}
	return strings.TrimSpace(cut) + "…"
}
