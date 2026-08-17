package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	secret := "test-app-secret"
	body := []byte(`{"object":"whatsapp_business_account","entry":[]}`)

	if !VerifySignature(secret, body, sign(secret, body)) {
		t.Error("valid signature rejected")
	}
	if VerifySignature(secret, body, sign("wrong-secret", body)) {
		t.Error("signature from wrong secret accepted")
	}
	if VerifySignature(secret, []byte(`{"tampered":true}`), sign(secret, body)) {
		t.Error("signature over different body accepted")
	}
	if VerifySignature(secret, body, "") {
		t.Error("empty header accepted")
	}
	if VerifySignature(secret, body, "md5=abcdef") {
		t.Error("header without sha256= prefix accepted")
	}
	if VerifySignature(secret, body, "sha256=not-hex!!") {
		t.Error("non-hex signature accepted")
	}
	if VerifySignature("", body, sign("", body)) {
		t.Error("empty app secret must never verify")
	}
}
