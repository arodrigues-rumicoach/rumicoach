# rumicoach-backend

Go backend for **Rumi**, the 24/7 voice-first AI life coach. This is the core service of the Rumi ecosystem: every client (mobile/web app `rumicoach-app`, admin dashboard `rumicoach-backoffice`) talks to it. It has two halves:

1. **REST API** — chi router, generated from `api/openapi.yaml` via oapi-codegen. Auth, user profile, goals & tasks, memories, Wheel of Life, daily growth, streaks, recommendations, minutes balance & transaction history, admin endpoints.
2. **Real-time voice engine** — a WebSocket proxy (`/ws/chat`) between the client and the **Gemini Live API** (native audio), driving scripted coaching sessions (onboarding, check-ins, deep sessions).
3. **Companion messaging channel** — async chat with Rumi over **WhatsApp** (text + voice notes, proactive follow-ups) via the Meta Cloud API and Gemini `generateContent` (STT + LLM + TTS). Provider-abstracted so Telegram can follow. See [docs/whatsapp-channel.md](docs/whatsapp-channel.md).

It is deployed as a single container image (see `Dockerfile`) in a **two-plane regional topology**:

- **Auth plane** (EU only) — owns the identity database, issues access tokens signed with a Cloud KMS EC P-256 key, exposes `/.well-known/jwks.json`.
- **Data planes** (EU and US) — own regional user data, verify tokens against the auth plane's JWKS. Clients route requests to the region encoded in their access token.

## Tech stack

- **Go 1.25**, [chi](https://github.com/go-chi/chi) router, [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) v2.7.0
- **GORM** with PostgreSQL (deployed) and SQLite (local tooling / simulator)
- **gorilla/websocket** for the voice proxy; **Gemini Live API** via Vertex AI or Google AI (`GEMINI_PROVIDER`)
- **zap** structured logging
- Google Cloud: **KMS** (token signing), **Firebase Admin** (FCM push), Cloud SQL
- **Twilio** (SMS codes), **SendGrid / SMTP2GO** (email codes) — all providers mockable via env
- **WhatsApp Business Cloud API** (Meta Graph API) for the companion channel; **ffmpeg** (runtime image) transcodes Gemini TTS output into OGG/Opus voice notes

## Quickstart

### Prerequisites

- Go 1.25+
- PostgreSQL (default DSN `postgresql://postgres:postgres@localhost:5432/rumi`) — or point `DATABASE_URL` elsewhere
- A Google API key with Gemini access (`GOOGLE_API_KEY`) for voice sessions

### Run locally

```bash
# The server reads .env from the repo root (godotenv)
go run cmd/server/main.go        # listens on :8000 (PORT)
```

Swagger UI is served at `http://localhost:8000/swagger`, the raw spec at `/openapi.yaml`, health at `/health`.

### Build & test

```bash
# Do NOT use `go build ./...` — scratch files with duplicate main() live in the
# repo root (read_db.go, test_regex.go, ...). Build the real packages:
go build ./cmd/server/ ./internal/... ./api/ ./database/

go test ./internal/... ./api/                     # tests
go test ./internal/services/chat/ -run TestName   # single test
```

Known pre-existing failure: `TestSubmitSessionFeedback` (rating-bounds validation in `internal/handlers/chat.go`).

### Key environment variables

Defaults in `config/config.go` make local dev work with almost nothing set.

| Variable | Purpose |
|---|---|
| `PORT`, `ENVIRONMENT`, `LOG_LEVEL` | Server basics (default port 8000) |
| `DATABASE_URL` | Main (regional) Postgres DSN |
| `AUTH_DATABASE_URL` | Identity DB — auth plane only; falls back to `DATABASE_URL` in dev |
| `JWT_SECRET` / `SECRET_KEY` | HS256 dev signing secret (production requires `AUTH_KMS_KEY_ID` or a real secret) |
| `AUTH_KMS_KEY_ID`, `AUTH_JWKS_URL`, `ALLOW_LEGACY_HS256` | KMS token signing / JWKS verification |
| `EXPECTED_REGION`, `DATA_PLANE_EU_URL`, `DATA_PLANE_US_URL`, `INTERNAL_ALLOWED_SAS` | Regional data-residency plumbing |
| `GOOGLE_API_KEY`, `GEMINI_PROVIDER`, `GEMINI_LIVE_MODEL` | Gemini Live voice engine |
| `GEMINI_REST_PROVIDER` | Provider for the `generateContent` calls (companion chat, transcription, TTS, recap, QA review, recommendations), separately from the live socket. Unset = follow `GEMINI_PROVIDER`. Set to `vertex` to move that traffic to Vertex while the voice engine stays on AI Studio, which it must: `gemini-3.1-flash-live-preview` has no Vertex endpoint. Requires `GCP_PROJECT_ID` + `GCP_REGION` |
| `GEMINI_CONTEXT_WINDOW_TRIGGER_TOKENS`, `GEMINI_CONTEXT_WINDOW_TARGET_TOKENS` | Live-API context compression |
| `GOOGLE_CLIENT_IDS` | Accepted Google SSO client IDs |
| `EMAIL_PROVIDER`, `SMS_PROVIDER`, `PUSH_PROVIDER` | `sendgrid`/`smtp2go`, `twilio`, `fcm` — or `mock` |
| `TWILIO_*`, `SENDGRID_API_KEY`, `SMTP2GO_API_KEY`, `EMAIL_FROM_*` | Provider credentials |
| `CORS_ALLOWED_ORIGINS`, `WS_ALLOWED_ORIGINS`, `FRONTEND_URL` | Browser origin allowlists |
| `BALANCE_ENFORCED` | Refuse voice sessions for post-onboarding users holding less than a full minute (default `true`; the balance ledger records usage regardless, so turning it off only stops the refusal) |
| `GCP_PROJECT_ID`, `GCP_REGION`, `FIREBASE_PROJECT_ID` | GCP / FCM project selection |
| `WHATSAPP_ENABLED`, `WHATSAPP_*`, `COMPANION_*`, `GEMINI_COMPANION_MODEL`, `GEMINI_TRANSCRIBE_MODEL`, `GEMINI_TTS_MODEL` | WhatsApp companion channel — see [docs/whatsapp-channel.md](docs/whatsapp-channel.md) |

### Database proxy scripts

`db-proxy_qa.sh` / `db-proxy_prod.sh` open a Cloud SQL Auth Proxy tunnel to the deployed databases (see `db-proxy-common.sh`).

## Where to go next

- [ARCHITECTURE.md](docs/ARCHITECTURE.md) — request/voice-session flow, plane topology
- [COMPONENT_MAP.md](docs/COMPONENT_MAP.md) — what lives where, high-risk files
- [CLAUDE.md](CLAUDE.md) — deep behavioral invariants of the voice engine (required reading before touching `internal/services/chat/`)
- `docs/` — `sessions.md` (growth journey), `control-markers.md` (silent UI glyphs), `api-goals-tasks.md`, `google-sso-setup.md`, `whatsapp-channel.md` (companion messaging channel)
