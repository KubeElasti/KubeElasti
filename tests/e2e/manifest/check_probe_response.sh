#!/bin/sh
# Verify ElastiService probeResponse rules are applied by the resolver (synthetic responses).
#
# This script is invoked TWICE with different first arguments — only one branch runs per invocation:
#   check_probe_response.sh initial   → exercises /healthz and /hook only (no /probe).
#   check_probe_response.sh updated   → exercises /probe only (must run AFTER the patch; see below).
#
# Kuttl test 11-probe-responses runs steps in filename order; /probe is added before "updated":
#   02-check-probe-response-initial.yaml  → initial
#   03-patch-probe-response.yaml        → kubectl patch adds GET /probe -> alive
#   04-wait-resolver-cache.yaml         → sleep for resolver CRD cache
#   05-check-probe-response-updated.yaml → updated
#
# Phases:
#   initial — From target-elastiservice.yaml:
#             GET  /healthz (PathPrefix) -> ok
#             POST /hook   (Exact)      -> {"ready":true}
#   updated — After patch_elastiservice_probe_response.sh (GET /probe -> alive); also verifies /healthz no longer returns "ok".
#
set -u

PHASE="${1:?usage: check_probe_response.sh <initial|updated> <namespace>}"
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

echo "${CYAN}=== Probe response E2E (${PHASE}) ===${NC}"
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

    echo "${GREEN}PASSED: initial probeResponse rules (PathPrefix /healthz, Exact POST /hook).${NC}"
    ;;

  updated)
    # /probe is not in target-elastiservice.yaml; tests/e2e/tests/11-probe-responses/03-patch-probe-response.yaml
    # runs patch_elastiservice_probe_response.sh to set GET /probe -> alive before this phase.
    echo "  Expect GET ${BASE_URL}/probe -> alive (patched rule)"
    body=$(curl_exec --max-time 30 -s "${BASE_URL}/probe")
    if [ "$body" != "alive" ]; then
      echo "${RED}FAILED: GET /probe expected 'alive', got '${body}'${NC}"
      kubectl logs -n elasti services/elasti-resolver-service --all-pods=true --tail=40 | sed 's/^/  /' || true
      exit 1
    fi

    echo "  Expect GET ${BASE_URL}/healthz to NOT return synthetic 'ok' (probeResponse list replaced)"
    body=$(curl_exec --max-time 15 -s "${BASE_URL}/healthz" || true)
    if [ "$body" = "ok" ]; then
      echo "${RED}FAILED: /healthz still returned probeResponse 'ok' — resolver cache may not have picked up the patch yet.${NC}"
      exit 1
    fi

    echo "${GREEN}PASSED: updated probeResponse (GET /probe); old /healthz rule no longer applies.${NC}"
    ;;

  *)
    echo "${RED}Unknown phase: $PHASE (use initial or updated)${NC}"
    exit 2
    ;;
esac
