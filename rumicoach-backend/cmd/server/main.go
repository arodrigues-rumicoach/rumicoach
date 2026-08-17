package main

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"strings"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/api/routes"
	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/handlers"
	"github.com/rumi/rumi-be/internal/services"
	"github.com/rumi/rumi-be/internal/services/companion"
	"github.com/rumi/rumi-be/internal/services/messaging"
	"github.com/rumi/rumi-be/internal/services/messaging/telegram"
	"github.com/rumi/rumi-be/internal/services/messaging/whatsapp"
	"github.com/rumi/rumi-be/internal/services/notification"
	"github.com/rumi/rumi-be/internal/services/quote"
	"github.com/rumi/rumi-be/internal/services/storage"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
)

// corsAllowedHeaders is every request header a browser client may send. A header missing
// from this list is not a soft failure: the preflight still answers 200, the browser then
// refuses to send the real request, and the server log shows an OPTIONS followed by
// nothing at all. It reads as a mysterious CORS error rather than a one-line omission,
// which is exactly how X-App-Version broke every endpoint the moment the web client
// started sending it.
//
// Native apps ignore CORS entirely, so this only ever bites on web — and only after
// someone has already shipped the client change.
var corsAllowedHeaders = []string{
	"Origin", "Content-Type", "Accept", "Authorization",
	"X-Platform", "X-App-Version", "X-Timezone",
}

