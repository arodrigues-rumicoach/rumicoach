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

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/aiusage"
)

// Gemini API structures
type Content struct {
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}

type GenerationConfig struct {
	ResponseMimeType string      `json:"responseMimeType,omitempty"`
	ResponseSchema   interface{} `json:"responseSchema,omitempty"`
}

type Tool struct {
	GoogleSearch          map[string]interface{} `json:"googleSearch,omitempty"`
	GoogleSearchRetrieval map[string]interface{} `json:"googleSearchRetrieval,omitempty"`
}

type GeminiRequest struct {
	Contents         []Content        `json:"contents"`
	GenerationConfig GenerationConfig `json:"generationConfig,omitempty"`
	Tools            []Tool           `json:"tools,omitempty"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	// Token spend for the cost ledger. This call is web-search-grounded and by far
	// the most expensive of the background generations, and it used to be recorded
	// nowhere at all.
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

// Background recommendation agent output structures
type RecommendationJSONList struct {
	Recommendations []struct {
		Title       string  `json:"title"`
		Type        string  `json:"type"` // book, article, video, podcast, other
		Author      *string `json:"author,omitempty"`
		URL         *string `json:"url,omitempty"`
		Description string  `json:"description"`
	} `json:"recommendations"`
}

var (
	geminiScheme           = "https"
	generativeLanguageHost = "generativelanguage.googleapis.com"
	vertexHostPattern      = "%s-aiplatform.googleapis.com"
)

// GenerateRecommendations calls the configured Gemini provider to search the web for high-quality resources
func GenerateRecommendations(ctx context.Context, topic, searchQuery, contextStr, userID, sessionID string) ([]models.Recommendation, error) {
	// The REST provider, not the live one — this is a generateContent call, and it
	// has to agree with the endpoint GeminiEndpoint below picks, or the Search
	// Grounding tool is shaped for the provider we are not talking to.
	isVertex := config.AppConfig.GeminiRESTProvider == "vertex"

	// Determine the model
	model := os.Getenv("GEMINI_RECOMMENDATION_MODEL")
	if model == "" {
		model = "gemini-3.1-pro-preview"
	}

	url, reqHeaders, err := GeminiEndpoint(ctx, model)
	if err != nil {
		return nil, err
	}

	// Structured JSON schema definition
	responseSchema := map[string]interface{}{
		"type": "OBJECT",
		"properties": map[string]interface{}{
			"recommendations": map[string]interface{}{
				"type": "ARRAY",
				"items": map[string]interface{}{
					"type": "OBJECT",
					"properties": map[string]interface{}{
						"title": map[string]interface{}{"type": "STRING"},
						"type": map[string]interface{}{
							"type": "STRING",
							"enum": []string{"book", "article", "video", "podcast", "other"},
						},
						"author":      map[string]interface{}{"type": "STRING"},
						"url":         map[string]interface{}{"type": "STRING"},
						"description": map[string]interface{}{"type": "STRING"},
					},
					"required": []string{"title", "type", "description"},
				},
			},
		},
		"required": []string{"recommendations"},
	}

	// Tools configuration (Search Grounding differs between AI Studio and Vertex)
	var tools []Tool
	if isVertex {
		tools = []Tool{
			{GoogleSearchRetrieval: map[string]interface{}{}},
		}
	} else {
		tools = []Tool{
			{GoogleSearch: map[string]interface{}{}},
		}
	}

	// Build the detailed prompt
	promptText := fmt.Sprintf(
		"Perform a grounded search to find 3 to 5 high-quality resources (books, articles, videos, podcasts, or tools) for: \n"+
			"Topic: %s\n"+
			"Search Query: %s\n"+
			"User Context & Preferences: %s\n\n"+
			"Instructions:\n"+
			"1. Find and include valid, working URLs where possible.\n"+
			"2. Write a personalized description for each item explaining why you are recommending it specifically to the user based on their context.\n"+
			"3. Ensure the output strictly adheres to the requested JSON schema.",
		topic, searchQuery, contextStr,
	)

	// Build the request payload
	payload := GeminiRequest{
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
		Tools: tools,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, vv := range reqHeaders {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call gemini api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini api returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var geminiResp GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, fmt.Errorf("failed to decode gemini response: %w", err)
	}

	// Before the content checks: the tokens were spent regardless of whether the
	// response is usable.
	aiusage.Write(ctx, aiusage.Record{
		UserID:       userID,
		Kind:         models.AIUsageRecommendation,
		Model:        model,
		RefType:      models.AIUsageRefSession,
		RefID:        sessionID,
		InputTokens:  geminiResp.UsageMetadata.PromptTokenCount,
		OutputTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
		TotalTokens:  geminiResp.UsageMetadata.TotalTokenCount,
	})

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini response contains no candidates or parts")
	}

	rawJSON := geminiResp.Candidates[0].Content.Parts[0].Text

	var list RecommendationJSONList
	if err := json.Unmarshal([]byte(rawJSON), &list); err != nil {
		return nil, fmt.Errorf("failed to parse structured recommendations JSON: %w", err)
	}

	// Map to GORM model slices
	recommendations := make([]models.Recommendation, len(list.Recommendations))
	for i, item := range list.Recommendations {
		recommendations[i] = models.Recommendation{
			UserID:      userID,
			SessionID:   sessionID,
			Title:       item.Title,
			Type:        item.Type,
			Author:      item.Author,
			URL:         item.URL,
			Description: item.Description,
		}
	}

	return recommendations, nil
}
