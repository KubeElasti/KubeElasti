---
title: "Istio integration with KubeElasti"
description: "Install Istio, configure Gateway and VirtualService for resolver routing, and use X-Envoy-Decorator-Operation with the Helm chart default."
keywords:
  - Istio KubeElasti
  - VirtualService scale to zero
  - X-Envoy-Decorator-Operation
icon: lucide/network
---

# Istio

Use this guide when [Istio](https://istio.io/) routes external traffic into workloads managed by KubeElasti.

## Resolver header

Istio's Envoy sidecar typically sets **`X-Envoy-Decorator-Operation`** to the destination service FQDN. The KubeElasti Helm chart defaults `headerForHost` to that header — no override needed when using the chart defaults.

If you run the resolver binary without Helm defaults, set the same header explicitly.

## Install Istio

```shell
curl -L https://istio.io/downloadIstio | sh -
mv istio-* ~/.istioctl
export PATH=$HOME/.istioctl/bin:$PATH

istioctl install --set profile=default -y
```

Enable sidecar injection on the application namespace:

```bash
kubectl create namespace target
kubectl label namespace target istio-injection=enabled
```

Create an ingress `Gateway` (playground manifest):

```bash
kubectl apply -f ./playground/config/gateway.yaml
```

The playground file installs a `Gateway` named `gateway` in `istio-system` bound to `istio-ingressgateway`.

## Route traffic to the target

A `VirtualService` should route to the **in-cluster FQDN** of the Kubernetes Service KubeElasti manages (the public Service name from your ElastiService spec).

Example for demo workload `target-deployment` in namespace `target`:

```yaml title="httpbin-virtualservice.yaml" linenums="1"
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: httpbin
  namespace: istio-system
spec:
  hosts:
    - "*"
  gateways:
    - gateway
  http:
    - match:
        - uri:
            prefix: /
      route:
        - destination:
            host: target-deployment.target.svc.cluster.local
            port:
              number: 80
```

Apply the playground sample (update `destination.host` if your service name differs):

```bash
kubectl apply -f ./playground/config/demo-virtualService.yaml
```

## Prometheus trigger

Example query using Istio request metrics:

```yaml
triggers:
  - type: prometheus
    metadata:
      query: sum(rate(istio_requests_total{destination_service_name="target-deployment.target.svc.cluster.local"}[1m])) or vector(0)
      serverAddress: http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090
      threshold: "0.01"
```

See [Triggers](../get-started/triggers.md) for more query patterns.

## Test (port-forward)

```bash
kubectl port-forward svc/istio-ingressgateway -n istio-system 8080:80
curl -v http://localhost:8080/headers
```

## See also

- [Integrations overview](./index.md)
- [Resolver request routing](../architecture/resolver.md#request-routing)
- [Playground](../development/playground.md) (Istio demo steps)
- [Demo setup](../../install/demo-setup.md)
