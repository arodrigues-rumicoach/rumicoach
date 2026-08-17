package sms

import (
	"go.uber.org/zap"
)

type MockProvider struct {
	logger *zap.Logger
}

func NewMockProvider(logger *zap.Logger) *MockProvider {
	return &MockProvider{logger: logger}
}

func (m *MockProvider) SendSMS(toPhone, content string) error {
	m.logger.Warn("!! MOCK SMS SENT !!",
		zap.String("to", toPhone),
		zap.String("content", content))
	return nil
}
