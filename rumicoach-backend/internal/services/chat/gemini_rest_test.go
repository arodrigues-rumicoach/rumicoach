package chat

import (
	"context"
	"strings"
	"testing"

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/internal/services/chat/providers"
)

// The live socket and the generateContent calls choose their provider separately,
// and the reason is not a preference: gemini-3.1-flash-live-preview has no Vertex
// endpoint in any region. Moving the REST traffic to Vertex must therefore leave the
// voice engine on AI Studio — if the two switches ever collapse back into one, every
// coaching session dies at connect while the tests that cover recaps and companion
// replies stay green.
func TestRESTProviderDoesNotMoveTheLiveSocket(t *testing.T) {
	original := config.AppConfig
	t.Cleanup(func() { config.AppConfig = original })

	config.AppConfig = &config.Config{
		GeminiProvider:     "google_ai",
		GeminiRESTProvider: "vertex",
		GCPProjectID:       "test-project",
		GCPRegion:          "europe-west1",
		GoogleAPIKey:       "fake-api-key",
	}

	if p := providers.NewGeminiProvider(); p.Name() != "AI Studio" {
		t.Errorf("live provider = %q, want the AI Studio one: the REST switch must not "+
			"move the voice socket to a provider that does not serve the live model", p.Name())
	}
}

// And the REST calls follow their own switch rather than the live one.
func TestGeminiEndpointFollowsRESTProvider(t *testing.T) {
	original := config.AppConfig
	t.Cleanup(func() { config.AppConfig = original })

	// Vertex on the live switch, AI Studio on the REST one — the inverse of the
	// deployment above, so a call site reading the wrong field cannot pass both tests.
	config.AppConfig = &config.Config{
		GeminiProvider:     "vertex",
		GeminiRESTProvider: "google_ai",
		GCPProjectID:       "test-project",
		GCPRegion:          "europe-west1",
		GoogleAPIKey:       "fake-api-key",
	}

	url, _, err := GeminiEndpoint(context.Background(), "gemini-3.7-flash")
	if err != nil {
		t.Fatalf("GeminiEndpoint: %v", err)
	}
	if !strings.Contains(url, generativeLanguageHost) {
		t.Errorf("url = %q, want the AI Studio host %q", url, generativeLanguageHost)
	}
	if strings.Contains(url, "aiplatform") {
		t.Errorf("url = %q went to Vertex; GeminiEndpoint read GEMINI_PROVIDER, not GEMINI_REST_PROVIDER", url)
	}
}
