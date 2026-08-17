package companion

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/messaging"
	"go.uber.org/zap"
)

// transcribeInbound downloads a voice note and transcribes it with Gemini
// (audio passed inline as multimodal input — no separate STT vendor).
// The returned seconds are how long the voice note runs, which is what the user is
// charged for — a spoken message is billed by its length, not at the flat text rate.
// Zero when it could not be measured; see audioDurationSeconds for why that is the
// safe direction.
func (s *Service) transcribeInbound(ctx context.Context, channel messaging.Channel, msg *models.ChannelMessage) (string, int64, Usage, error) {
	var usage Usage
	if msg.MediaID == nil {
		return "", 0, usage, fmt.Errorf("audio message %s has no media id", msg.ID)
	}
	data, mimeType, err := channel.DownloadMedia(ctx, *msg.MediaID)
	if err != nil {
		return "", 0, usage, fmt.Errorf("download media: %w", err)
	}
	seconds := audioDurationSeconds(ctx, data, s.logger)
	if mimeType == "" {
		mimeType = "audio/ogg"
	}
	// WhatsApp reports "audio/ogg; codecs=opus"; Gemini wants the bare type.
	if i := strings.IndexByte(mimeType, ';'); i > 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}

	req := Request{
		Contents: []Content{{
			Role: "user",
			Parts: []Part{
				{Text: "Transcribe this voice message verbatim, in the language the speaker is using. Output ONLY the transcript text — no labels, no quotes, no commentary. If the audio is unintelligible or contains no speech, output an empty string."},
				{InlineData: &Blob{MimeType: mimeType, Data: base64.StdEncoding.EncodeToString(data)}},
			},
		}},
	}
	resp, err := callGemini(ctx, transcribeModel(), req)
	if err != nil {
		return "", seconds, usage, fmt.Errorf("transcribe: %w", err)
	}
	usage.AddResponse(resp)
	candidate, err := firstCandidate(resp)
	if err != nil {
		return "", seconds, usage, fmt.Errorf("transcribe: %w", err)
	}
	var transcript string
	for _, part := range candidate.Parts {
		transcript += part.Text
	}
	return strings.TrimSpace(transcript), seconds, usage, nil
}

// sendVoiceReply synthesizes the reply with Gemini TTS, transcodes the PCM to
// OGG/Opus (WhatsApp's voice-note format), and sends it. Any failure returns
// an error so the caller can fall back to text.
// The returned seconds are the length of the voice note sent, which the user is
// charged for. Computed from the raw PCM rather than probed: Gemini TTS hands back
// s16le/24kHz/mono, so the byte count IS the duration, exactly and for free.
func (s *Service) sendVoiceReply(ctx context.Context, channel messaging.Channel, to messaging.Address, user *models.User, text string) (string, int64, Usage, error) {
	if !ffmpegAvailable(s.logger) {
		return "", 0, Usage{}, fmt.Errorf("ffmpeg not available for voice-note transcoding")
	}

	pcm, usage, err := s.synthesize(ctx, user, text)
	if err != nil {
		return "", 0, usage, err
	}
	ogg, err := transcodePCMToOggOpus(ctx, pcm)
	if err != nil {
		return "", 0, usage, fmt.Errorf("transcode to ogg/opus: %w", err)
	}
	providerMsgID, err := channel.SendAudio(ctx, to, ogg, "audio/ogg", true)
	if err != nil {
		// Nothing reached the user, so nothing is billable.
		return "", 0, usage, err
	}
	return providerMsgID, pcmDurationSeconds(pcm), usage, nil
}

// ttsSampleRate / ttsBytesPerSample describe Gemini TTS output: s16le, 24 kHz, mono.
// They must match the -ar/-f flags transcodePCMToOggOpus passes ffmpeg.
const (
	ttsSampleRate     = 24000
	ttsBytesPerSample = 2
)

// pcmDurationSeconds is how long a raw PCM buffer plays, in whole seconds.
//
// Truncated rather than rounded up, matching how a voice session's elapsed time is
// debited: partial units are not charged. A voice note under a second is therefore
// free, which is a rounding error in the user's favour and not worth code to prevent.
func pcmDurationSeconds(pcm []byte) int64 {
	return int64(len(pcm)) / (ttsSampleRate * ttsBytesPerSample)
}

