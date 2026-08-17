# Brief: scoped data deletion + emailed export

Two changes to `/me/data`:

- **Part A/B** — `DELETE /me/data?scope=` so the user can erase one category
  instead of everything.
- **Part C** — `POST /me/data/export` to send the export by email, because on a
  phone the download ends in a share sheet.


Implementation brief for the backend. Written against the **working tree** as of
this document's creation (which already contains uncommitted changes to
`internal/handlers/user.go`), not against `HEAD`. Re-verify before starting if
that has moved.

The app already ships a **Manage Data** screen (in the separate `rumicoach-app`
repo, `app/(settings)/manageData.tsx`) with *Download my data* and *Delete my
data*. It is waiting on per-category deletes to fill in the rest. Nothing in this
brief requires touching that repo.

---

## Ground rules

Read these first — most of the work is *not* the deletes, it is keeping derived
state consistent afterwards.

1. **Never delete `balance_transactions`, never touch `users.balance_seconds`.**
   The minutes are paid for; erasing coaching data is not a refund. The ledger
   stands alone and is the accounting record. (Rationale already in
   `user.go` around the `DeleteCurrentUserData` transaction.)
2. **Never delete `integrations` in any `scope`.** A channel binding is closer to
   a registered device than to coaching data; revoking it silently disconnects
   the user's WhatsApp. Only account deletion removes it.
3. **Every scope runs in one transaction.** Multiple scopes in one request run in
   the same transaction.
4. **No partial success.** If a scope cannot complete its derived-state repair,
   roll the whole thing back.

---

## Part A — two erasure gaps, land these first

Independent of the feature. Worth shipping on their own.

### A1. Channel messages outlive account deletion

`models.ChannelMessage` holds WhatsApp/Telegram message bodies (`Body`,
`user_id`-scoped). It is deleted by **nothing**: not `DeleteCurrentUserData`, not
`DeleteCurrentUser` (`internal/handlers/user.go:353-484`), no `BeforeDelete` hook,
no `ON DELETE CASCADE`. Same for `models.ChannelFollowUp`.

A user who deletes their account leaves their message bodies in the database
permanently.

**Do:** add `channel_messages` and `channel_follow_ups` to **both** handlers.

### A2. Account deletion erases less than data deletion

`DeleteCurrentUserData` purges `communication_sessions`. `DeleteCurrentUser` does
not — verified across the full handler body. The stronger action leaves the
transcripts, recaps and AI notes standing.

**Do:** add `communication_sessions` to `DeleteCurrentUser`.

### A3. `twilio_logs` — out of scope, needs its own policy

`models.TwilioLog` (`internal/models/twilio_log.go:5-12`) has **no `user_id`
column**, is written only at `internal/handlers/twilio_webhook.go:41`, and is read
by nothing and deleted by nothing.

It **cannot** participate in a per-user delete. Do not try. It needs a time-based
retention job (drop rows older than N days). Flagged so it is not assumed covered
by "delete my chat".

---

## Part B — the `scope` parameter

Extend the existing endpoint rather than adding five new ones:

```
DELETE /me/data?scope=<comma-separated>
```

| rule | behaviour |
|---|---|
| `scope` omitted | `all` — today's callers keep working unchanged |
| valid values | `memories`, `chat`, `commitments`, `progress`, `all` |
| unknown value | `400`, name the offending value |
| multiple | `?scope=memories,chat` — one transaction |
| `all` with others | `all` wins, ignore the rest |
| success | `204`, as today |

Update `api/openapi.yaml` and regenerate (`api/server.gen.go`).

---

## Trap 1: badges are never revoked

`badge.EvaluateAndAward` (`internal/services/badge/badge.go:38`) only inserts —
the loop at `badge.go:141-152` never deletes. But every criterion re-reads live
tables. So deleting any category leaves the badge grid asserting achievements
whose counters now read zero: **badge lit, counter at 0, on the same screen.**

The existing full reset dodges this by also deleting `user_badges`. Each scope
below therefore names which badge rows it must drop. All 15 badges are accounted
for:

| badge | criterion reads | dropped by |
|---|---|---|
| `tenInsights` | `user_memories` where `category='insight'` (`badge.go:63`) | `memories` |
| `firstCommitment`, `twentyFiveCommitments` | `commitmentsKept` = `commitments` where `done` **+** `behavior_check_ins` where `kept` (`badge.go:58-60`) | `commitments` |
| `firstSession`, `twentySessions`, `hundredSessions` | `doneSessions` ← `communication_sessions` (`badge.go:180-186`) | `progress` |
| `firstDeepSession`, `allThemesExplored` | `communication_sessions` (`badge.go:66-73`) | `progress` |
| `threeDayStreak`, `sevenDayStreak`, `thirtyDayStreak` | `longestStreak` ← `communication_sessions` (`badge.go:180`) | `progress` |
| `alwaysWithYou` | `integrations` where active (`badge.go:76`) | none — integrations survive every scope |
| `visionSet`, `wheelRemapped`, `areaImproved` | `users.ideal_life_vision`, `wheel_of_life_exercises` (`badge.go:81,120`) | `all` only |

