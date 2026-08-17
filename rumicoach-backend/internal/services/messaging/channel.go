// Package messaging defines the abstraction for external messaging channels
// (WhatsApp today, Telegram later): sending text, audio, and template messages
// to a user's channel identity and fetching inbound media. Inbound webhooks are
// provider-specific and live with the HTTP handlers; this package only covers
// the outbound/media surface plus a provider registry.
package messaging

import (
	"context"
	"fmt"
	"sync"
)

// Address identifies a recipient on a channel: the provider name plus the
// provider-side identity (E.164 phone for WhatsApp, chat id for Telegram).
type Address struct {
	Provider   string
	ExternalID string
}

// TemplateMessage is a pre-approved proactive message (WhatsApp requires
// templates outside the 24h customer-service window).
type TemplateMessage struct {
	Name       string
	Language   string // provider locale, e.g. "en", "pt_PT"
	BodyParams []string
}

// Channel is one messaging provider. Send methods return the provider's
// message id so it can be persisted on the outbound ChannelMessage row.
type Channel interface {
	Provider() string
	SendText(ctx context.Context, to Address, text string) (providerMsgID string, err error)
	// SendAudio uploads and sends an audio message. asVoiceNote asks the
	// provider to render it as a voice-note bubble where supported (WhatsApp
	// requires OGG/Opus for that).
	SendAudio(ctx context.Context, to Address, audio []byte, mimeType string, asVoiceNote bool) (string, error)
	SendTemplate(ctx context.Context, to Address, tmpl TemplateMessage) (string, error)
	DownloadMedia(ctx context.Context, mediaID string) (data []byte, mimeType string, err error)
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Channel{}
)

// Register adds a channel to the process-wide registry (called from main.go wiring).
func Register(ch Channel) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[ch.Provider()] = ch
}

// Get returns the channel registered for a provider name.
func Get(provider string) (Channel, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	ch, ok := registry[provider]
	if !ok {
		return nil, fmt.Errorf("no messaging channel registered for provider %q", provider)
	}
	return ch, nil
}
