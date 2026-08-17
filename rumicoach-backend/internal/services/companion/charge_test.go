package companion

import (
	"testing"

	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/balance"
)

// Each side of an exchange is priced for what it actually was: audio by its length,
// text by the flat reply fee. A spoken message is minutes of the coach's attention
// however cheap it was to transcribe, which is why token spend does not decide this.
func TestChargeSeconds(t *testing.T) {
	const flat = balance.MessageCostSeconds // 5

	for _, c := range []struct {
		name      string
		spend     Spend
		replyType string
		want      int64
	}{
		{
			// The common case: typed question, typed answer. Only the reply is priced.
			name:      "text in, text out",
			replyType: models.ChannelMessageTypeText,
			want:      flat,
		},
		{
			// Both recordings are charged — the one the user spoke and the one they
			// listened to.
			name:      "voice note answered by voice",
			spend:     Spend{InboundAudioSeconds: 30, OutboundAudioSeconds: 12},
			replyType: models.ChannelMessageTypeAudio,
			want:      42,
		},
		{
			// The ordinary case for someone whose integration is set to ReplyMode
			// "text": they may speak, we always write back. Reached by a TTS failure
			// falling back too — the price is the same either way, because what the
			// user spoke and what they were sent are what is being charged, not why.
			name:      "voice note answered in text",
			spend:     Spend{InboundAudioSeconds: 30},
			replyType: models.ChannelMessageTypeText,
			want:      30 + flat,
		},
		{
			// Typed question, spoken answer (the user asked for voice replies). Asking
			// costs nothing; the recording they receive is charged by its length.
			name:      "text answered by voice",
			spend:     Spend{OutboundAudioSeconds: 8},
			replyType: models.ChannelMessageTypeAudio,
			want:      8,
		},
		{
			// The inbound duration could not be measured (no ffprobe, corrupt media).
			// That side is not charged rather than guessed at.
			name:      "unmeasurable voice note answered in text",
			spend:     Spend{InboundAudioSeconds: 0},
			replyType: models.ChannelMessageTypeText,
			want:      flat,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := c.spend.ChargeSeconds(c.replyType); got != c.want {
				t.Errorf("ChargeSeconds = %d, want %d", got, c.want)
			}
		})
	}
}

// The token counts and the audio seconds are different currencies and must not be
// confused: a long voice note is few tokens and many seconds.
func TestChargeSecondsIgnoresTokens(t *testing.T) {
	spend := Spend{
		Chat:                Usage{InputTokens: 900_000, OutputTokens: 900_000},
		STT:                 Usage{InputTokens: 500_000},
		TTS:                 Usage{OutputTokens: 700_000},
		InboundAudioSeconds: 3,
	}
	if got := spend.ChargeSeconds(models.ChannelMessageTypeText); got != 3+balance.MessageCostSeconds {
		t.Errorf("ChargeSeconds = %d, want %d — token spend must not reach the user's bill",
			got, 3+balance.MessageCostSeconds)
	}
}

// Gemini TTS returns s16le/24kHz/mono, so the byte count is the duration exactly.
// Truncated, matching how a voice session's elapsed time is debited.
func TestPCMDurationSeconds(t *testing.T) {
	const bytesPerSecond = ttsSampleRate * ttsBytesPerSample

	for _, c := range []struct {
		name  string
		bytes int
		want  int64
	}{
		{"silence", 0, 0},
		{"exactly one second", bytesPerSecond, 1},
		{"ten seconds", 10 * bytesPerSecond, 10},
		{"a partial second is not charged", bytesPerSecond - 1, 0},
		{"partial seconds truncate down", 3*bytesPerSecond + bytesPerSecond/2, 3},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := pcmDurationSeconds(make([]byte, c.bytes)); got != c.want {
				t.Errorf("pcmDurationSeconds(%d bytes) = %d, want %d", c.bytes, got, c.want)
			}
		})
	}
}
