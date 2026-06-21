#!/usr/bin/env bash
#
# smoke-test.sh — end-to-end smoke test for the Iris KumoMTA Admin UI backend.
#
# Validates the quickstart flow against a running backend:
#   - health (/healthz) and readiness (/readyz)
#   - create two VMTAs (unique IP / EHLO)
#   - create a weighted VMTA group referencing them
#   - create a mailclass routing rule and a recipient-domain routing rule
#   - list vmtas / vmta-groups / routing-rules and assert created items appear
#   - fetch /v1/dashboard/summary
#
# Usage:
#   ./scripts/smoke-test.sh
#   BASE_URL=http://localhost:8080 ./scripts/smoke-test.sh
#
# Requires: bash, curl. Uses jq when available (recommended), otherwise falls
# back to grep-based checks. Exits non-zero on the first failed step.

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"

# Unique suffix so re-runs do not collide on unique IP/EHLO/name constraints.
RUN_ID="$(date +%s)$$"

# --- output helpers ---------------------------------------------------------
if [ -t 1 ]; then
  C_GREEN='\033[0;32m'; C_RED='\033[0;31m'; C_BLUE='\033[0;34m'; C_RESET='\033[0m'
else
  C_GREEN=''; C_RED=''; C_BLUE=''; C_RESET=''
fi

STEP=0
pass() { printf "${C_GREEN}PASS${C_RESET} [%02d] %s\n" "$STEP" "$1"; }
info() { printf -- "${C_BLUE}----${C_RESET}      %s\n" "$1"; }
fail() {
  printf "${C_RED}FAIL${C_RESET} [%02d] %s\n" "$STEP" "$1" >&2
  local body="${2:-}"
  if [ -z "$body" ] && [ -f "${RESP_FILE:-}" ]; then
    body="$(cat "$RESP_FILE")"
  fi
  if [ -n "$body" ]; then
    printf "        response: %s\n" "$body" >&2
  fi
  exit 1
}
step() { STEP=$((STEP + 1)); }

# --- tooling ----------------------------------------------------------------
command -v curl >/dev/null 2>&1 || { echo "ERROR: curl is required" >&2; exit 2; }

HAVE_JQ=0
if command -v jq >/dev/null 2>&1; then
  HAVE_JQ=1
else
  info "jq not found; falling back to grep-based assertions"
fi

# http_call writes the response body to $RESP_FILE (a file, so it survives the
# command-substitution subshell used to capture the status code) and prints the
# HTTP status code. RESP_BODY mirrors the file for convenience in messages.
RESP_FILE="$(mktemp)"
trap 'rm -f "$RESP_FILE"' EXIT
RESP_BODY=""
http_call() {
  local method="$1" url="$2" body="${3:-}"
  local status
  if [ -n "$body" ]; then
    status="$(curl -sS -o "$RESP_FILE" -w '%{http_code}' \
      -X "$method" "$url" \
      -H 'Content-Type: application/json' \
      --data "$body")"
  else
    status="$(curl -sS -o "$RESP_FILE" -w '%{http_code}' -X "$method" "$url")"
  fi
  printf '%s' "$status"
}

# json_get EXPR  -> extract a value from the last response using jq.
json_get() { jq -r "$1" < "$RESP_FILE"; }

# body_contains STRING -> 0 if the last response contains STRING.
body_contains() { grep -qF -- "$1" "$RESP_FILE"; }

info "Target backend: $BASE_URL  (run id: $RUN_ID)"

# --- 1. health --------------------------------------------------------------
step
status="$(http_call GET "$BASE_URL/healthz")"
[ "$status" = "200" ] || fail "/healthz returned HTTP $status" "$RESP_BODY"
pass "/healthz is healthy (HTTP 200)"

step
status="$(http_call GET "$BASE_URL/readyz")"
[ "$status" = "200" ] || fail "/readyz returned HTTP $status" "$RESP_BODY"
pass "/readyz is ready (HTTP 200)"

# --- 2. create two VMTAs ----------------------------------------------------
VMTA1_NAME="smoke-vmta-a-$RUN_ID"
VMTA2_NAME="smoke-vmta-b-$RUN_ID"
# Use TEST-NET-3 (203.0.113.0/24, RFC 5737) addresses to avoid clashes.
OCTET1=$(( (RANDOM % 200) + 1 ))
OCTET2=$(( (RANDOM % 200) + 1 ))
VMTA1_IP="203.0.113.$OCTET1"
VMTA2_IP="203.0.113.$OCTET2"
[ "$VMTA1_IP" = "$VMTA2_IP" ] && VMTA2_IP="203.0.113.$(( (OCTET2 % 200) + 1 ))"
VMTA1_EHLO="a-$RUN_ID.smoke.example"
VMTA2_EHLO="b-$RUN_ID.smoke.example"

step
status="$(http_call POST "$BASE_URL/v1/vmtas" \
  "{\"name\":\"$VMTA1_NAME\",\"ip_address\":\"$VMTA1_IP\",\"ehlo_name\":\"$VMTA1_EHLO\"}")"
