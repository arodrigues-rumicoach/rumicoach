package email

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/rumi/rumi-be/config"
	"github.com/smtp2go-oss/smtp2go-go"
	"go.uber.org/zap"
)

type SMTP2GOProvider struct {
	apiKey    string
	fromEmail string
	fromName  string
	logger    *zap.Logger
}

func NewSMTP2GOProvider(logger *zap.Logger) *SMTP2GOProvider {
	if config.AppConfig.SMTP2GOAPIKey != "" {
		return &SMTP2GOProvider{
			apiKey:    config.AppConfig.SMTP2GOAPIKey,
			fromEmail: config.AppConfig.EmailFromEmail,
			fromName:  config.AppConfig.EmailFromName,
			logger:    logger,
		}
	}
	return nil
}

func (s *SMTP2GOProvider) SendEmailWithSender(fromName, fromEmail, toEmail, subject, htmlBody, textBody string) error {
	if s.apiKey == "" {
		s.logger.Warn("SMTP2GO API Key not configured.")
		return nil
	}

	os.Setenv("SMTP2GO_API_KEY", s.apiKey)

	email := &smtp2go.Email{
		From:     fmt.Sprintf("%s <%s>", fromName, fromEmail),
		To:       []string{toEmail},
		Subject:  subject,
		HtmlBody: htmlBody,
		TextBody: textBody,
	}

	res, err := smtp2go.Send(email)
	if err != nil {
		s.logger.Error("Failed to send SMTP2GO email (Network/HTTP)", zap.Error(err))
		return err
	}

	// Check for internal API errors
	if res.Data.Error != "" {
		s.logger.Error("SMTP2GO API Error",
			zap.String("error", res.Data.Error),
			zap.String("error_code", res.Data.ErrorCode),
			zap.String("request_id", res.RequestId))
		return fmt.Errorf("smtp2go api error: %s (code: %s)", res.Data.Error, res.Data.ErrorCode)
	}

	s.logger.Info("SMTP2GO Email sent successfully", zap.String("request_id", res.RequestId))
	return nil
}

func (s *SMTP2GOProvider) SendEmail(toEmail, subject, htmlBody, textBody string) error {
	return s.SendEmailWithSender(s.fromName, s.fromEmail, toEmail, subject, htmlBody, textBody)
}

func (s *SMTP2GOProvider) SendEmailWithAttachments(toEmail, subject, htmlBody, textBody string, attachments []Attachment) error {
	if s.apiKey == "" {
		s.logger.Warn("SMTP2GO API Key not configured.")
		return nil
	}

	os.Setenv("SMTP2GO_API_KEY", s.apiKey)

	files := make([]*smtp2go.EmailBinaryData, 0, len(attachments))
	for _, a := range attachments {
		files = append(files, &smtp2go.EmailBinaryData{
			Filename: a.Filename,
			Fileblob: base64.StdEncoding.EncodeToString(a.Content),
			MimeType: a.MimeType,
		})
	}

	email := &smtp2go.Email{
		From:        fmt.Sprintf("%s <%s>", s.fromName, s.fromEmail),
		To:          []string{toEmail},
		Subject:     subject,
		HtmlBody:    htmlBody,
		TextBody:    textBody,
		Attachments: files,
	}

	res, err := smtp2go.Send(email)
	if err != nil {
		s.logger.Error("Failed to send SMTP2GO email with attachments", zap.Error(err))
		return err
	}
	if res.Data.Error != "" {
		s.logger.Error("SMTP2GO API Error",
			zap.String("error", res.Data.Error),
			zap.String("error_code", res.Data.ErrorCode),
			zap.String("request_id", res.RequestId))
		return fmt.Errorf("smtp2go api error: %s (code: %s)", res.Data.Error, res.Data.ErrorCode)
	}

	s.logger.Info("SMTP2GO email with attachments sent",
		zap.String("request_id", res.RequestId), zap.Int("attachments", len(files)))
	return nil
}
