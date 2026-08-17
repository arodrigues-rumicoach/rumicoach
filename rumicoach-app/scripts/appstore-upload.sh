#!/usr/bin/env bash
# Upload a signed IPA to App Store Connect.
#
#   scripts/appstore-upload.sh <ipa-path>
#
# Uses an App Store Connect API key rather than an Apple ID + app-specific password, so
# nothing here depends on a human's 2FA and the same key also drives
# scripts/appstore-subscriptions.ts.
#
# altool wants the private key on disk at a fixed location it discovers by key ID, so the
# .p8 is written to ~/.appstoreconnect/private_keys and removed on exit.
#
# Required environment (Bitbucket secured variables):
#   ASC_KEY_ID        e.g. ABCD123456 — shown when the key is created
#   ASC_ISSUER_ID     UUID, same for every key in the account
#   ASC_KEY_P8_B64    base64 of the AuthKey_<ASC_KEY_ID>.p8 file
#
# App Store Connect prerequisites, all one-time:
#   1. Users and Access → Integrations → Team Keys → generate a key with the
#      "App Manager" role, and download the .p8 (it is offered exactly once).
#   2. Agreements, Tax, and Banking: the Paid Applications agreement must be active,
#      otherwise the in-app purchases cannot be reviewed alongside the build.
#
# Note this only uploads. The build then takes several minutes to finish processing before
# it can be attached to a version, and submitting for review stays a deliberate manual act
# in App Store Connect.

set -euo pipefail

IPA_PATH="${1:?usage: appstore-upload.sh <ipa-path>}"

: "${ASC_KEY_ID:?ASC_KEY_ID is not set}"
: "${ASC_ISSUER_ID:?ASC_ISSUER_ID is not set}"
: "${ASC_KEY_P8_B64:?ASC_KEY_P8_B64 is not set}"

[ -f "$IPA_PATH" ] || { echo "No IPA at $IPA_PATH"; exit 1; }

KEY_DIR="$HOME/.appstoreconnect/private_keys"
KEY_FILE="$KEY_DIR/AuthKey_${ASC_KEY_ID}.p8"
mkdir -p "$KEY_DIR"
# The key outlives this script only if the runner is killed mid-step; on a shared
# self-hosted runner that would leave a usable credential on disk, so clean up on any exit.
trap 'rm -f "$KEY_FILE"' EXIT
echo "$ASC_KEY_P8_B64" | base64 --decode > "$KEY_FILE"
chmod 600 "$KEY_FILE"

echo "Validating $IPA_PATH before upload…"
# Validation catches the common rejections — wrong bundle ID, missing icon, a build number
# already used — in seconds, instead of after a full upload and a rejection email.
xcrun altool --validate-app \
  --file "$IPA_PATH" \
  --type ios \
  --apiKey "$ASC_KEY_ID" \
  --apiIssuer "$ASC_ISSUER_ID"

echo "Uploading $IPA_PATH to App Store Connect…"
xcrun altool --upload-app \
  --file "$IPA_PATH" \
  --type ios \
  --apiKey "$ASC_KEY_ID" \
  --apiIssuer "$ASC_ISSUER_ID"

echo "Upload accepted. The build will appear under TestFlight once Apple finishes"
echo "processing it (usually 5–15 minutes), and can then be attached to iOS 1.0."
