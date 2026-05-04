---
title: "KubeElasti Load Testing - Performance Testing with k6"
description: "Perform load testing on KubeElasti with k6. Learn how to test performance, scaling behavior, and system limits under load."
keywords:
  - KubeElasti load testing
  - k6 performance testing
  - Kubernetes load testing
  - performance testing
  - scale testing
  - stress testing
icon: lucide/gauge
---

# Load testing

KubeElasti ships a small [k6](https://k6.io) load harness under `tests/load/` to
exercise scale-from-zero and steady-state behavior against a target service.

## 1. Update the k6 script

Edit `tests/load/load.js` to point at your target URL (for example, the ingress
or port-forwarded service) and adjust the virtual user count or duration as
needed.

## 2. Run the load test

The wrapper script writes a k6 web dashboard report and logs into a `logs/`
directory next to the script, so create that directory the first time you run
it.

```bash
cd ./tests/load
mkdir -p logs
chmod +x ./generate_load.sh
./generate_load.sh
```

By default the script runs `k6 run --vus 100 --duration 30s load.js` and
exposes the live dashboard on `http://localhost:5665`.
