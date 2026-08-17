package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRegisterFCMToken(t *testing.T) {
	// Initialize in-memory SQLite DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	database.DB = db

	err = db.Exec(`CREATE TABLE user_devices (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		fcm_token TEXT UNIQUE NOT NULL,
		platform TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	)`).Error
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}

	logger := zap.NewNop()
	server := NewServer(logger, nil, nil, nil)

	userID := "user-123"

	t.Run("Unauthorized request", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/me/fcm-token", nil)
		w := httptest.NewRecorder()

		server.RegisterFCMToken(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})

	t.Run("Invalid JSON body", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/me/fcm-token", bytes.NewBufferString("{invalid"))
		ctx := auth.WithUserID(req.Context(), userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		server.RegisterFCMToken(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	t.Run("Missing token", func(t *testing.T) {
		body, _ := json.Marshal(api.FCMTokenRequest{
			Platform: func(s string) *string { return &s }("ios"),
			Token:    "",
		})
		req := httptest.NewRequest("POST", "/v1/me/fcm-token", bytes.NewBuffer(body))
		ctx := auth.WithUserID(req.Context(), userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		server.RegisterFCMToken(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	t.Run("Register new token", func(t *testing.T) {
		token := "my-fcm-token-123"
		platform := "ios"
		body, _ := json.Marshal(api.FCMTokenRequest{
			Platform: &platform,
			Token:    token,
		})
		req := httptest.NewRequest("POST", "/v1/me/fcm-token", bytes.NewBuffer(body))
		ctx := auth.WithUserID(req.Context(), userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		server.RegisterFCMToken(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("Expected status 204, got %d", w.Code)
		}

		// Verify database
		var device models.UserDevice
		if err := db.Where("fcm_token = ?", token).First(&device).Error; err != nil {
			t.Fatalf("Expected device to be saved in DB: %v", err)
		}
		if device.UserID != userID {
			t.Errorf("Expected user ID %s, got %s", userID, device.UserID)
		}
		if device.Platform != platform {
			t.Errorf("Expected platform %s, got %s", platform, device.Platform)
		}
	})

	t.Run("Update existing token owner", func(t *testing.T) {
		token := "my-fcm-token-123"
		newUserID := "user-456"
		newPlatform := "android"

		body, _ := json.Marshal(api.FCMTokenRequest{
			Platform: &newPlatform,
			Token:    token,
		})
		req := httptest.NewRequest("POST", "/v1/me/fcm-token", bytes.NewBuffer(body))
		ctx := auth.WithUserID(req.Context(), newUserID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		server.RegisterFCMToken(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("Expected status 204, got %d", w.Code)
		}

		// Verify database
		var devices []models.UserDevice
		if err := db.Where("fcm_token = ?", token).Find(&devices).Error; err != nil {
			t.Fatalf("DB query failed: %v", err)
		}
		if len(devices) != 1 {
			t.Fatalf("Expected exactly 1 device, got %d", len(devices))
		}
		if devices[0].UserID != newUserID {
			t.Errorf("Expected updated user ID %s, got %s", newUserID, devices[0].UserID)
		}
		if devices[0].Platform != newPlatform {
			t.Errorf("Expected updated platform %s, got %s", newPlatform, devices[0].Platform)
		}
	})
}

func TestExportCurrentUserData(t *testing.T) {
	// Hand-written DDL, not AutoMigrate: GORM writes the models' Postgres column types
	// verbatim, and SQLite then refuses to scan `timestamp with time zone` back into a
	// *time.Time. Every query silently errored, the handler skipped every block, and this
	// test passed on an empty `{}` because it only ever checked the status code.
	seedResetFixture(t)
	db := database.DB
	userID := "u1"

	linkCode := "RUMI-7K3M2X"
	db.Exec(`UPDATE users SET state = 'VISION_WHEEL_OF_LIFE', latest_session_handle = 'gemini-resume-handle' WHERE id = ?`, userID)
	db.Exec(`UPDATE integrations SET external_id = '351912345678', link_code = ? WHERE id = 'i1'`, linkCode)
	db.Exec(`UPDATE user_devices SET fcm_token = 'fcm-secret-token' WHERE id = 'd1'`)
	db.Exec(`UPDATE commitments SET title = 'Drink water' WHERE id = 'c1'`)
	db.Exec(`UPDATE commitment_completions SET date = '2026-08-01' WHERE id = 'cc1'`)
	db.Exec(`UPDATE user_memories SET content = 'Sou pai de dois' WHERE id = 'm1'`)

	logger := zap.NewNop()
	server := NewServer(logger, nil, nil, nil)

	req := httptest.NewRequest("GET", "/v1/me/data", nil)
	ctx := auth.WithUserID(req.Context(), userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	server.ExportCurrentUserData(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	if disp := resp.Header.Get("Content-Disposition"); !strings.Contains(disp, "attachment") {
		t.Errorf("expected Content-Disposition to be an attachment, got %v", disp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	raw := string(body)

	// The export is built from explicit view structs precisely so that none of this can
	// appear — not by being stripped afterwards, but by never being put in. A field that
	// leaks here is a field somebody added to a view struct on purpose.
	for _, forbidden := range []struct{ what, needle string }{
		{"a live push credential", "fcm-secret-token"},
		{"a live one-time link code", linkCode},
		{"the provider-side identity", "351912345678"},
		{"internal session state", "VISION_WHEEL_OF_LIFE"},
		{"a Gemini resumption handle", "gemini-resume-handle"},
		{"our surrogate keys", `"userId"`},
		{"our surrogate keys", `"commitmentId"`},
	} {
		if strings.Contains(raw, forbidden.needle) {
			t.Errorf("export leaks %s (%q):\n%s", forbidden.what, forbidden.needle, raw)
		}
	}

	// And what the user is owed is there.
	if !strings.Contains(raw, "Drink water") || !strings.Contains(raw, "Sou pai de dois") {
		t.Errorf("export is missing the user's own content:\n%s", raw)
	}
	// Completions are nested on their commitment: stripped of the id they would have been
	// a list of bare dates meaning nothing.
	if !strings.Contains(raw, `"completedOn"`) || !strings.Contains(raw, "2026-08-01") {
		t.Errorf("completion dates should be nested under their commitment:\n%s", raw)
	}
	// The connection is reported without the identifiers that make it work.
	integrations, ok := data["integrations"].([]interface{})
	if !ok || len(integrations) != 1 {
		t.Fatalf("integrations should be exported, got: %v", data["integrations"])
	}
	if got := integrations[0].(map[string]interface{})["provider"]; got != "telegram" {
		t.Errorf("integration provider = %v, want telegram", got)
	}
	// The device is reported without its token.
	devices, ok := data["devices"].([]interface{})
	if !ok || len(devices) != 1 {
		t.Fatalf("devices should be exported, got: %v", data["devices"])
	}
	if got := devices[0].(map[string]interface{})["platform"]; got != "ios" {
		t.Errorf("device platform = %v, want ios", got)
	}
}
