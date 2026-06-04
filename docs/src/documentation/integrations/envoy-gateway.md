---
title: "Envoy Gateway integration with KubeElasti"
description: "Install Envoy Gateway, use HTTPRoute URLRewrite for Host-based resolver routing, and keep headerForHost at Host."
keywords:
  - Envoy Gateway KubeElasti
  - Gateway API HTTPRoute
  - URLRewrite hostname
icon: lucide/network
---

# Envoy Gateway

Use this guide when [Envoy Gateway](https://gateway.envoyproxy.io/) (Gateway API) fronts workloads managed by KubeElasti.

## Resolver header

Envoy Gateway does **not** inject `X-Envoy-Decorator-Operation` (that header comes from Istio / Envoy sidecars). Use the resolver's default **`Host`** header:

```shell
helm upgrade --install elasti ./charts/elasti -n elasti \
  --set elastiResolver.proxy.env.headerForHost=Host
```

## Install Envoy Gateway

```bash
helm install eg oci://docker.io/envoyproxy/gateway-helm \
  --version v1.5.9 \
  -n envoy-gateway-system --create-namespace

kubectl wait --timeout=5m -n envoy-gateway-system \
  deployment/envoy-gateway --for=condition=Available

kubectl apply -f ./playground/config/envoy-gateway.yaml
```

This creates `GatewayClass` and `Gateway` `eg` in `envoy-gateway-system` with an HTTP listener on port 80.

## Route traffic to the target

Use the Gateway API **`URLRewrite`** filter (`urlRewrite.hostname`) on an `HTTPRoute` to rewrite the upstream **`Host`** header to the service FQDN. The resolver reads that header the same way NGINX `upstream-vhost` does.

Example for demo workload `target-deployment` in namespace `target`:

```yaml title="httpbin-httproute.yaml" linenums="1"
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: httpbin
  namespace: target
spec:
  parentRefs:
    - name: eg
      namespace: envoy-gateway-system
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      filters:
        - type: URLRewrite
          urlRewrite:
            hostname: target-deployment.target.svc.cluster.local
      backendRefs:
        - name: target-deployment
          port: 80
```

Apply the playground manifest:

```bash
kubectl apply -f ./playground/config/demo-httproute.yaml
```

## Prometheus trigger

Envoy Gateway exposes metrics via its data plane; for the demo httpbin flow you can reuse NGINX-style ingress metrics if you terminate at another layer, or define a trigger on Envoy / gateway metrics available in your cluster. A minimal idle trigger while testing:

```yaml
triggers:
  - type: prometheus
    metadata:
      query: vector(0)
      serverAddress: http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090
      threshold: "0.01"
```

Replace with a metric that reflects real traffic through your `HTTPRoute` in production.

## Test (port-forward)

Envoy Gateway creates a Service in `envoy-gateway-system`; the name is generated from your `Gateway`:

```bash
kubectl get svc -n envoy-gateway-system
kubectl port-forward -n envoy-gateway-system svc/<envoy-gateway-service> 8080:80
curl -v http://localhost:8080/
```

## See also

- [Integrations overview](./index.md)
- [Resolver request routing](../architecture/resolver.md#request-routing)
- [Demo setup](../../install/demo-setup.md)
