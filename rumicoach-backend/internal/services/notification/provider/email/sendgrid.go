package email

import (
	"encoding/base64"

	"github.com/rumi/rumi-be/config"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"go.uber.org/zap"
)

type SendGridProvider struct {
	client    *sendgrid.Client
	fromEmail string
	fromName  string
	logger    *zap.Logger
}

func NewSendGridProvider(logger *zap.Logger) *SendGridProvider {
	if config.AppConfig.SendgridAPIKey != "" {
		client := sendgrid.NewSendClient(config.AppConfig.SendgridAPIKey)
		return &SendGridProvider{
			client:    client,
			fromEmail: config.AppConfig.EmailFromEmail,
			fromName:  config.AppConfig.EmailFromName,
			logger:    logger,
		}
	}
	return nil
}

func (s *SendGridProvider) SendEmailWithSender(fromName, fromEmail, toEmail, subject, htmlBody, textBody string) error {
	if s.client == nil {
		s.logger.Warn("SendGrid client not initialized.")
		return nil
	}

	from := mail.NewEmail(fromName, fromEmail)
	to := mail.NewEmail("", toEmail)
	message := mail.NewSingleEmail(from, subject, to, textBody, htmlBody)

	response, err := s.client.Send(message)
	if err != nil {
		s.logger.Error("Failed to send SendGrid email", zap.Error(err))
		return err
	}

	s.logger.Info("SendGrid Email sent", zap.Int("status_code", response.StatusCode))
	return nil
}

func (s *SendGridProvider) SendEmail(toEmail, subject, htmlBody, textBody string) error {
	return s.SendEmailWithSender(s.fromName, s.fromEmail, toEmail, subject, htmlBody, textBody)
}

func (s *SendGridProvider) SendEmailWithAttachments(toEmail, subject, htmlBody, textBody string, attachments []Attachment) error {
	if s.client == nil {
		s.logger.Warn("SendGrid client not initialized.")
		return nil
	}

	from := mail.NewEmail(s.fromName, s.fromEmail)
	to := mail.NewEmail("", toEmail)
	message := mail.NewSingleEmail(from, subject, to, textBody, htmlBody)

	for _, a := range attachments {
		att := mail.NewAttachment()
		att.SetContent(base64.StdEncoding.EncodeToString(a.Content))
		att.SetType(a.MimeType)
		att.SetFilename(a.Filename)
		att.SetDisposition("attachment")
		message.AddAttachment(att)
	}

	response, err := s.client.Send(message)
	if err != nil {
		s.logger.Error("Failed to send SendGrid email with attachments", zap.Error(err))
		return err
	}

	s.logger.Info("SendGrid email with attachments sent",
		zap.Int("status_code", response.StatusCode), zap.Int("attachments", len(attachments)))
	return nil
}
