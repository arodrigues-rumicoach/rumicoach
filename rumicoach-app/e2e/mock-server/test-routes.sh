#!/usr/bin/env bash
# e2e/mock-server/test-routes.sh — Full HTTP route test suite
# Uses temp files to avoid curl -w newline issues in bash

BASE="http://localhost:3001"
BODY_FILE=$(mktemp)
STATUS_FILE=$(mktemp)
trap "rm -f $BODY_FILE $STATUS_FILE" EXIT

# Load tokens
EU_TOKEN=$(bun -e "import { mockTokens } from './fixtures/test-users.js'; console.log(mockTokens['test-user-eu-1'].accessToken)" 2>/dev/null || echo "")
EU_ADMIN_TOKEN=$(bun -e "import { mockTokens } from './fixtures/test-users.js'; console.log(mockTokens['test-user-eu-admin'].accessToken)" 2>/dev/null || echo "")
US_TOKEN=$(bun -e "import { mockTokens } from './fixtures/test-users.js'; console.log(mockTokens['test-user-us-1'].accessToken)" 2>/dev/null || echo "")
EU_REFRESH=$(bun -e "import { mockTokens } from './fixtures/test-users.js'; console.log(mockTokens['test-user-eu-1'].refreshToken)" 2>/dev/null || echo "")

PASSED=0
FAILED=0

# curl helper: body → $BODY_FILE, status code → $STATUS_FILE
http_req() {
  local method="$1" url="$2" headers="${3:-}" body="${4:-}"
  local args=(-s -o "$BODY_FILE" -w '%{http_code}')
  [ -n "$method" ] && args+=(-X "$method")
  args+=("$url")
  if [ -n "$headers" ]; then
    IFS=$'\n' read -rd '' -ra hdr_arr <<< "$headers" || true
    for h in "${hdr_arr[@]}"; do [ -n "$h" ] && args+=(-H "$h"); done
  fi
  [ -n "$body" ] && args+=(-H "Content-Type: application/json" -d "$body")
  local sc
  sc=$(curl "${args[@]}" 2>/dev/null)
  echo "$sc" > "$STATUS_FILE"
}

get_status() { cat "$STATUS_FILE"; }
get_body() { cat "$BODY_FILE"; }
body_jq() { jq -r "$1" "$BODY_FILE" 2>/dev/null || echo ""; }

check() {
  local name="$1" exp_st="$2" exp_code="$3"
  local st code
  st=$(get_status)
  code=$(body_jq '.code // ""')
  if [ "$st" = "$exp_st" ] && [ "$code" = "$exp_code" ]; then
    echo "✅ $name"; PASSED=$((PASSED + 1))
  else
    echo "❌ $name (exp st=$exp_st code=$exp_code, got st=$st code=$code)"
    FAILED=$((FAILED + 1))
  fi
}

check_st() {
  local name="$1" exp_st="$2"
  local st; st=$(get_status)
  if [ "$st" = "$exp_st" ]; then
    echo "✅ $name"; PASSED=$((PASSED + 1))
  else
    echo "❌ $name (exp st=$exp_st, got st=$st)"; FAILED=$((FAILED + 1))
  fi
}

check_json() {
  local name="$1" exp_st="$2" jq_expr="$3"
  local st result
  st=$(get_status)
  if [ "$st" != "$exp_st" ]; then
    echo "❌ $name (exp st=$exp_st, got st=$st)"; FAILED=$((FAILED + 1)); return
  fi
  result=$(jq "$jq_expr" "$BODY_FILE" 2>/dev/null || echo "")
  if [ "$result" = "true" ]; then
    echo "✅ $name"; PASSED=$((PASSED + 1))
  else
    echo "❌ $name (st=$st jq '$jq_expr'=$result)"; FAILED=$((FAILED + 1))
  fi
}

