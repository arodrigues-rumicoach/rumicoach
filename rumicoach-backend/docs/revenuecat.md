# RevenueCat payments → minutes balance

The mobile app sells subscriptions and consumable top-ups through the app
stores via RevenueCat. The backend never talks to the stores: RevenueCat is the
source of truth, and it pushes every purchase event to
`POST /v1/webhooks/revenuecat` (`internal/handlers/revenuecat_webhook.go`),
which converts purchases into minutes-bank credits through
`balance.CreditPurchase`.

## What credits, what doesn't

| Event type | Action |
| --- | --- |
| `INITIAL_PURCHASE`, `RENEWAL` | credit, ledger type `subscription` |
| `NON_RENEWING_PURCHASE` | credit, ledger type `top_up` (consumables) |
| `CANCELLATION` with `cancel_reason: CUSTOMER_SUPPORT` | refund — claws the purchase's grant back out of the balance (`balance.RefundPurchase`, ledger type `refund`; may go negative) |
| everything else (other `CANCELLATION` reasons, `EXPIRATION`, `BILLING_ISSUE`, `TRANSFER`, `PRODUCT_CHANGE`, `TEST`, …) | acked with 200 and logged — the minutes bank keeps already-granted minutes, so no balance action |

Trial and intro periods **do** credit (an `INITIAL_PURCHASE` with
`period_type: TRIAL` still grants the product's minutes — otherwise a trialist
with balance enforcement on could never start a session). `CUSTOMER_SUPPORT` is
the only cancel reason that means money went back (there is no dedicated REFUND
event type); every other reason is an auto-renew change and leaves granted
minutes alone.

## How many minutes a product grants

`REVENUECAT_PRODUCT_MINUTES` maps store product ids to minutes, e.g.

```
REVENUECAT_PRODUCT_MINUTES=rumi_monthly_120:120,rumi_topup_60:60
```

A purchase for a product missing from the mapping is answered **500** on
purpose: that's a config gap, and RevenueCat's retry backoff gives a fixed
mapping the chance to catch the event. Every other unprocessable condition
(unknown user, duplicate, ignored type, malformed body) is answered 200 so
RevenueCat stops retrying.

## Idempotency

RevenueCat redelivers webhooks until it sees a 2xx. Every credit is keyed by
the event id (`reference_id = "revenuecat:<event id>"` on
`balance_transactions`, unique index): a redelivery hits
`balance.ErrDuplicateReference` and is acked without a second credit. This is
the credit-side mirror of the unique `session_id` index that prevents
double-debiting a session.

## Auth

The route self-authenticates, like the other webhooks: the RevenueCat dashboard
lets you attach an Authorization header value to a webhook, and the handler
constant-time-compares the incoming header against `REVENUECAT_WEBHOOK_AUTH`
(the full header value, e.g. `Bearer <random secret>`). Unset disables the
endpoint (503).

## User resolution & regional planes

The app must call `Purchases.logIn(<backend user id>)` after signup so
RevenueCat's `app_user_id` is our user id. The handler tries `app_user_id`,
`original_app_user_id`, then every alias, skipping `$RCAnonymousID:*`
placeholders. Configure **one webhook per regional data plane** (RevenueCat
supports multiple webhooks) pointing at that plane's
`/v1/webhooks/revenuecat`: the plane that doesn't own the user 200-acks and
drops the event, the one that does credits it. There is no cross-plane
forwarding, same as WhatsApp.

## Sandbox

Sandbox purchases (`environment: SANDBOX`) are dropped only when `ENVIRONMENT`
is `production`. The QA planes run as `ENVIRONMENT=qa` — hardened exactly like
production (every permissive fallback in the backend is keyed on
`"development"`), but sandbox purchases credit real minutes there, because
sandbox purchases are the only purchases a QA plane ever sees. See the
ENVIRONMENT taxonomy in `config.LoadConfig`.

QA used to run as `production` (there was no third value), and this guard
dropped its test purchases: acked with `{"status":"sandbox"}`, never retried,
silently never credited. `app_environment` is set per plane in
`rumicoach-infra` (`"qa"` in `environments/qa-*/main.tf`, module default
`"production"`).

## Environment

| Var | Meaning |
| --- | --- |
| `REVENUECAT_WEBHOOK_AUTH` | exact Authorization header value configured on the RevenueCat webhook; empty disables the endpoint |
| `REVENUECAT_PRODUCT_MINUTES` | comma-separated `product_id:minutes` pairs |
| `ENVIRONMENT` | `production` drops sandbox purchases; `qa` (test planes) credits them |

