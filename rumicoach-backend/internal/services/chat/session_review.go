package chat

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
	"github.com/rumi/rumi-be/internal/services/aiusage"
	"github.com/rumi/rumi-be/internal/services/chat/session"
)

type SessionReviewRequest struct {
	Contents         []Content        `json:"contents"`
	GenerationConfig GenerationConfig `json:"generationConfig,omitempty"`
}

type SessionReviewResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	// Token spend for the cost ledger. Shared with the session recap, which decodes
	// into this same struct — both calls used to discard it, so every session quietly
	// spent on two generations that appeared in no accounting anywhere.
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		// TotalTokenCount also covers thinking tokens, which candidatesTokenCount
		// excludes on Gemini 3.
		TotalTokenCount int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

// usageRecord turns a decoded response into a cost-ledger record for one of the
// background generations, so the three call sites cannot drift on how they attribute.
func (r *SessionReviewResponse) usageRecord(kind, model, userID, sessionID string) aiusage.Record {
	return aiusage.Record{
		UserID:       userID,
		Kind:         kind,
		Model:        model,
		RefType:      models.AIUsageRefSession,
		RefID:        sessionID,
		InputTokens:  r.UsageMetadata.PromptTokenCount,
		OutputTokens: r.UsageMetadata.CandidatesTokenCount,
		TotalTokens:  r.UsageMetadata.TotalTokenCount,
	}
}

type SessionReviewJSON struct {
	AINotes      string  `json:"ai_notes"`
	AIEvaluation float64 `json:"ai_evaluation"`
}

// GenerateSessionReview calls the configured Gemini provider to review the session transcript, extract notes, and rate it.
//
// userID and sessionID are carried only to attribute the token spend in the cost
// ledger — the review itself is about the coach, not the user.
func GenerateSessionReview(ctx context.Context, transcript, reviewPromptTemplate, userID, sessionID string) (string, float64, error) {
	// Determine the model
	model := os.Getenv("GEMINI_REVIEW_MODEL")
	if model == "" {
		model = "gemini-3.7-flash"
	}

	_ = os.WriteFile("scratch/last_transcript_sent_to_llm.txt", []byte(transcript), 0644)

	url, reqHeaders, err := GeminiEndpoint(ctx, model)
	if err != nil {
		return "", 0.0, err
	}

	// Structured JSON schema definition
	responseSchema := map[string]interface{}{
		"type": "OBJECT",
		"properties": map[string]interface{}{
			"ai_notes": map[string]interface{}{
				"type":        "STRING",
				"description": "Comprehensive developer notes/feedback on prompt efficacy, AI repetitions, UX flow anomalies, potential bugs, or user frustration.",
			},
			"ai_evaluation": map[string]interface{}{
				"type":        "NUMBER",
				"description": "A numeric rating from 1 to 10 evaluating the overall quality/success of the coaching session (e.g., user engagement and goal progression).",
			},
		},
		"required": []string{"ai_notes", "ai_evaluation"},
	}

	if reviewPromptTemplate == "" {
		reviewPromptTemplate = session.DefaultReviewPrompt
	}
	promptText := fmt.Sprintf(reviewPromptTemplate, transcript)

	// Build the request payload
	payload := SessionReviewRequest{
		Contents: []Content{
			{
				Parts: []Part{
					{Text: promptText},
				},
			},
		},
		GenerationConfig: GenerationConfig{
			ResponseMimeType: "application/json",
			ResponseSchema:   responseSchema,
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", 0.0, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	client := &http.Client{Timeout: 45 * time.Second}
	var resp *http.Response
	var reqErr error

	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
		if err != nil {
			return "", 0.0, fmt.Errorf("failed to create http request: %w", err)
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
		return "", 0.0, fmt.Errorf("failed to call gemini api after retries: %w", reqErr)
	}
	defer resp.Body.Close()

	var geminiResp SessionReviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return "", 0.0, fmt.Errorf("failed to decode gemini response: %w", err)
	}

	// Recorded before the content checks below: the tokens were spent whether or not
	// the response turns out to be usable, and a generation that came back malformed
	// is exactly the kind of cost worth being able to see.
	aiusage.Write(ctx, geminiResp.usageRecord(models.AIUsageReview, model, userID, sessionID))

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", 0.0, fmt.Errorf("gemini response contains no candidates or parts")
	}

	rawJSON := geminiResp.Candidates[0].Content.Parts[0].Text

	var result SessionReviewJSON
	if err := json.Unmarshal([]byte(rawJSON), &result); err != nil {
		return "", 0.0, fmt.Errorf("failed to parse structured session review JSON: %w", err)
	}

	return result.AINotes, result.AIEvaluation, nil
}
