---
title: "KubeElasti Resolver Architecture - Proxy and Traffic Management"
description: "Understand KubeElasti resolver architecture for traffic proxying, request queueing, and service discovery in Kubernetes serverless environments."
keywords:
  - KubeElasti resolver
  - traffic proxy
  - request queueing
  - service discovery
  - proxy architecture
  - Kubernetes proxy
icon: lucide/network
---

# Resolver Architecture


``` mermaid
flowchart LR
  %% ── USER & ENTRY ─────────────────────
  User(("Client")) --> RP["Proxy<br/>:8012"] --> Main["Main<br/>cmd/main.go"] --> IS["Metrics<br/>:8013"]

  %% ── CORE MODULES ─────────────────────
  subgraph Mods["Core Modules"]
    Handler["Handler"]:::core
    Hosts["Hosts"]:::core
    Thr["Throttle"]:::core
    Oper["Operator Comm"]:::core
    Obs["Observability"]:::core
  end
  Main -- uses --> Handler & Hosts & Thr & Oper & Obs

  %% Request flow (compact arrows)
  Handler --> Hosts
  Handler --> Thr
  Thr --> Handler
  Handler --> Obs
  Handler -.-> Sentry["Sentry"]

  %% Operator comm
  Handler -.-> Oper
  Oper -.-> OpSvc["Operator Svc"]

  %% External deps
  Thr -.-> K8sAPI["K8s API"]
  Obs -.-> Prom["Prometheus"]

```

## Request Routing

The resolver identifies the target service by reading a single header on each
incoming request. Two things decide which header is read:

- By default, the resolver binary reads the standard HTTP `Host` header
  (`resolver/cmd/main.go:51`, `HeaderForHost string ... default:"Host"`).
- The shipped Helm chart overrides that default to `X-Envoy-Decorator-Operation`
  (`charts/elasti/values.yaml`, `elastiResolver.proxy.env.headerForHost`) so
  that deployments fronted by Envoy, Istio, or similar proxies that rewrite the
  Host header keep working.

Whichever header is selected, its value is parsed into `namespace` and
`service` by `GetHost` in `hostmanager.go` and used to look up the target
service to forward the request to.

This has a practical consequence. Whatever routes traffic to the resolver,
whether an Ingress controller, a service mesh, a `Service` selector, or a
direct client, must arrive at the resolver with a header value that matches
the target service's in-cluster FQDN format (for example,
`target-deployment.target.svc.cluster.local`). If the header does not match,
the resolver cannot tell which service the request was meant for and will not
forward the request.

The ingress example in
[Demo setup](../../install/demo-setup.md)
shows one way to satisfy this: the NGINX annotation
`nginx.ingress.kubernetes.io/upstream-vhost` rewrites the Host header to the
service FQDN before the request reaches the resolver.

To use a different header (for example, a custom one set by your edge proxy),
override the Helm value at install time:

```shell
--set elastiResolver.proxy.env.headerForHost=X-My-Custom-Host
```
