package providers

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/rumi/rumi-be/config"
	"go.uber.org/zap"
	"golang.org/x/oauth2/google"
)

// VertexProvider implements GeminiProvider for Google Cloud Vertex AI.
type VertexProvider struct{}

func (p *VertexProvider) Name() string {
	return "Vertex AI"
}

func (p *VertexProvider) Connect(logger *zap.Logger) (*websocket.Conn, error) {
	region := config.AppConfig.GCPRegion
	projectID := config.AppConfig.GCPProjectID
	modelConfig := config.AppConfig.GeminiLiveModel

	if region == "" {
		return nil, fmt.Errorf("GCP_REGION is not set")
	}
	if projectID == "" {
		return nil, fmt.Errorf("GCP_PROJECT_ID is not set")
	}
	if modelConfig == "" {
		return nil, fmt.Errorf("GEMINI_LIVE_MODEL is not set")
	}

	logger.Info("Connecting to Gemini via Vertex AI",
		zap.String("region", region),
		zap.String("project", projectID),
		zap.String("model", modelConfig),
	)

	// Get OAuth2 access token via Application Default Credentials
	ctx := context.Background()
	tokenSrc, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("gcp token source error: %w", err)
	}
	token, err := tokenSrc.Token()
	if err != nil {
		return nil, fmt.Errorf("gcp token fetch error: %w", err)
	}

	// Vertex AI WebSocket Endpoint for Live API
	geminiUrl := fmt.Sprintf(
		"wss://%s-aiplatform.googleapis.com/ws/google.cloud.aiplatform.v1beta1.LlmBidiService/BidiGenerateContent",
		region,
	)

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token.AccessToken)

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

func (p *VertexProvider) BuildSetupMessage(voiceName string, systemInstruction string, tools []map[string]interface{}, sessionHandle string, languageCode string, turn TurnDetection) map[string]interface{} {
	modelPath := fmt.Sprintf(
		"projects/%s/locations/%s/publishers/google/models/%s",
		config.AppConfig.GCPProjectID,
		config.AppConfig.GCPRegion,
		config.AppConfig.GeminiLiveModel,
	)

	// Vertex AI uses "setup" as the top-level key
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

func (p *VertexProvider) BuildPromptMessage(text string) map[string]interface{} {
	// Vertex AI uses clientContent with turns
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

func (p *VertexProvider) BuildPromptMessageNoTrigger(text string) map[string]interface{} {
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

func (p *VertexProvider) BuildAudioInputMessage(audioData []byte) map[string]interface{} {
	// Use realtimeInput.audio (mediaChunks is deprecated)
	return map[string]interface{}{
		"realtimeInput": map[string]interface{}{
			"audio": map[string]interface{}{
				"data":     audioData,
				"mimeType": "audio/pcm;rate=16000",
			},
		},
	}
}
