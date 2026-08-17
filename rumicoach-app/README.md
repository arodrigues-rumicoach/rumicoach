# rumicoach-app

The **Rumi** client app — a voice-first AI life-coaching app built with **Expo / React Native**, running on iOS, Android, and web (the web build is deployed as `app.rumi.coach`). It talks to `rumicoach-backend` over REST (`/v1`) and a real-time audio WebSocket (`/ws/chat`) that streams microphone PCM to the backend's Gemini Live proxy and plays coach audio back.

## Tech stack

- **Expo SDK 56** / **React Native 0.85** / **React 19**, TypeScript
- **expo-router** (file-based routing in `app/`)
- **Tamagui v2** for UI (config in `tamagui.config.ts`), components organized atomic-design style (atoms → molecules → organisms → templates)
- **Audio**: `@speechmatics/expo-two-way-audio` (native PCM in/out), Web Audio + `@ricky0123/vad-web` / `onnxruntime-web` (web voice activity detection) — unified behind `src/adapters/session-audio`
- **Auth**: Google Sign-In (`@react-native-google-signin`, `expo-auth-session`) + email/SMS code login against the backend
- **Firebase**: analytics, crashlytics, messaging (push)
- **i18n-js** with 20 locales (`src/i18n`)
- `axios`, `zod`, `react-hook-form`
- Package manager: **bun** (`bun.lock`); tests with **jest-expo**

## Quickstart

### Prerequisites

- Node 20+ / bun, Xcode and/or Android toolchain for native builds
- A running backend (`rumicoach-backend` on `localhost:8000` for local dev)

### Install & run

```bash
bun install

bun run web        # web on port 3000 (fastest loop, no native toolchain)
bun run ios        # expo run:ios
bun run android    # expo run:android
bun run start      # bare expo start
bun test           # jest
```

### Environment variables (`.env`)

Only `EXPO_PUBLIC_`-prefixed vars reach the client:

| Variable | Purpose |
|---|---|
| `EXPO_PUBLIC_API` | Base domain for QA and PROD (`qa.rumi.coach`, `rumi.coach`). Derives hosts: `auth.*`, `eu.*`, `us.*`, and website pages |
| `EXPO_PUBLIC_API_AUTH` | Override for auth backend endpoint (e.g. `http://localhost:8000/v1`) |
| `EXPO_PUBLIC_API_EU`, `EXPO_PUBLIC_API_US` | Overrides for regional backend endpoints (e.g. `http://localhost:8000/v1`) |
| `EXPO_PUBLIC_WEBSITE_URL` | Optional website URL override for support/about links (e.g. `https://qa.rumi.coach`) |
| `EXPO_PUBLIC_GOOGLE_CLIENT_ID_{WEB,IOS,ANDROID}` | Google OAuth client IDs (public identifiers, per environment) |
| `EXPO_PUBLIC_{EU,US}_WS_URL` | Optional explicit WebSocket URL overrides |

Region routing: auth calls go to the auth host; all other calls go to the region encoded in the user's access token (`eu`/`us`), with URLs derived in `src/api/backend-url.ts`.

### Build environments

`app.config.js` merges over `app.json`, keyed by `APP_ENV` (set per EAS profile in `eas.json` and per CI branch):

- `APP_ENV=qa` (default) → app id `coach.rumi.app.qa`, QA Firebase files
- `APP_ENV=production` → `coach.rumi.app`, prod Firebase files (`GoogleService-Info.prod.plist`, `plugins/google-services.prod.json` — must be downloaded from the prod Firebase project before a production prebuild)

CI (`bitbucket-pipelines.yml`): `development` → QA, `main` → production. Builds the web bundle (`expo export --platform web`) to a GCS bucket and distributes Android via Firebase App Distribution, using keyless OIDC/WIF auth. iOS is not built in CI yet.

## More docs

- [ARCHITECTURE.md](docs/ARCHITECTURE.md) — layers, session/audio data flow
- [COMPONENT_MAP.md](docs/COMPONENT_MAP.md) — directory guide + high-risk files
- [CONTRIBUTING.md](docs/CONTRIBUTING.md) — conventions and testing
