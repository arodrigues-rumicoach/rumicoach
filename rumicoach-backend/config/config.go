package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Environment        string
	Port               string
	LogLevel           string
	DatabaseURL        string
	JWTSecret          string
	AccessTokenExpire  int
	RefreshTokenExpire int
	GoogleClientIDs    string
	AppleClientIDs     string
	TwilioAccountSID   string
	TwilioAuthToken    string
	TwilioFromNumber   string
	SendgridAPIKey     string
	SMTP2GOAPIKey      string
	EmailFromEmail     string
	EmailFromName      string
	// AuthEmailFromEmail is the sender for auth emails (verification codes),
	// isolated on its own subdomain so their deliverability is independent of
	// the general app mail stream.
	AuthEmailFromEmail string
	EmailProvider      string // "sendgrid", "smtp2go", "mock"
	SMSProvider        string // "twilio", "mock"
	FrontendURL        string
	// WSAllowedOrigins is a comma-separated allowlist of browser origins permitted to
	// open the chat WebSocket (in addition to FrontendURL). Native app clients send no
	// Origin header and are always allowed; in development an empty list allows all.
	WSAllowedOrigins string
	// CORSAllowedOrigins is a comma-separated allowlist of browser origins allowed by
	// CORS. When empty (local dev) all origins are allowed but without credentials,
	// since a wildcard origin with credentials is invalid. Native apps ignore CORS.
	CORSAllowedOrigins string
	// BalanceEnforced gates the WebSocket session-start balance check (post-onboarding
	// users with balance <= 0 are refused). The ledger and session debits always run
	// regardless. Defaults on: it was held off until users had a way to buy minutes,
	// and the membership and top-up products now exist. Set BALANCE_ENFORCED=false to
	// give the whole user base free sessions again — the only reason to.
	BalanceEnforced bool
	GCPProjectID    string
	// FirebaseProjectID overrides GCPProjectID for FCM. Needed in local dev, where
	// GCP_PROJECT_ID points at the Gemini API project but push notifications must
	// target the Firebase (qa-rumi-coach) project. Deployed envs leave it unset —
	// there the Cloud Run project is the Firebase project.
	FirebaseProjectID string
	GCPRegion         string
	GeminiLiveModel   string
	// GeminiSpeechLanguageHint, when true, passes the user's preferred language as a
	// speech languageCode hint on the Live API setup (experimental; native-audio models
	// normally auto-detect, but QA saw pt-PT speech transcribed as es/it/ko).
	GeminiSpeechLanguageHint bool
	// GeminiTurnSilenceMs is how long (ms) the user must fall silent before the Live API
	// treats their turn as finished (realtimeInputConfig.automaticActivityDetection). 0
	// leaves the provider default, which is tuned for quick back-and-forth and cuts across
	// anyone thinking mid-sentence (QA: Rumi talked over a user pausing to reflect — the
	// "take your time" cue itself interrupted him). Set to 0 to fall back to the provider
	// default if a model ever rejects the field.
	GeminiTurnSilenceMs int
	// GeminiReflectiveTurnSilenceMs is the same, for the phases built around long silent
	// reflection (the ideal-life visualization and the Wheel of Life), where a user
	// searching for words needs visibly more room than a normal conversational beat.
	GeminiReflectiveTurnSilenceMs int
	GoogleAPIKey                  string
	GeminiProvider                string // "vertex" or "google_ai" — the live voice WebSocket
	// GeminiRESTProvider is the same choice for the generateContent calls (companion
	// chat, voice-note transcription and synthesis, session recap, QA review,
	// recommendations), which can be pointed somewhere else than the live socket.
	//
	// They are separate because the providers do not serve the same models: the live
	// model this runs on, gemini-3.1-flash-live-preview, exists only on AI Studio and
	// has no Vertex endpoint in any region, so a single switch could never put the
	// REST traffic on Vertex without pointing the voice engine at a model that is not
	// there. Empty means "whatever GEMINI_PROVIDER says", so a deployment that never
	// sets it behaves exactly as it did when there was one switch.
	GeminiRESTProvider string // "vertex" or "google_ai"
	// Context window compression (contextWindowCompression in setup)
	GeminiContextWindowTriggerTokens int    // triggerTokens: when to fire compression ("Target context size" in Vertex UI)
	GeminiContextWindowTargetTokens  int    // slidingWindow.targetTokens: size to compress DOWN to ("Max context size" in Vertex UI)
	PushProvider                     string // "fcm", "mock"
	// SendAITranscriptPartial controls whether streaming "ai_transcript_partial" messages
	// are forwarded to the client. Disabled by default; enable with SEND_AI_TRANSCRIPT_PARTIAL=true.
	SendAITranscriptPartial bool

	// --- Regional data-residency / auth plane settings ---
	// AuthKMSKeyID is the Cloud KMS asymmetric signing key (EC_SIGN_P256_SHA256) used to
	// sign access tokens. Only the auth plane (EU deployment) has this set; when empty and
	// not in development, this service cannot issue tokens.
	AuthKMSKeyID string
	// AuthJWKSURL is where data planes fetch the auth service's public keys to verify tokens.
	AuthJWKSURL string
	// AuthDatabaseURL is the dedicated identity database (identities, verification
	// codes), used only by the auth plane. Empty on data planes; in development it
	// falls back to DatabaseURL so single-DB local setups keep working.
	AuthDatabaseURL string
	// RegionCode is this deployment's residency region ("eu" or "us"), derived from
	// EXPECTED_REGION (a GCP region like europe-west1 / us-central1). Empty means
	// region enforcement is disabled (local development).
	RegionCode string
	// DataPlaneEUURL / DataPlaneUSURL are the base URLs of the regional data planes,
	// used by the auth plane for server-to-server provisioning and deletion.
	DataPlaneEUURL string
	DataPlaneUSURL string
	// InternalAllowedSAs is a comma-separated list of service account emails allowed to
	// call /internal endpoints with a Google-signed ID token.
	InternalAllowedSAs string
	// AllowLegacyHS256 keeps accepting old HS256 tokens during the migration window.
	AllowLegacyHS256 bool

	// --- WhatsApp companion channel (Meta Business Cloud API) ---
	// Each regional data plane runs its own Meta app + WhatsApp Business number,
	// so these are per-deployment values and message traffic stays in-region.
	WhatsAppEnabled       bool
	WhatsAppAccessToken   string
	WhatsAppPhoneNumberID string
	// WhatsAppBusinessNumber is the human-facing number in digits-only form,
	// used to build wa.me deep links (e.g. "351912345678").
	WhatsAppBusinessNumber string
	// WhatsAppAppSecret signs webhook payloads (X-Hub-Signature-256).
	WhatsAppAppSecret string
	// WhatsAppVerifyToken answers Meta's webhook GET verification handshake.
	WhatsAppVerifyToken string
	// WhatsAppTemplateReengage is the approved template used for proactive
	// messages outside the 24h customer-service window.
	WhatsAppTemplateReengage string
	// --- Telegram companion channel ---
	TelegramEnabled       bool
	TelegramBotToken      string
	TelegramBotUsername   string
	TelegramWebhookSecret string

	// --- RevenueCat payments webhook ---
	// RevenueCatWebhookAuth is the exact Authorization header value RevenueCat is
	// configured to send with webhook deliveries (set per webhook in the RevenueCat
	// dashboard, e.g. "Bearer <random secret>"). Empty disables the webhook.
	RevenueCatWebhookAuth string
	// RevenueCatProductMinutes maps store product identifiers to the minutes each
	// purchase credits, as comma-separated "product_id:minutes" pairs (e.g.
	// "rumi_monthly_120:120,rumi_topup_60:60"). A purchase for a product missing
	// here is answered 500 so RevenueCat keeps retrying until the mapping ships.
	RevenueCatProductMinutes string

	// CompanionDailyMessageCap limits inbound messages answered per user per day.
	CompanionDailyMessageCap int
	// CompanionNudgeHourUTC is the UTC hour at which daily nudges are enqueued.
	CompanionNudgeHourUTC int
	// CompanionEphemeralHistory, when enabled, deletes a conversation's stored
	// messages after CompanionEphemeralAfterHours with no activity in either
	// direction. Long-term context still persists via user memories
	// (save_memory); only the raw message log is ephemeral.
	CompanionEphemeralHistory    bool
	CompanionEphemeralAfterHours int

	// FeedbackEmail receives general feedback and feature requests; FeedbackEmailBugs
	// receives bug reports. Split because they want different eyes and different urgency.
	FeedbackEmail     string
	FeedbackEmailBugs string
	// FeedbackBucket is the object-storage bucket for feedback screenshots. Set per
	// deployment so each data plane writes to a bucket in its own region — that is what
	// keeps residency correct without any region branching in the code. Empty disables
	// uploads (local development).
	FeedbackBucket string

	// --- Review / demo accounts (YC application, Apple App Store & Google Play review) ---
	// ReviewAccounts is a comma-separated list of email:code pairs (e.g.
	// "yc-review@rumi.coach:424242,apple-review@rumi.coach:171717"). These email
	// identities get their fixed code as the verification code instead of a random
	// one, and no email is actually sent — the mailbox need not exist. Everything
	// else about the code lifecycle (expiry, attempt caps, rate limits) stays the
	// same. Empty (the default) disables the feature.
	ReviewAccounts string
}

