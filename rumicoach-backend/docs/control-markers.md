# Control Markers (Silent In-Speech Commands)

Rumi's coaching scripts can embed **silent control markers** — short glyph pairs the
native-audio model reproduces in its text stream but **never speaks aloud**. The backend
parses them out of the transcript, runs the corresponding side effect (open a screen /
insert a silence), and replaces them with a processed placeholder before the transcript is
forwarded to the client.

All parsing lives in [`internal/services/chat/live_api.go`](../internal/services/chat/live_api.go)
(see the marker block near the top of the file).

---

## Why glyphs instead of words

Earlier versions used readable bracket tags (`[SS=memories]`, `[P=2]`). The audio model
occasionally **vocalized** them ("ess-ess-equals-memories"). The current scheme uses an
**adjacency pair** — a command glyph immediately followed by a parameter glyph — drawn from
the Unicode Geometric-Shapes / Block-Elements ranges. The model reproduces them reliably,
the audio model does not speak them, and a lone glyph never triggers a false positive.

Every marker is the form: **`<command glyph><parameter glyph>`** with no space between.

---

## 1. Screen markers — `◆` + screen glyph

Open a **data-less** screen in the app, synced to the audio position. Command glyph: **`◆`** (U+25C6).

| Marker | Screen name | Meaning |
|--------|-------------|---------|
| `◆▣`   | `memories`  | Open the Memories screen (`▣` = U+25A3) — the intro tour's first stop |
| `◆▤`   | `session`   | Open the Session screen, the tab labelled Talk (`▤` = U+25A4) — the intro tour's free-conversation stop |
| `◆▥`   | `growth`    | Open the Growth screen (`▥` = U+25A5) — used by the intro's mini tour |
| `◆▦`   | *(session summary)* | Reveal the session-summary panel WITHOUT its next-session card (`▦` = U+25A6) — **no longer in any script** (kept parsed as a legacy safety); the reveal moved to `◆▧` |
| `◆▧`   | *(session summary + next)* | Reveal the full session-summary panel, next-session card included (`▧` = U+25A7) — mid-goodbye, right after the script's announcement sentence |
| `◆▨`   | `profile`   | Open the Profile screen (`▨` = U+25A8) — the intro tour's last stop |

- Source of truth: `screenSymbolToName` in `live_api.go`.
- In the transcript sent to the client, a parsed screen marker is replaced with `[PROCESSED_SCREEN]`.
- The reveal is **delayed to sync with the audio** (see `queueShowScreen`).

**Neither `◆▦` nor `◆▧` is a real screen.** They resolve to the internal pseudo-names
`sessionSummaryMarkerScreen` ("session_summary") and `sessionSummaryNextMarkerScreen`
("session_summary_next"), both deliberately absent from `showScreenAllowed` — the
`show_screen` *tool* can never produce either, only these markers can. When parsed, they
bypass `queueShowScreen`'s allowlist/suppression checks and go straight to
`scheduleScreenReveal`, whose scheduled action is `emitAndPersistSessionSummary(bool)`
(send + persist the summary payload, with or without the `next_session` field) instead of
a `show_screen` client message.

The panel reveals **once, at the goodbye**: the Vision ending script first announces the
card in words ("in a moment, a card will appear with the essence of today...") and then
carries `◆▧` immediately after, so the reveal lands right after the user has been told
what is coming. The earlier two-stage design — `◆▦` revealing the panel at the start of
the Step 3 synthesis — was QA-rejected twice over: the card appeared with no explanation,
and the user had to read it while Rumi was still speaking the synthesis. `◆▦` therefore
appears in **no script** anymore; the parser still handles it as a legacy safety.

Each stage remains idempotent (`sessionSummaryEmitted` / `sessionSummaryNextSessionSent`),
and a final safety-net call with `includeNextSession=true` still fires from
`handleTerminateSession` in case `◆▧` is ever dropped — it is also the *only* live reveal
for the single-prompt deep sessions (Movement, Values, Energy, Decisions, Beliefs,
Identity, Acceptance), which have no goodbye marker. Every deep session has a card
(`summaryCapableSessions` in `session_summary.go`); every closing script announces it
verbally right before the goodbye so the reveal is expected, and `Cleanup` re-persists
the payload as a last resort when the closing was reached (insight saved) but
`terminate_session` never fired.

> ⚠️ **Never** open a **data-bearing** screen (e.g. the Wheel of Life) with a screen marker —
> it would open empty. Those are shown by the tool that carries their data
> (`set_wheel_of_life_categories` → `wheel_of_life_update`). `show_screen` is whitelisted to
> `memories`, `session`, `tasks`, `journey`, and `profile` (`showScreenAllowed` in [`tools.go`](../internal/services/chat/tools.go)).

