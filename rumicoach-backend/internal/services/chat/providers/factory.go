package providers

import (
	"github.com/rumi/rumi-be/config"
)

// NewGeminiProvider creates the appropriate provider based on config.
func NewGeminiProvider() GeminiProvider {
	if config.AppConfig.GeminiProvider == "google_ai" {
		return &AIStudioProvider{}
	}
	return &VertexProvider{}
}