// ReviewAccountCode returns the fixed verification code configured for a review/demo
// account email (case-insensitive), or "" when the email is not a review account.
// Safe on a nil receiver (tests run without LoadConfig).
func (c *Config) ReviewAccountCode(email string) string {
	if c == nil || c.ReviewAccounts == "" || email == "" {
		return ""
	}
	for _, pair := range strings.Split(c.ReviewAccounts, ",") {
		accountEmail, code, ok := strings.Cut(strings.TrimSpace(pair), ":")
		if ok && code != "" && strings.EqualFold(accountEmail, email) {
			return code
		}
	}
	return ""
}

// RevenueCatProductSeconds returns how many balance seconds a store product
// credits, from the REVENUECAT_PRODUCT_MINUTES mapping. Safe on a nil receiver
// (tests run without LoadConfig).
func (c *Config) RevenueCatProductSeconds(productID string) (int64, bool) {
	if c == nil || c.RevenueCatProductMinutes == "" || productID == "" {
		return 0, false
	}
	for _, pair := range strings.Split(c.RevenueCatProductMinutes, ",") {
		pair = strings.TrimSpace(pair)
		// Split on the LAST colon, not the first. Google Play product ids contain a colon
		// themselves — RevenueCat sends "<subscription_id>:<base_plan_id>" for any Play
		// subscription set up after February 2023, so ours arrive as
		// "coach.rumi.app.membership:monthly". Cutting on the first colon would read the
		// id as "coach.rumi.app.membership" and the minutes as "monthly:150".
		sep := strings.LastIndex(pair, ":")
		if sep < 0 {
			continue
		}
		id, minutesStr := pair[:sep], pair[sep+1:]
		if id != productID {
			continue
		}
		minutes, err := strconv.Atoi(strings.TrimSpace(minutesStr))
		if err != nil || minutes <= 0 {
			return 0, false
		}
		return int64(minutes) * 60, true
	}
	return 0, false
}