// audioDurationSeconds measures a received voice note with ffprobe, which ships in
// the same package as the ffmpeg the transcoder already needs.
//
// The providers do not tell us: neither webhook carries a duration, so the only way
// to know how long the user spoke is to look at the bytes.
//
// A failure returns 0, and the caller charges nothing for that side. Undercharging on
// a measurement we could not make is the safe direction — the alternative is inventing
// a number and billing someone for it — but it is silent, so it logs at Error: if
// ffprobe were missing in production every voice note would quietly become free.
func audioDurationSeconds(ctx context.Context, data []byte, logger *zap.Logger) int64 {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		"-i", "pipe:0",
	)
	cmd.Stdin = bytes.NewReader(data)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		logger.Error("companion: could not measure inbound audio; it will not be charged",
			zap.Error(err), zap.String("ffprobe", truncate(errBuf.String(), 256)))
		return 0
	}
	secs, err := strconv.ParseFloat(strings.TrimSpace(out.String()), 64)
	if err != nil || secs < 0 {
		logger.Error("companion: ffprobe returned no usable duration; the audio will not be charged",
			zap.String("output", truncate(out.String(), 128)))
		return 0
	}
	return int64(secs) // truncated, like pcmDurationSeconds
}

// synthesize runs Gemini TTS and returns raw PCM (s16le, 24 kHz, mono).
func (s *Service) synthesize(ctx context.Context, user *models.User, text string) ([]byte, Usage, error) {
	req := Request{
		Contents: []Content{{Role: "user", Parts: []Part{{Text: text}}}},
		GenerationConfig: &GenerationConfig{
			ResponseModalities: []string{"AUDIO"},
			SpeechConfig: &SpeechConfig{
				VoiceConfig: &VoiceConfig{PrebuiltVoiceConfig: &PrebuiltVoiceConfig{VoiceName: ttsVoiceForUser(user)}},
			},
		},
	}
	var usage Usage
	resp, err := callGemini(ctx, ttsModel(), req)
	if err != nil {
		return nil, usage, fmt.Errorf("tts: %w", err)
	}
	usage.AddResponse(resp)
	candidate, err := firstCandidate(resp)
	if err != nil {
		return nil, usage, fmt.Errorf("tts: %w", err)
	}
	for _, part := range candidate.Parts {
		if part.InlineData != nil && part.InlineData.Data != "" {
			pcm, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
			if err != nil {
				return nil, usage, fmt.Errorf("tts: decode audio: %w", err)
			}
			return pcm, usage, nil
		}
	}
	return nil, usage, fmt.Errorf("tts: response contained no audio data")
}

// ttsVoiceForUser mirrors the live engine's voice selection (live_api.go):
// the user's chosen coach voice, else gender default. Gemini TTS expects the
// capitalized prebuilt-voice names.
func ttsVoiceForUser(user *models.User) string {
	voice := "gacrux"
	if user != nil {
		if user.CoachVoice != nil && *user.CoachVoice != "" {
			voice = *user.CoachVoice
		} else if user.CoachGender != nil && *user.CoachGender == "male" {
			voice = "charon"
		}
	}
	return strings.ToUpper(voice[:1]) + voice[1:]
}

// transcodePCMToOggOpus pipes raw PCM (s16le 24 kHz mono — Gemini TTS output)
// through ffmpeg into OGG/Opus, the only format WhatsApp renders as a true
// voice-note bubble.
func transcodePCMToOggOpus(ctx context.Context, pcm []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-f", "s16le", "-ar", "24000", "-ac", "1", "-i", "pipe:0",
		"-c:a", "libopus", "-b:a", "32k", "-application", "voip",
		"-f", "ogg", "pipe:1",
	)
	cmd.Stdin = bytes.NewReader(pcm)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, truncate(errBuf.String(), 512))
	}
	return out.Bytes(), nil
}

var (
	ffmpegOnce  sync.Once
	ffmpegFound bool
)

// ffmpegAvailable checks (once) whether ffmpeg is on PATH; without it voice
// replies degrade to text.
func ffmpegAvailable(logger *zap.Logger) bool {
	ffmpegOnce.Do(func() {
		_, err := exec.LookPath("ffmpeg")
		ffmpegFound = err == nil
		if !ffmpegFound {
			logger.Warn("companion: ffmpeg not found on PATH — voice-note replies will fall back to text")
		}
	})
	return ffmpegFound
}
