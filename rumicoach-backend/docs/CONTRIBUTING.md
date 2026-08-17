# Contributing — rumicoach-backend

## Workflow

- Branch model: `development` → QA, `main` → production (Bitbucket Pipelines builds the Docker image and deploys to Cloud Run; auth is keyless via Workload Identity Federation).
- API changes are **spec-first**: edit `api/openapi.yaml`, then regenerate deterministically (oapi-codegen v2.7.0, no config file):

```bash
oapi-codegen -generate types -package api api/openapi.yaml > api/types.gen.go
oapi-codegen -generate chi-server -package api api/openapi.yaml > api/server.gen.go
```

- **Every new authenticated route must be added to the `selectiveAuth` allowlist** in `cmd/server/main.go`. The generated code does not enforce the OpenAPI `security` sections. (Exception: external webhooks like `/webhooks/whatsapp` are registered outside `/v1` and must self-authenticate, e.g. via provider HMAC signatures.)

## Build & test

```bash
# NOT `go build ./...` — root scratch files with duplicate main() break it
go build ./cmd/server/ ./internal/... ./api/ ./database/

go test ./internal/... ./api/
go test ./internal/services/chat/ -run TestName
./run_tests.sh                       # convenience wrapper
```

- Tests are standard Go `_test.go` files next to the code (`internal/handlers/`, `internal/services/chat/`, `api/routes/`, `pkg/auth/`).
- Known pre-existing failure: `TestSubmitSessionFeedback` — don't chase it unless you're working on feedback validation.
- LSP diagnostics often show stale `api.SessionType undefined` after codegen; trust `go build`.

## Coding standards observed here

- Layered flow: generated server interface → `internal/handlers` (thin) → `internal/services` / `internal/models` (logic). One handler file and one model file per resource.
- Config only through `config.AppConfig`; new settings get an env var with a dev-friendly default and a doc comment in `config/config.go`.
- Structured logging with zap; the voice engine logs per-window inbound-audio stats (`Client audio inbound window`) to triage client vs backend vs Gemini failures.
- External providers (email/SMS/push/Gemini) are behind provider-selection env vars with `mock` implementations — keep new integrations mockable.
- Errors returned via `internal/apierror` helpers with generated response types.

## Voice-engine rules (the hard-won ones)

These come from production incidents; see `CLAUDE.md` for the full list:

1. **Language**: prompts are English *templates* translated at runtime — never branch on English phrase-matching of AI/user text.
2. **Tools**: never gate a tool's declaration differently from its trigger; every mandatory-tool step needs a TURN_COMPLETE safety net; `schedule_notifications` must be declared on every connection.
3. **Turn lifecycle**: per-turn flags are captured/reset only in the TURN_COMPLETE handler — that's also where all behavioral safety nets live.
4. **Model-behavior fixes ship in pairs**: a prompt hardening **and** a server-side guard/corrective. Prompts alone don't reliably stick.
5. Data-bearing screens (Wheel of Life) are opened only by the tool that carries their data, never by marker/`show_screen`.
6. When editing files containing marker glyphs (◆●▣▤▁–█), don't use `perl -CSD` (double-encodes UTF-8); use byte-level replacement or a proper editor tool.
7. The persona in `session/persona.go` is duplicated in `onboarding/system_prompt.go` — mirror edits.

## Debugging sessions

- QA supplies transcript logs in `test_logs/` (`[timestamp] [ROLE]` format).
- Reset a QA user's fixtures with `POST /v1/admin/test-setup` (preserves `communication_sessions`).
- The customer simulator (`cmd/customer_simulator/`) drives scripted end-to-end voice sessions.
- Tunnel to deployed DBs with `./db-proxy_qa.sh` / `./db-proxy_prod.sh`.
