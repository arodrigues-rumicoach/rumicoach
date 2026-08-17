package fcm

import "context"

type Provider interface {
	SendPush(ctx context.Context, token, title, body string) error
}