func main() {
	// 1. Load Config
	config.LoadConfig()

	// 2. Initialize Logger natively using Zap. Structured JSON on every deployed
	// plane — production and qa alike, Cloud Run indexes it — and the human-readable
	// development logger only on a laptop.
	var logger *zap.Logger
	if config.AppConfig.Environment != "development" {
		logger, _ = zap.NewProduction()
	} else {
		logger, _ = zap.NewDevelopment()
	}
	zap.ReplaceGlobals(logger) // Set global zap.L() and zap.S() for third-party libraries if any

	logger.Info("Starting Rumi Backend Service")

	// 3. Initialize Database and Auto-Migrate
	database.ConnectDB(logger)

	// The auth plane also connects the dedicated identity database (identities,
	// verification codes). Data planes never hold identity data.
	if config.AppConfig.IsAuthPlane() {
		database.ConnectAuthDB(logger)
	}

	// 3a. Initialize token signing/verification for this plane.
	// Auth plane (AUTH_KMS_KEY_ID set) signs via Cloud KMS; data planes only verify,
	// against the auth service's JWKS.
	if err := auth.InitSigner(context.Background()); err != nil {
		logger.Fatal("Failed to initialize token signer", zap.Error(err))
	}
	if config.AppConfig.AuthJWKSURL != "" {
		auth.InitJWKSCache(config.AppConfig.AuthJWKSURL)
	}
	logger.Info("Token trust configured",
		zap.Bool("auth_plane", config.AppConfig.IsAuthPlane()),
		zap.String("region_code", config.AppConfig.RegionCode),
		zap.Bool("legacy_hs256", config.AppConfig.AllowLegacyHS256))

	// 3b. Initialize Services
	// Object storage for feedback screenshots. The bucket is per deployment, so each data
	// plane writes into its own region and residency needs no branching in the code. With
	// none configured (local dev) uploads are discarded rather than failing the request.
	if store, err := storage.New(context.Background(), config.AppConfig.FeedbackBucket, logger); err != nil {
		logger.Error("failed to initialize object storage; uploads will be discarded", zap.Error(err))
	} else {
		storage.Global = store
	}

	notification.InitNotificationService(logger)
	// Delivery loop for end-of-session scheduled notifications: picks the
	// channel at send time (active messaging channel first, push as last
	// resort), so it runs regardless of which channels are enabled.
	notification.StartDispatcher(logger)
	services.InitVerificationService(logger)
	services.InitAdminService(logger)
	quote.InitQuoteService(logger)

	// 3c. Messaging channels (WhatsApp companion chat). Real client when
	// credentials are configured; mock (log-only) in development so the flow
	// can be exercised without a Meta app.
	if config.AppConfig.WhatsAppEnabled {
		if wa := whatsapp.NewClient(config.AppConfig.WhatsAppAccessToken, config.AppConfig.WhatsAppPhoneNumberID, logger); wa != nil {
			messaging.Register(wa)
			logger.Info("Configured WhatsApp messaging channel")
		} else if config.AppConfig.Environment == "development" {
			messaging.Register(messaging.NewMockChannel("whatsapp", logger))
			logger.Warn("WhatsApp credentials missing — using mock messaging channel")
		} else {
			logger.Error("WHATSAPP_ENABLED is set but WHATSAPP_ACCESS_TOKEN/WHATSAPP_PHONE_NUMBER_ID are missing; disabling WhatsApp")
			config.AppConfig.WhatsAppEnabled = false
		}
	}

	if config.AppConfig.TelegramEnabled {
		if tg := telegram.NewClient(config.AppConfig.TelegramBotToken, logger); tg != nil {
			messaging.Register(tg)
			logger.Info("Configured Telegram messaging channel")
		} else if config.AppConfig.Environment == "development" {
			messaging.Register(messaging.NewMockChannel("telegram", logger))
			logger.Warn("Telegram token missing — using mock messaging channel")
		} else {
			logger.Error("TELEGRAM_ENABLED is set but TELEGRAM_BOT_TOKEN is missing; disabling Telegram")
			config.AppConfig.TelegramEnabled = false
		}
	}

	// 4. Setup Chi Router
	router := chi.NewRouter()

	// 5. Basic Middleware
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	// 6. Setup CORS. Origins come from CORS_ALLOWED_ORIGINS (per-environment). With an
	// explicit allowlist we can safely send credentials; with no list configured
	// (local dev) we fall back to a wildcard, which requires credentials off since a
	// wildcard origin combined with credentials is rejected by browsers. Native app
	// clients are unaffected — CORS is a browser-only mechanism.
	corsOrigins := []string{}
	for _, o := range strings.Split(config.AppConfig.CORSAllowedOrigins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			corsOrigins = append(corsOrigins, o)
		}
	}
	corsAllowCredentials := true
	if len(corsOrigins) == 0 {
		corsOrigins = []string{"*"}
		corsAllowCredentials = false
	}
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   corsAllowedHeaders,
		ExposedHeaders:   []string{"Content-Length"},
		AllowCredentials: corsAllowCredentials,
	}))

	// 7. Basic Health Routes
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "ok"}`))
	})
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "ok"}`))
	})

	// Expose OpenAPI spec
	router.Get("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "api/openapi.yaml")
	})

	// Expose Swagger UI
	router.Get("/swagger", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Rumi API - Swagger UI</title>
  <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
  <style>
    html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin: 0; background: #fafafa; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
  <script>
    window.onload = function() {
      const ui = SwaggerUIBundle({
        url: "/openapi.yaml",
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        plugins: [
          SwaggerUIBundle.plugins.DownloadUrl
        ],
        layout: "StandaloneLayout"
      });
      window.ui = ui;
    };
  </script>
</body>
</html>`))
	})

	// JWKS: public signing keys, served only where a signer exists (auth plane / dev)
	if auth.Signer() != nil {
		router.Get("/.well-known/jwks.json", auth.JWKSHandler)
	}

	// Webhooks
	twilioWebhook := handlers.NewTwilioWebhookHandler(logger)
	var whatsappWebhook *handlers.WhatsAppWebhookHandler
	var telegramWebhook *handlers.TelegramWebhookHandler
	var companionSvc *companion.Service

	if config.AppConfig.WhatsAppEnabled || config.AppConfig.TelegramEnabled {
		companionSvc = companion.NewService(logger, notification.GetLocalizer())
		companion.StartDispatcher(companionSvc, logger)
		// Scheduled notifications land in the user's chat as real messages written by
		// the companion, not as push copy pasted into a conversation. Injected rather
		// than imported so the notification package keeps no dependency on companion.
		notification.ChatComposer = companionSvc.ComposeScheduledNotification
		logger.Info("Companion dispatcher registered")
	}

	if config.AppConfig.WhatsAppEnabled {
		whatsappWebhook = handlers.NewWhatsAppWebhookHandler(logger, companionSvc)
	}

	if config.AppConfig.TelegramEnabled {
		telegramWebhook = handlers.NewTelegramWebhookHandler(logger, companionSvc)
	}

	// 8. Register OpenAPI Handlers mapped to /v1
	server := handlers.NewServer(logger, whatsappWebhook, telegramWebhook, twilioWebhook)

	// Server-to-server API (profile provisioning, identity erasure)
	handlers.RegisterInternalRoutes(router, server, config.AppConfig.IsAuthPlane())

	// Expose test-data documentation at the root level /test-data without authentication
	router.Get("/test-data", server.GetTestDataDocs)

	// Helper to check if path matches a prefix exactly or as a directory segment
	hasPathPrefix := func(path, prefix string) bool {
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}

	// Selective auth middleware for private routes and /v1/admin
	selectiveAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Data planes never handle credentials or signup: those endpoints only exist
			// on auth.rumi.coach. The load balancer already routes this way; this is the
			// in-process backstop.
			if !config.AppConfig.IsAuthPlane() &&
				(hasPathPrefix(r.URL.Path, "/v1/auth") ||
					hasPathPrefix(r.URL.Path, "/v1/waitlist")) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusMisdirectedRequest)
				w.Write([]byte(`{"error": "Authentication is handled by the auth service (auth domain), not the regional API"}`))
				return
			}

			if hasPathPrefix(r.URL.Path, "/v1/admin") {
				auth.AuthMiddleware(auth.AdminMiddleware(next)).ServeHTTP(w, r)
			} else if hasPathPrefix(r.URL.Path, "/v1/auth/me") ||
				hasPathPrefix(r.URL.Path, "/v1/me") ||
				hasPathPrefix(r.URL.Path, "/v1/commitments") ||
				hasPathPrefix(r.URL.Path, "/v1/memories") ||
				hasPathPrefix(r.URL.Path, "/v1/feedback") ||
				hasPathPrefix(r.URL.Path, "/v1/streak") ||
				hasPathPrefix(r.URL.Path, "/v1/usage-calendar") ||
				hasPathPrefix(r.URL.Path, "/v1/wheel-of-life") ||
				hasPathPrefix(r.URL.Path, "/v1/eisenhower-matrix") ||
				hasPathPrefix(r.URL.Path, "/v1/journey") ||
				hasPathPrefix(r.URL.Path, "/v1/quotes/random") ||
				hasPathPrefix(r.URL.Path, "/v1/recommendations") ||
				hasPathPrefix(r.URL.Path, "/v1/chat/sessions") ||
				hasPathPrefix(r.URL.Path, "/v1/sessions") {
				// Execute auth middleware wrapper
				auth.AuthMiddleware(next).ServeHTTP(w, r)
			} else {
				next.ServeHTTP(w, r)
			}
		})
	}

	api.HandlerWithOptions(server, api.ChiServerOptions{
		BaseURL:     "/v1",
		BaseRouter:  router,
		Middlewares: []api.MiddlewareFunc{selectiveAuth},
	})

	// 9. Register Custom Chat Websocket mapped under /v1
	router.Route("/v1", func(r chi.Router) {
		routes.RegisterChatRoutes(r, logger)
	})

	// 10. Start Server
	logger.Info("Starting REST server", zap.String("port", config.AppConfig.Port))
	if err := http.ListenAndServe(":"+config.AppConfig.Port, router); err != nil {
		logger.Error("Server failed to start", zap.Error(err))
	}
}
