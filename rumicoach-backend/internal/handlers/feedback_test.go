package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// A one-pixel PNG and a one-pixel GIF, as the real bytes: the handler sniffs the format
// rather than believing the client, so the tests have to carry genuine files.
var (
	tinyPNG, _ = base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	tinyGIF, _ = base64.StdEncoding.DecodeString("R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7")
)

func setupFeedbackDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	database.DB = db
	for _, ddl := range []string{
		`CREATE TABLE feedbacks (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, category TEXT NOT NULL,
			description TEXT NOT NULL, platform TEXT, app_version TEXT, os_version TEXT,
			device_model TEXT, context TEXT, created_at DATETIME)`,
		`CREATE TABLE feedback_attachments (id TEXT PRIMARY KEY, feedback_id TEXT NOT NULL,
			user_id TEXT NOT NULL, object_path TEXT, content_type TEXT,
			size_bytes INTEGER NOT NULL DEFAULT 0, created_at DATETIME)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	return db
}

func postFeedback(t *testing.T, server *Server, userID string, body map[string]interface{}, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/feedback", bytes.NewReader(raw))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if userID != "" {
		req = req.WithContext(auth.WithUserID(req.Context(), userID))
	}
	w := httptest.NewRecorder()
	server.SubmitFeedback(w, req)
	return w
}

func TestSubmitFeedbackStoresTheReport(t *testing.T) {
	db := setupFeedbackDB(t)
	server := NewServer(zap.NewNop(), nil, nil, nil)

	w := postFeedback(t, server, "u1", map[string]interface{}{
		"category":    "bug",
		"description": "  The wheel screen is blank after a session.  ",
	}, map[string]string{"X-Platform": "ios", "X-App-Version": "1.4.2"})

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var stored models.Feedback
	if err := db.First(&stored).Error; err != nil {
		t.Fatalf("nothing stored: %v", err)
	}
	if stored.Description != "The wheel screen is blank after a session." {
		t.Errorf("description not trimmed: %q", stored.Description)
	}
	// Platform and version come from the headers the client already sends. Without them
	// every bug report costs a round trip before anyone can act on it.
	if stored.Platform == nil || *stored.Platform != "ios" {
		t.Errorf("platform = %v", stored.Platform)
	}
	if stored.AppVersion == nil || *stored.AppVersion != "1.4.2" {
		t.Errorf("app version = %v", stored.AppVersion)
	}
	if stored.UserID != "u1" {
		t.Errorf("user = %q", stored.UserID)
	}
}

func TestSubmitFeedbackValidates(t *testing.T) {
	setupFeedbackDB(t)
	server := NewServer(zap.NewNop(), nil, nil, nil)

	cases := []struct {
		name string
		body map[string]interface{}
	}{
		{"unknown category", map[string]interface{}{"category": "complaint", "description": "x"}},
		{"empty description", map[string]interface{}{"category": "bug", "description": "   "}},
		{"too many images", map[string]interface{}{"category": "bug", "description": "x",
			"images": []string{b64(tinyPNG), b64(tinyPNG), b64(tinyPNG), b64(tinyPNG)}}},
		{"not base64", map[string]interface{}{"category": "bug", "description": "x",
			"images": []string{"!!!not base64!!!"}}},
		{"empty image", map[string]interface{}{"category": "bug", "description": "x",
			"images": []string{""}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if w := postFeedback(t, server, "u1", tc.body, nil); w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}

	// Nothing rejected may have been written.
	var n int64
	database.DB.Model(&models.Feedback{}).Count(&n)
	if n != 0 {
		t.Errorf("%d rejected reports were stored anyway", n)
	}

	if w := postFeedback(t, server, "", map[string]interface{}{"category": "bug", "description": "x"}, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated = %d, want 401", w.Code)
	}
}

// The declared type is never trusted. A request must not be able to store an executable,
// or anything else, by calling it a PNG — the bytes decide.
func TestSubmitFeedbackSniffsTheRealFormat(t *testing.T) {
	setupFeedbackDB(t)
	server := NewServer(zap.NewNop(), nil, nil, nil)

	// A shell script wearing a PNG data-URL prefix.
	disguised := "data:image/png;base64," + b64([]byte("#!/bin/sh\nrm -rf /\n"))
	w := postFeedback(t, server, "u1", map[string]interface{}{
		"category": "bug", "description": "x", "images": []string{disguised},
	}, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("a disguised file should be rejected, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "only PNG") {
		t.Errorf("the error should name what is accepted: %s", w.Body.String())
	}

	// And real images pass, data-URL prefix or not.
	for _, img := range []string{b64(tinyPNG), "data:image/gif;base64," + b64(tinyGIF)} {
		w := postFeedback(t, server, "u1", map[string]interface{}{
			"category": "feedback", "description": "looks good", "images": []string{img},
		}, nil)
		if w.Code != http.StatusCreated {
			t.Errorf("a real image was rejected: %d %s", w.Code, w.Body.String())
		}
	}
}

// With no bucket configured the upload is discarded, and that must not cost the report:
// somebody took the trouble to describe a problem, and their words are the valuable part.
func TestSubmitFeedbackSurvivesStorageBeingDisabled(t *testing.T) {
	db := setupFeedbackDB(t)
	server := NewServer(zap.NewNop(), nil, nil, nil)

	w := postFeedback(t, server, "u1", map[string]interface{}{
		"category": "bug", "description": "crash on open", "images": []string{b64(tinyPNG)},
	}, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var reports int64
	db.Model(&models.Feedback{}).Count(&reports)
	if reports != 1 {
		t.Errorf("the report should be stored even with no bucket, got %d", reports)
	}
}

// Feedback is the user's words and its screenshots are whatever was on their screen. It
// has to leave with them — the whole point of today's erasure work.
func TestFeedbackIsErasedWithTheAccount(t *testing.T) {
	seedResetFixture(t)
	database.DB.Exec(`CREATE TABLE feedbacks (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, category TEXT NOT NULL,
		description TEXT NOT NULL, platform TEXT, app_version TEXT, os_version TEXT,
		device_model TEXT, context TEXT, created_at DATETIME)`)
	database.DB.Exec(`CREATE TABLE feedback_attachments (id TEXT PRIMARY KEY, feedback_id TEXT NOT NULL,
		user_id TEXT NOT NULL, object_path TEXT, content_type TEXT, size_bytes INTEGER NOT NULL DEFAULT 0, created_at DATETIME)`)
	database.DB.Exec(`INSERT INTO feedbacks (id, user_id, category, description) VALUES ('f1','u1','bug','it broke')`)
	database.DB.Exec(`INSERT INTO feedbacks (id, user_id, category, description) VALUES ('f2','u2','bug','theirs')`)
	database.DB.Exec(`INSERT INTO feedback_attachments (id, feedback_id, user_id, object_path) VALUES ('a1','f1','u1','feedback/u1/f1/1.png')`)

	callScopeOK(t, newResetServer(), "u1", "all")

	if n := countRows(t, "feedbacks", "u1"); n != 0 {
		t.Errorf("feedback survived the erasure: %d rows", n)
	}
	if n := countRows(t, "feedback_attachments", "u1"); n != 0 {
		t.Errorf("attachments survived the erasure: %d rows", n)
	}
	if n := countRows(t, "feedbacks", "u2"); n != 1 {
		t.Errorf("another user's feedback was erased: %d rows, want 1", n)
	}
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// Everything the client can tell us has to survive to the record. The four that get
// grouped by are columns; the rest travels as JSON. A report missing them is a round trip
// to the user before anyone can act, which usually means it is never acted on.
func TestSubmitFeedbackCapturesTheDiagnostics(t *testing.T) {
	db := setupFeedbackDB(t)
	server := NewServer(zap.NewNop(), nil, nil, nil)

	w := postFeedback(t, server, "u1", map[string]interface{}{
		"category":    "bug",
		"description": "blank screen",
		"context": map[string]string{
			"appVersion":  "1.4.2",
			"buildNumber": "482",
			"osVersion":   "iOS 17.2",
			"deviceModel": "iPhone15,3",
			"locale":      "pt-PT",
			"timezone":    "Europe/Lisbon",
			"screen":      "(tabs)/journey",
			"screenSize":  "393x852",
			"userAgent":   "Mozilla/5.0",
		},
	}, map[string]string{"X-Platform": "ios"})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var stored models.Feedback
	if err := db.First(&stored).Error; err != nil {
		t.Fatalf("nothing stored: %v", err)
	}
	for _, c := range []struct {
		name, want string
		got        *string
	}{
		{"platform", "ios", stored.Platform},
		{"appVersion", "1.4.2", stored.AppVersion},
		{"osVersion", "iOS 17.2", stored.OSVersion},
		{"deviceModel", "iPhone15,3", stored.DeviceModel},
	} {
		if c.got == nil || *c.got != c.want {
			t.Errorf("%s = %v, want %q", c.name, c.got, c.want)
		}
	}
	if stored.Context == nil {
		t.Fatal("the long-tail diagnostics were dropped")
	}
	for _, want := range []string{"Europe/Lisbon", "(tabs)/journey", "393x852", "pt-PT", "482"} {
		if !strings.Contains(*stored.Context, want) {
			t.Errorf("context is missing %q: %s", want, *stored.Context)
		}
	}

	// And they reach the team's email in a fixed order, blanks skipped.
	rendered := contextFields(stored)
	var labels []string
	for _, kv := range rendered {
		labels = append(labels, kv[0])
	}
	joined := strings.Join(labels, ",")
	if !strings.Contains(joined, "Platform") || !strings.Contains(joined, "Screen") || !strings.Contains(joined, "Timezone") {
		t.Errorf("email context block is incomplete: %v", labels)
	}
}

// A report with no context at all must still work: web and mobile can tell us different
// amounts, and the form is worth having on any of them.
func TestSubmitFeedbackWithoutContext(t *testing.T) {
	db := setupFeedbackDB(t)
	server := NewServer(zap.NewNop(), nil, nil, nil)

	w := postFeedback(t, server, "u1", map[string]interface{}{
		"category": "feature", "description": "let me export as PDF",
	}, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var stored models.Feedback
	db.First(&stored)
	if stored.Context != nil {
		t.Errorf("no diagnostics should mean no blob, got %q", *stored.Context)
	}
	// The user id is always there, so the block is never empty.
	if len(contextFields(stored)) == 0 {
		t.Error("the email block should still identify the reporter")
	}
}
