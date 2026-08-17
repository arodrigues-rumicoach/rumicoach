package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/cors"
)

// A preflight that answers 200 while omitting a requested header is the worst shape this
// failure can take: the browser blocks the real request, nothing reaches the server, and
// the log shows a successful OPTIONS. This walks the actual middleware for every header the
// client sends, so adding one to the client without adding it here fails here first.
func TestCORSAllowsEveryHeaderTheClientSends(t *testing.T) {
	handler := cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:8081"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   corsAllowedHeaders,
		AllowCredentials: true,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, header := range corsAllowedHeaders {
		t.Run(header, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, "/v1/feedback", nil)
			req.Header.Set("Origin", "http://localhost:8081")
			req.Header.Set("Access-Control-Request-Method", "POST")
			req.Header.Set("Access-Control-Request-Headers", header)

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			allowed := w.Header().Get("Access-Control-Allow-Headers")
			if !strings.Contains(strings.ToLower(allowed), strings.ToLower(header)) {
				t.Errorf("preflight for %q answered %d but did not allow it: %q",
					header, w.Code, allowed)
			}
		})
	}
}

// The header the app actually sends on every request, spelled as the browser sends it.
// X-App-Version is here by name because forgetting it broke every endpoint at once, not
// only the screen it was added for.
func TestCORSAllowsTheHeadersTheAppSendsOnEveryRequest(t *testing.T) {
	for _, header := range []string{"authorization", "content-type", "x-platform", "x-app-version"} {
		found := false
		for _, allowed := range corsAllowedHeaders {
			if strings.EqualFold(allowed, header) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("the client sends %q on every request but CORS does not allow it", header)
		}
	}
}
