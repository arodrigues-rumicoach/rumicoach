# WhatsApp companion channel

Users chat with Rumi over WhatsApp (text + voice notes), and Rumi proactively
follows up (daily nudge, post-session check-in). This is the **companion**
pipeline — async STT + LLM (+ TTS) over Gemini `generateContent` — completely
separate from the live voice engine (`internal/services/chat/live_api.go`).
Telegram can be added later as a second `messaging.Channel` implementation.

## Architecture

```
Meta Cloud API ──webhook──▶ /webhooks/whatsapp (HMAC-verified, per-region plane)
                              │ dedupe by wamid (channel_messages unique index)
                              ▼
                    companion.Service.HandleInbound
                      ├─ voice note → DownloadMedia → Gemini transcription
                      ├─ system prompt (persona + memories/actions context)
                      ├─ Gemini chat + tool loop (save_memory)
                      └─ reply: text, or Gemini TTS → ffmpeg → OGG/Opus voice note
```

- **Packages**: `internal/services/messaging` (channel abstraction + WhatsApp
  Graph API client), `internal/services/companion` (conversation orchestrator,
  prompt, tools, audio, proactive dispatcher).
- **Models**: `channel_bindings` (user ↔ phone, link code, reply mode, 24h
  window tracking), `channel_messages` (per-message log, dedupe key, tokens),
  `channel_follow_ups` (proactive queue).
- **Multi-plane**: one Meta app + WhatsApp Business number **per region**
  (EU/US). Each data plane receives its own webhooks; message content never
  touches the auth plane.

## Account linking

`POST /v1/me/channels/whatsapp/link` (authenticated) → `{code, waLink,
expiresAt}`. The app shows the wa.me link; the user sends the pre-filled
`RUMI-XXXXXX` code (15-min TTL) and the webhook binds their number. Also:
`GET /v1/me/channels`, `PATCH /v1/me/channels/{id}` (`replyMode: text|audio`),
`DELETE /v1/me/channels/{id}`. Unknown numbers get one "get the app" reply per
24h.

## Proactive messages

A 60s dispatcher (started in `main.go`, replica-safe via `FOR UPDATE SKIP
LOCKED` + advisory lock) drains `channel_follow_ups`:

- `daily_nudge` — enqueued at `COMPANION_NUDGE_HOUR_UTC` for bindings idle that
  day, hinted with `growth.PlannedSessionForToday`.
- `post_session` — enqueued by the live engine's `Cleanup` 4h after a 5+ min
  session.

Inside WhatsApp's 24h customer-service window (last inbound < 24h) the nudge is
free-form Gemini text; outside it only the pre-approved template
`WHATSAPP_TEMPLATE_REENGAGE` is sent (register it per-region in Meta Business
Manager — approval takes time).

## Configuration

| Env | Meaning |
|---|---|
| `WHATSAPP_ENABLED` | Master switch (webhook routes + dispatcher). |
| `WHATSAPP_ACCESS_TOKEN` / `WHATSAPP_PHONE_NUMBER_ID` | Graph API credentials. Missing in development → mock channel (logs sends). |
| `WHATSAPP_BUSINESS_NUMBER` | Digits-only number for wa.me links. |
| `WHATSAPP_APP_SECRET` / `WHATSAPP_VERIFY_TOKEN` | Webhook HMAC secret / GET-handshake token. |
| `WHATSAPP_TEMPLATE_REENGAGE` | Template name (default `rumi_reengage`). |
| `COMPANION_DAILY_MESSAGE_CAP` | Inbound messages answered per user per day (default 50; no balance debit in v1). |
| `COMPANION_NUDGE_HOUR_UTC` | Daily nudge hour (default 17). |
| `COMPANION_EPHEMERAL_HISTORY` | When `true`, a conversation's stored messages are deleted after `COMPANION_EPHEMERAL_AFTER_HOURS` (default 6) with no activity in either direction. Long-term continuity is unaffected — it lives in `user_memories` via `save_memory`, not the raw log. Note: purging also resets the daily-cap count for that user. |
| `GEMINI_COMPANION_MODEL` / `GEMINI_TRANSCRIBE_MODEL` / `GEMINI_TTS_MODEL` | Model overrides (defaults in `companion/gemini.go`). |

Voice-note replies require **ffmpeg** (in the Docker image); without it the
reply degrades to text.

## Gotchas

- Gemini 3 function calls carry a `thoughtSignature` that MUST be echoed back
  when replaying the model turn in the tool loop, or the API 400s
  (`companion.Part` round-trips it).
- Meta redelivers webhooks: every insert into `channel_messages` goes through
  `ON CONFLICT (provider_message_id) DO NOTHING`; zero rows affected = drop.
- The webhook route lives outside `/v1` on purpose — `selectiveAuth` leaves
  unlisted `/v1/*` paths unauthenticated, and this endpoint self-authenticates
  via the HMAC signature instead.
- Simulate webhooks locally:
  `SIG=$(printf '%s' "$PAYLOAD" | openssl dgst -sha256 -hmac "$WHATSAPP_APP_SECRET" -hex | sed 's/^.*= //')` then
  `curl -X POST localhost:8000/webhooks/whatsapp -H "X-Hub-Signature-256: sha256=$SIG" -d "$PAYLOAD"`.
