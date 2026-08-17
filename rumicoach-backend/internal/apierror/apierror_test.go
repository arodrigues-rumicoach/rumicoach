package apierror

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	Write(rec, 400, CodeInvalidCode, "Invalid or expired code")

	if rec.Code != 400 {
		t.Fatalf("status: got %d, want 400", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type: got %q", ct)
	}

	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != "INVALID_CODE" {
		t.Fatalf("code: got %q, want INVALID_CODE", body.Code)
	}
	if body.Error != "Invalid or expired code" {
		t.Fatalf("error: got %q", body.Error)
	}
}