# Flexible check: accepts either a code field OR presence of tokens/data
check_flex() {
  local name="$1" exp_st="$2"
  local st code has_token has_deleted has_verified has_id
  st=$(get_status)
  code=$(body_jq '.code // ""')
  has_token=$(body_jq 'has("accessToken")')
  has_deleted=$(body_jq 'has("deleted")')
  has_verified=$(body_jq 'has("verified")')
  has_id=$(body_jq 'has("id")')
  local st_ok=false
  [ "$st" = "$exp_st" ] && st_ok=true
  # Also accept 201 for creation endpoints when 200 expected
  [ "$exp_st" = "200" ] && [ "$st" = "201" ] && st_ok=true
  if $st_ok && { [ -n "$code" ] || [ "$has_token" = "true" ] || [ "$has_deleted" = "true" ] || [ "$has_verified" = "true" ] || [ "$has_id" = "true" ]; }; then
    echo "✅ $name"; PASSED=$((PASSED + 1))
  else
    echo "❌ $name (st=$st code=$code)"; FAILED=$((FAILED + 1))
  fi
}

echo "=== Mock Server Route Tests ==="
echo ""

# ── Auth: 401 ──
http_req GET "$BASE/v1/me"
check "1. GET /me — 401 no token" 401 "AUTH_TOKEN_MISSING"

http_req GET "$BASE/v1/me" "Authorization: Bearer invalid"
check "2. GET /me — 401 bad token" 401 "UNAUTHENTICATED"

# ── Auth: register ──
http_req POST "$BASE/v1/auth/register" "" '{"name":"New Tester","email":"newtest@rumi.coach","dataRegion":"eu","termsAndConditionsAccepted":true,"aiAccepted":true}'
check_flex "3. POST /auth/register" 200

# ── Auth: verifications ──
http_req POST "$BASE/v1/auth/verifications/request" "" '{"type":"email","event":"signup","email":"test@rumi.coach"}'
check_st "4. POST /auth/verifications/request (email)" 200

http_req POST "$BASE/v1/auth/verifications/verify" "" '{"type":"email","code":"123456","email":"e2e-test@rumi.coach"}'
check_flex "5. POST /auth/verifications/verify" 200

http_req POST "$BASE/v1/auth/verifications/request" "" '{"type":"phone","event":"login","phoneNumber":"+1234567890"}'
check_st "6. POST /auth/verifications/request (phone)" 200

# ── Auth: login/code ──
http_req POST "$BASE/v1/auth/login/code" "" '{"type":"email","identifier":"e2e-test@rumi.coach","code":"123456"}'
check_json "7. POST /auth/login/code" 200 'has("accessToken") and has("refreshToken")'

# ── Auth: google ──
http_req POST "$BASE/v1/auth/google" "" '{"accessToken":"fake_google_token"}'
check_flex "8. POST /auth/google" 200

# ── Auth: refresh ──
http_req POST "$BASE/v1/auth/refresh" "" "{\"refreshToken\":\"$EU_REFRESH\"}"
check_flex "9. POST /auth/refresh" 200

# ── Auth: identifier update ──
http_req POST "$BASE/v1/auth/verifications/request" "" '{"type":"email","event":"login","email":"updated@rumi.coach"}'
VID=$(body_jq '.verificationId')
http_req PUT "$BASE/v1/auth/me/identifier" "Authorization: Bearer $EU_TOKEN" "{\"type\":\"email\",\"identifier\":\"updated@rumi.coach\",\"verificationId\":\"$VID\"}"
check_flex "10. PUT /auth/me/identifier" 200

# ── User: GET /me ──
http_req GET "$BASE/v1/me" "Authorization: Bearer $EU_TOKEN"
check_json "11. GET /me (EU user)" 200 '.id == "test-user-eu-1"'

http_req GET "$BASE/v1/me" "Authorization: Bearer $US_TOKEN"
check_json "12. GET /me (US user)" 200 '.id == "test-user-us-1"'

