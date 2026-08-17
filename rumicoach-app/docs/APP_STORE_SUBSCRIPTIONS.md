# App Store subscriptions and pricing

How the Rumi membership is configured in App Store Connect, and where the numbers come from.

Prices and product names are **not** decided here. The commercial source of truth is the
website:

- `rumicoach-website/src/utils/pricing.ts` — price per currency zone
- `rumicoach-website/src/website/translations/*.ts` (`pricing` block) — product names, per locale

Those values are mirrored into [`src/subscriptions/catalog.ts`](../src/subscriptions/catalog.ts)
and [`src/subscriptions/appStoreLocalizations.ts`](../src/subscriptions/appStoreLocalizations.ts),
which is what you paste from when filling in App Store Connect. If the website changes a
price, change it in both places — a price that differs between web and iOS is a support ticket.

## The model

One membership, no tiers. Every feature is unlocked; what you buy is time.

- **150 guided voice minutes per month**, included with the membership
- **Unused minutes roll over** and never expire
- **Top-ups** add minutes when someone runs out, as one-time purchases
- **Annual is exactly 10× monthly**, which is how "2 months free" stays true in every currency

The free first session is granted server-side, not by StoreKit, so there is **no introductory
offer** on either plan. Do not add one in App Store Connect — it would stack with the free
session and give away more than intended.

## Products

Two auto-renewable subscriptions in one group, plus three consumables.

All five exist on the record (app `6799373415`) as of 2026-08-09.

| Product | Type | Product ID (prod) | Apple ID | Duration / Minutes |
|---|---|---|---|---|
| Membership Monthly | Auto-renewable | `coach.rumi.app.membership.monthly` | 6799446065 | 1 Month |
| Membership Annual | Auto-renewable | `coach.rumi.app.membership.annual` | 6799446593 | 1 Year |
| Quick Boost | Consumable | `coach.rumi.app.topup.quick` | 6799446981 | 60 min |
| Deep Dive | Consumable | `coach.rumi.app.topup.deep` | 6799447172 | 120 min |
| Power User | Consumable | `coach.rumi.app.topup.power` | 6799447540 | 240 min |

Each has pricing set (Germany as base territory), availability in all 175 territories,
and an `en-US` localization. The remaining 19 locales per product are not entered.

The QA app record (`coach.rumi.app.qa`) needs its own products, with `.qa.` inserted —
`coach.rumi.app.qa.membership.monthly`, and so on. This is not optional: Apple scopes
product IDs to the **account**, not the app, and an ID can never be reused once created.
Using a production ID for a QA product burns it permanently.

Both subscriptions go in a single group so a user can move between monthly and annual without
cancelling. The group already exists on the record as ID **22295334**, reference name
`Rumi Coach`; we reuse it and rename it to **Rumi Membership** (reference names are internal
and safe to change). Set the annual plan to a **higher service level** than monthly, so
upgrades take effect immediately and downgrades wait for renewal.

### The pre-existing `monthly` product

The record arrived with one subscription: reference name `Monthly`, product ID **`monthly`**,
Apple ID `6799374061`, 1 month, never submitted, no localizations, no availability. Its bare
unnamespaced ID is why we replace it rather than fill it in.

It is deleted as part of the setup run. Note that this does **not** free the identifier —
Apple scopes product IDs to the account permanently, so the string `monthly` is burned
whether we keep the product or not. Deleting only avoids carrying a confusing dead product
inside the group.

## Pricing

Three currency zones, matching `getPricing()` on the website:

Website intent, and what is actually live on the App Store:

| | Monthly | Annual | Quick Boost | Deep Dive | Power User |
|---|---|---|---|---|---|
| USD | $39.99 | $399.99 *(site: 399.90)* | $15.99 | $30.99 | $57.99 |
| GBP | £34.99 | £349.99 *(site: 349.90)* | £13.99 | £26.99 | £50.99 |
| EUR | €39.99 | €399.99 *(site: 399.90)* | €15.99 | €30.99 | €57.99 |

Every price matches the website except the annual plan, where Apple has no `.90` price point
at that magnitude — see below.

### Set the base territory to Germany, not the US

Anchoring the base price on **Germany (EUR)** makes Apple equalize the UK and all 25
eurozone territories to exactly the website's numbers, leaving only the US to override by
hand. Anchoring on the US instead produces €449.99 in the eurozone and needs 26 manual
overrides. All five products use Germany as base.

The trade-off: Apple never auto-adjusts the base territory, but *does* adjust the rest. So
Germany stays pinned while the US and UK prices can drift with FX over time. Re-check them
after any Apple pricing notice.

### The website's currency zones do not survive contact with the App Store

The website bills in three currencies and treats the Nordics, Poland, Turkey and Ukraine as
EUR. Apple does not: it bills each storefront in its **own local currency**. The live app
record already shows Denmark at `kr 299.00` (DKK), and Sweden, Norway, Poland and Turkey are
likewise SEK, NOK, PLN and TRY.

