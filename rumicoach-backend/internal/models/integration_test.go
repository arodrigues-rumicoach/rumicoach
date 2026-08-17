package models

import (
	"testing"
	"time"
)

func TestInsideServiceWindow(t *testing.T) {
	window := 24 * time.Hour

	integration := Integration{}
	if integration.InsideServiceWindow(window) {
		t.Error("nil LastInboundAt must be outside the window")
	}

	recent := time.Now().Add(-23 * time.Hour)
	integration.LastInboundAt = &recent
	if !integration.InsideServiceWindow(window) {
		t.Error("23h-old inbound must be inside the 24h window")
	}

	stale := time.Now().Add(-25 * time.Hour)
	integration.LastInboundAt = &stale
	if integration.InsideServiceWindow(window) {
		t.Error("25h-old inbound must be outside the 24h window")
	}
}

func TestCanSendFreeForm(t *testing.T) {
	window := 24 * time.Hour
	stale := time.Now().Add(-25 * time.Hour)

	waIntegration := Integration{
		Provider:      ChannelProviderWhatsApp,
		LastInboundAt: &stale,
	}
	if waIntegration.CanSendFreeForm(window) {
		t.Error("WhatsApp outside 24h window must return false for CanSendFreeForm")
	}

	tgIntegration := Integration{
		Provider:      ChannelProviderTelegram,
		LastInboundAt: &stale,
	}
	if !tgIntegration.CanSendFreeForm(window) {
		t.Error("Telegram must return true for CanSendFreeForm even outside 24h window")
	}
}