[ "$status" = "200" ] || fail "create VMTA #1 returned HTTP $status" "$RESP_BODY"
if [ "$HAVE_JQ" = "1" ]; then
  VMTA1_ID="$(json_get '.id')"
  [ -n "$VMTA1_ID" ] && [ "$VMTA1_ID" != "null" ] || fail "create VMTA #1: no id in response" "$RESP_BODY"
else
  body_contains "$VMTA1_NAME" || fail "create VMTA #1: name missing from response" "$RESP_BODY"
  VMTA1_ID="$(printf '%s' "$RESP_BODY" | grep -o '"id"[ ]*:[ ]*"[^"]*"' | head -1 | sed 's/.*"id"[ ]*:[ ]*"\([^"]*\)".*/\1/')"
fi
pass "created VMTA #1 ($VMTA1_NAME, $VMTA1_IP) id=$VMTA1_ID"

step
status="$(http_call POST "$BASE_URL/v1/vmtas" \
  "{\"name\":\"$VMTA2_NAME\",\"ip_address\":\"$VMTA2_IP\",\"ehlo_name\":\"$VMTA2_EHLO\"}")"
[ "$status" = "200" ] || fail "create VMTA #2 returned HTTP $status" "$RESP_BODY"
if [ "$HAVE_JQ" = "1" ]; then
  VMTA2_ID="$(json_get '.id')"
  [ -n "$VMTA2_ID" ] && [ "$VMTA2_ID" != "null" ] || fail "create VMTA #2: no id in response" "$RESP_BODY"
else
  body_contains "$VMTA2_NAME" || fail "create VMTA #2: name missing from response" "$RESP_BODY"
  VMTA2_ID="$(printf '%s' "$RESP_BODY" | grep -o '"id"[ ]*:[ ]*"[^"]*"' | head -1 | sed 's/.*"id"[ ]*:[ ]*"\([^"]*\)".*/\1/')"
fi
pass "created VMTA #2 ($VMTA2_NAME, $VMTA2_IP) id=$VMTA2_ID"

# --- 3. create weighted VMTA group ------------------------------------------
GROUP_NAME="smoke-group-$RUN_ID"
step
status="$(http_call POST "$BASE_URL/v1/vmta-groups" \
  "{\"name\":\"$GROUP_NAME\",\"members\":[{\"vmta_id\":\"$VMTA1_ID\",\"weight\":70},{\"vmta_id\":\"$VMTA2_ID\",\"weight\":30}]}")"
[ "$status" = "200" ] || fail "create VMTA group returned HTTP $status" "$RESP_BODY"
if [ "$HAVE_JQ" = "1" ]; then
  GROUP_ID="$(json_get '.id')"
  [ -n "$GROUP_ID" ] && [ "$GROUP_ID" != "null" ] || fail "create group: no id in response" "$RESP_BODY"
else
  body_contains "$GROUP_NAME" || fail "create group: name missing from response" "$RESP_BODY"
  GROUP_ID="$(printf '%s' "$RESP_BODY" | grep -o '"id"[ ]*:[ ]*"[^"]*"' | head -1 | sed 's/.*"id"[ ]*:[ ]*"\([^"]*\)".*/\1/')"
fi
pass "created weighted VMTA group ($GROUP_NAME, 70/30) id=$GROUP_ID"

# --- 4. create routing rules ------------------------------------------------
RULE_MC_NAME="smoke-rule-mailclass-$RUN_ID"
step
status="$(http_call POST "$BASE_URL/v1/routing-rules" \
  "{\"name\":\"$RULE_MC_NAME\",\"match_type\":\"mailclass\",\"match_value\":\"transactional\",\"priority\":100,\"target_type\":\"vmta_group\",\"target_id\":\"$GROUP_ID\"}")"
[ "$status" = "200" ] || fail "create mailclass routing rule returned HTTP $status" "$RESP_BODY"
if [ "$HAVE_JQ" = "1" ]; then
  RULE_MC_ID="$(json_get '.id')"
  [ -n "$RULE_MC_ID" ] && [ "$RULE_MC_ID" != "null" ] || fail "create mailclass rule: no id" "$RESP_BODY"
else
  body_contains "$RULE_MC_NAME" || fail "create mailclass rule: name missing" "$RESP_BODY"
fi
pass "created mailclass routing rule ($RULE_MC_NAME)"

RULE_RD_NAME="smoke-rule-domain-$RUN_ID"
step
status="$(http_call POST "$BASE_URL/v1/routing-rules" \
  "{\"name\":\"$RULE_RD_NAME\",\"match_type\":\"recipient_domain\",\"match_value\":\"example.com\",\"priority\":200,\"target_type\":\"vmta_group\",\"target_id\":\"$GROUP_ID\"}")"
[ "$status" = "200" ] || fail "create recipient-domain routing rule returned HTTP $status" "$RESP_BODY"
if [ "$HAVE_JQ" = "1" ]; then
  RULE_RD_ID="$(json_get '.id')"
  [ -n "$RULE_RD_ID" ] && [ "$RULE_RD_ID" != "null" ] || fail "create domain rule: no id" "$RESP_BODY"