So exact parity with the website is only achievable where Apple bills in a currency the
website also uses:

| Zone | Parity achievable? | Territories |
|---|---|---|
| USD | Yes, for the US storefront | USA |
| GBP | Yes | GBR |
| EUR | Yes | The true eurozone — PRT, ESP, DEU, FRA, ITA, NLD, FIN |
| Everything else | No — Apple's local-currency price | sv-SE (SEK), da-DK (DKK), nb-NO (NOK), pl-PL (PLN), tr-TR (TRY), pt-BR (BRL), ja-JP (JPY), zh-CN (CNY), ko-KR (KRW), hi-IN (INR) |

Pin those nine territories by hand and accept Apple's generated price everywhere else. The
generated prices are approximately equivalent, and they move when Apple revises its FX tables,
so re-check the pinned nine after any Apple pricing notice.

One caveat on "USD": Apple bills roughly a hundred storefronts in USD, and it does **not**
give them a uniform amount — the live record shows Afghanistan at `$34.99` next to Albania at
`$39.99`, because equalization accounts for local tax treatment. Pinning `USA` fixes the US
price only. Making every USD storefront read `$39.99` means pinning each one individually;
`scripts/appstore-subscriptions.ts` can do it, but it is a deliberate choice about whether
uniform nominal pricing is worth overriding Apple's tax-adjusted defaults.

### If a price picker hangs on "Loading…"

Apple's price-point endpoint intermittently 500s per product+territory:

```
GET /iris/v2/inAppPurchases/<id>/pricePoints?filter[territory]=GBR → 500
```

When that happens the dropdown never populates and the price cannot be selected. It is
transient — Deep Dive's GBP price hit this and went through on a later attempt. Retry rather
than working around it.

### The `.90` annual price points do not exist

Verified in the UI: searching `399.90` returns **No Results**; `399.99` exists. The same
holds in GBP and EUR. So the annual plan is `.99` on iOS and `.90` on the website — a 9-cent
gap that makes annual 10.0025 months of monthly rather than exactly 10. "2 months free"
remains true for any practical purpose.

Non-`.99` endings *do* exist in some currencies (the record shows Brazil at `R$229.90`), and
the extended ladder — reachable via **See Additional Prices** — carries `.90`/`.95` variants
at lower magnitudes. There is simply no `399.90` in USD/GBP/EUR. Every monthly and top-up
price maps exactly.

If exact parity matters, the options are to request a custom price point from Apple, or to
change the website to `399.99`.

### Apple's commission is not priced in

These are the website's prices, where we keep the full amount minus payment processing. On
iOS, Apple takes 15–30%, so matching the web price means materially lower margin on every
iOS subscriber. Price parity across platforms is usually the right call for trust and for the
"cancel anytime" promise, but it is a decision worth making deliberately rather than
inheriting from the website by default.

### What is already on the record

The app record (`6799373415`) arrived with pricing already set across 175 territories, and it
is **not** the table above: a mix of `$34.99` and `$39.99` across USD storefronts, alongside
`€39.99` in the eurozone, `R$229.90` Brazil, `A$59.99` Australia, `C$49.99` Canada, `¥228`
China, `Kč 999` Czech Republic, `CLP 39,990` Chile and `COP 149,900` Colombia. Only the
eurozone row matches what we want. Repricing replaces all of it.

## Localized metadata

Every string is in [`appStoreLocalizations.ts`](../src/subscriptions/appStoreLocalizations.ts),
keyed by App Store Connect locale — 20 locales × (group name + 2 plans + 3 top-ups).

Names are taken from the website's own translations rather than retranslated, so the App
Store, the web paywall and the app all name the same thing the same way.

Three App Store locale codes differ from our i18n codes: `it-IT` → `it`, `zh-CN` → `zh-Hans`,
`nb-NO` → `no`. The mapping is `APP_STORE_LOCALE_BY_APP_LOCALE`.

Apple's limits are 30 characters for a display name and 45 for a description, and Apple
**truncates silently** rather than rejecting — a name that is one character too long ships
with a missing letter. `appStoreLocalizations.test.ts` enforces the budgets, so run it after
any copy change:

```bash
bun x jest src/subscriptions
```

The French Power User name (`Utilisateur intensif – 240 min`) sits at exactly 30 characters.
It fits, but there is no headroom — if that string needs to change, it needs to get shorter.

## Review notes

App Review rejects subscription submissions that do not state the terms on the paywall
itself. The paywall must show price, billing period, what 150 minutes buys, and links to
Terms and Privacy — [app/legal/terms.tsx](../app/legal/terms.tsx) and
[app/legal/privacy.tsx](../app/legal/privacy.tsx) — plus a working **Restore Purchases**
action. Guideline 3.1.2 is the one to read before submitting.
