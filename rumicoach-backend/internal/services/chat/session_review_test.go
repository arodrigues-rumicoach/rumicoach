package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/rumi/rumi-be/config"
)

func TestGenerateSessionReview(t *testing.T) {
	// Initialize config
	config.AppConfig = &config.Config{
		GeminiRESTProvider: "google_ai",
		GoogleAPIKey:       "fake-api-key",
	}

	// Mock Gemini response data
	mockResponseText := `{
		"ai_notes": "### Developer Notes\nNo issues.",
		"ai_evaluation": 8.5
	}`

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Map to match SessionReviewResponse structure
		resp := SessionReviewResponse{}
		resp.Candidates = make([]struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		}, 1)

		resp.Candidates[0].Content.Parts = make([]struct {
			Text string `json:"text"`
		}, 1)
		resp.Candidates[0].Content.Parts[0].Text = mockResponseText

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Parse host from test server URL and override package host
	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Failed to parse test server URL: %v", err)
	}

	originalHost := generativeLanguageHost
	originalScheme := geminiScheme
	generativeLanguageHost = parsedURL.Host
	geminiScheme = "http"
	defer func() {
		generativeLanguageHost = originalHost
		geminiScheme = originalScheme
	}()

	// Execute session review generation
	ctx := context.Background()
	notes, score, err := GenerateSessionReview(ctx, "[2026-06-18 10:00:00] [USER] Hello\n[2026-06-18 10:00:05] [AI] Welcome", "", "u1", "s1")
	if err != nil {
		t.Fatalf("GenerateSessionReview failed: %v", err)
	}

	expectedNotes := "### Developer Notes\nNo issues."
	if notes != expectedNotes {
		t.Errorf("Expected notes to be %q, got %q", expectedNotes, notes)
	}
	if score != 8.5 {
		t.Errorf("Expected score to be 8.5, got %f", score)
	}
}
