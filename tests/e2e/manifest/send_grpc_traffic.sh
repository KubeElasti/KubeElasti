#!/bin/bash

# Enhanced gRPC traffic testing script with comprehensive debugging
set -u

# --- Color Definitions ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# --- Default Values ---
ADDR=""
TEST_TYPE="both"
CLIENT_NAMESPACE=""
TARGET_RESOURCE=""
TARGET_NAME=""
CLIENT_POD_NAME="grpc-client-pod"
MAX_RETRIES=5
RETRY_SLEEP=10

# --- Argument Parsing ---
while [ "$#" -gt 0 ]; do
    case "$1" in
        --addr)
            ADDR="$2"
            shift 2
            ;;
        --test)
            TEST_TYPE="$2"
            shift 2
            ;;
        --namespace)
            CLIENT_NAMESPACE="$2"
            shift 2
            ;;
        --target-resource)
            TARGET_RESOURCE="$2"
            shift 2
            ;;
        --target-name)
            TARGET_NAME="$2"
            shift 2
            ;;
        *)
            echo "${RED}Unknown option: $1${NC}"
            echo "Usage: $0 --addr <host:port> --namespace <ns> --target-resource <type> --target-name <name> [--test <unary|stream|both>]"
            exit 1
            ;;
    esac
done

# --- Validate Required Arguments ---
if [ -z "$ADDR" ]; then
    echo "${RED}ERROR: --addr flag is required.${NC}"
    exit 1
fi

if [ -z "$CLIENT_NAMESPACE" ]; then
    echo "${RED}ERROR: --namespace flag is required.${NC}"
    exit 1
fi

if [ -z "$TARGET_RESOURCE" ] || [ -z "$TARGET_NAME" ]; then
    echo "${RED}ERROR: --target-resource and --target-name flags are required.${NC}"
    exit 1
fi

# --- Helper Functions ---

# log_failure_details: Centralized function to log detailed debugging information on failure
log_failure_details() {
    start_time="${1}"

    echo "${RED}--- DETAILED FAILURE ANALYSIS ---${NC}"

    # Target EndpointSlice Status
    echo "${CYAN}  Status of target Private EndpointSlice in ${CLIENT_NAMESPACE}:${NC}"
    kubectl get endpointslice -n "$CLIENT_NAMESPACE" -l endpointslice.kubernetes.io/managed-by=endpointslice-controller.k8s.io -o yaml | sed 's/^/    /' || echo "${YELLOW}    - Could not retrieve EndpointSlice status${NC}"

    # Resolver Logs
    echo "${CYAN}  Logs from elasti-resolver:${NC}"
    kubectl logs -n elasti services/elasti-resolver-service --since-time="${start_time}" --all-pods=true | sed 's/^/    /' || echo "${YELLOW}    - Could not retrieve resolver logs${NC}"

    # Controller Logs
    echo "${CYAN}  Logs from elasti-controller:${NC}"
    kubectl logs -n elasti services/elasti-operator-controller-service --since-time="${start_time}" | sed 's/^/    /' || echo "${YELLOW}    - Could not retrieve controller logs${NC}"

    # Target Logs
    echo "${CYAN}  Logs from target (${TARGET_RESOURCE}/${TARGET_NAME}):${NC}"
    kubectl logs -n "$CLIENT_NAMESPACE" "${TARGET_RESOURCE}/${TARGET_NAME}" --since-time="${start_time}" | sed 's/^/    /' || echo "${YELLOW}    - Could not retrieve target pod logs${NC}"

    echo "${RED}-----------------------------------${NC}"
}

# --- Configuration Banner ---
echo "${CYAN}=== gRPC Traffic Test Configuration ===${NC}"
echo "  ${CYAN}Target Address:${NC}  $ADDR"
echo "  ${CYAN}Test Type:${NC}       $TEST_TYPE"
echo "  ${CYAN}Client Pod:${NC}      $CLIENT_POD_NAME (in $CLIENT_NAMESPACE namespace)"
echo "  ${CYAN}Target:${NC}          ${TARGET_RESOURCE}/${TARGET_NAME} (in $CLIENT_NAMESPACE namespace)"
echo "  ${CYAN}Retries:${NC}         $MAX_RETRIES"
echo "  ${CYAN}Retry Sleep:${NC}     ${RETRY_SLEEP}s"
echo "  ${CYAN}Timestamp:${NC}       $(date)"
echo "${CYAN}========================================${NC}"

# --- Check Client Pod ---
echo "Checking gRPC client pod status..."
if ! kubectl get pod "$CLIENT_POD_NAME" -n "$CLIENT_NAMESPACE" >/dev/null 2>&1; then
    echo "${RED}ERROR: gRPC client pod $CLIENT_POD_NAME not found in namespace $CLIENT_NAMESPACE${NC}"
    kubectl get pods -n "$CLIENT_NAMESPACE" || true
    exit 4
fi

POD_STATUS=$(kubectl get pod "$CLIENT_POD_NAME" -n "$CLIENT_NAMESPACE" -o jsonpath='{.status.phase}')
echo "${GREEN}gRPC client pod status: $POD_STATUS${NC}"

if [ "$POD_STATUS" != "Running" ]; then
    echo "${YELLOW}WARNING: gRPC client pod is not in Running state. Describing pod...${NC}"
    kubectl describe pod "$CLIENT_POD_NAME" -n "$CLIENT_NAMESPACE" || true
fi

echo ""
echo "${CYAN}--- Starting gRPC Traffic Test ---${NC}"

success_count=0

for i in $(seq 1 $MAX_RETRIES); do
    echo "\n${CYAN}--- Request $i/$MAX_RETRIES ---${NC}"
    echo "  ${CYAN}Time:${NC} $(date)"

    echo "  ${CYAN}Executing: kubectl exec -n $CLIENT_NAMESPACE $CLIENT_POD_NAME -- /grpc-client --addr=$ADDR --test=$TEST_TYPE${NC}"
    start_time_rfc=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    start_time=$(date +%s)

    if kubectl exec -n "$CLIENT_NAMESPACE" "$CLIENT_POD_NAME" -- /grpc-client --addr="$ADDR" --test="$TEST_TYPE" 2>&1; then
        end_time=$(date +%s)
        duration=$((end_time - start_time))
        echo "${GREEN}SUCCESS: Request $i completed successfully (${duration}s)${NC}"
        success_count=$((success_count + 1))
    else
        end_time=$(date +%s)
        duration=$((end_time - start_time))
        echo "${RED}FAILED: Request $i failed (${duration}s)${NC}"

        log_failure_details "${start_time_rfc}"
    fi

    if [ $i -lt $MAX_RETRIES ]; then
        echo "  ${CYAN}Sleeping ${RETRY_SLEEP}s before next retry...${NC}"
        sleep $RETRY_SLEEP
    fi
    echo ""
done

echo "\n${CYAN}=== Test Summary ===${NC}"
echo "  ${CYAN}Successes:${NC}    $success_count / $MAX_RETRIES"
echo "  ${CYAN}Target:${NC}       $ADDR"
echo "  ${CYAN}Completed at:${NC} $(date)"

if [ "$success_count" -gt 0 ]; then
    echo "${GREEN}Test PASSED: $success_count/$MAX_RETRIES requests succeeded.${NC}"
    echo "${CYAN}====================${NC}"
    exit 0
else
    echo "${RED}Test FAILED: All $MAX_RETRIES requests failed.${NC}"
    echo "${CYAN}====================${NC}"
    exit 1
fi
