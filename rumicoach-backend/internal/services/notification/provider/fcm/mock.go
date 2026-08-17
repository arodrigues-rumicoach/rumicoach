package fcm

import (
	"context"

	"go.uber.org/zap"
)

type MockProvider struct {
	logger *zap.Logger
}

func NewMockProvider(logger *zap.Logger) *MockProvider {
	return &MockProvider{logger: logger}
}

func (m *MockProvider) SendPush(ctx context.Context, token, title, body string) error {
	m.logger.Warn("!! MOCK FCM PUSH SENT !!",
		zap.String("token", token),
		zap.String("title", title),
		zap.String("body", body))
	return nil
}