`BALANCE_ENFORCED` (see `internal/services/balance/`) is on by default now that the
products exist. The webhook credits regardless of the flag, exactly like session
debits — the flag only controls whether a user under the minimum is refused. That
minimum is a full minute (`balance.MinimumStartSeconds`), not one second: a session
that cuts out seconds after the greeting is worse than a paywall.

## What is free

The introductory sessions — the onboarding intro and the Vision session it hands over
to — are free: never gated, never debited. Everything else (check-ins, deep sessions)
is billed out of the balance, whatever state the account is in.

Whether a session is free is `balance.FreeSessionAvailable`: the server-resolved
session type must be one of the two introductory ones, **and** the opening pair must
still be unfinished (`balance.OpeningPairUnfinished` — the profile details the intro
collects or the ideal-life vision the Vision session writes are still missing), with
`balance.FreeSessionCap` (10 substantive sessions) as an abuse backstop. Two earlier
rules are documented at those functions: reading `users.state` exempted anyone who hung
up mid-Vision forever, and counting session rows spent a free session on a five-second
dropped connection, paywalling users mid-onboarding.

The cap is deliberately **not** scoped by `models.JourneySessions`: erasing progress
must not hand the free tier back on demand.

The refusal is a `402` on the session WebSocket handshake, before the upgrade. A
browser or React Native `WebSocket` cannot read the status of a failed handshake, so
the app never sees that body: it runs the same rule client-side off `GET /me`
(`balanceSeconds` plus `inFirstJourney`) and shows a paywall instead. That check is a
courtesy, not a control — this one is the control.

Both vars are wired by Terraform in `rumicoach-infra` (`modules/rumi-infra`), for all four
planes. `REVENUECAT_PRODUCT_MINUTES` has a committed per-environment default that mirrors
`rumicoach-app/src/subscriptions/catalog.ts`; the annual plan maps to 1800 minutes, not
150, because `RENEWAL` fires once a year for an annual subscription and the whole year is
credited at once (minutes roll over and never expire, so that matches the advertised 150
a month).

Each subscription needs **two** entries, because the stores name the same plan differently.
Apple sends the product id as registered (`coach.rumi.app.membership.monthly`), while
RevenueCat sends a Google Play subscription as `<subscription_id>:<base_plan_id>` — ours
arrive as `coach.rumi.app.membership:monthly`. The top-ups are one-time products and carry
the same id on both stores, so one entry each is enough. Since a Play id contains a colon
of its own, `RevenueCatProductSeconds` splits each pair on its *last* colon.

`revenuecat_webhook_auth` has no default and must be set in each environment's
`terraform.tfvars` — a missing value fails `terraform plan` on purpose, because a deployed
plane with an empty value answers 503 to every delivery and would leave paying customers
at a zero balance and paywalled.

## Setting up a plane

Per data plane, because each has its own webhook (see above):

| Plane | Webhook URL |
| --- | --- |
| QA EU | `https://eu.qa.rumi.coach/v1/webhooks/revenuecat` |
| QA US | `https://us.qa.rumi.coach/v1/webhooks/revenuecat` |
| Prod EU | `https://eu.rumi.coach/v1/webhooks/revenuecat` |
| Prod US | `https://us.rumi.coach/v1/webhooks/revenuecat` |

1. Generate a random secret and set `revenuecat_webhook_auth = "Bearer <secret>"` in that
   environment's `terraform.tfvars` (gitignored). Use a different secret per plane so one
   leak does not authenticate against the others.
2. `terraform apply` the environment. This creates the Secret Manager entry, grants the
   Cloud Run SA access, and sets both env vars on the service.
3. In the RevenueCat dashboard, add a webhook per plane pointing at the URL above, with
   the Authorization header set to the exact same `Bearer <secret>` string.
4. Send a test event from the dashboard. It arrives as type `TEST`, which is acked with
   200 and logged without touching any balance — enough to confirm auth and routing.
5. Make a sandbox purchase against a QA build. QA planes run as `ENVIRONMENT=qa` and
   credit sandbox events (see the Sandbox section); production drops them, so this is
   safe to repeat.

Purchases only credit the user if the app called `Purchases.logIn(<backend user id>)`, so
verify `app_user_id` on the delivered event is a real user id and not `$RCAnonymousID:*`.
