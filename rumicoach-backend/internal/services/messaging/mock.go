package messaging

import (
	"context"
	"fmt"
	"sync/atomic"

	"go.uber.org/zap"
)

// MockChannel logs sends instead of calling a real provider. Used in local
// development (no WhatsApp credentials) and in tests.
type MockChannel struct {
	ProviderName string
	logger       *zap.Logger
	counter      atomic.Int64
}

func NewMockChannel(provider string, logger *zap.Logger) *MockChannel {
	return &MockChannel{ProviderName: provider, logger: logger}
}

func (m *MockChannel) Provider() string { return m.ProviderName }

func (m *MockChannel) SendText(ctx context.Context, to Address, text string) (string, error) {
	m.logger.Warn("!! MOCK CHANNEL TEXT SENT !!",
		zap.String("provider", m.ProviderName),
		zap.String("to", to.ExternalID),
		zap.String("text", text))
	return m.nextID(), nil
}

func (m *MockChannel) SendAudio(ctx context.Context, to Address, audio []byte, mimeType string, asVoiceNote bool) (string, error) {
	m.logger.Warn("!! MOCK CHANNEL AUDIO SENT !!",
		zap.String("provider", m.ProviderName),
		zap.String("to", to.ExternalID),
		zap.String("mimeType", mimeType),
		zap.Int("bytes", len(audio)),
		zap.Bool("voiceNote", asVoiceNote))
	return m.nextID(), nil
}

func (m *MockChannel) SendTemplate(ctx context.Context, to Address, tmpl TemplateMessage) (string, error) {
	m.logger.Warn("!! MOCK CHANNEL TEMPLATE SENT !!",
		zap.String("provider", m.ProviderName),
		zap.String("to", to.ExternalID),
		zap.String("template", tmpl.Name),
		zap.String("language", tmpl.Language),
		zap.Strings("params", tmpl.BodyParams))
	return m.nextID(), nil
}

func (m *MockChannel) DownloadMedia(ctx context.Context, mediaID string) ([]byte, string, error) {
	m.logger.Warn("!! MOCK CHANNEL MEDIA DOWNLOAD !!", zap.String("mediaID", mediaID))
	return nil, "", fmt.Errorf("mock channel has no media for id %q", mediaID)
}

func (m *MockChannel) nextID() string {
	return fmt.Sprintf("mock-msg-%d", m.counter.Add(1))
}
