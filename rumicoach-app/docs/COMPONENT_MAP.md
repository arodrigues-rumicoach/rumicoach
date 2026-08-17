# Component Map — rumicoach-app

## Directory breakdown

| Path | What lives here |
|---|---|
| `app/` | expo-router routes. `(auth)/` signin & signup, `(tabs)/` main app (`session`, `growth`, `memories`, `profile`), `(settings)/` settings stack, `legal/`, `onboarding.tsx`, `index.tsx` (entry redirect), `_layout.tsx` (providers + navigation shell), `+html.tsx` (web HTML shell). |
| `src/api/` | `client.ts` (axios instance, token storage, 401 refresh-and-retry, `auth:invalid` event), `backend-url.ts` (auth-plane vs regional data-plane URL resolution, WS URL), `jwt.ts` (region claim parsing), `errors.ts`, `auth/` (auth API calls). |
| `src/adapters/` | Platform seam — each subdir has `index.ts` (interface + switch), `native.ts`, `web.ts`: `auth/` (Google Sign-In flows), `storage/` (SecureStore vs localStorage), `notifications/`, `session-audio/` (two-way PCM audio), `firebase/`, plus `platform.ts` (`isWeb`). |
| `src/context/` | App state: `AuthContext` (login/session tokens), `SessionContext` (**voice session orchestration**: WS lifecycle, audio streaming, resampling, server messages), `AudioContext` (levels/visualizer), `SettingsContext`, `BlurContext`, `ScrollNavContext`, `TabHistoryContext`. |
| `src/hooks/` | Thin accessors over contexts (`useAuth`, `useSession`, `useAudio`, `useSettings`) + `useVoicePreview`, `useThemeAssetUri`. |
| `src/components/` | Tamagui UI in atomic-design tiers: `atoms/`, `molecules/`, `organisms/`, `templates/`. |
| `src/i18n/` | i18n-js setup (`instance.ts`, `I18nProvider.tsx`) + 20 locale files (en-US … zh-CN). |
| `src/styles/`, `src/types/`, `src/utils/` | Theme/style helpers, shared TS types, utilities (incl. `AppEvents` event bus). |
| `src/firebase/` | Firebase initialization. |
| `assets/`, `plugins/` | Images/fonts; Expo config plugins + `google-services*.json`. |
| `app.json` + `app.config.js` | Base Expo config + `APP_ENV`-keyed overrides (bundle ids, Firebase files, build number). |
| `eas.json` | EAS build profiles (development / preview / production) with per-env Google client IDs. |
| `bitbucket-pipelines.yml` | CI: web → GCS, Android → Firebase App Distribution; `development`→QA, `main`→prod. |
| `credentials/`, `credentials.json`, `GoogleService-Info*.plist` | Signing and Firebase config material — **do not commit changes casually; never log or copy elsewhere**. |
| `_archived/`, `android_build*.log`, `dist/` | Archived experiments, build logs, web export output — not part of the app source. |

## Critical / high-risk files

| File | Why it's risky |
|---|---|
| `src/context/SessionContext.tsx` | The product's core loop: WS connect/teardown, mic streaming, PCM playback, resampling, server-event handling. Regressions break live coaching sessions on all platforms. |
| `src/adapters/session-audio/*` | Native/web audio capture & playback. Platform-specific edge cases (audio modes, VAD) are easy to break and hard to test in CI. |
| `src/api/client.ts` | Token storage + refresh race handling (`refreshPromise` de-dup); mistakes log every user out or send requests to the wrong region. |
| `src/api/backend-url.ts` | Auth-plane vs data-plane routing. A wrong URL here silently violates data residency or breaks login. Covered by `src/api/__tests__/backend-url.test.ts` — keep it that way. |
| `src/context/AuthContext.tsx` | Login state machine, Google + code login, `auth:invalid` handling. |
| `app.config.js` / `eas.json` / `bitbucket-pipelines.yml` | Environment identity (bundle ids, Firebase project, OAuth clients). A QA/prod mix-up ships builds pointed at the wrong backend/Firebase. |
| `tamagui.config.ts` | Theme/tokens used by every component. |
