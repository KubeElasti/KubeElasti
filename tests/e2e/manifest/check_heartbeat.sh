#!/bin/sh
# Verify ElastiService heartbeat rules are applied by the resolver (synthetic responses).
#
# Phases:
#   initial — Expectations from tests/e2e/manifest/test-template/target-elastiservice.yaml:
#             GET  /healthz (PathPrefix) -> ok
#             POST /hook   (Exact)      -> {"ready":true}
#   updated — After patch_elastiservice_heartbeat.sh:
#             GET /probe (Exact) -> alive
#
set -u

PHASE="${1:?usage: check_heartbeat.sh <initial|updated> <namespace>}"
NAMESPACE="${2:?namespace required}"

CURL_POD_NAME="curl-target-gw"
CURL_NAMESPACE="default"
BASE_URL="http://target-deployment.${NAMESPACE}.svc.cluster.local"

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

curl_exec() {
  kubectl exec -n "$CURL_NAMESPACE" "$CURL_POD_NAME" -- curl "$@"
}

echo "${CYAN}=== Heartbeat E2E (${PHASE}) ===${NC}"
echo "  Base URL: $BASE_URL"
echo "${CYAN}============================${NC}"

case "$PHASE" in
  initial)
    echo "  Expect GET ${BASE_URL}/healthz -> ok"
    body=$(curl_exec --max-time 30 -s "${BASE_URL}/healthz")
    if [ "$body" != "ok" ]; then
      echo "${RED}FAILED: GET /healthz expected body 'ok', got '${body}'${NC}"
      kubectl logs -n elasti services/elasti-resolver-service --all-pods=true --tail=40 | sed 's/^/  /' || true
      exit 1
    fi

    echo "  Expect POST ${BASE_URL}/hook -> {\"ready\":true}"
    body=$(curl_exec --max-time 30 -s -X POST "${BASE_URL}/hook")
    if [ "$body" != '{"ready":true}' ]; then
      echo "${RED}FAILED: POST /hook expected '{\"ready\":true}', got '${body}'${NC}"
      kubectl logs -n elasti services/elasti-resolver-service --all-pods=true --tail=40 | sed 's/^/  /' || true
      exit 1
    fi

    echo "${GREEN}PASSED: initial heartbeat rules (PathPrefix /healthz, Exact POST /hook).${NC}"
    ;;

  updated)
    echo "  Expect GET ${BASE_URL}/probe -> alive (patched rule)"
    body=$(curl_exec --max-time 30 -s "${BASE_URL}/probe")
    if [ "$body" != "alive" ]; then
      echo "${RED}FAILED: GET /probe expected 'alive', got '${body}'${NC}"
      kubectl logs -n elasti services/elasti-resolver-service --all-pods=true --tail=40 | sed 's/^/  /' || true
      exit 1
    fi

    echo "  Expect GET ${BASE_URL}/healthz to NOT return synthetic 'ok' (heartbeat list replaced)"
    body=$(curl_exec --max-time 15 -s "${BASE_URL}/healthz" || true)
    if [ "$body" = "ok" ]; then
      echo "${RED}FAILED: /healthz still returned heartbeat 'ok' — resolver cache may not have picked up the patch yet.${NC}"
      exit 1
    fi

    echo "${GREEN}PASSED: updated heartbeat (GET /probe); old /healthz rule no longer applies.${NC}"
    ;;

  *)
    echo "${RED}Unknown phase: $PHASE (use initial or updated)${NC}"
    exit 2
    ;;
esac
