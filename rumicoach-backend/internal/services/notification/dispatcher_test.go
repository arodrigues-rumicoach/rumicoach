package notification

import (
	"context"
	"testing"
	"time"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/messaging"
	"github.com/rumi/rumi-be/internal/services/notification/provider/fcm"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeChannel struct {
	sentTexts []string
}

func (f *fakeChannel) Provider() string { return models.ChannelProviderWhatsApp }

func (f *fakeChannel) SendText(ctx context.Context, to messaging.Address, text string) (string, error) {
	f.sentTexts = append(f.sentTexts, text)
	return "wamid.test", nil
}

func (f *fakeChannel) SendAudio(ctx context.Context, to messaging.Address, audio []byte, mimeType string, asVoiceNote bool) (string, error) {
	return "", nil
}

func (f *fakeChannel) SendTemplate(ctx context.Context, to messaging.Address, tmpl messaging.TemplateMessage) (string, error) {
	return "", nil
}

func (f *fakeChannel) DownloadMedia(ctx context.Context, mediaID string) ([]byte, string, error) {
	return nil, "", nil
}

func setupDeliverTest(t *testing.T) *fakeChannel {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	database.DB = db
	if err := db.AutoMigrate(&models.Notification{}, &models.ChannelMessage{}, &models.UserDevice{}); err != nil {
		t.Fatalf("failed to migrate test tables: %v", err)
	}
	// Created via raw SQL: AutoMigrate keeps the postgres "timestamp with time
	// zone" column type, which the sqlite driver cannot scan back into time.Time.
	if err := db.Exec(`CREATE TABLE integrations (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		external_id TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		link_code TEXT,
		link_code_expires_at DATETIME,
		reply_mode TEXT NOT NULL DEFAULT 'text',
		last_inbound_at DATETIME,
		last_outbound_at DATETIME,
		daily_inbound_count INTEGER NOT NULL DEFAULT 0,
		daily_inbound_date DATETIME,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("failed to create integrations table: %v", err)
	}

	logger := zap.NewNop()
	GlobalNotificationService = &NotificationService{
		fcmProvider: fcm.NewMockProvider(logger),
		logger:      logger,
	}

	channel := &fakeChannel{}
	messaging.Register(channel)
	return channel
}

func TestDeliverPrefersActiveChannelInsideWindow(t *testing.T) {
	channel := setupDeliverTest(t)

	recent := time.Now().Add(-1 * time.Hour)
	phone := "+351910000000"
	integration := models.Integration{
		UserID:        "user-1",
		Provider:      models.ChannelProviderWhatsApp,
		ExternalID:    &phone,
		Status:        models.IntegrationActive,
		LastInboundAt: &recent,
	}
	if err := database.DB.Create(&integration).Error; err != nil {
		t.Fatalf("failed to create integration: %v", err)
	}

	n := models.Notification{UserID: "user-1", Title: "Check in", Message: "How is your plan going?", DelayHours: 1}
	via, err := Deliver(context.Background(), zap.NewNop(), &n)
	if err != nil {
		t.Fatalf("Deliver failed: %v", err)
	}
	if via != models.ChannelProviderWhatsApp {
		t.Errorf("expected delivery via whatsapp, got %q", via)
	}
	if len(channel.sentTexts) != 1 || channel.sentTexts[0] != "Check in\n\nHow is your plan going?" {
		t.Errorf("unexpected channel sends: %#v", channel.sentTexts)
	}

	var recorded int64
	database.DB.Model(&models.ChannelMessage{}).
		Where("binding_id = ? AND direction = ?", integration.ID, models.ChannelMessageOutbound).
		Count(&recorded)
	if recorded != 1 {
		t.Errorf("expected 1 outbound channel message recorded, got %d", recorded)
	}
}

func TestDeliverFallsBackToPushOutsideServiceWindow(t *testing.T) {
	channel := setupDeliverTest(t)

	stale := time.Now().Add(-48 * time.Hour)
	phone := "+351910000001"
	integration := models.Integration{
		UserID:        "user-2",
		Provider:      models.ChannelProviderWhatsApp,
		ExternalID:    &phone,
		Status:        models.IntegrationActive,
		LastInboundAt: &stale,
	}
	if err := database.DB.Create(&integration).Error; err != nil {
		t.Fatalf("failed to create integration: %v", err)
	}

	n := models.Notification{UserID: "user-2", Title: "Hello", Message: "Body", DelayHours: 1}
	via, err := Deliver(context.Background(), zap.NewNop(), &n)
	if err != nil {
		t.Fatalf("Deliver failed: %v", err)
	}
	if via != models.NotificationChannelPush {
		t.Errorf("expected push fallback outside service window, got %q", via)
	}
	if len(channel.sentTexts) != 0 {
		t.Errorf("expected no channel sends, got %#v", channel.sentTexts)
	}
}

func TestDeliverFallsBackToPushWithoutBinding(t *testing.T) {
	channel := setupDeliverTest(t)

	n := models.Notification{UserID: "user-3", Title: "Hello", Message: "Body", DelayHours: 1}
	via, err := Deliver(context.Background(), zap.NewNop(), &n)
	if err != nil {
		t.Fatalf("Deliver failed: %v", err)
	}
	if via != models.NotificationChannelPush {
		t.Errorf("expected push delivery without integrations, got %q", via)
	}
	if len(channel.sentTexts) != 0 {
		t.Errorf("expected no channel sends, got %#v", channel.sentTexts)
	}
}
