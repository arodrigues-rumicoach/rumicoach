# Google SSO Setup (new GCP/Firebase projects)

Re-provisioning guide after migrating to the two new GCP/Firebase projects (**QA** and
**Production**). No code changes are needed anywhere — the backend endpoint
(`POST /v1/auth/google`), the mobile sign-in adapters, and the Terraform wiring all exist.
Only credentials and config values change.

**Where each moving part lives:**

| What | Where |
|---|---|
| Backend token validation | `rumi-be/internal/handlers/auth.go` → `validateGoogleToken` (validates the ID token's `aud` against a comma-separated client-ID list) |
| Backend env (local dev) | `rumi-be/.env` → `GOOGLE_CLIENT_IDS` (falls back to `GOOGLE_CLIENT_ID`) |
| Backend env (deployed) | `rumi-infra/environments/<env>/terraform.tfvars` → `google_client_id` → Cloud Run env `GOOGLE_CLIENT_ID` |
| Mobile web-client ID | `rumi-mobile` env: `EXPO_PUBLIC_GOOGLE_CLIENT_ID_WEB` (read in `src/config.ts`, used by `GoogleSignin.configure` in `src/adapters/auth/native.ts`) |
| Mobile Firebase files | `rumi-mobile/GoogleService-Info.plist` (iOS) and `rumi-mobile/plugins/google-services.json` (Android) — referenced from `app.json` |
| Push notifications (deployed) | No env needed — Cloud Run uses ADC; the service account already has `roles/firebase.admin` (`rumi-infra/modules/rumi-infra/secrets.tf`) |
| Push notifications (local dev) | `rumi-be/.env` → `FIREBASE_CREDENTIALS_BASE64` |

---

## Step 1 — Console work (repeat for BOTH projects: QA and Production)

Do this in each new GCP/Firebase project. Collect the values in the sheet at the bottom.

1. **OAuth consent screen** (GCP Console → APIs & Services → OAuth consent screen):
   app name, support email, authorized domains. **Publish to Production** — in Testing
   mode only allow-listed test users can sign in and tokens expire quickly.

2. **Register the apps in Firebase** (Firebase Console → Project settings → Your apps).
   This auto-creates the platform OAuth clients:
   - **iOS app**: bundle ID from `rumi-mobile/app.json` → download the new
     `GoogleService-Info.plist`.
   - **Android app**: package name from `rumi-mobile/app.json` **plus every SHA-1** that
     will sign a build → download the new `google-services.json`. Required SHA-1s:
     - debug keystore,
     - release keystore (if you build with EAS: `eas credentials` shows it),
     - **Play App Signing** SHA-1 from the Play Console (for store builds).
     A missing SHA-1 in the new project is the classic Android `DEVELOPER_ERROR` (code 10).

3. **Web OAuth client** (GCP Console → APIs & Services → Credentials — Firebase usually
   auto-creates a "Web client"): add the web app's URL to **Authorized JavaScript
   origins** and redirect URIs (must match the backend `FRONTEND_URL` for that
   environment). **Its client ID is the important one** — the native sign-in SDK issues
   ID tokens with `aud` = this web client ID on all platforms.

4. Note the **iOS** and **Android** client IDs too (Credentials page).

5. **(Local dev only) Service account for FCM**: Firebase Console → Project settings →
   Service accounts → Generate new private key. Encode it for `.env`:
   ```bash
   base64 -i <downloaded-key>.json | tr -d '\n'
   ```
   Do NOT commit the JSON — `.gitignore` now blocks `*firebase-adminsdk*.json`. The old
   key (`rumi-26f42-...`) was removed from the repo; **revoke it** in the old project and
   note that it remains in git history.

---

## Step 2 — Backend deployed envs (`rumi-infra`)

Edit `google_client_id` in each environment's tfvars (comma-separated list; the backend
splits on commas). Use each environment's OWN project's IDs — QA and prod currently share
one stale value, which is wrong going forward:

- `environments/qa-eu/terraform.tfvars` (and `qa-us` if in use):
  ```hcl
  google_client_id = "<QA_WEB_ID>,<QA_IOS_ID>,<QA_ANDROID_ID>"
  ```
- `environments/prod-eu/terraform.tfvars` (and `prod-us` if in use):
  ```hcl
  google_client_id = "<PROD_WEB_ID>,<PROD_IOS_ID>,<PROD_ANDROID_ID>"
  ```

Then `terraform apply` per environment. (Strictly only the web ID appears as the token
audience, but listing all three is harmless and robust to SDK changes.)

## Step 3 — Backend local dev (`rumi-be/.env`)

```bash
GOOGLE_CLIENT_IDS=<QA_WEB_ID>,<QA_IOS_ID>,<QA_ANDROID_ID>
FIREBASE_CREDENTIALS_BASE64=<output of the base64 command from Step 1.5, QA project>
```

## Step 4 — Frontend (`rumi-mobile`)

The app identity is environment-switched by `APP_ENV` in `app.config.js` (set per EAS
build profile in `eas.json`): QA builds are `coach.rumi.app.qa` ("Rumi QA"), production
builds are `coach.rumi.app` ("Rumi"). The Google iOS URL scheme is derived from
`EXPO_PUBLIC_GOOGLE_CLIENT_ID_IOS`, so no hardcoded reversed-client-ID lives in config.

1. Place the Firebase files from each project's downloads:
   - QA: `GoogleService-Info.plist` (repo root) and `plugins/google-services.json`
   - Prod: `GoogleService-Info.prod.plist` and `plugins/google-services.prod.json`
2. Set the env vars for each build profile / environment (`.env` files and/or `eas.json`
   profile env):
   ```bash
   EXPO_PUBLIC_GOOGLE_CLIENT_ID_WEB=<WEB_ID of that environment's project>
   EXPO_PUBLIC_GOOGLE_CLIENT_ID_IOS=<IOS_ID>
   EXPO_PUBLIC_GOOGLE_CLIENT_ID_ANDROID=<ANDROID_ID>
   ```
3. **Rebuild the native apps** (dev clients and EAS builds): the plist/json are baked in
   at build time; builds made before the swap keep talking to the old project. No config
   change needed in `app.json` — the google-signin plugin reads the iOS URL scheme from
   the plist's `REVERSED_CLIENT_ID`.

## Step 5 — Verify

Sign in on each platform against each environment and check the backend log for
`Google ID Token validation succeeded` with the new client ID. Failure signatures:

| Symptom | Cause |
|---|---|
| Android `DEVELOPER_ERROR` / code 10 | SHA-1 or package name not registered in the new project |
| Backend 401 `Invalid Google Token` | Token `aud` not in `GOOGLE_CLIENT_IDS` — usually a `EXPO_PUBLIC_GOOGLE_CLIENT_ID_WEB` ↔ tfvars mismatch, or QA app pointed at prod backend (or vice versa) |
| iOS crash on sign-in tap | Stale plist / URL scheme — rebuild the app |
| 403 `No account found` | Expected: Google login links to existing accounts by google_id/email; it does not create accounts |

---

## Value sheet (fill in while doing Step 1)

| Value | QA project | Production project |
|---|---|---|
| Web client ID | | |
| iOS client ID | | |
| Android client ID | | |
| SHA-1s registered (debug / release / Play) | | |
| Service-account key generated + encoded (local dev) | | n/a |
