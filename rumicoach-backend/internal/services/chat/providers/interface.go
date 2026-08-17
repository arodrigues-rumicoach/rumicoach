package providers

import (
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// TurnDetection tunes how patiently the Live API waits before deciding the user has
// finished speaking. The provider default is tuned for quick back-and-forth, which talks
// over anyone pausing mid-thought — fatal in a coaching session built on reflection (QA).
type TurnDetection struct {
	// SilenceDurationMs is the non-speech duration required before end-of-speech is
	// committed. Zero leaves the provider default untouched, so an unsupported field can
	// never break a session — the setup simply goes out as it did before.
	SilenceDurationMs int
}

// GeminiProvider abstracts the differences between Vertex AI and AI Studio
// for the Gemini Multimodal Live API WebSocket connection.
type GeminiProvider interface {
	// Connect establishes a WebSocket connection to the Gemini Live API.
	Connect(logger *zap.Logger) (*websocket.Conn, error)

	// BuildSetupMessage constructs the initial configuration message
	// sent immediately after the WebSocket connection is established.
	// languageCode, when non-empty, is applied as a speech/transcription language hint
	// (experimental; empty = provider default of automatic detection).
	BuildSetupMessage(voiceName string, systemInstruction string, tools []map[string]interface{}, sessionHandle string, languageCode string, turn TurnDetection) map[string]interface{}

	// BuildAudioInputMessage constructs the message used to send audio
	// data from the client to Gemini. The format differs between providers.
	BuildAudioInputMessage(audioData []byte) map[string]interface{}

	// BuildPromptMessage constructs the message used to inject a text prompt
	// into the session using clientContent with turnComplete: true.
	BuildPromptMessage(text string) map[string]interface{}

	// BuildPromptMessageNoTrigger constructs the message used to inject a text prompt
	// into the session using clientContent with turnComplete: false.
	BuildPromptMessageNoTrigger(text string) map[string]interface{}

	// Name returns a human-readable provider name for logging.
	Name() string
}
