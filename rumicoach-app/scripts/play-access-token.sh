#!/usr/bin/env bash
# Print an androidpublisher access token, minted from Bitbucket's OIDC identity.
#
#   PLAY_ACCESS_TOKEN=$(bash scripts/play-access-token.sh)
#
# Keyless by necessity: the org enforces iam.disableServiceAccountKeyCreation, so there is no
# service-account JSON to download. The step exchanges its OIDC token for federated
# credentials via STS, then impersonates the deployer service account.
#
# Requires, and the step must declare oidc: true:
#   BITBUCKET_STEP_OIDC_TOKEN   injected by Bitbucket
#   GCP_WIF_PROVIDER            workload identity provider resource name
#   GCP_SERVICE_ACCOUNT         deployer SA email to impersonate
#
# Extracted from play-upload.sh so the AAB upload and the product sync cannot drift apart on
# how they authenticate. Locally, skip this and use:
#   gcloud auth print-access-token --impersonate-service-account=$GCP_SERVICE_ACCOUNT

set -euo pipefail

: "${BITBUCKET_STEP_OIDC_TOKEN:?not set — does the step have oidc: true?}"
: "${GCP_WIF_PROVIDER:?not set}"
: "${GCP_SERVICE_ACCOUNT:?not set}"

# python3 ships with macOS, jq does not.
json() { python3 -c "import json,sys; print(json.load(sys.stdin)['$1'])"; }

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

curl -sS -X POST \
  "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/${GCP_SERVICE_ACCOUNT}:generateAccessToken" \
  -H "Authorization: Bearer ${STS_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d '{"scope":["https://www.googleapis.com/auth/androidpublisher"]}' | json accessToken
