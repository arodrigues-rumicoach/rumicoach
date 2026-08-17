# Architecture — rumicoach-backend

## Structural pattern

A pragmatic **layered architecture** with a spec-first REST layer and a stateful real-time engine beside it:

```
api/openapi.yaml ──oapi-codegen──▶ api/types.gen.go + api/server.gen.go (chi server interface)
                                          │
cmd/server/main.go  (wiring, middleware, selectiveAuth)
                                          │
internal/handlers/   (one file per resource; implements the generated interface)
                                          │
internal/services/   (chat = voice engine, companion + messaging = WhatsApp channel,
                      growth, notification, quote, regional)
                                          │
internal/models/  +  database/db.go  (GORM models, AutoMigrate + custom backfills)
```

Cross-cutting packages: `config/` (env-driven config singleton), `pkg/auth/` (JWT issue/verify, KMS signer, JWKS, middleware), `internal/apierror/`.

## Deployment topology: auth plane + data planes

The same binary runs in two roles, selected by configuration:

- **Auth plane (EU)** — has `AUTH_KMS_KEY_ID` and `AUTH_DATABASE_URL`. Owns *identities* and *verification codes*, performs Google SSO and email/SMS code login, and signs access tokens with a Cloud KMS EC P-256 key. Serves `/.well-known/jwks.json`. Provisions/deletes users on data planes server-to-server (`DATA_PLANE_EU_URL` / `DATA_PLANE_US_URL`), authenticating with Google-signed ID tokens checked against `INTERNAL_ALLOWED_SAS` (`internal/handlers/internal_api.go`).
- **Data planes (EU, US)** — own all coaching data for users resident in that region. They verify access tokens against the auth plane's JWKS (`AUTH_JWKS_URL`) and enforce `EXPECTED_REGION` → the token's region claim must match. `ALLOW_LEGACY_HS256` keeps old HS256 tokens working during migration.

