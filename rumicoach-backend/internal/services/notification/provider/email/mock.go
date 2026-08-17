package email

import (
	"go.uber.org/zap"
)

type MockProvider struct {
	logger *zap.Logger
}

func NewMockProvider(logger *zap.Logger) *MockProvider {
	return &MockProvider{logger: logger}
}

func (m *MockProvider) SendEmail(toEmail, subject, htmlBody, textBody string) error {
	m.logger.Warn("!! MOCK EMAIL SENT !!",
		zap.String("to", toEmail),
		zap.String("subject", subject),
		zap.String("text", textBody),
		zap.Int("html_bytes", len(htmlBody)))
	return nil
}

func (m *MockProvider) SendEmailWithSender(fromName, fromEmail, toEmail, subject, htmlBody, textBody string) error {
	m.logger.Warn("!! MOCK EMAIL SENT !!",
		zap.String("from", fromEmail),
		zap.String("to", toEmail),
		zap.String("subject", subject),
		zap.String("text", textBody),
		zap.Int("html_bytes", len(htmlBody)))
	return nil
}

func (m *MockProvider) SendEmailWithAttachments(toEmail, subject, htmlBody, textBody string, attachments []Attachment) error {
	// Attachments are the user's personal data — log their size, never their contents.
	total := 0
	names := make([]string, 0, len(attachments))
	for _, a := range attachments {
		total += len(a.Content)
		names = append(names, a.Filename)
	}
	m.logger.Warn("!! MOCK EMAIL WITH ATTACHMENTS SENT !!",
		zap.String("to", toEmail),
		zap.String("subject", subject),
		zap.Strings("filenames", names),
		zap.Int("attachment_bytes", total))
	return nil
}