---

## Trap 2: the chat rate limits are derived from the message log

Two guards `COUNT` rows in `channel_messages`:

| guard | today | line |
|---|---|---|
| daily message cap | inbound since UTC midnight | `companion/service.go:389-391` |
| proactive quiet period (6h) | outbound in the window | `companion/dispatcher.go:99-102` |

Deleting the messages resets both. A user who clears their chat for privacy can
then be messaged **immediately** — the exact opposite of what they asked for. It
is also an abuse path: delete to reset your daily cap.

**Do: move the state onto `integrations`, where it survives a purge.** The model
already carries `LastInboundAt` for the provider's 24h customer-service window,
so this follows an established pattern:

```go
// mirrors LastInboundAt; gates proactive sends without reading the message log
LastOutboundAt *time.Time `gorm:"type:timestamp with time zone"`
// daily cap state, reset when DailyInboundDate rolls over
DailyInboundCount int        `gorm:"not null;default:0"`
DailyInboundDate  *time.Time `gorm:"type:date"`
```

Then:

- `mayReachOutNow` reads `LastOutboundAt` instead of counting (`dispatcher.go:99`)
- `dailyCapState` reads `DailyInboundCount`, resetting when `DailyInboundDate`
  is not today (`service.go:389`)
- the send and receive paths write these columns

**This repair is worth doing whether or not `scope=chat` ships:** both guards are
`COUNT`s over a monotonically growing table, executed on every message.

---

## The five scopes

### `memories`

| delete | filter |
|---|---|
| `user_memories` | `user_id = ?` |
| `user_badges` | `user_id = ? AND badge_type = 'tenInsights'` |

Survives: everything else.

Accepted consequences — these *are* the user's intent, do not compensate for
them: profile `insightsDiscovered` → 0 (`handlers/profile.go:73`); the live
coach's system prompt loses its memory block (`chat/live_api.go:786-789`); the
companion loses foundational memories (`companion/journey_context.go:322-327`);
future session summaries have no `key_insight` (`chat/session_summary.go:69-73`).

The cleanest scope: nothing breaks, Rumi simply forgets.

### `chat`

| delete | filter |
|---|---|
| `channel_messages` | `user_id = ?` |
| `channel_follow_ups` | `user_id = ?` |

Survives: the `integrations` row — the user stays connected. This scope clears the
conversation, it does not disconnect the channel.

Requires Trap 2 to be fixed first, or it ships the spam regression.

Deleting `channel_follow_ups` also drops queued-but-unsent proactive messages
(`dispatcher.go:56-63`) — correct — and the same-day nudge dedupe
(`dispatcher.go:185-188`), which the `DailyInboundDate` state above replaces.

### `commitments`

| delete | filter | why |
|---|---|---|
| `commitment_completions` | `user_id = ?` | orphans otherwise; `journey_context.go:219` joins them in memory |
| `commitments` | `user_id = ?` | |
| `behavior_plans` | `user_id = ?` | see below |
| `behavior_check_ins` | `user_id = ?` | half of `commitmentsKept` |
| `user_badges` | `badge_type IN ('firstCommitment','twentyFiveCommitments')` | |

**`behavior_plans` must go too.** `BehaviorPlan.TaskID`
(`internal/models/behavior_plan.go:63`) points at the recurring Commitment that
carries daily execution. Delete the commitment and leave the plan, and `TaskID`
dangles: the next sync (`chat/behavior_plans.go:202`) updates zero rows **in
silence** and the habit never re-projects. If you would rather keep the plans,
the alternative is `UPDATE behavior_plans SET task_id = NULL` plus re-projection
on next sync — deleting is simpler and matches the user's ask.

**`behavior_check_ins` cannot be left out.** `commitmentsKept` is
`commitments WHERE done` **plus** `behavior_check_ins WHERE kept`. Dropping the
two badges while half their input survives re-awards them on the next
`EvaluateAndAward`.

Survives: sessions, memories, vision, wheel, integrations, balance.

### `progress`

Named for what the user loses, not for the table. `communication_sessions` is the
**only** source of the streak — current, best, total days and the calendar
(`internal/handlers/streak.go:59-63`) — so it cannot be deleted without deleting
the streak.

