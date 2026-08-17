package telegram

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rumi/rumi-be/internal/services/messaging"
	"go.uber.org/zap"
)

// QA: every voice reply came back as text because Telegram answered "there is no voice
// in the request". The API method and the multipart field name are different things —
// sendVoice reads the file from a field called "voice", not one called "sendVoice".
func TestSendAudioUsesTheFieldNameTelegramExpects(t *testing.T) {
	cases := []struct {
		name        string
		asVoiceNote bool
		wantPath    string
		wantField   string
	}{
		{"voice note", true, "/sendVoice", "voice"},
		{"plain audio", false, "/sendAudio", "audio"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var gotPath, gotField, gotChatID string
			var gotBytes int

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path

				_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if err != nil {
					t.Errorf("request is not multipart: %v", err)
					return
				}
				mr := multipart.NewReader(r.Body, params["boundary"])
				for {
					part, err := mr.NextPart()
					if err != nil {
						break
					}
					switch part.FormName() {
					case "chat_id":
						b, _ := io.ReadAll(part)
						gotChatID = string(b)
					default:
						if part.FileName() != "" {
							gotField = part.FormName()
							b, _ := io.ReadAll(part)
							gotBytes = len(b)
						}
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))
			}))
			defer srv.Close()

			client := &Client{httpClient: srv.Client(), baseURL: srv.URL, token: "t", logger: zap.NewNop()}
			audio := []byte("fake-ogg-opus-bytes")

			if _, err := client.SendAudio(context.Background(),
				messaging.Address{ExternalID: "5991058878"}, audio, "audio/ogg", c.asVoiceNote); err != nil {
				t.Fatalf("SendAudio: %v", err)
			}

			if gotPath != c.wantPath {
				t.Errorf("path = %s, want %s", gotPath, c.wantPath)
			}
			if gotField != c.wantField {
				t.Errorf("multipart file field = %q, want %q — Telegram rejects anything else", gotField, c.wantField)
			}
			if gotChatID != "5991058878" {
				t.Errorf("chat_id = %q, want the address external id", gotChatID)
			}
			if gotBytes != len(audio) {
				t.Errorf("uploaded %d bytes, want %d", gotBytes, len(audio))
			}
		})
	}
}

// QA: inbound voice notes failed transcription with "Unsupported MIME type:
// application/octet-stream". Telegram's file CDN serves them with that generic type, so
// the format has to come from the file path instead.
func TestMimeFromPath(t *testing.T) {
	cases := map[string]string{
		"voice/file_12.oga":  "audio/ogg",
		"voice/file_12.ogg":  "audio/ogg",
		"voice/file_12.opus": "audio/ogg",
		"audio/file_9.mp3":   "audio/mpeg",
		"audio/file_9.m4a":   "audio/mp4",
		"audio/file_9.wav":   "audio/wav",
		"photos/file_3.jpg":  "image/jpeg",
		"photos/file_3.png":  "image/png",
		// Telegram voice notes are always OGG/Opus, so that is the safest fallback for
		// an unknown extension — octet-stream is guaranteed to be rejected.
		"voice/file_12":     "audio/ogg",
		"voice/file_12.bin": "audio/ogg",
	}
	for path, want := range cases {
		if got := mimeFromPath(path); got != want {
			t.Errorf("mimeFromPath(%q) = %q, want %q", path, got, want)
		}
	}

	// Extension matching must be case-insensitive: Telegram has served .OGA before.
	if got := mimeFromPath("voice/FILE.OGA"); got != "audio/ogg" {
		t.Errorf("uppercase extension not handled: got %q", got)
	}
	// Whatever happens, never hand the transcription API the type it rejects.
	for _, p := range []string{"", "weird", "a/b/c.xyz"} {
		if got := mimeFromPath(p); got == "application/octet-stream" {
			t.Errorf("mimeFromPath(%q) returned the type that breaks transcription", p)
		}
	}
}