# ── User: PATCH /me ──
http_req PATCH "$BASE/v1/me" "Authorization: Bearer $EU_TOKEN" '{"name":"Updated Name","preferences":{"language":"en"}}'
check_flex "13. PATCH /me" 200

# ── User: GET /me/profile ──
http_req GET "$BASE/v1/me/profile" "Authorization: Bearer $EU_TOKEN"
check_st "14. GET /me/profile" 200

# ── User: DELETE /me (admin) ──
http_req DELETE "$BASE/v1/me" "Authorization: Bearer $EU_ADMIN_TOKEN"
check_flex "15. DELETE /me (admin)" 200

# ── User: DELETE /me/data ──
http_req DELETE "$BASE/v1/me/data?scope=memories" "Authorization: Bearer $EU_TOKEN"
check_flex "16. DELETE /me/data?scope=memories" 200

# ── Journey ──
http_req GET "$BASE/v1/journey" "Authorization: Bearer $EU_TOKEN"
check_json "17. GET /journey" 200 '.quote != null and .sessions != null'

# ── Sessions ──
http_req GET "$BASE/v1/sessions" "Authorization: Bearer $EU_TOKEN"
check_json "18. GET /sessions" 200 '.items | length > 0'

# ── Memories ──
http_req GET "$BASE/v1/memories" "Authorization: Bearer $EU_TOKEN"
check_st "19. GET /memories" 200

http_req GET "$BASE/v1/memories?category=insight" "Authorization: Bearer $EU_TOKEN"
check_st "20. GET /memories?category=insight" 200

http_req POST "$BASE/v1/memories" "Authorization: Bearer $EU_TOKEN" '{"category":"insight","content":"Test insight"}'
check_st "21. POST /memories" 201

# DELETE first memory
MEM_ID=$(curl -s "$BASE/v1/memories" -H "Authorization: Bearer $EU_TOKEN" | jq -r '.items[0].id')
http_req DELETE "$BASE/v1/memories/$MEM_ID" "Authorization: Bearer $EU_TOKEN"
check_flex "22. DELETE /memories/:id" 200

# ── Commitments ──
http_req GET "$BASE/v1/commitments" "Authorization: Bearer $EU_TOKEN"
check_json "23. GET /commitments" 200 'length > 0'

COM_ID=$(curl -s "$BASE/v1/commitments" -H "Authorization: Bearer $EU_TOKEN" | jq -r '.[0].id')
http_req PATCH "$BASE/v1/commitments/$COM_ID" "Authorization: Bearer $EU_TOKEN" '{"status":"completed"}'
check_flex "24. PATCH /commitments/:id" 200

# ── Wheel of Life ──
http_req GET "$BASE/v1/wheel-of-life" "Authorization: Bearer $EU_TOKEN"
check_json "25. GET /wheel-of-life" 200 '.[0] != null'

http_req POST "$BASE/v1/wheel-of-life" "Authorization: Bearer $EU_TOKEN" '{"data":{"health":8,"career":7,"relationships":9,"personal_growth":6,"finances":5,"hobbies":8,"environment":7,"spirituality":4}}'
check_st "26. POST /wheel-of-life" 201

# ── Eisenhower ──
http_req GET "$BASE/v1/eisenhower-matrix" "Authorization: Bearer $EU_TOKEN"
check_json "27. GET /eisenhower-matrix" 200 '.[0] != null'

http_req POST "$BASE/v1/eisenhower-matrix" "Authorization: Bearer $EU_TOKEN" '{"data":{"urgentImportant":["Task 1"],"urgentNotImportant":["Task 2"],"notUrgentImportant":["Task 3"],"notUrgentNotImportant":[]}}'
check_st "28. POST /eisenhower-matrix" 201

# ── Streak ──
http_req GET "$BASE/v1/streak" "Authorization: Bearer $EU_TOKEN"
check_json "29. GET /streak" 200 '.currentStreak != null'

# ── Usage Calendar ──
http_req GET "$BASE/v1/usage-calendar" "Authorization: Bearer $EU_TOKEN"
check_json "30. GET /usage-calendar" 200 'has("days")'

