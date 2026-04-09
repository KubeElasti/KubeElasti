#!/bin/sh
# Replaces spec.heartbeat with a single rule so e2e can verify resolver cache updates.
set -eu
NS="${1:?namespace required}"

kubectl patch elastiservice target-elastiservice -n "$NS" --type merge -p \
  '{"spec":{"heartbeat":[{"method":"GET","path":{"type":"Exact","value":"/probe"},"response":"alive"}]}}'
