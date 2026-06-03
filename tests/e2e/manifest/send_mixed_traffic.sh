#!/bin/bash
# Sends HTTP/1.1 and gRPC traffic concurrently to validate that H2C does not
# break HTTP/1.1 and both protocols can be served simultaneously.
set -u

if [ "$#" -lt 6 ]; then
    echo "Usage: $0 --http-url <url> --grpc-addr <host:port> --namespace <ns> --target-resource <type> --target-name <name> --grpc-target-resource <type> --grpc-target-name <name>"
    exit 1
fi

HTTP_URL=""
GRPC_ADDR=""
NAMESPACE=""
TARGET_RESOURCE=""
TARGET_NAME=""
GRPC_TARGET_RESOURCE=""
GRPC_TARGET_NAME=""

while [ "$#" -gt 0 ]; do
    case "$1" in
        --http-url)         HTTP_URL="$2"; shift 2 ;;
        --grpc-addr)        GRPC_ADDR="$2"; shift 2 ;;
        --namespace)        NAMESPACE="$2"; shift 2 ;;
        --target-resource)  TARGET_RESOURCE="$2"; shift 2 ;;
        --target-name)      TARGET_NAME="$2"; shift 2 ;;
        --grpc-target-resource) GRPC_TARGET_RESOURCE="$2"; shift 2 ;;
        --grpc-target-name)     GRPC_TARGET_NAME="$2"; shift 2 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "=== Starting mixed traffic (HTTP + gRPC) concurrently ==="

# Run HTTP traffic in background
bash "$SCRIPT_DIR/send_traffic.sh" "$HTTP_URL" \
    --namespace "$NAMESPACE" \
    --target-resource "$TARGET_RESOURCE" \
    --target-name "$TARGET_NAME" &
HTTP_PID=$!

# Run gRPC traffic in background
bash "$SCRIPT_DIR/send_grpc_traffic.sh" \
    --addr "$GRPC_ADDR" \
    --test both \
    --namespace "$NAMESPACE" \
    --target-resource "$GRPC_TARGET_RESOURCE" \
    --target-name "$GRPC_TARGET_NAME" &
GRPC_PID=$!

# Wait for both and capture exit codes
HTTP_EXIT=0
GRPC_EXIT=0
wait $HTTP_PID || HTTP_EXIT=$?
wait $GRPC_PID || GRPC_EXIT=$?

echo ""
echo "=== Mixed Traffic Results ==="
echo "  HTTP exit code:  $HTTP_EXIT"
echo "  gRPC exit code:  $GRPC_EXIT"

if [ "$HTTP_EXIT" -ne 0 ] || [ "$GRPC_EXIT" -ne 0 ]; then
    echo "FAILED: One or both traffic streams failed."
    exit 1
fi

echo "PASSED: Both HTTP and gRPC traffic succeeded concurrently."
exit 0
