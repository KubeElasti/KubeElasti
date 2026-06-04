---
title: "NGINX Ingress integration with KubeElasti"
description: "Install NGINX Ingress, set upstream-vhost for resolver routing, configure Prometheus triggers, and test scale-from-zero."
keywords:
  - NGINX Ingress KubeElasti
  - upstream-vhost
  - scale to zero ingress
icon: lucide/network
---

# NGINX Ingress

Use this guide when [NGINX Ingress Controller](https://kubernetes.github.io/ingress-nginx/) fronts workloads managed by KubeElasti.

## Resolver header

Leave `headerForHost` at **`Host`** (the resolver binary default). If you installed via the Helm chart with its default `X-Envoy-Decorator-Operation`, override:

```shell
helm upgrade --install elasti ./charts/elasti -n elasti \
  --set elastiResolver.proxy.env.headerForHost=Host
```

## Install NGINX Ingress

```bash
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo update
helm upgrade --install nginx-ingress ingress-nginx/ingress-nginx \
  --namespace nginx \
  --set controller.metrics.enabled=true \
  --set controller.metrics.serviceMonitor.enabled=true \
  --create-namespace
```

Enable Prometheus as described in [Prerequisites](../../install/pre-requisites.md) so NGINX metrics are available for ElastiService triggers.

## Route traffic to the target

The annotation `nginx.ingress.kubernetes.io/upstream-vhost` sets the **upstream `Host` header** to the service FQDN before the request reaches the resolver. The resolver reads that header to select the target service.

Example Ingress for the [demo setup](../../install/demo-setup.md) httpbin workload (`target-deployment` in namespace `target`):

```yaml title="httpbin-ingress.yaml" linenums="1"
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: httpbin-ingress
  namespace: target
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /$2
    nginx.ingress.kubernetes.io/service-upstream: "true"
    nginx.ingress.kubernetes.io/upstream-vhost: "target-deployment.target.svc.cluster.local"
spec:
  ingressClassName: nginx
  rules:
    - http:
        paths:
          - path: /httpbin(/|$)(.*)
            pathType: ImplementationSpecific
            backend:
              service:
                name: target-deployment
                port:
                  number: 80
```

`service-upstream: "true"` sends traffic to the cluster Service (including the Elasti-managed endpoint) rather than bypassing it via Endpoints.

## Prometheus trigger

Example query for scale-down when ingress traffic is idle:

```yaml
triggers:
  - type: prometheus
    metadata:
      query: sum(rate(nginx_ingress_controller_requests{namespace="target", ingress="httpbin-ingress"}[1m])) or vector(0)
      serverAddress: http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090
      threshold: "0.01"
```

More examples are in [Triggers](../get-started/triggers.md).

## Test (port-forward)

```bash
kubectl port-forward svc/nginx-ingress-ingress-nginx-controller -n nginx 8080:80
curl -v http://localhost:8080/httpbin
```

## See also

- [Integrations overview](./index.md)
- [Resolver request routing](../architecture/resolver.md#request-routing)
- [Demo setup](../../install/demo-setup.md)