var AppConfig *Config

func LoadConfig() {
	// Try loading from .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on environment variables.")
	}

	accessTokenExpire, _ := strconv.Atoi(getEnv("ACCESS_TOKEN_EXPIRE_MINUTES", "30"))
	refreshTokenExpire, _ := strconv.Atoi(getEnv("REFRESH_TOKEN_EXPIRE_DAYS", "30"))

	// One of exactly three values, each answering a different question:
	//
	//   "production"  — the planes taking real money. Store-sandbox purchases are
	//                   dropped, logs are structured JSON.
	//   "qa"          — deployed test planes. Hardened exactly like production —
	//                   every permissive fallback below (ephemeral dev signer, mock
	//                   messaging, open internal API) is keyed on "development" and
	//                   ignores this value — but sandbox purchases credit minutes,
	//                   because sandbox purchases are the only purchases QA sees.
	//   "development" — a laptop. Ephemeral signing key, mock channels when
	//                   credentials are missing, single-process regional fallbacks.
	//
	// QA used to run as "production" (there was no third value), which made the two
	// meanings of that word collide: the RevenueCat webhook dropped QA's sandbox
	// purchases as if they were sandbox receipts hitting the real production plane,
	// and testers bought subscriptions that credited nothing.
	environment := getEnv("ENVIRONMENT", "development")
	defaultFromEmail := "rumi@app.qa.rumi.coach"
	defaultAuthFromEmail := "verify@auth.qa.rumi.coach"
	if environment == "production" {
		defaultFromEmail = "rumi@app.rumi.coach"
		defaultAuthFromEmail = "verify@auth.rumi.coach"
	}

	AppConfig = &Config{
		Environment:              environment,
		Port:                     getEnv("PORT", "8000"),
		GeminiSpeechLanguageHint: getEnv("GEMINI_SPEECH_LANGUAGE_HINT", "false") == "true",
		// These have moved twice and the reasons pull in opposite directions, so both are
		// recorded. 1200 was the first attempt and cut people off mid-thought — QA heard
		// "Calma, deixa eu falar — estás a interromper-me" in the daily check-in. 2000/2600
		// fixed that but overshot the other way: waiting that long before answering makes
		// an ordinary exchange feel like the coach is slow rather than attentive. 1500/2000
		// sits between the two failures, still comfortably above the provider default.
		GeminiTurnSilenceMs:           mustAtoi(getEnv("GEMINI_TURN_SILENCE_MS", "1500")),
		GeminiReflectiveTurnSilenceMs: mustAtoi(getEnv("GEMINI_REFLECTIVE_TURN_SILENCE_MS", "2000")),
		LogLevel:                      getEnv("LOG_LEVEL", "INFO"),
		DatabaseURL:                   getEnv("DATABASE_URL", "postgresql://postgres:postgres@localhost:5432/rumi"),
		JWTSecret:                     getEnv("JWT_SECRET", getEnv("SECRET_KEY", "supersecretkey")),
		AccessTokenExpire:             accessTokenExpire,
		RefreshTokenExpire:            refreshTokenExpire,
		GoogleClientIDs:               getEnv("GOOGLE_CLIENT_IDS", getEnv("GOOGLE_CLIENT_ID", "")),
		// Sign in with Apple audiences: the iOS bundle IDs the identity token may be
		// minted for. Not secrets, and stable per app — the default covers both the
		// QA and production apps so no per-environment infra change is needed.
		AppleClientIDs:                   getEnv("APPLE_CLIENT_IDS", getEnv("APPLE_CLIENT_ID", "coach.rumi.app,coach.rumi.app.qa")),
		TwilioAccountSID:                 getEnv("TWILIO_ACCOUNT_SID", ""),
		TwilioAuthToken:                  getEnv("TWILIO_AUTH_TOKEN", ""),
		TwilioFromNumber:                 getEnv("TWILIO_FROM_NUMBER", ""),
		SendgridAPIKey:                   getEnv("SENDGRID_API_KEY", ""),
		SMTP2GOAPIKey:                    getEnv("SMTP2GO_API_KEY", ""),
		EmailFromEmail:                   getEnv("EMAIL_FROM_EMAIL", defaultFromEmail),
		EmailFromName:                    getEnv("EMAIL_FROM_NAME", "Rumi"),
		AuthEmailFromEmail:               getEnv("AUTH_EMAIL_FROM_EMAIL", defaultAuthFromEmail),
		EmailProvider:                    getEnv("EMAIL_PROVIDER", "smtp2go"),
		SMSProvider:                      getEnv("SMS_PROVIDER", "twilio"),
		FrontendURL:                      getEnv("FRONTEND_URL", "http://localhost:3000"),
		WSAllowedOrigins:                 getEnv("WS_ALLOWED_ORIGINS", ""),
		CORSAllowedOrigins:               getEnv("CORS_ALLOWED_ORIGINS", ""),
		BalanceEnforced:                  getEnvBool("BALANCE_ENFORCED", true),
		GCPProjectID:                     getEnv("GCP_PROJECT_ID", ""),
		FirebaseProjectID:                getEnv("FIREBASE_PROJECT_ID", ""),
		GCPRegion:                        getEnv("GCP_REGION", "europe-west1"),                         //europe-west1 or us-central1
		GeminiLiveModel:                  getEnv("GEMINI_LIVE_MODEL", "gemini-3.1-flash-live-preview"), //gemini-live-2.5-flash-native-audio or gemini-3.1-flash-live-preview
		GoogleAPIKey:                     getEnv("GOOGLE_API_KEY", ""),
		GeminiProvider:                   getEnv("GEMINI_PROVIDER", "google_ai"), // "vertex" or "google_ai"
		GeminiRESTProvider:               getEnv("GEMINI_REST_PROVIDER", ""),    // "" = follow GEMINI_PROVIDER
		GeminiContextWindowTriggerTokens: mustAtoi(getEnv("GEMINI_CONTEXT_WINDOW_TRIGGER_TOKENS", "128000")),
		GeminiContextWindowTargetTokens:  mustAtoi(getEnv("GEMINI_CONTEXT_WINDOW_TARGET_TOKENS", "102400")),
		PushProvider:                     getEnv("PUSH_PROVIDER", "fcm"),
		SendAITranscriptPartial:          getEnvBool("SEND_AI_TRANSCRIPT_PARTIAL", false),
		AuthKMSKeyID:                     getEnv("AUTH_KMS_KEY_ID", ""),
		AuthJWKSURL:                      getEnv("AUTH_JWKS_URL", ""),
		AuthDatabaseURL:                  getEnv("AUTH_DATABASE_URL", ""),
		RegionCode:                       regionCodeFromGCPRegion(getEnv("EXPECTED_REGION", "")),
		FeedbackEmail:                    getEnv("FEEDBACK_EMAIL", "feedback@rumi.coach"),
		FeedbackEmailBugs:                getEnv("FEEDBACK_EMAIL_BUGS", "support@rumi.coach"),
		FeedbackBucket:                   getEnv("FEEDBACK_BUCKET", ""),
		DataPlaneEUURL:                   getEnv("DATA_PLANE_EU_URL", ""),
		DataPlaneUSURL:                   getEnv("DATA_PLANE_US_URL", ""),
		InternalAllowedSAs:               getEnv("INTERNAL_ALLOWED_SAS", ""),
		AllowLegacyHS256:                 getEnvBool("ALLOW_LEGACY_HS256", true),
		WhatsAppEnabled:                  getEnvBool("WHATSAPP_ENABLED", false),
		WhatsAppAccessToken:              getEnv("WHATSAPP_ACCESS_TOKEN", ""),
		WhatsAppPhoneNumberID:            getEnv("WHATSAPP_PHONE_NUMBER_ID", ""),
		WhatsAppBusinessNumber:           getEnv("WHATSAPP_BUSINESS_NUMBER", ""),
		WhatsAppAppSecret:                getEnv("WHATSAPP_APP_SECRET", ""),
		WhatsAppVerifyToken:              getEnv("WHATSAPP_VERIFY_TOKEN", ""),
		WhatsAppTemplateReengage:         getEnv("WHATSAPP_TEMPLATE_REENGAGE", "rumi_reengage"),
		TelegramEnabled:                  getEnvBool("TELEGRAM_ENABLED", false),
		TelegramBotToken:                 getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramBotUsername:              getEnv("TELEGRAM_BOT_USERNAME", ""),
		TelegramWebhookSecret:            getEnv("TELEGRAM_WEBHOOK_SECRET", ""),
		RevenueCatWebhookAuth:            getEnv("REVENUECAT_WEBHOOK_AUTH", ""),
		RevenueCatProductMinutes:         getEnv("REVENUECAT_PRODUCT_MINUTES", ""),
		CompanionDailyMessageCap:         mustAtoi(getEnv("COMPANION_DAILY_MESSAGE_CAP", "50")),
		CompanionNudgeHourUTC:            mustAtoi(getEnv("COMPANION_NUDGE_HOUR_UTC", "17")),
		CompanionEphemeralHistory:        getEnvBool("COMPANION_EPHEMERAL_HISTORY", false),
		CompanionEphemeralAfterHours:     mustAtoi(getEnv("COMPANION_EPHEMERAL_AFTER_HOURS", "6")),
		ReviewAccounts:                   getEnv("REVIEW_ACCOUNTS", ""),
	}

	// Unset means the REST calls follow the live socket, which is what every
	// deployment did before the two could differ.
	if AppConfig.GeminiRESTProvider == "" {
		AppConfig.GeminiRESTProvider = AppConfig.GeminiProvider
	}

	// Vertex addresses a model by project and region; AI Studio does not. Checked here
	// rather than at the first generateContent call, because that call is a session
	// recap or a companion reply — a background job whose failure is a log line nobody
	// reads, hours after the deploy that caused it.
	if AppConfig.GeminiRESTProvider == "vertex" && (AppConfig.GCPProjectID == "" || AppConfig.GCPRegion == "") {
		log.Fatal("GEMINI_REST_PROVIDER=vertex requires GCP_PROJECT_ID and GCP_REGION")
	}

	// Any deployed plane — production or qa — refuses to run on the default secret.
	// Only "development" (a laptop) may, because there the ephemeral dev signer takes
	// over anyway. Keyed on != "development" rather than == "production" so the QA
	// planes did not silently lose this check when they got their own value.
	if AppConfig.Environment != "development" && AppConfig.JWTSecret == "supersecretkey" && AppConfig.AuthKMSKeyID == "" {
		log.Fatal("Refusing to start in a deployed environment without a real signing key: set AUTH_KMS_KEY_ID or a non-default JWT_SECRET/SECRET_KEY")
	}
}

// IsAuthPlane reports whether this deployment can issue tokens: it holds the KMS signing
// key, or is a development environment (where an ephemeral local key is used).
func (c *Config) IsAuthPlane() bool {
	return c.AuthKMSKeyID != "" || c.Environment == "development"
}

// DataPlaneURL returns the base URL of the data plane for a residency region code.
func (c *Config) DataPlaneURL(regionCode string) string {
	switch regionCode {
	case "us":
		return c.DataPlaneUSURL
	case "eu":
		return c.DataPlaneEUURL
	}
	return ""
}

// regionCodeFromGCPRegion maps a GCP region (europe-west1, us-central1) to the residency
// region code carried in token claims ("eu" / "us"). Unknown/empty regions disable enforcement.
func regionCodeFromGCPRegion(gcpRegion string) string {
	switch {
	case strings.HasPrefix(gcpRegion, "europe-"):
		return "eu"
	case strings.HasPrefix(gcpRegion, "us-"):
		return "us"
	case gcpRegion == "eu" || gcpRegion == "us":
		return gcpRegion
	}
	return ""
}

func getEnvBool(key string, defaultVal bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultVal
}

func getEnv(key, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultVal
}

func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
