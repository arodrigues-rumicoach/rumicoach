package fcm

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"go.uber.org/zap"
	"google.golang.org/api/option"
)

type FCMProvider struct {
	logger *zap.Logger
	client *messaging.Client
}

func NewFCMProvider(ctx context.Context, logger *zap.Logger, projectID string) (*FCMProvider, error) {
	var fbConfig *firebase.Config
	if projectID != "" {
		fbConfig = &firebase.Config{ProjectID: projectID}
	}

	var opts []option.ClientOption

	if credsBase64 := os.Getenv("FIREBASE_CREDENTIALS_BASE64"); credsBase64 != "" {
		credsBytes, err := base64.StdEncoding.DecodeString(credsBase64)
		if err != nil {
			return nil, fmt.Errorf("error decoding FIREBASE_CREDENTIALS_BASE64: %w", err)
		}
		opts = append(opts, option.WithCredentialsJSON(credsBytes))
	} else if credsJSON := os.Getenv("FIREBASE_CREDENTIALS_JSON"); credsJSON != "" {
		opts = append(opts, option.WithCredentialsJSON([]byte(credsJSON)))
	}

	app, err := firebase.NewApp(ctx, fbConfig, opts...)
	if err != nil {
		return nil, fmt.Errorf("error initializing firebase app: %w", err)
	}

	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting firebase messaging client: %w", err)
	}

	return &FCMProvider{
		logger: logger,
		client: client,
	}, nil
}

func (f *FCMProvider) SendPush(ctx context.Context, token, title, body string) error {
	message := &messaging.Message{
		Token: token,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
	}

	response, err := f.client.Send(ctx, message)
	if err != nil {
		// The token is a device credential, not a diagnostic. Its tail is enough to tell two
		// devices apart in a log without writing the key itself.
		f.logger.Error("failed to send FCM push notification", zap.String("token_tail", tokenTail(token)), zap.Error(err))
		return err
	}

	f.logger.Info("successfully sent FCM push notification", zap.String("message_id", response), zap.String("token_tail", tokenTail(token)))
	return nil
}

// tokenTail is the last few characters of a device token — enough to follow one device
// through a log without writing the credential itself.
func tokenTail(token string) string {
	if len(token) <= 6 {
		return "…"
	}
	return "…" + token[len(token)-6:]
}