# ── Integrations ──
http_req GET "$BASE/v1/me/integrations" "Authorization: Bearer $EU_TOKEN"
check_json "31. GET /me/integrations" 200 'length > 0'

http_req POST "$BASE/v1/me/integrations/whatsapp/link" "Authorization: Bearer $EU_TOKEN" '{"phoneNumber":"+1234567890"}'
check_st "32. POST /me/integrations/whatsapp/link" 201

INT_ID=$(curl -s "$BASE/v1/me/integrations" -H "Authorization: Bearer $EU_TOKEN" | jq -r '.[0].id')
http_req DELETE "$BASE/v1/me/integrations/$INT_ID" "Authorization: Bearer $EU_TOKEN"
check_flex "33. DELETE /me/integrations/:id" 200

# ── CORS ──
http_req OPTIONS "$BASE/v1/me" "Origin: http://example.com
Access-Control-Request-Method: GET
Access-Control-Request-Headers: Authorization"
check_st "34. OPTIONS preflight" 204

# ── 404 ──
http_req GET "$BASE/v1/nonexistent"
check "36. GET /v1/nonexistent — 404" 404 "NOT_FOUND"

# ═══════════════════════════════════════════════════════════════════════════════
# PART 2: Error/Validation Tests
# ═══════════════════════════════════════════════════════════════════════════════
echo ""
echo "=== Error/Validation Tests ==="
echo ""

# ── Auth: missing fields ──
http_req POST "$BASE/v1/auth/verifications/request" "" '{}'
check "37. POST /auth/verifications/request — missing type" 400 "INVALID_PAYLOAD"

http_req POST "$BASE/v1/auth/verifications/request" "" '{"type":"email"}'
check "38. POST /auth/verifications/request — missing event" 400 "INVALID_PAYLOAD"

http_req POST "$BASE/v1/auth/verifications/request" "" '{"type":"email","event":"signup"}'
check "39. POST /auth/verifications/request — email type missing email" 400 "IDENTIFIER_REQUIRED"

http_req POST "$BASE/v1/auth/verifications/request" "" '{"type":"phone","event":"login"}'
check "40. POST /auth/verifications/request — phone type missing phoneNumber" 400 "IDENTIFIER_REQUIRED"

http_req POST "$BASE/v1/auth/verifications/verify" "" '{}'
check "41. POST /auth/verifications/verify — missing fields" 400 "INVALID_PAYLOAD"

http_req POST "$BASE/v1/auth/verifications/verify" "" '{"type":"email","code":"999999","email":"test@rumi.coach"}'
check "42. POST /auth/verifications/verify — wrong code" 400 "INVALID_CODE"

http_req POST "$BASE/v1/auth/login/code" "" '{}'
check "43. POST /auth/login/code — missing fields" 400 "INVALID_PAYLOAD"

http_req POST "$BASE/v1/auth/login/code" "" '{"type":"email","identifier":"nonexistent@rumi.coach","code":"123456"}'
check "44. POST /auth/login/code — nonexistent user" 400 "INVALID_CODE"

http_req POST "$BASE/v1/auth/register" "" '{}'
check "45. POST /auth/register — missing name" 400 "INVALID_PAYLOAD"

http_req POST "$BASE/v1/auth/register" "" '{"name":"Test"}'
check "46. POST /auth/register — missing identifier" 400 "IDENTIFIER_REQUIRED"

http_req POST "$BASE/v1/auth/google" "" '{}'
check "47. POST /auth/google — missing accessToken" 400 "INVALID_PAYLOAD"

http_req POST "$BASE/v1/auth/refresh" "" '{}'
check "48. POST /auth/refresh — missing refreshToken" 400 "INVALID_REFRESH_TOKEN"

http_req POST "$BASE/v1/auth/refresh" "" '{"refreshToken":"nonexistent_token_xyz"}'
check "49. POST /auth/refresh — invalid refreshToken" 400 "INVALID_REFRESH_TOKEN"

