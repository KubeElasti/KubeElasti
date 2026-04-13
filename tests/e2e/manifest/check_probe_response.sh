#!/bin/sh
# Verifies ElastiService spec.probeResponse is honored by the resolver in proxy mode (target at 0).
# Expects target-elastiservice from test-template with GET PathPrefix /healthz and POST Exact /hook.
#
# Retries: the resolver CR cache may not include probe rules until the next poll (see values-elasti.yaml).

set -u

TARGET_NAMESPACE=""
while [ "$#" -gt 0 ]; do
    case "$1" in
        --namespace)
            TARGET_NAMESPACE="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 --namespace <ns>"
            exit 2
            ;;
    esac
done

if [ -z "$TARGET_NAMESPACE" ]; then
    echo "Usage: $0 --namespace <ns>"
    exit 2
fi

CURL_POD_NAME="curl-target-gw"
CURL_NAMESPACE="default"
BASE_URL="http://target-deployment.${TARGET_NAMESPACE}.svc.cluster.local"
# Per-attempt cap: real probes return immediately; proxy+scale path can hang.
ATTEMPT_TIMEOUT=90
MAX_ROUNDS=24
SLEEP_BETWEEN=10

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

echo "${CYAN}=== probeResponse E2E ===${NC}"
echo "  Base URL:     $BASE_URL"
echo "  Namespace:    $TARGET_NAMESPACE"
echo "  Curl pod:     $CURL_POD_NAME ($CURL_NAMESPACE)"
echo "${CYAN}=========================${NC}"

normalize_body() {
    # trim whitespace / CR for stable compare
    printf '%s' "$1" | tr -d '\r\n' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//'
}

attempt=0
GET_OK=0
while [ "$attempt" -lt "$MAX_ROUNDS" ]; do
    attempt=$((attempt + 1))
    echo "Attempt ${attempt}/${MAX_ROUNDS}: GET ${BASE_URL}/healthz ..."

    if ! get_raw=$(kubectl exec -n "$CURL_NAMESPACE" "$CURL_POD_NAME" -- \
        curl -sS --max-time "$ATTEMPT_TIMEOUT" "${BASE_URL}/healthz" 2>&1); then
        echo "  curl exec failed: $get_raw"
        get_raw=""
    fi
    get_body=$(normalize_body "$get_raw")
    if [ "$get_body" = '{"ok":true}' ]; then
        GET_OK=1
        echo "${GREEN}GET /healthz returned probe body${NC}"
        break
    fi
    echo "  unexpected body (len=${#get_body}): $get_raw"
    sleep "$SLEEP_BETWEEN"
done

if [ "$GET_OK" != 1 ]; then
    echo "${RED}FAILED: GET /healthz never returned {\"ok\":true}${NC}"
    echo "${CYAN}Resolver logs:${NC}"
    kubectl logs -n elasti services/elasti-resolver-service --all-pods=true --tail=100 | sed 's/^/  /' || true
    exit 1
fi

echo "Checking POST ${BASE_URL}/hook ..."
if ! post_raw=$(kubectl exec -n "$CURL_NAMESPACE" "$CURL_POD_NAME" -- \
    curl -sS --max-time "$ATTEMPT_TIMEOUT" -X POST "${BASE_URL}/hook" 2>&1); then
    echo "${RED}FAILED: POST /hook curl error: $post_raw${NC}"
    exit 1
fi
post_body=$(normalize_body "$post_raw")
if [ "$post_body" != '{"ready":true}' ]; then
    echo "${RED}FAILED: POST /hook expected {\"ready\":true}, got: $post_raw${NC}"
    kubectl logs -n elasti services/elasti-resolver-service --all-pods=true --tail=100 | sed 's/^/  /' || true
    exit 1
fi
echo "${GREEN}POST /hook returned probe body${NC}"

echo "${GREEN}probeResponse E2E PASSED${NC}"
