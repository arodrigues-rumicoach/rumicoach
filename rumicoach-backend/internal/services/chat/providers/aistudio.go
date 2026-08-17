package providers

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/rumi/rumi-be/config"
	"go.uber.org/zap"
	"golang.org/x/oauth2/google"
)

// AIStudioProvider implements GeminiProvider for Google AI Studio (Generative Language API).
type AIStudioProvider struct{}

func (p *AIStudioProvider) Name() string {
	return "AI Studio"
}

func (p *AIStudioProvider) Connect(logger *zap.Logger) (*websocket.Conn, error) {
	modelConfig := config.AppConfig.GeminiLiveModel
	apiKey := config.AppConfig.GoogleAPIKey

	if modelConfig == "" {
		return nil, fmt.Errorf("GEMINI_LIVE_MODEL is not set")
	}

	logger.Info("Connecting to Gemini via AI Studio", zap.String("model", modelConfig))

	var geminiUrl string
	headers := http.Header{}

	if apiKey != "" {
		// AI Studio uses v1beta endpoint with API key as query parameter
		geminiUrl = fmt.Sprintf(
			"wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent?key=%s",
			apiKey,
		)
	} else {
		// Use Application Default Credentials (ADC)
		ctx := context.Background()
		tokenSrc, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/generative-language", "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			return nil, fmt.Errorf("failed to get default token source: %w", err)
		}
		token, err := tokenSrc.Token()
		if err != nil {
			return nil, fmt.Errorf("failed to get token from default token source: %w", err)
		}
		geminiUrl = "wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent"
		headers.Set("Authorization", "Bearer "+token.AccessToken)
	}

	dialer := websocket.DefaultDialer
	conn, resp, err := dialer.Dial(geminiUrl, headers)
	if err != nil {
		if resp != nil && resp.Body != nil {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("websocket error: %v, body: %s", err, string(bodyBytes))
		}
		return nil, err
	}

	return conn, nil
}

func (p *AIStudioProvider) BuildSetupMessage(voiceName string, systemInstruction string, tools []map[string]interface{}, sessionHandle string, languageCode string, turn TurnDetection) map[string]interface{} {
	modelPath := "models/" + config.AppConfig.GeminiLiveModel

	// AI Studio raw protocol also uses "setup" as the top-level key.
	// The "config" naming is often used in high-level SDKs but not in the raw WebSocket JSON.
	setupData := map[string]interface{}{
		"model": modelPath,
		"generationConfig": map[string]interface{}{
			"responseModalities": []string{"AUDIO"},
			"speechConfig": map[string]interface{}{
				"voiceConfig": map[string]interface{}{
					"prebuiltVoiceConfig": map[string]interface{}{
						"voiceName": voiceName,
					},
				},
			},
		},
		"systemInstruction": map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": systemInstruction},
			},
		},
		"tools": tools,
		// Enable transcriptions
		"inputAudioTranscription":  map[string]interface{}{},
		"outputAudioTranscription": map[string]interface{}{},
		// Context window compression — allows sessions to run beyond the token limit.
		"contextWindowCompression": map[string]interface{}{
			"triggerTokens": config.AppConfig.GeminiContextWindowTriggerTokens,
			"slidingWindow": map[string]interface{}{
				"targetTokens": config.AppConfig.GeminiContextWindowTargetTokens,
			},
		},
	}

	if languageCode != "" {
		gc := setupData["generationConfig"].(map[string]interface{})
		sc := gc["speechConfig"].(map[string]interface{})
		sc["languageCode"] = languageCode
	}

	// Turn detection: how long the user must fall silent before Gemini decides they have
	// finished. Left untouched when zero, so an unsupported field can never break a session.
	if turn.SilenceDurationMs > 0 {
		setupData["realtimeInputConfig"] = map[string]interface{}{
			"automaticActivityDetection": map[string]interface{}{
				"endOfSpeechSensitivity": "END_SENSITIVITY_LOW",
				"silenceDurationMs":      turn.SilenceDurationMs,
			},
		}
	}

	if sessionHandle != "" {
		setupData["sessionResumption"] = map[string]interface{}{
			"handle": sessionHandle,
		}
	}

	return map[string]interface{}{"setup": setupData}
}

func (p *AIStudioProvider) BuildPromptMessage(text string) map[string]interface{} {
	// Use clientContent with turnComplete to trigger immediate response generation
	return map[string]interface{}{
		"clientContent": map[string]interface{}{
			"turns": []map[string]interface{}{
				{
					"role": "user",
					"parts": []map[string]interface{}{
						{"text": text},
					},
				},
			},
			"turnComplete": true,
		},
	}
}

func (p *AIStudioProvider) BuildPromptMessageNoTrigger(text string) map[string]interface{} {
	// Use clientContent with turnComplete: false to update prompt without triggering response
	return map[string]interface{}{
		"clientContent": map[string]interface{}{
			"turns": []map[string]interface{}{
				{
					"role": "user",
					"parts": []map[string]interface{}{
						{"text": text},
					},
				},
			},
			"turnComplete": false,
		},
	}
}

func (p *AIStudioProvider) BuildAudioInputMessage(audioData []byte) map[string]interface{} {
	// AI Studio uses base64-encoded audio data in JSON via realtimeInput.audio
	encoded := base64.StdEncoding.EncodeToString(audioData)
	return map[string]interface{}{
		"realtimeInput": map[string]interface{}{
			"audio": map[string]interface{}{
				"data":     encoded,
				"mimeType": "audio/pcm;rate=16000",
			},
		},
	}
}