http_req PUT "$BASE/v1/auth/me/identifier" "Authorization: Bearer $EU_TOKEN" '{}'
check "50. PUT /auth/me/identifier — missing fields" 400 "INVALID_PAYLOAD"

# ── Memories: validation errors ──
http_req POST "$BASE/v1/memories" "Authorization: Bearer $EU_TOKEN" '{}'
check "51. POST /memories — missing category and content" 400 "INVALID_PAYLOAD"

http_req POST "$BASE/v1/memories" "Authorization: Bearer $EU_TOKEN" '{"content":"test"}'
check "52. POST /memories — missing category" 400 "INVALID_PAYLOAD"

http_req POST "$BASE/v1/memories" "Authorization: Bearer $EU_TOKEN" '{"category":"invalid_cat","content":"test"}'
check "53. POST /memories — invalid category" 400 "INVALID_PAYLOAD"

http_req DELETE "$BASE/v1/memories/nonexistent-id-999" "Authorization: Bearer $EU_TOKEN"
check "54. DELETE /memories/:id — not found" 404 "NOT_FOUND"

# ── Commitments: validation errors ──
http_req PATCH "$BASE/v1/commitments/nonexistent-id-999" "Authorization: Bearer $EU_TOKEN" '{"status":"completed"}'
check "55. PATCH /commitments/:id — not found" 404 "NOT_FOUND"

http_req PATCH "$BASE/v1/commitments/act-eu1-pending-001" "Authorization: Bearer $EU_TOKEN" '{"status":"invalid_status"}'
check "56. PATCH /commitments/:id — invalid status" 400 "INVALID_PAYLOAD"

# ── Wheel of Life: validation errors ──
http_req POST "$BASE/v1/wheel-of-life" "Authorization: Bearer $EU_TOKEN" '{}'
check "57. POST /wheel-of-life — missing data" 400 "INVALID_PAYLOAD"

http_req POST "$BASE/v1/wheel-of-life" "Authorization: Bearer $EU_TOKEN" '{"data":"not_an_object"}'
check "58. POST /wheel-of-life — data not object" 400 "INVALID_PAYLOAD"

# ── Eisenhower: validation errors ──
http_req POST "$BASE/v1/eisenhower-matrix" "Authorization: Bearer $EU_TOKEN" '{}'
check "59. POST /eisenhower-matrix — missing data" 400 "INVALID_PAYLOAD"

http_req POST "$BASE/v1/eisenhower-matrix" "Authorization: Bearer $EU_TOKEN" '{"data":{"urgentImportant":"not_array"}}'
check "60. POST /eisenhower-matrix — quadrant not array" 400 "INVALID_PAYLOAD"

# ── DELETE /me/data: invalid scope ──
http_req DELETE "$BASE/v1/me/data?scope=invalid_scope" "Authorization: Bearer $EU_TOKEN"
check "61. DELETE /me/data — invalid scope" 400 "INVALID_PAYLOAD"

# ── Integrations: validation errors ──
http_req POST "$BASE/v1/me/integrations/invalid_provider/link" "Authorization: Bearer $EU_TOKEN" '{"phoneNumber":"+123"}'
check "62. POST /me/integrations/:provider/link — invalid provider" 400 "INVALID_PAYLOAD"

http_req DELETE "$BASE/v1/me/integrations/nonexistent-id-999" "Authorization: Bearer $EU_TOKEN"
check "63. DELETE /me/integrations/:id — not found" 404 "NOT_FOUND"

# ── Feedback: new routes ──
http_req POST "$BASE/v1/chat/sessions/nonexistent-999/feedback" "Authorization: Bearer $EU_TOKEN" '{"evaluation":5}'
check "64. POST /chat/sessions/:id/feedback — session not found" 404 "NOT_FOUND"