Clients (see `rumicoach-app`'s `src/api/backend-url.ts`) send auth calls to the auth plane and everything else to the regional host encoded in their token.

## Request flow (REST)

1. Request hits chi router built in `cmd/server/main.go` (CORS → zap logging → `selectiveAuth`).
2. **`selectiveAuth` is a hand-maintained path allowlist**: paths listed as public skip JWT validation; everything else requires a Bearer token verified by `pkg/auth`. ⚠️ A new `/v1/...` route is *unauthenticated* until its prefix is added there — the OpenAPI `security` sections are **not** enforced by generated code (handlers then 401 on missing userID, but don't rely on that).
3. The generated `api/server.gen.go` dispatches to `internal/handlers/*`.
4. Handlers call services / GORM models; responses are the generated `api` types.

## Voice session flow (WebSocket)

1. Client connects to `GET /ws/chat` (`api/routes/chat.go`; browser origins checked against `WS_ALLOWED_ORIGINS`, native clients send no Origin and are allowed). With `BALANCE_ENFORCED=true`, post-onboarding users holding less than a full minute are refused pre-upgrade with HTTP 402 `INSUFFICIENT_BALANCE`.
2. `ChatSession` (`internal/services/chat/live_api.go`) authenticates the user, terminates any previous live session for that user (per-user `activeSessions` registry), picks the session type via the growth journey (`internal/services/growth/`), and opens a second WebSocket to the **Gemini Live API**.
3. Audio and events are proxied both ways. The engine parses Gemini events, dispatches tool calls (`tools.go`), enforces the scripted session flow, injects correctives at TURN_COMPLETE, and handles hard restarts at session boundaries (system instruction + tools are fixed per Gemini connection).
4. Session types live in a **registry** (`internal/services/chat/session/`): `api.SessionType` → implementation (`onboarding`, `checkin`, `movement`, `values`, `energy`, `decisions`, `beliefs`). Each session owns its prompts, persona, transitions, restart boundaries, and tool list.
5. On disconnect, closing housekeeping (push-notification scheduling prompt) runs on the still-open Gemini connection; transcripts are persisted (`communication_sessions`), the session's elapsed seconds are debited from the user's minutes balance (onboarding sessions are free), and Gemini session-resumption handles are stored for a 10-second resume window.

See `CLAUDE.md` and `docs/control-markers.md` for the many behavioral invariants (turn lifecycle flags, marker glyphs, language rules) — they are the highest-risk logic in the repo.

## Companion channel flow (WhatsApp)

Async messaging with Rumi, separate from the live voice engine (see `docs/whatsapp-channel.md` for full detail):

1. Meta POSTs to `/webhooks/whatsapp` (registered outside `/v1`; per-region Meta app so each data plane receives its own traffic). The handler verifies the `X-Hub-Signature-256` HMAC, 200-acks immediately, and processes asynchronously.
2. Inbound messages are deduped by provider message id into `channel_messages`, then `companion.Service.HandleInbound` runs the turn: voice notes are transcribed by Gemini, a chat turn runs over Gemini `generateContent` with the user's memories/actions context and a `save_memory` tool, and the reply is sent as text or (per-user `replyMode`) a Gemini-TTS → ffmpeg → OGG/Opus voice note.
3. Accounts link via `POST /v1/me/channels/whatsapp/link` → one-time `RUMI-XXXXXX` code sent through a wa.me deep link; the webhook binds the sender's number (`channel_bindings`).
4. A dispatcher loop (per data plane, `FOR UPDATE SKIP LOCKED`) drains `channel_follow_ups` for proactive messages — free-form inside WhatsApp's 24h customer-service window, approved template outside — and enqueues daily nudges / post-session check-ins. With `COMPANION_EPHEMERAL_HISTORY` it also purges conversations idle beyond the configured window; long-term context persists as user memories, not raw transcript.

## Core domain

- **Growth journey** (`internal/services/growth/`): `deepSequence` = onboarding → movement → values → energy → decisions → beliefs. Movement follows onboarding immediately; later deep sessions come 7 days after the last deep session; otherwise the daily `checkin` (which can hand off to the planned session via `start_planned_session`).
- **Focus area & actions**: goals were removed — the user's commitment is `users.focus_area` plus `Action` rows (`origin` = `plan`|`manual`|`behavior`); per-day completion lives in the `DailyGrowth.Actions` JSONB snapshot, recomputed from `models.ActiveActionsForDate`. See `docs/api-goals-tasks.md`.
- **Minutes balance** (`internal/services/balance/`): bank-style ledger for the upcoming subscriptions/top-ups. `users.balance_seconds` + immutable `balance_transactions` rows (credits from purchases, debits from sessions), mutated atomically under a row lock; balance rides on `GET /v1/me`, history on `GET /v1/me/transactions`, manual credits via `POST /v1/admin/users/{id}/credits` until a payment provider is integrated. Session-start enforcement is gated by `BALANCE_ENFORCED` (default on): a billable session needs at least `balance.MinimumStartSeconds` (60s) left.
- **Memories, Wheel of Life, streaks, quotes, recommendations** each have a model + handler; recommendations also have an agent (`internal/services/chat/recommendation_agent.go`) using `GEMINI_RECOMMENDATION_MODEL`.

## External dependencies

| Dependency | Used for | Boundary |
|---|---|---|
| Gemini Live API (Vertex AI or Google AI) | Real-time voice coaching | `internal/services/chat/providers` |
| Gemini (batch models) | Recommendations, session review, companion chat/STT/TTS | `recommendation_agent.go`, `session_review.go`, `internal/services/companion/` |
| WhatsApp Business Cloud API (Meta Graph API) | Companion messaging channel | `internal/services/messaging/whatsapp/` |
| ffmpeg (runtime binary) | PCM → OGG/Opus voice-note transcoding | `companion/audio.go`, `Dockerfile` |
| Cloud KMS | Access-token signing (auth plane) | `pkg/auth/kms_signer.go` |
| Firebase Admin / FCM | Push notifications | `internal/services/notification/` |
| Twilio | SMS verification codes | `SMS_PROVIDER=twilio` |
| SendGrid / SMTP2GO | Email verification codes | `EMAIL_PROVIDER` |
| PostgreSQL (Cloud SQL) | Persistence | GORM, `database/db.go` |

All messaging providers have `mock` implementations selected by env var, so local dev needs no external accounts except Gemini for voice.

## Data & migrations

GORM `AutoMigrate` plus **idempotent custom pre-migrations** in `database/db.go` (e.g. the `tasks` → `actions` table rename and origin `goal` → `plan` fold). There is no migration-file system; schema changes ship as code.
