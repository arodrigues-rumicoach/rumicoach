package chat

import (
	"context"
	"fmt"
	"net/http"

	"github.com/rumi/rumi-be/config"
	"golang.org/x/oauth2/google"
)

// GeminiEndpoint resolves the generateContent REST URL and auth headers for a
// model under the configured provider: Vertex AI (ADC bearer token) or AI
// Studio (API key, falling back to ADC). Shared by the recommendation agent,
// session recap and review, and the companion channel.
//
// This reads GeminiRESTProvider, not GeminiProvider: the live voice socket is
// pinned to AI Studio by model availability (see the config field), so the REST
// traffic chooses its provider separately.
func GeminiEndpoint(ctx context.Context, model string) (string, http.Header, error) {
	headers := make(http.Header)

	if config.AppConfig.GeminiRESTProvider == "vertex" {
		region := config.AppConfig.GCPRegion
		project := config.AppConfig.GCPProjectID
		if region == "" || project == "" {
			return "", nil, fmt.Errorf("missing GCP_REGION or GCP_PROJECT_ID configuration")
		}
		token, err := adcToken(ctx, "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			return "", nil, err
		}
		url := fmt.Sprintf(
			"%s://%s/v1beta1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
			geminiScheme, fmt.Sprintf(vertexHostPattern, region), project, region, model,
		)
		headers.Set("Authorization", "Bearer "+token)
		return url, headers, nil
	}

	// AI Studio (Google AI)
	if apiKey := config.AppConfig.GoogleAPIKey; apiKey != "" {
		url := fmt.Sprintf(
			"%s://%s/v1beta/models/%s:generateContent?key=%s",
			geminiScheme, generativeLanguageHost, model, apiKey,
		)
		return url, headers, nil
	}
	token, err := adcToken(ctx, "https://www.googleapis.com/auth/generative-language", "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", nil, err
	}
	url := fmt.Sprintf(
		"%s://%s/v1beta/models/%s:generateContent",
		geminiScheme, generativeLanguageHost, model,
	)
	headers.Set("Authorization", "Bearer "+token)
	return url, headers, nil
}

func adcToken(ctx context.Context, scopes ...string) (string, error) {
	tokenSrc, err := google.DefaultTokenSource(ctx, scopes...)
	if err != nil {
		return "", fmt.Errorf("failed to load GCP default token source: %w", err)
	}
	token, err := tokenSrc.Token()
	if err != nil {
		return "", fmt.Errorf("failed to get GCP auth token: %w", err)
	}
	return token.AccessToken, nil
}
