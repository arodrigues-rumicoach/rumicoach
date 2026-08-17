package sms

import (
	"github.com/rumi/rumi-be/config"
	"github.com/twilio/twilio-go"
	twilioApi "github.com/twilio/twilio-go/rest/api/v2010"
	"go.uber.org/zap"
)

type TwilioProvider struct {
	client     *twilio.RestClient
	fromNumber string
	logger     *zap.Logger
}

func NewTwilioProvider(logger *zap.Logger) *TwilioProvider {
	if config.AppConfig.TwilioAccountSID != "" && config.AppConfig.TwilioAuthToken != "" {
		client := twilio.NewRestClientWithParams(twilio.ClientParams{
			Username: config.AppConfig.TwilioAccountSID,
			Password: config.AppConfig.TwilioAuthToken,
		})
		return &TwilioProvider{
			client:     client,
			fromNumber: config.AppConfig.TwilioFromNumber,
			logger:     logger,
		}
	}
	return nil
}

func (t *TwilioProvider) SendSMS(toPhone, content string) error {
	if t.client == nil {
		t.logger.Warn("Twilio client not initialized.")
		return nil
	}

	params := &twilioApi.CreateMessageParams{}
	params.SetTo(toPhone)
	params.SetFrom(t.fromNumber)
	params.SetBody(content)

	resp, err := t.client.Api.CreateMessage(params)
	if err != nil {
		t.logger.Error("Failed to send Twilio SMS", zap.Error(err))
		return err
	}

	t.logger.Info("Twilio SMS sent successfully", zap.String("sid", *resp.Sid))
	return nil
}
