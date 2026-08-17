# Coaching Sessions — Order & Cadence

Authoritative source: [`internal/services/growth/growth.go`](../internal/services/growth/growth.go)
(`deepSequence` + `ProposeSession`), the `SessionType` enum in
[`api/types.gen.go`](../api/types.gen.go), and `DisplayName` in
[`internal/services/chat/session/session.go`](../internal/services/chat/session/session.go).

---

## The journey at a glance

| # | Session | `SessionType` | Display name | When it becomes available |
|---|---------|---------------|--------------|---------------------------|
| 1 | Onboarding | `onboarding` | — | First ever session (forced until complete) |
| 2 | Movement | `session_movement` | Obstacles and Movement | **Immediately** after onboarding (no wait) |
| 3 | Values | `session_values` | Values | 7 days after the previous deep session |
| 4 | Energy | `session_energy` | Energy and Vitality | 7 days after the previous deep session |
| 5 | Decisions | `session_decisions` | Fears and Decisions | 7 days after the previous deep session |
| 6 | Beliefs | `session_beliefs` | Beliefs | 7 days after the previous deep session |
| 7 | Identity | `session_identity` | Identity | 7 days after the previous deep session |
| 8 | Acceptance | `session_acceptance` | Expectations and Acceptance | 7 days after the previous deep session |
| 9 | Priorities | `session_priorities` | Priorities | 7 days after the previous deep session |
| — | Check-in | `checkin` | session | **Daily** — the default/resting session on any day no deep session is due |

The ordered "deep" developmental track is `deepSequence`:
`onboarding → session_movement → session_values → session_energy → session_decisions → session_beliefs → session_identity → session_acceptance → session_priorities`.

---

## Cadence rules (`ProposeSession`)

What session the user is offered *right now* is computed as:

1. **Onboarding takes precedence.** If the user is still in an onboarding state
   (`ONBOARDING_INTRO`, `ONBOARDING_IDEAL_LIFE_VISION`, `ONBOARDING_WHEEL_OF_LIFE`,
   `ONBOARDING_METAPHOR`), they are routed back to **onboarding** until it is finished.
2. **Next uncompleted deep session.** Otherwise, find the first session in `deepSequence` the
   user has not yet done.
   - **Movement** is offered **immediately** after onboarding — no waiting period.
   - **Every subsequent deep session** unlocks **7 days (one week) after the last deep session
     was completed** (`time.Since(lastDeepTime) >= 7*24h`).
3. **Otherwise → daily check-in** (`checkin`). This is the resting state between deep sessions
   and the fallback when no deep session is due yet.

So the typical rhythm after onboarding is: **Movement right away, then one new deep session
per week**, with **daily check-ins** available in between.

---

## The daily Check-in is a gateway

`checkin` is the everyday session and the default. When a check-in starts, it first asks
whether a deep session is **planned for today** (`PlannedSessionForToday`, read from the
persisted `DailyGrowth` snapshot):

- If a deep session is planned **and not yet done today**, the check-in offers it and, if the
  user accepts, switches into that session (`start_planned_session`).
- If nothing is planned (no snapshot, the proposal is just the daily check-in, or the planned
  session was already completed today), it proceeds as a normal check-in.

Every `start_planned_session` handover (this one, and the onboarding intro handing over to
Vision) also splits the `communication_sessions` record: the row for the session being left
is ended there and then (transcript, duration, debit when billable, background recap/review),
and a fresh row typed as the planned session is opened. The journey gates count typed, ended
rows, so a single row spanning both halves would hide the second session from them.

---

## Onboarding internal phases (session 1 only)

Onboarding is a single session that advances through these states in order:

1. `ONBOARDING_INTRO` — greeting, privacy, memories screen, roadmap.
2. `ONBOARDING_IDEAL_LIFE_VISION` — visualize & explore the ideal life → `save_ideal_life_vision`.
3. `ONBOARDING_WHEEL_OF_LIFE` — score each life area one at a time → `update_wheel_of_life`.
4. `ONBOARDING_METAPHOR` — wheel metaphor + pick the priority area + why → `save_focus`.
5. `ONBOARDING_EMOTIONAL_CLOSING` — ask for the key insight → `save_memory` + `complete_current_task`.
6. `ONBOARDING_ENDING_SESSION` — final goodbye → `terminate_session`.

On completion the user's resting state becomes **`CHECKIN`**; the next deep session
(Movement, then weekly) is chosen by `ProposeSession` from session history.

> The Gemini connection is **restarted only around the Wheel of Life** (entering and leaving);
> all other onboarding phase changes happen in-session.

---

## Notes

- The 7-day cadence is measured **only from the deep sessions** in `deepSequence`
  (Movement, Values, Energy, Decisions, Beliefs; onboarding seeds the timer). **Daily check-ins
  are NOT counted** — they never affect or reset the weekly timer. Completing any one deep
  session resets the week-long countdown for the next.
- The `checkin_*` / `onboarding_*` values in `SetupTestDataRequestSessionType`
  ([`api/types.gen.go`](../api/types.gen.go)) are **test-setup fixtures**, not runtime session
  types — don't confuse them with the `SessionType` values above.
