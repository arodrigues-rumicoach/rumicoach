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

func TestGenerateRecommendations(t *testing.T) {
	// Initialize config
	config.AppConfig = &config.Config{
		GeminiRESTProvider: "google_ai",
		GoogleAPIKey:       "fake-api-key",
	}

	// Mock Gemini response data
	mockResponseText := `{
		"recommendations": [
			{
				"title": "Atomic Habits",
				"type": "book",
				"author": "James Clear",
				"url": "https://jamesclear.com/atomic-habits",
				"description": "Excellent book on habit formation."
			}
		]
	}`

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Map to match GeminiResponse structure
		resp := GeminiResponse{}
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

	// Execute recommendations generation
	ctx := context.Background()
	recs, err := GenerateRecommendations(ctx, "habits", "books on habits", "user wants to build clean habits", "user-id", "session-id")
	if err != nil {
		t.Fatalf("GenerateRecommendations failed: %v", err)
	}

	// Assert recommendations contents
	if len(recs) != 1 {
		t.Fatalf("Expected 1 recommendation, got %d", len(recs))
	}

	r := recs[0]
	if r.UserID != "user-id" {
		t.Errorf("Expected UserID to be 'user-id', got '%s'", r.UserID)
	}
	if r.SessionID != "session-id" {
		t.Errorf("Expected SessionID to be 'session-id', got '%s'", r.SessionID)
	}
	if r.Title != "Atomic Habits" {
		t.Errorf("Expected Title to be 'Atomic Habits', got '%s'", r.Title)
	}
	if r.Type != "book" {
		t.Errorf("Expected Type to be 'book', got '%s'", r.Type)
	}
	if r.Author == nil || *r.Author != "James Clear" {
		t.Errorf("Expected Author to be 'James Clear', got '%v'", r.Author)
	}
	if r.URL == nil || *r.URL != "https://jamesclear.com/atomic-habits" {
		t.Errorf("Expected URL to be 'https://jamesclear.com/atomic-habits', got '%v'", r.URL)
	}
	if r.Description != "Excellent book on habit formation." {
		t.Errorf("Expected Description to be 'Excellent book on habit formation.', got '%s'", r.Description)
	}
}
