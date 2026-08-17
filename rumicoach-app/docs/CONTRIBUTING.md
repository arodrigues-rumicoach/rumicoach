# Contributing — rumicoach-app

## Workflow

- Package manager is **bun** (`bun.lock`). `bun install`, `bun run <script>`.
- Branch model: `development` → QA, `main` → production (CI in `bitbucket-pipelines.yml`).
- Fast local loop: `bun run web` (port 3000) against a local backend (`EXPO_PUBLIC_API=qa.rumi.coach`, `EXPO_PUBLIC_API_AUTH=http://localhost:8000`, `EXPO_PUBLIC_API_EU=http://localhost:8000` in `.env`). Use native runs (`bun run ios` / `bun run android`) when touching adapters or audio.

## Coding standards observed in this codebase

- **Platform differences go in `src/adapters/`** — never `Platform.OS` branching inside screens or components. Add `types.ts`/`index.ts` + `native.ts` + `web.ts` and switch in `index.ts`.
- **State lives in contexts, accessed via hooks** (`src/hooks/useX` wrapping `src/context/XContext`). Screens in `app/` stay thin.
- UI follows atomic design under `src/components/` using Tamagui primitives and tokens from `tamagui.config.ts`.
- **Every user-facing string goes through i18n** (`src/i18n`, 20 locales). No hardcoded English in components.
- Forms use `react-hook-form` + `zod` resolvers.
- Cross-cutting signals use the `AppEvents` bus (e.g. `auth:invalid`) rather than prop drilling.
- Config values come from `EXPO_PUBLIC_*` env vars surfaced via `src/config.ts` / `src/api/backend-url.ts` — never hardcode hosts or client IDs in components.

## Testing

- **Runner**: jest with `jest-expo` preset + `@testing-library/react-native` (`jest.config.js`).
- **Location**: `__tests__/` folders next to the code — currently `src/api/__tests__/` (client, backend-url, jwt), `src/context/__tests__/` (Auth, Session, Audio, Settings), `src/adapters/__tests__/`, `src/utils/__tests__/`.

```bash
bun test                      # or: npx jest
npx jest src/api              # one folder
npx jest -t "refresh"         # by test name
```

- The highest-value tests are around **auth/refresh logic and region URL resolution** — extend them when touching `src/api/`.
- Audio/session behavior is only partially unit-testable; verify voice sessions manually on web *and* at least one native platform before merging changes to `SessionContext` or `session-audio`.

## Environment gotchas

- Only `EXPO_PUBLIC_`-prefixed vars are available in client code; changing `.env` requires restarting the bundler with cache clear (`--clear`).
- `APP_ENV` (not `.env`) decides app identity at prebuild time — QA and prod are different bundle ids and Firebase projects. Production prebuilds need the prod Firebase files present locally.
- Google OAuth client IDs are public identifiers and are intentionally committed in `eas.json`/CI; real secrets never belong in this repo.
