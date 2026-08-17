# Architecture — rumicoach-app

## Structural pattern

Layered React Native app with a **platform-adapter boundary**:

```
app/                    expo-router file-based routes (screens only, thin)
src/components/         Tamagui UI, atomic design (atoms → molecules → organisms → templates)
src/context/ + hooks/   State & orchestration (Auth, Session, Audio, Settings, …)
src/api/                HTTP client, region-aware URL resolution, JWT utils
src/adapters/           Platform seam: native vs web implementations behind one interface
src/i18n/               i18n-js, 20 locales
```

**Adapters are the key pattern.** Anything that differs between native and web — auth (Google Sign-In vs OAuth web flow), storage (SecureStore vs localStorage), notifications, session audio — lives in `src/adapters/<name>/` with `index.ts` (interface + platform switch), `native.ts`, and `web.ts`. Screens and contexts never import platform APIs directly.

## Data flow

### REST

1. Screens/contexts call `src/api` functions built on the axios client (`src/api/client.ts`).
2. **URL resolution is region-aware** (`src/api/backend-url.ts`): endpoints are derived from `EXPO_PUBLIC_API` (`auth.${EXPO_PUBLIC_API}` and `${region}.${EXPO_PUBLIC_API}`). In local dev or specialized environments, endpoints can be individually overridden with `EXPO_PUBLIC_API_AUTH`, `EXPO_PUBLIC_API_EU`, `EXPO_PUBLIC_API_US` (e.g. `localhost`).
3. The client stores `rumi_auth_token` / `rumi_refresh_token` via the storage adapter, refreshes the access token on 401 with a de-duplicated in-flight `refreshPromise`, and emits `auth:invalid` (via `src/utils/AppEvents`) to force logout when refresh fails. `AuthContext` owns login state and listens for that event.

### Voice session (the core product loop)

1. `SessionContext` (`src/context/SessionContext.tsx`) opens a WebSocket to the region's `/ws/chat` (`regionWebSocketUrl`).
2. The **session-audio adapter** captures microphone PCM — natively via `@speechmatics/expo-two-way-audio`, on web via Web Audio with VAD (`@ricky0123/vad-web` + `onnxruntime-web`) — and `SessionContext` streams it up the socket (with resampling, e.g. 24 kHz → 16 kHz, done in-context).
3. Downstream messages carry coach audio (PCM played through the adapter), transcripts, tool-driven UI events (e.g. open memories/session screens, Wheel of Life data), and session-state changes. `AudioContext` tracks volume levels for the visualizer.
4. Disconnect/teardown restores the audio mode and closes the adapter.

### Push & analytics

Firebase messaging (FCM token registered via `PUT /v1/me/fcm-token`), analytics and crashlytics are initialized in `src/firebase` / `src/adapters/firebase`.

## Routing

`app/` uses expo-router groups:

- `index.tsx` — entry/redirect logic; `onboarding.tsx` — pre-auth onboarding
- `(auth)/` — `login`, `signup`
- `(tabs)/` — the main app: `session` (voice coaching), `growth` (goals/tasks/wheel), `memories`, `profile`
- `(settings)/` — settings stack; `legal/` — in-app legal docs

## External dependencies / boundaries

| Boundary | Detail |
|---|---|
| `rumicoach-backend` REST `/v1` | All data. Contract defined by the backend's `api/openapi.yaml`. |
| `rumicoach-backend` WS `/ws/chat` | Bidirectional PCM audio + JSON control messages. |
| Google OAuth | Client IDs per platform/environment (public identifiers, set in `eas.json` / CI). |
| Firebase (QA + prod projects) | Config files selected by `APP_ENV` in `app.config.js`. |

## Environments & builds

- `app.json` is the base config; `app.config.js` overrides it based on `APP_ENV` (`qa` default, `production`): app name, bundle id (`coach.rumi.app[.qa]`), Firebase files, Google URL scheme (derived from the iOS client ID), and CI build number.
- EAS profiles (`eas.json`): `development` (dev client), `preview` (QA APK), `production`.
- CI (`bitbucket-pipelines.yml`): `development` branch → QA web bucket + Firebase App Distribution Android build; `main` → production equivalents. Keyless GCP auth via Bitbucket OIDC → Workload Identity Federation.