`memories`, `journey`, `session` and `profile` reveals are additionally **suppressed during
the Vision exercise phases** (ideal life, wheel, metaphor): the model tends to re-emit the
intro tour's markers there, and the screen would pop up mid-exercise.

**Deferred landing.** `show_screen` may carry `data.at = "session_end"`, which means "this is
where the client should land once it closes the session", not "open it now". The onboarding
intro sends `{screen: "growth", at: "session_end"}` from `terminate_session` so a user who
declined the first exercise ends up on the growth screen — but only after the goodbye has
finished playing, since the client owns the close (see `handleTerminateSession` and the
`session_terminated` handler in the app's `SessionContext`).

---

## 2. Pause markers — `●` + block glyph  ⏸️ *(currently disabled at the prompt level)*

Insert a precise silence into the audio stream. Command glyph: **`●`** (U+25CF). The
parameter is a **lower-block glyph whose height encodes the number of seconds**.

| Marker | Seconds | Block glyph |
|--------|---------|-------------|
| `●▁`   | 1       | U+2581 |
| `●▂`   | 2       | U+2582 |
| `●▃`   | 3       | U+2583 |
| `●▄`   | 4       | U+2584 |
| `●▅`   | 5       | U+2585 |
| `●▆`   | 6       | U+2586 |
| `●▇`   | 7       | U+2587 |
| `●█`   | 8       | U+2588 |

- Source of truth: `pauseSymbolToSeconds` in `live_api.go`.
- In the transcript sent to the client, a parsed pause marker is replaced with `[PROCESSED_PAUSE: N]`.
- The silence is injected as sample-accurate PCM via `injectSilence(seconds)`.

> 🚧 **Status: pauses are OFF.** The pause markers have been removed from **all scripts** and
> from the **prompt directives** that taught the model the mechanism (the model was inventing
> its own pauses, e.g. `●▂`, in places it shouldn't). The **parsing/injection code is kept
> intact** — re-enabling pauses is just a matter of re-adding `●` markers to scripts and
> restoring the directive text. While disabled, the parser still strips any stray `●` marker
> the model emits, so it is never spoken.

---

## 3. Legacy fallback tags (parsed, never emitted)

Still recognized on input as a fallback, but **not used in any current prompt**:

| Tag            | Equivalent | Notes |
|----------------|------------|-------|
| `[SS=<name>]`  | `◆<glyph>` | e.g. `[SS=memories]`; same whitelist applies |
| `[P=<n>]`      | `●<glyph>` | e.g. `[P=2]` → 2-second pause |

Regexes: `screenRegex`, `pauseRegex` in `live_api.go`.

---

## 4. Processed placeholders (what the client sees)

After parsing, markers are replaced in the `ai_transcript` payload so the client transcript
stays clean and readable:

| Raw marker      | Processed placeholder    |
|-----------------|--------------------------|
| `◆▣` / `[SS=…]` | `[PROCESSED_SCREEN]`     |
| `●▂` / `[P=2]`  | `[PROCESSED_PAUSE: 2]`   |

---

## 5. Silence the markers do **not** control

There is one silence the backend injects **on its own**, unrelated to any marker: a **1-second
gap before a system-driven follow-up turn** (e.g. Wheel intro → first-area question), so two
back-to-back AI turns with no user turn between them don't sound glued. This stays active even
though pause markers are disabled. See `injectSilence(1)` at the transition-prompt injection
point in `live_api.go`.

---

## 6. How to add a new marker

**New data-less screen:**
1. Pick an unused parameter glyph and add it to the `◆([…])` character class in `screenSymbolRegex`.
2. Add `"<glyph>": "<screen-name>"` to `screenSymbolToName`.
3. Add `"<screen-name>": true` to `showScreenAllowed` in `tools.go`.
4. Reference `◆<glyph>` in the relevant script, and ensure the language/persona directives still
   describe the `◆` screen marker.

**Re-enabling pauses:**
1. Add the `●<glyph>` marker back into the desired script lines.
2. Restore the pause-marker description in the LANGUAGE directive, the persona "No Mirroring" /
   "Spell Out" rules, and `languageContinuityReminder` (so the model preserves and never speaks them).

---

## 7. Authoring rules (already enforced in the system prompt)

- Markers are **silent**: never read aloud, named, spelled, or translated.
- Keep markers **byte-for-byte unchanged** even when a script is translated into the user's language.
- A marker is `<command glyph><parameter glyph>` with **no space** between the two glyphs.
- A lone `◆` or `●` in normal prose is **not** a marker and is left untouched.