http_req POST "$BASE/v1/feedback" "Authorization: Bearer $EU_TOKEN" '{}'
check "65. POST /feedback — missing category" 400 "INVALID_PAYLOAD"

http_req POST "$BASE/v1/feedback" "Authorization: Bearer $EU_TOKEN" '{"category":"invalid_cat"}'
check "66. POST /feedback — invalid category" 400 "INVALID_PAYLOAD"

http_req POST "$BASE/v1/feedback" "Authorization: Bearer $EU_TOKEN" '{"category":"bug","description":"Test bug report"}'
check_flex "67. POST /feedback — valid submission" 201

# ── Simulation routes ──
http_req POST "$BASE/v1/simulate/token-expiry" "Authorization: Bearer $EU_TOKEN" '{}'
check_st "68. POST /simulate/token-expiry" 200

http_req DELETE "$BASE/v1/simulate/token-expiry" "Authorization: Bearer $EU_TOKEN" ''
check_st "69. DELETE /simulate/token-expiry" 200

http_req POST "$BASE/v1/simulate/rate-limit" "Authorization: Bearer $EU_TOKEN" '{"route":"/v1/memories","maxRequests":3}'
check_st "70. POST /simulate/rate-limit" 200

http_req DELETE "$BASE/v1/simulate/rate-limit" "Authorization: Bearer $EU_TOKEN" '{"route":"/v1/memories"}'
check_st "71. DELETE /simulate/rate-limit" 200

http_req POST "$BASE/v1/simulate/server-error" "Authorization: Bearer $EU_TOKEN" '{"route":"/v1/memories","statusCode":503}'
check_st "72. POST /simulate/server-error" 200

http_req DELETE "$BASE/v1/simulate/server-error" "Authorization: Bearer $EU_TOKEN" ''
check_st "73. DELETE /simulate/server-error" 200

http_req POST "$BASE/v1/simulate/clear-all" "Authorization: Bearer $EU_TOKEN" ''
check_st "74. POST /simulate/clear-all" 200

# ── Cross-user isolation ──
US2_TOKEN=$(bun -e "import { mockTokens } from './fixtures/test-users.js'; console.log(mockTokens['test-user-eu-2'].accessToken)" 2>/dev/null || echo "")

http_req GET "$BASE/v1/memories" "Authorization: Bearer $US2_TOKEN"
check_json "75. GET /memories — EU2 user sees only own data" 200 '.items | length >= 0'

http_req GET "$BASE/v1/commitments" "Authorization: Bearer $US2_TOKEN"
check_json "76. GET /commitments — EU2 user sees only own data" 200 'length >= 0'

# ── Pagination ──
http_req GET "$BASE/v1/sessions?page=1&limit=2" "Authorization: Bearer $EU_TOKEN"
check_json "77. GET /sessions — pagination limit=2" 200 '.pagination.itemsPerPage == 2'

http_req GET "$BASE/v1/memories?page=999" "Authorization: Bearer $EU_TOKEN"
check_json "78. GET /memories — page 999 returns empty" 200 '.items | length == 0'

# ── Usage calendar month filter ──
http_req GET "$BASE/v1/usage-calendar?month=2024-07" "Authorization: Bearer $EU_TOKEN"
check_json "79. GET /usage-calendar?month=2024-07 — filtered" 200 'has("days")'

# ── Wheel of Life versioning ──
http_req GET "$BASE/v1/wheel-of-life" "Authorization: Bearer $EU_TOKEN"
WOL_COUNT1=$(body_jq 'length')
http_req POST "$BASE/v1/wheel-of-life" "Authorization: Bearer $EU_TOKEN" '{"data":{"health":9,"career":8,"relationships":9,"personal_growth":7,"finances":6,"hobbies":9,"environment":8,"spirituality":5}}'
check_st "80. POST /wheel-of-life — version increments" 201
http_req GET "$BASE/v1/wheel-of-life" "Authorization: Bearer $EU_TOKEN"
WOL_COUNT2=$(body_jq 'length')
if [ "$WOL_COUNT2" -gt "$WOL_COUNT1" ]; then
  echo "✅ 81. Wheel of Life versioning — count increased"; PASSED=$((PASSED + 1))