| delete | filter |
|---|---|
| `communication_sessions` | `user_id = ?` |
| `daily_growth` | `user_id = ?` |
| `user_app_opens` | `user_id = ?` |
| `user_badges` | the eight session/streak badges above |

Survives: memories, commitments, vision, wheel, integrations, balance.

Accepted consequences: streak/best/total days/calendar → 0
(`streak.go:59,183`); profile totals → 0 (`profile.go:51`); the growth journey
re-proposes deep sessions already done (`services/growth/growth.go:101`) — this is
the intent of "start over"; the check-in prompt reverts to its first-return tone
(`chat/session/checkin/prompts.go:39`); the companion loses "in your last
session…" (`journey_context.go:66,147`). `balance_transactions.session_id` values
dangle — already documented as harmless: no FK, fresh UUID per session, the
unique index still prevents a double debit.

### `all`

Today's `DeleteCurrentUserData` table list **plus** `channel_messages` and
`channel_follow_ups` (Part A1). Everything else unchanged, including the two
deliberate survivors from the ground rules.

---

## Part C — email the export instead of downloading it

On a phone the current export ends in a share sheet, which is an awkward place to
keep a file. Emailing it is the natural delivery, and it is what every large
provider does for a GDPR export.

### Do NOT add a flag to `GET /me/data`

`GET` has to stay safe and idempotent. A browser prefetch, a link scanner in a
corporate mail gateway, or the axios retry interceptor already in the app would
each put the user's full personal export in their inbox — possibly more than
once, unprompted. Use a separate verb:

```
POST /me/data/export     →  202 Accepted
```

`GET /me/data` stays exactly as it is, for the direct download the web app uses.

| case | response |
|---|---|
| queued | `202`, empty body |
| user has no email on file | `409` + `{"error":"no email address"}` |
| rate limited | `429` (see below) |
| unauthenticated | `401` |

The app already gates on `user.email` client-side and routes to Manage Account
when it is missing, but the server must not rely on that.

### Rate limit it

An export is expensive to build and lands in an inbox. One per user per hour is
plenty; return `429` beyond that. Without a limit this is a spam amplifier —
anyone with a stolen session can flood the account owner's inbox.

### Send a link, not an attachment

**Recommendation:** email a **signed, expiring URL** (24h is typical) rather than
attaching the JSON.

The export contains everything the user ever told Rumi. Email is not
end-to-end encrypted, it is retained in the mailbox indefinitely, and it is
frequently synced to third-party clients. An expiring link keeps the sensitive
payload on infrastructure you control and limits the blast radius of a
compromised or stale mailbox — which matters here precisely because the address
may be the one the user is about to stop using.

If you attach the JSON instead, that is a defensible product decision, but it
should be a deliberate one rather than the default.

### Localise the email

Send in `users.preferred_language`. All 20 locales already carry the app-side
strings for this flow; the email template should match rather than defaulting to
English.

---

## Acceptance criteria

Extend `internal/handlers/user_data_reset_test.go`. Per scope, assert the deletes
**and** that the survivors survive.

- **`memories`** → `user_memories` empty; `communication_sessions` and
  `commitments` untouched; `tenInsights` row gone; other badges intact.
- **`chat`** → `channel_messages` and `channel_follow_ups` empty; the
  `integrations` row still `active`; **`mayReachOutNow` still returns `false`
  immediately after the purge** — this is the regression the scope exists to
  avoid, test it explicitly; the daily cap still counts.
- **`commitments`** → no `commitment_completions` rows left keyed to deleted
  commitments; no `behavior_plans` with a dangling `task_id`; both commitment
  badges gone; `EvaluateAndAward` run afterwards does **not** re-award them.
- **`progress`** → `GET /streak` returns 0 current and 0 best; growth proposes the
  first deep session again; memories and commitments untouched.
- **`all`** → matches today's behaviour plus the two chat tables; `balance_seconds`
  unchanged and `balance_transactions` intact; `integrations` intact.
- **`DELETE /me`** → `communication_sessions`, `channel_messages` and
  `channel_follow_ups` all gone (A1, A2).
- **parameter** → omitted behaves as `all`; unknown value returns `400`;
  `memories,chat` performs both.
- **`POST /me/data/export`** → `202` and an email arrives; a user with no email
  gets `409` and **no** mail is sent; a second call within the hour gets `429`;
  `GET /me/data` still returns the JSON and sends nothing.

## Files likely touched

- `internal/handlers/user.go` — both delete handlers, scope parsing
- `internal/models/integration.go` — the three new columns (Trap 2)
- `internal/services/companion/service.go`, `dispatcher.go` — read the new columns
- `api/openapi.yaml` + regenerate `api/server.gen.go`
- `internal/handlers/user_data_reset_test.go` — coverage
