package routes

import (
	"net/http/httptest"
	"testing"

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/internal/models"
)

func TestWSToken(t *testing.T) {
	t.Run("from Sec-WebSocket-Protocol header", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/v1/ws/chat", nil)
		r.Header.Set("Sec-WebSocket-Protocol", "rumi-auth, the.jwt.token")
		if got := wsToken(r); got != "the.jwt.token" {
			t.Fatalf("got %q, want %q", got, "the.jwt.token")
		}
	})

	t.Run("query-param fallback for legacy clients", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/v1/ws/chat?token=legacy.jwt", nil)
		if got := wsToken(r); got != "legacy.jwt" {
			t.Fatalf("got %q, want %q", got, "legacy.jwt")
		}
	})

	t.Run("header wins over query", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/v1/ws/chat?token=legacy.jwt", nil)
		r.Header.Set("Sec-WebSocket-Protocol", "rumi-auth, header.jwt")
		if got := wsToken(r); got != "header.jwt" {
			t.Fatalf("got %q, want %q", got, "header.jwt")
		}
	})

	t.Run("missing everywhere", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/v1/ws/chat", nil)
		if got := wsToken(r); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
}

func TestCheckOrigin(t *testing.T) {
	orig := config.AppConfig
	t.Cleanup(func() { config.AppConfig = orig })
	config.AppConfig = &config.Config{}

	t.Run("native client with no Origin is allowed", func(t *testing.T) {
		config.AppConfig.WSAllowedOrigins = "https://app.rumi.coach"
		r := httptest.NewRequest("GET", "/v1/ws/chat", nil)
		if !checkOrigin(r) {
			t.Fatal("no-Origin (native) request should be allowed")
		}
	})

	t.Run("enforcement off when allowlist empty", func(t *testing.T) {
		config.AppConfig.WSAllowedOrigins = ""
		r := httptest.NewRequest("GET", "/v1/ws/chat", nil)
		r.Header.Set("Origin", "https://evil.example")
		if !checkOrigin(r) {
			t.Fatal("with no allowlist configured, all origins should be allowed (opt-in enforcement)")
		}
	})

	t.Run("allowed origin passes, others rejected", func(t *testing.T) {
		config.AppConfig.WSAllowedOrigins = "https://app.qa.rumi.coach"
		config.AppConfig.FrontendURL = "http://localhost:3000"

		ok := httptest.NewRequest("GET", "/v1/ws/chat", nil)
		ok.Header.Set("Origin", "https://app.qa.rumi.coach")
		if !checkOrigin(ok) {
			t.Fatal("configured origin should be allowed")
		}

		fe := httptest.NewRequest("GET", "/v1/ws/chat", nil)
		fe.Header.Set("Origin", "http://localhost:3000")
		if !checkOrigin(fe) {
			t.Fatal("FrontendURL origin should be allowed")
		}

		bad := httptest.NewRequest("GET", "/v1/ws/chat", nil)
		bad.Header.Set("Origin", "https://evil.example")
		if checkOrigin(bad) {
			t.Fatal("unlisted origin should be rejected")
		}
	})
}

// The gate needs a full minute, not a single second: a session that dies moments
// after the greeting is worse than a paywall.
//
// Whether the session is free is decided by balance.FreeSessionAvailable — read from
// the artifacts the introductory sessions produce, covered by that package's tests —
// and handed in. Two earlier versions of that decision lived closer to here: one read
// users.state, which exempted every account that had not finished Vision cleanly, and
// one counted session rows, which spent a free session on a connection the user
// dropped after five seconds.
func TestTooLowToStart(t *testing.T) {
	cases := []struct {
		name string
		user models.User
		free bool
		want bool
	}{
		{"billable, plenty", models.User{BalanceSeconds: 600}, false, false},
		{"billable, exactly one minute", models.User{BalanceSeconds: 60}, false, false},
		{"billable, a second under", models.User{BalanceSeconds: 59}, false, true},
		{"billable, empty", models.User{BalanceSeconds: 0}, false, true},
		{"billable, overdrawn", models.User{BalanceSeconds: -120}, false, true},

		// Inside the allowance the balance is not consulted at all: a brand-new account
		// has nothing in it and still has to be able to start onboarding.
		{"free session, empty balance", models.User{BalanceSeconds: 0}, true, false},
		{"free session, overdrawn", models.User{BalanceSeconds: -30}, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := tooLowToStart(&c.user, c.free); got != c.want {
				t.Errorf("tooLowToStart(balance=%d, free=%v) = %v, want %v",
					c.user.BalanceSeconds, c.free, got, c.want)
			}
		})
	}
}
