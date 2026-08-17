#!/usr/bin/env bash
# Print the Bitbucket repository variables the iOS pipeline step needs, derived from the
# signing assets on this machine rather than typed by hand.
#
#   scripts/bitbucket-ios-vars.sh            checklist + per-variable copy commands
#   scripts/bitbucket-ios-vars.sh --reveal   same, but prints the base64 blobs in full
#   scripts/bitbucket-ios-vars.sh --copy IOS_DIST_CERT_P12_B64
#
# Add them at:
#   https://bitbucket.org/rumicoach/rumicoach-app/admin/pipelines/repository-variables
#
# The two base64 blobs are a private key and a signing identity, so the default output
# truncates them — a terminal scrollback is easy to paste somewhere it shouldn't go. Use
# --copy to move one straight to the clipboard, or --reveal if you want them on screen.
#
# Paths can be overridden by env var if your assets live elsewhere.

set -uo pipefail

SIGNING_DIR="${SIGNING_DIR:-$HOME/ios-signing}"
P12="${P12:-$SIGNING_DIR/dist.p12}"
PROFILE="${PROFILE:-$(ls "$SIGNING_DIR"/*.mobileprovision 2>/dev/null | head -1)}"
P8="${P8:-$(ls "$HOME"/.appstoreconnect/AuthKey_*.p8 2>/dev/null | head -1)}"

# Not a secret — an account-wide identifier, useless without the .p8. Overridable in case
# this script is ever reused for another Apple account.
ASC_ISSUER_ID="${ASC_ISSUER_ID:-cc3ceaec-0077-45dc-8222-57503d1d4047}"

MODE="${1:-}"
WANT="${2:-}"

# Decode the profile's own Name field rather than guessing from the filename: the pipeline
# passes this to PROVISIONING_PROFILE_SPECIFIER, which matches on the portal name and fails
# the archive if it is even slightly off.
profile_name() {
  [ -f "$PROFILE" ] || return 1
  # Via a temp file, not a pipe: plistlib.load() seeks, and stdin is not seekable.
  local tmp
  tmp=$(mktemp -t rumiprof) || return 1
  security cms -D -i "$PROFILE" >"$tmp" 2>/dev/null \
    && python3 -c 'import plistlib,sys; print(plistlib.load(open(sys.argv[1],"rb"))["Name"])' "$tmp" 2>/dev/null
  local rc=$?
  rm -f "$tmp"
  return $rc
}

b64() { [ -f "$1" ] && base64 -i "$1" || return 1; }

key_id() {
  [ -f "$P8" ] || return 1
  basename "$P8" | sed -E 's/AuthKey_(.*)\.p8/\1/'
}

# --copy: put one value on the clipboard and say nothing else.
if [ "$MODE" = "--copy" ]; then
  case "$WANT" in
    IOS_DIST_CERT_P12_B64)        b64 "$P12" | pbcopy ;;
    IOS_PROVISIONING_PROFILE_B64) b64 "$PROFILE" | pbcopy ;;
    ASC_KEY_P8_B64)               b64 "$P8" | pbcopy ;;
    IOS_PROVISIONING_PROFILE_NAME) profile_name | tr -d '\n' | pbcopy ;;
    ASC_KEY_ID)                   key_id | tr -d '\n' | pbcopy ;;
    ASC_ISSUER_ID)                printf '%s' "$ASC_ISSUER_ID" | pbcopy ;;
    *) echo "Don't know how to copy '$WANT'"; exit 1 ;;
  esac
  [ $? -eq 0 ] && echo "Copied $WANT to the clipboard." || { echo "Could not read the source file for $WANT"; exit 1; }
  exit 0
fi

REVEAL=0
[ "$MODE" = "--reveal" ] && REVEAL=1

# value, or a truncated preview, depending on --reveal
show() {
  local name="$1" value="$2"
  local n=${#value}
  if [ "$REVEAL" -eq 1 ] || [ "$n" -le 60 ]; then
    printf '%s\n' "$value"
  else
    printf '%s… (%d chars)\n' "${value:0:48}" "$n"
    printf '      copy with: scripts/bitbucket-ios-vars.sh --copy %s\n' "$name"
  fi
}

row() {
  local name="$1" value="$2" note="${3:-}"
  if [ -z "$value" ]; then
    printf '\n  [MISSING] %s\n' "$name"
    [ -n "$note" ] && printf '      %s\n' "$note"
  else
    printf '\n  [ok] %s\n      ' "$name"
    show "$name" "$value"
  fi
}

echo "Bitbucket repository variables — iOS pipeline"
echo "https://bitbucket.org/rumicoach/rumicoach-app/admin/pipelines/repository-variables"
echo
echo "Sources:"
printf '  p12     %s %s\n' "$P12" "$([ -f "$P12" ] && echo '' || echo '(not found)')"
printf '  profile %s %s\n' "${PROFILE:-<none>}" "$([ -f "${PROFILE:-}" ] && echo '' || echo '(not found)')"
printf '  p8      %s %s\n' "${P8:-<none>}" "$([ -f "${P8:-}" ] && echo '' || echo '(not found)')"
echo
echo "Leave 'Secured' ticked on everything except ASC_KEY_ID and ASC_ISSUER_ID —"
echo "those two are identifiers, and having them readable helps when debugging a failed upload."

row IOS_DIST_CERT_P12_B64 "$(b64 "$P12" 2>/dev/null)" \
  "Build it first: cd $SIGNING_DIR && openssl x509 -in embedded_cert.der -inform DER -out ios_dist.pem -outform PEM && openssl pkcs12 -export -inkey ios_dist.key -in ios_dist.pem -out dist.p12"

row IOS_DIST_CERT_PASSWORD "" \
  "Only you know this — it is the export password you typed when creating dist.p12."

row IOS_PROVISIONING_PROFILE_B64 "$(b64 "$PROFILE" 2>/dev/null)" \
  "No .mobileprovision found in $SIGNING_DIR"

row IOS_PROVISIONING_PROFILE_NAME "$(profile_name)" \
  "Could not decode a Name from the profile"

row ASC_KEY_ID "$(key_id)" \
  "No AuthKey_*.p8 found in ~/.appstoreconnect"

row ASC_ISSUER_ID "$ASC_ISSUER_ID"

row ASC_KEY_P8_B64 "$(b64 "$P8" 2>/dev/null)" \
  "No AuthKey_*.p8 found in ~/.appstoreconnect"

echo
echo "  [ok] IOS_KEYCHAIN_PASSWORD — any throwaway string; the pipeline creates and deletes"
echo "       that keychain within one run, so it never needs to be remembered."
echo "       Generate one with: openssl rand -base64 24"
echo
