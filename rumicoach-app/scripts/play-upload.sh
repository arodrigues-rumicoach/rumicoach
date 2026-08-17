#!/usr/bin/env bash
# Upload a signed AAB to Google Play and assign it to a track.
#
#   scripts/play-upload.sh <aab-path> <package-name> <track>
#
# Keyless, like the rest of the pipeline: the Bitbucket OIDC token is exchanged for a
# short-lived access token via Workload Identity Federation, then service-account
# impersonation. Nothing is stored in Bitbucket beyond the WIF provider name.
#
# The Android build image has no gcloud, so the exchange is done with curl directly rather
# than `gcloud auth print-access-token`.
#
# Required environment:
#   BITBUCKET_STEP_OIDC_TOKEN   provided by Bitbucket when the step sets `oidc: true`
#   GCP_WIF_PROVIDER            full provider resource name (no leading //iam.googleapis.com/)
#   GCP_SERVICE_ACCOUNT         deployer SA email to impersonate
#
# Play-side prerequisites, all one-time and none of them settable from here:
#   1. Play Console → Setup → API access: link the GCP project that owns the SA.
#   2. Play Console → Users and permissions: invite GCP_SERVICE_ACCOUNT and grant
#      "Release to testing tracks" (plus "Release to production" if TRACK=production).
#   3. Enable androidpublisher.googleapis.com on that GCP project.
#   4. The app must already have one build uploaded by hand — Play refuses the very first
#      upload of a package over the API.

set -euo pipefail

AAB_PATH="${1:?usage: play-upload.sh <aab-path> <package-name> <track>}"
PACKAGE="${2:?missing package name}"
TRACK="${3:?missing track}"

: "${BITBUCKET_STEP_OIDC_TOKEN:?not set — does the step have oidc: true?}"
: "${GCP_WIF_PROVIDER:?not set}"
: "${GCP_SERVICE_ACCOUNT:?not set}"

[ -f "$AAB_PATH" ] || { echo "No AAB at $AAB_PATH"; exit 1; }

API="https://androidpublisher.googleapis.com/androidpublisher/v3/applications/${PACKAGE}"

# Pull a single field out of a JSON response. python3 ships with macOS, jq does not.
#
# Prints Play's own error when the field is missing. The bare version of this raised
# "KeyError: 'id'" and threw the response away, which is all #188 reported — the actual
# cause (which of the four prerequisites above is unmet) was in the body it discarded.
json() {
  python3 -c "
import json, sys
field = sys.argv[1]
raw = sys.stdin.read()
try:
    data = json.loads(raw)
except ValueError:
    sys.stderr.write('Play returned a non-JSON response:\n' + raw[:2000] + '\n')
    sys.exit(1)
if not isinstance(data, dict) or field not in data:
    body = json.dumps(data.get('error', data), indent=2) if isinstance(data, dict) else raw
    sys.stderr.write('Play did not return \'' + field + '\'. Response was:\n' + body[:2000] + '\n')
    sys.exit(1)
print(data[field])
" "$1"
}

# The track and commit calls below stage and then publish the release. Their output used to
# go to /dev/null, so an error there was invisible and the script still printed its success
# line. This makes them speak up instead.
check_ok() {
  python3 -c "
import json, sys
step = sys.argv[1]
raw = sys.stdin.read()
try:
    data = json.loads(raw) if raw.strip() else {}
except ValueError:
    sys.stderr.write(step + ' returned a non-JSON response:\n' + raw[:2000] + '\n')
    sys.exit(1)
if isinstance(data, dict) and 'error' in data:
    sys.stderr.write(step + ' failed:\n' + json.dumps(data['error'], indent=2)[:2000] + '\n')
    sys.exit(1)
" "$1"
}

echo "→ Exchanging OIDC token for federated credentials"
STS_TOKEN=$(curl -sS -X POST https://sts.googleapis.com/v1/token \
  -H 'Content-Type: application/json' \
  -d "{
    \"audience\": \"//iam.googleapis.com/${GCP_WIF_PROVIDER}\",
    \"grantType\": \"urn:ietf:params:oauth:grant-type:token-exchange\",
    \"requestedTokenType\": \"urn:ietf:params:oauth:token-type:access_token\",
    \"scope\": \"https://www.googleapis.com/auth/cloud-platform\",
    \"subjectTokenType\": \"urn:ietf:params:oauth:token-type:jwt\",
    \"subjectToken\": \"${BITBUCKET_STEP_OIDC_TOKEN}\"
  }" | json access_token)

echo "→ Impersonating ${GCP_SERVICE_ACCOUNT}"
ACCESS_TOKEN=$(curl -sS -X POST \
  "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/${GCP_SERVICE_ACCOUNT}:generateAccessToken" \
  -H "Authorization: Bearer ${STS_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d '{"scope":["https://www.googleapis.com/auth/androidpublisher"]}' | json accessToken)

AUTH="Authorization: Bearer ${ACCESS_TOKEN}"

# An "edit" is Play's transaction: everything below is staged against it and only becomes
# real at :commit, so a failure part-way leaves the listing untouched.
echo "→ Opening edit"
EDIT_ID=$(curl -sS -X POST "${API}/edits" -H "$AUTH" -H 'Content-Length: 0' | json id)
echo "   edit ${EDIT_ID}"

echo "→ Uploading $(basename "$AAB_PATH") ($(du -h "$AAB_PATH" | cut -f1))"
VERSION_CODE=$(curl -sS -X POST \
  "https://androidpublisher.googleapis.com/upload/androidpublisher/v3/applications/${PACKAGE}/edits/${EDIT_ID}/bundles?uploadType=media" \
  -H "$AUTH" \
  -H 'Content-Type: application/octet-stream' \
  --data-binary "@${AAB_PATH}" | json versionCode)
echo "   versionCode ${VERSION_CODE}"

echo "→ Assigning to '${TRACK}'"
curl -sS -X PATCH "${API}/edits/${EDIT_ID}/tracks/${TRACK}" \
  -H "$AUTH" \
  -H 'Content-Type: application/json' \
  -d "{
    \"track\": \"${TRACK}\",
    \"releases\": [{
      \"versionCodes\": [\"${VERSION_CODE}\"],
      \"status\": \"completed\"
    }]
  }" | check_ok "Assigning to track '${TRACK}'"

echo "→ Committing edit"
curl -sS -X POST "${API}/edits/${EDIT_ID}:commit" -H "$AUTH" -H 'Content-Length: 0' | check_ok "Committing the edit"

echo "✓ ${PACKAGE} versionCode ${VERSION_CODE} live on '${TRACK}'"