else
  body_contains "$RULE_RD_NAME" || fail "create domain rule: name missing" "$RESP_BODY"
fi
pass "created recipient-domain routing rule ($RULE_RD_NAME)"

# --- 5. list and assert presence --------------------------------------------
step
status="$(http_call GET "$BASE_URL/v1/vmtas?page.page_size=200")"
[ "$status" = "200" ] || fail "list vmtas returned HTTP $status" "$RESP_BODY"
if [ "$HAVE_JQ" = "1" ]; then
  json_get ".items[].name" | grep -qx "$VMTA1_NAME" || fail "VMTA #1 not found in list" "$RESP_BODY"
  json_get ".items[].name" | grep -qx "$VMTA2_NAME" || fail "VMTA #2 not found in list" "$RESP_BODY"
else
  body_contains "$VMTA1_NAME" || fail "VMTA #1 not found in list" "$RESP_BODY"
  body_contains "$VMTA2_NAME" || fail "VMTA #2 not found in list" "$RESP_BODY"
fi
pass "both created VMTAs appear in /v1/vmtas"

step
status="$(http_call GET "$BASE_URL/v1/vmta-groups?page.page_size=200")"
[ "$status" = "200" ] || fail "list vmta-groups returned HTTP $status" "$RESP_BODY"
if [ "$HAVE_JQ" = "1" ]; then
  json_get ".items[].name" | grep -qx "$GROUP_NAME" || fail "group not found in list" "$RESP_BODY"
else
  body_contains "$GROUP_NAME" || fail "group not found in list" "$RESP_BODY"
fi
pass "created VMTA group appears in /v1/vmta-groups"

step
status="$(http_call GET "$BASE_URL/v1/routing-rules?page.page_size=200")"
[ "$status" = "200" ] || fail "list routing-rules returned HTTP $status" "$RESP_BODY"
if [ "$HAVE_JQ" = "1" ]; then
  json_get ".items[].name" | grep -qx "$RULE_MC_NAME" || fail "mailclass rule not found in list" "$RESP_BODY"
  json_get ".items[].name" | grep -qx "$RULE_RD_NAME" || fail "domain rule not found in list" "$RESP_BODY"
else
  body_contains "$RULE_MC_NAME" || fail "mailclass rule not found in list" "$RESP_BODY"
  body_contains "$RULE_RD_NAME" || fail "domain rule not found in list" "$RESP_BODY"
fi
pass "both routing rules appear in /v1/routing-rules"

# --- 6. dashboard summary ---------------------------------------------------
step
status="$(http_call GET "$BASE_URL/v1/dashboard/summary")"
[ "$status" = "200" ] || fail "dashboard summary returned HTTP $status" "$RESP_BODY"
if [ "$HAVE_JQ" = "1" ]; then
  SERVICE_STATE="$(json_get '.service_state // empty')"
  pass "fetched /v1/dashboard/summary (service_state=${SERVICE_STATE:-<empty>})"
else
  body_contains "service_state" || info "dashboard summary returned, service_state field not detected"
  pass "fetched /v1/dashboard/summary"
fi

# --- 7. KumoMTA config generation -------------------------------------------
step
status="$(http_call GET "$BASE_URL/v1/kumomta/config:generate")"
[ "$status" = "200" ] || fail "config generate returned HTTP $status" "$RESP_BODY"
if [ "$HAVE_JQ" = "1" ]; then
  body_contains "define_egress_source" || fail "generated config missing egress sources" "$RESP_BODY"
  CKSUM="$(json_get '.checksum')"
  [ -n "$CKSUM" ] && [ "$CKSUM" != "null" ] || fail "generated config has no checksum" "$RESP_BODY"
  pass "generated KumoMTA policy (vmtas=$(json_get '.vmtaCount') pools=$(json_get '.poolCount') routes=$(json_get '.routeCount'))"
else
  body_contains "define_egress_source" || fail "generated config missing egress sources" "$RESP_BODY"
  pass "generated KumoMTA policy"
fi

# --- 8. KumoMTA config apply (confirmation-gated) ---------------------------
step
status="$(http_call POST "$BASE_URL/v1/kumomta/config:apply" '{}')"
[ "$status" = "400" ] || fail "apply without confirmation should be 400, got $status" "$RESP_BODY"
pass "config apply correctly rejected without confirmation"

step
status="$(http_call POST "$BASE_URL/v1/kumomta/config:apply" "{\"confirmation_id\":\"smoke-$RUN_ID\"}")"
[ "$status" = "200" ] || fail "config apply returned HTTP $status" "$RESP_BODY"
if [ "$HAVE_JQ" = "1" ]; then
  APPLY_STATUS="$(json_get '.status')"
  [ "$APPLY_STATUS" = "succeeded" ] || fail "config apply status was '$APPLY_STATUS', expected succeeded" "$RESP_BODY"
  pass "applied KumoMTA config ($(json_get '.resultSummary'))"
else
  body_contains "succeeded" || fail "config apply did not succeed" "$RESP_BODY"
  pass "applied KumoMTA config"
fi

printf "\n${C_GREEN}ALL %d SMOKE STEPS PASSED${C_RESET}\n" "$STEP"
