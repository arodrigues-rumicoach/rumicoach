package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func setupChannelsTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	database.DB = db

	// Create tables manually to avoid PostgreSQL timezone datatype parsing issues on SQLite
	if err := db.Exec(`CREATE TABLE integrations (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		external_id TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		link_code TEXT UNIQUE,
		link_code_expires_at DATETIME,
		reply_mode TEXT NOT NULL DEFAULT 'text',
		last_inbound_at DATETIME,
		last_outbound_at DATETIME,
		daily_inbound_count INTEGER NOT NULL DEFAULT 0,
		daily_inbound_date DATETIME,
		created_at DATETIME,
		updated_at DATETIME,
		UNIQUE(provider, external_id)
	)`).Error; err != nil {
		t.Fatalf("failed to create integrations: %v", err)
	}
	if err := db.Exec(`CREATE TABLE channel_messages (
		id TEXT PRIMARY KEY,
		binding_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		direction TEXT NOT NULL,
		provider_message_id TEXT UNIQUE,
		type TEXT NOT NULL,
		body TEXT,
		media_id TEXT,
		status TEXT NOT NULL,
		input_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		expires_at DATETIME, created_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("failed to create channel_messages: %v", err)
	}

	config.AppConfig = &config.Config{
		WhatsAppEnabled:        true,
		WhatsAppBusinessNumber: "15550783881",
	}
}

func TestGenerateLinkCodeFormat(t *testing.T) {
	// Must match the webhook's matcher so codes sent over WhatsApp are recognized.
	for i := 0; i < 50; i++ {
		code, err := generateLinkCode()
		if err != nil {
			t.Fatalf("generateLinkCode: %v", err)
		}
		if got := linkCodePattern.FindString(code); got != code {
			t.Fatalf("code %q does not match webhook pattern", code)
		}
	}
}

func TestMaskExternalID(t *testing.T) {
	if got := maskExternalID("351912345678"); got != "3519••••••78" {
		t.Errorf("maskExternalID = %q", got)
	}
	if got := maskExternalID("1234"); got != "•••" {
		t.Errorf("short id mask = %q", got)
	}
}

func TestChannelLinkFlow(t *testing.T) {
	setupChannelsTestDB(t)
	server := NewServer(zap.NewNop(), nil, nil, nil)
	userID := "user-1"

	do := func(method, path string, body []byte, fn func(w http.ResponseWriter, r *http.Request)) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req = req.WithContext(auth.WithUserID(req.Context(), userID))
		rec := httptest.NewRecorder()
		fn(rec, req)
		return rec
	}

	// 1. Create a link code
	rec := do(http.MethodPost, "/v1/me/integrations/whatsapp/link", nil, server.LinkWhatsAppIntegration)
	if rec.Code != http.StatusCreated {
		t.Fatalf("link: status %d: %s", rec.Code, rec.Body.String())
	}
	var link api.ChannelLinkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &link); err != nil {
		t.Fatalf("link: bad body: %v", err)
	}
	if !regexp.MustCompile(`^RUMI-[A-Z0-9]{6}$`).MatchString(link.Code) {
		t.Errorf("link code format: %q", link.Code)
	}
	if !strings.Contains(link.WaLink, "wa.me/15550783881") || !strings.Contains(link.WaLink, link.Code) {
		t.Errorf("wa.me link malformed: %q", link.WaLink)
	}
	if !link.ExpiresAt.After(time.Now()) {
		t.Errorf("expiry not in the future: %v", link.ExpiresAt)
	}

	// 2. Re-linking replaces the pending integration
	rec = do(http.MethodPost, "/v1/me/integrations/whatsapp/link", nil, server.LinkWhatsAppIntegration)
	if rec.Code != http.StatusCreated {
		t.Fatalf("relink: status %d", rec.Code)
	}
	var active int64
	database.DB.Model(&models.Integration{}).Where("user_id = ? AND status <> ?", userID, models.IntegrationRevoked).Count(&active)
	if active != 1 {
		t.Errorf("expected 1 non-revoked integration after relink, got %d", active)
	}

	// 3. List channels
	rec = do(http.MethodGet, "/v1/me/integrations", nil, server.GetIntegrations)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status %d", rec.Code)
	}
	var listed []api.Integration
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("list: bad body: %v", err)
	}
	if len(listed) != 1 || listed[0].Status != models.IntegrationPending || listed[0].ReplyMode != models.ChannelReplyModeText {
		t.Fatalf("unexpected listing: %+v", listed)
	}
	bindingID := listed[0].Id

	// 4. Update reply mode (invalid then valid)
	rec = do(http.MethodPatch, "/v1/me/integrations/"+bindingID, []byte(`{"replyMode":"video"}`), func(w http.ResponseWriter, r *http.Request) {
		server.UpdateIntegration(w, r, bindingID)
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid replyMode: status %d", rec.Code)
	}
	rec = do(http.MethodPatch, "/v1/me/integrations/"+bindingID, []byte(`{"replyMode":"audio"}`), func(w http.ResponseWriter, r *http.Request) {
		server.UpdateIntegration(w, r, bindingID)
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update: status %d: %s", rec.Code, rec.Body.String())
	}
	var updated api.Integration
	json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.ReplyMode != models.ChannelReplyModeAudio {
		t.Errorf("replyMode not updated: %+v", updated)
	}

	// 5. Another user cannot touch the integration
	otherReq := httptest.NewRequest(http.MethodDelete, "/v1/me/integrations/"+bindingID, nil)
	otherReq = otherReq.WithContext(auth.WithUserID(otherReq.Context(), "user-2"))
	otherRec := httptest.NewRecorder()
	server.DeleteIntegration(otherRec, otherReq, bindingID)
	if otherRec.Code != http.StatusNotFound {
		t.Errorf("cross-user delete: status %d", otherRec.Code)
	}

	// 6. Revoke, then it disappears and a second delete 404s
	rec = do(http.MethodDelete, "/v1/me/integrations/"+bindingID, nil, func(w http.ResponseWriter, r *http.Request) {
		server.DeleteIntegration(w, r, bindingID)
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status %d", rec.Code)
	}
	rec = do(http.MethodGet, "/v1/me/integrations", nil, server.GetIntegrations)
	listed = nil
	json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed) != 0 {
		t.Errorf("revoked integration still listed: %+v", listed)
	}
	rec = do(http.MethodDelete, "/v1/me/integrations/"+bindingID, nil, func(w http.ResponseWriter, r *http.Request) {
		server.DeleteIntegration(w, r, bindingID)
	})
	if rec.Code != http.StatusNotFound {
		t.Errorf("second delete: status %d", rec.Code)
	}
}

func TestChannelLinkDisabled(t *testing.T) {
	setupChannelsTestDB(t)
	config.AppConfig.WhatsAppEnabled = false
	server := NewServer(zap.NewNop(), nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/me/integrations/whatsapp/link", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-1"))
	rec := httptest.NewRecorder()
	server.LinkWhatsAppIntegration(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when WhatsApp disabled, got %d", rec.Code)
	}
}

// TestChannelMessageDedupe verifies the ON CONFLICT DO NOTHING insert used by
// the webhook handler drops Meta's redeliveries (same wamid).
func TestChannelMessageDedupe(t *testing.T) {
	setupChannelsTestDB(t)

	wamid := "wamid.DUP=="
	body := "hello"
	insert := func() *gorm.DB {
		msg := models.ChannelMessage{
			BindingID:         "b-1",
			UserID:            "user-1",
			Provider:          models.ChannelProviderWhatsApp,
			Direction:         models.ChannelMessageInbound,
			ProviderMessageID: &wamid,
			Type:              models.ChannelMessageTypeText,
			Body:              &body,
			Status:            models.ChannelMessageReceived,
		}
		return database.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "provider_message_id"}},
			DoNothing: true,
		}).Create(&msg)
	}

	first := insert()
	if first.Error != nil || first.RowsAffected != 1 {
		t.Fatalf("first insert: err=%v rows=%d", first.Error, first.RowsAffected)
	}
	second := insert()
	if second.Error != nil {
		t.Fatalf("duplicate insert errored: %v", second.Error)
	}
	if second.RowsAffected != 0 {
		t.Errorf("duplicate insert affected %d rows, want 0", second.RowsAffected)
	}
}

// A provider-name lookup must resolve to the live binding, never a revoked one
// left behind by an earlier link. The revoked row here sorts first by primary
// key — the order GORM's bare `First` would apply — so this fails if the status
// filter or the explicit ordering is dropped from findLiveIntegration.
func TestLookupByProviderSkipsRevoked(t *testing.T) {
	setupChannelsTestDB(t)
	server := NewServer(zap.NewNop(), nil, nil, nil)
	userID := "user-1"

	old := "+351900000001"
	now := time.Now()
	seed := []models.Integration{
		{ID: "aaa-revoked", UserID: userID, Provider: models.ChannelProviderTelegram,
			ExternalID: &old, Status: models.IntegrationRevoked,
			ReplyMode: models.ChannelReplyModeText, CreatedAt: now.Add(-time.Hour)},
		{ID: "zzz-live", UserID: userID, Provider: models.ChannelProviderTelegram,
			Status:    models.IntegrationActive,
			ReplyMode: models.ChannelReplyModeText, CreatedAt: now},
	}
	for _, in := range seed {
		if err := database.DB.Create(&in).Error; err != nil {
			t.Fatalf("seed %s: %v", in.ID, err)
		}
	}

	do := func(method, path string, body []byte, fn func(http.ResponseWriter, *http.Request, string), ref string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req = req.WithContext(auth.WithUserID(req.Context(), userID))
		rec := httptest.NewRecorder()
		fn(rec, req, ref)
		return rec
	}

	// GET by provider name resolves the live binding.
	rec := do(http.MethodGet, "/v1/me/integrations/telegram", nil, server.GetIntegration, "telegram")
	if rec.Code != http.StatusOK {
		t.Fatalf("get: status %d: %s", rec.Code, rec.Body.String())
	}
	var got api.Integration
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("get: bad body: %v", err)
	}
	if got.Id != "zzz-live" {
		t.Errorf("get by provider returned %q, want the live binding zzz-live", got.Id)
	}

	// PATCH by provider name updates the live binding, not the revoked one.
	body, _ := json.Marshal(map[string]string{"replyMode": models.ChannelReplyModeAudio})
	rec = do(http.MethodPatch, "/v1/me/integrations/telegram", body, server.UpdateIntegration, "telegram")
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: status %d: %s", rec.Code, rec.Body.String())
	}
	var after models.Integration
	if err := database.DB.First(&after, "id = ?", "zzz-live").Error; err != nil {
		t.Fatalf("reload live: %v", err)
	}
	if after.ReplyMode != models.ChannelReplyModeAudio {
		t.Errorf("live binding replyMode = %q, want audio", after.ReplyMode)
	}
	var revoked models.Integration
	if err := database.DB.First(&revoked, "id = ?", "aaa-revoked").Error; err != nil {
		t.Fatalf("reload revoked: %v", err)
	}
	if revoked.ReplyMode != models.ChannelReplyModeText {
		t.Errorf("revoked binding was modified: replyMode = %q", revoked.ReplyMode)
	}

	// A fully revoked provider has no live binding to return.
	database.DB.Model(&models.Integration{}).Where("id = ?", "zzz-live").
		Update("status", models.IntegrationRevoked)
	rec = do(http.MethodGet, "/v1/me/integrations/telegram", nil, server.GetIntegration, "telegram")
	if rec.Code != http.StatusNotFound {
		t.Errorf("get after full revoke: status %d, want 404", rec.Code)
	}
}

// QA: linked Telegram on mobile and everything showed connected; hours later the app
// said it was not. Cause: POST .../link revoked whatever binding existed before creating
// a fresh pending one, so merely reaching that endpoint again destroyed a working
// connection — which the app did on its own when the integrations list had not loaded
// yet and it could not tell there was anything to preserve.
//
// The endpoint must refuse instead, leaving the live binding untouched.
func TestLinkRefusesWhenAlreadyConnected(t *testing.T) {
	setupChannelsTestDB(t)
	config.AppConfig.TelegramEnabled = true
	config.AppConfig.TelegramBotUsername = "rumibot"
	server := NewServer(zap.NewNop(), nil, nil, nil)
	userID := "user-1"

	external := "5991058878"
	if err := database.DB.Create(&models.Integration{
		ID: "i1", UserID: userID, Provider: models.ChannelProviderTelegram,
		ExternalID: &external, Status: models.IntegrationActive,
		ReplyMode: models.ChannelReplyModeText, CreatedAt: time.Now().Add(-3 * time.Hour),
	}).Error; err != nil {
		t.Fatalf("seed integration: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/me/integrations/telegram/link", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), userID))
	rec := httptest.NewRecorder()
	server.LinkTelegramIntegration(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 — linking must not silently replace a live connection", rec.Code)
	}
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Code != "INTEGRATION_ALREADY_ACTIVE" {
		t.Errorf("error code = %q, want INTEGRATION_ALREADY_ACTIVE so the app can react", body.Code)
	}

	// The whole point: the connection survives, external id and all.
	var after models.Integration
	if err := database.DB.First(&after, "id = ?", "i1").Error; err != nil {
		t.Fatalf("integration disappeared: %v", err)
	}
	if after.Status != models.IntegrationActive {
		t.Errorf("status = %q, want it left active", after.Status)
	}
	if after.ExternalID == nil || *after.ExternalID != external {
		t.Errorf("external id = %v, want it preserved", after.ExternalID)
	}
	// And no stray pending row alongside it.
	var count int64
	database.DB.Model(&models.Integration{}).Where("user_id = ?", userID).Count(&count)
	if count != 1 {
		t.Errorf("integration rows = %d, want 1 — a refused link must create nothing", count)
	}
}

// The guard protects a WORKING connection; it must not block the flow otherwise.
func TestLinkStillWorksWhenNotConnected(t *testing.T) {
	setupChannelsTestDB(t)
	config.AppConfig.TelegramEnabled = true
	config.AppConfig.TelegramBotUsername = "rumibot"
	server := NewServer(zap.NewNop(), nil, nil, nil)
	userID := "user-1"

	link := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/me/integrations/telegram/link", nil)
		req = req.WithContext(auth.WithUserID(req.Context(), userID))
		rec := httptest.NewRecorder()
		server.LinkTelegramIntegration(rec, req)
		return rec
	}

	if rec := link(); rec.Code != http.StatusCreated {
		t.Fatalf("first link: status %d (%s)", rec.Code, rec.Body.String())
	}
	// A stale pending row must not block a retry — the user may have abandoned the
	// first attempt and come back to it.
	if rec := link(); rec.Code != http.StatusCreated {
		t.Errorf("retry over a pending link: status %d, want 201", rec.Code)
	}
	// Nor a revoked one from an earlier disconnect.
	database.DB.Model(&models.Integration{}).Where("user_id = ?", userID).
		Update("status", models.IntegrationRevoked)
	if rec := link(); rec.Code != http.StatusCreated {
		t.Errorf("relink after disconnecting: status %d, want 201", rec.Code)
	}

	// Another user's active connection is not this user's business.
	other := "999"
	database.DB.Create(&models.Integration{
		ID: "other", UserID: "someone-else", Provider: models.ChannelProviderTelegram,
		ExternalID: &other, Status: models.IntegrationActive, CreatedAt: time.Now(),
	})
	if rec := link(); rec.Code != http.StatusCreated {
		t.Errorf("another user's connection blocked this one: status %d", rec.Code)
	}
}