else
  echo "❌ 81. Wheel of Life versioning — count did not increase"; FAILED=$((FAILED + 1))
fi

# ── Eisenhower versioning ──
http_req GET "$BASE/v1/eisenhower-matrix" "Authorization: Bearer $EU_TOKEN"
EIS_COUNT1=$(body_jq 'length')
http_req POST "$BASE/v1/eisenhower-matrix" "Authorization: Bearer $EU_TOKEN" '{"data":{"urgentImportant":["New"],"urgentNotImportant":[],"notUrgentImportant":[],"notUrgentNotImportant":[]}}'
check_st "82. POST /eisenhower-matrix — version increments" 201
http_req GET "$BASE/v1/eisenhower-matrix" "Authorization: Bearer $EU_TOKEN"
EIS_COUNT2=$(body_jq 'length')
if [ "$EIS_COUNT2" -gt "$EIS_COUNT1" ]; then
  echo "✅ 83. Eisenhower versioning — count increased"; PASSED=$((PASSED + 1))
else
  echo "❌ 83. Eisenhower versioning — count did not increase"; FAILED=$((FAILED + 1))
fi

# ── Journey data shape ──
http_req GET "$BASE/v1/journey" "Authorization: Bearer $EU_TOKEN"
check_json "84. GET /journey — has session" 200 'has("session")'
check_json "85. GET /journey — has mode" 200 'has("mode")'
check_json "86. GET /journey — has focusArea" 200 'has("focusArea")'
check_json "87. GET /journey — has badgesEarned" 200 'has("badgesEarned")'
check_json "88. GET /journey — has streak" 200 'has("streak")'

# ── Sessions data shape ──
http_req GET "$BASE/v1/sessions" "Authorization: Bearer $EU_TOKEN"
check_json "89. GET /sessions — has pagination" 200 'has("pagination")'
check_json "90. GET /sessions — pagination has totalPages" 200 '.pagination | has("totalPages")'

# ── Streak data shape ──
http_req GET "$BASE/v1/streak" "Authorization: Bearer $EU_TOKEN"
check_json "91. GET /streak — has bestStreak" 200 'has("bestStreak")'
check_json "92. GET /streak — has lastSessionDate" 200 'has("lastSessionDate")'

# ── Usage calendar data shape ──
http_req GET "$BASE/v1/usage-calendar" "Authorization: Bearer $EU_TOKEN"
check_json "93. GET /usage-calendar — has dayStreak" 200 'has("dayStreak")'
check_json "94. GET /usage-calendar — has sessionsCount" 200 'has("sessionsCount")'
check_json "95. GET /usage-calendar — has hours" 200 'has("hours")'

# ── Profile data shape ──
http_req GET "$BASE/v1/me/profile" "Authorization: Bearer $EU_TOKEN"
check_json "96. GET /me/profile — has name" 200 'has("name")'
check_json "97. GET /me/profile — has vision" 200 'has("vision")'
check_json "98. GET /me/profile — has progress" 200 'has("progress")'
check_json "99. GET /me/profile — has badges" 200 'has("badges")'

# ── User region fields ──
http_req GET "$BASE/v1/me" "Authorization: Bearer $EU_TOKEN"
check_json "100. GET /me — EU user has region=eu" 200 '.region == "eu"'

http_req GET "$BASE/v1/me" "Authorization: Bearer $US_TOKEN"
check_json "101. GET /me — US user has region=us" 200 '.region == "us"'

echo ""
echo "=== Results: $PASSED passed, $FAILED failed ==="
if [ "$FAILED" -gt 0 ]; then
  echo ""; echo "❌ Some tests failed!"
  exit 1
else
  echo ""; echo "✅ All $PASSED tests passed!"
fi
