# API Update — Goals Removed, Tasks Renamed to Actions (Frontend Hand-off)

**Breaking change.** Goals no longer exist as a resource. The user's commitment is now just:

1. **`focusArea`** on the user (`GET /me`) — the Wheel of Life area they committed to during
   onboarding, stored in the user's language.
2. **Actions** — the renamed tasks resource. An action stands on its own (no goal link).

All endpoints require the usual `Authorization: Bearer <token>` header.

---

## What was removed

| Old | Replacement |
|---|---|
| `GET /goals` | `GET /me` → `focusArea` (the area) + `GET /actions` (the plan) |
| `PUT /goals/{goalId}/tasks/{taskId}` | `PATCH /actions/{actionId}` or `PUT /daily-growth/actions/{actionId}` |
| `goals` field in `GET /daily-growth` | gone (was already deprecated) |
| `goalId` field on the task object | gone |
| The goal sentence ("goal" text) | dropped entirely |

## Renames

| Old | New |
|---|---|
| `GET/POST /tasks` | `GET/POST /actions` |
| `PATCH/DELETE /tasks/{taskId}` | `PATCH/DELETE /actions/{actionId}` |
| `PUT /daily-growth/tasks/{taskId}` | `PUT /daily-growth/actions/{actionId}` |
| `tasks` field in `GET /daily-growth` | `actions` |
| origin `"goal"` | origin `"plan"` |

## The `Action` object

```jsonc
{
  "id": "uuid",
  "title": "Go to the gym",
  "type": "one_time" | "recurring",
  "origin": "plan" | "manual" | "behavior",
  "days": [1, 3, 5],                  // recurring only; 1 = Monday … 7 = Sunday
  "date": "2026-07-01",               // one_time only; YYYY-MM-DD
  "status": "pending" | "completed" | "overdue"  // computed by the backend
}
```

- `origin`: `"plan"` = created by a coaching session as part of the action plan
  (`update_action_plan`/`save_actions` tools), `"manual"` = added by the user or via
  `add_action` in a check-in, `"behavior"` = projected from a behavior plan.
- `status` is **derived** (you don't send it): `completed` if done, `overdue` if a `one_time`
  date is in the past and not done, otherwise `pending`.
- For **recurring** actions, completion is **per day** — toggling done on the daily screen only
  affects today; it resets to undone the next day.

## Endpoints

- `GET /actions` — all the user's actions (past-dated done `one_time` actions excluded).
- `POST /actions` — create a manual action (`{title, type, days?, date?}`).
- `PATCH /actions/{actionId}` — partial update (`title`, `type`, `days`, `date`, `done`).
- `DELETE /actions/{actionId}` — delete.
- `GET /daily-growth` — returns `actions`: the flat list active **today** (recurring actions
  matching today's weekday + one-time actions due or overdue), each with today's done state.
- `PUT /daily-growth/actions/{actionId}` — toggle today's completion (`{"done": true|false}`);
  returns the refreshed daily-growth payload.

## In-session WebSocket payloads (names unchanged; one shape changed)

- `tasks_updated` — still fires whenever the board changes mid-session (refetch `/daily-growth`).
- `action_plan_update` — now `{"area": "<focusArea>", "actions": [...]}` (was `{"goal", "tasks"}`).
- `session_tasks_update` — unchanged shape (the in-session commitments board).

## Server-side migration (automatic, idempotent)

The `tasks` table is renamed to `actions`, the `daily_growths.tasks` JSONB column to `actions`,
and existing rows with origin `"goal"` are folded into `"plan"`. The legacy `goals` table is left
orphaned (still purged on account deletion) and is no longer read or written.
