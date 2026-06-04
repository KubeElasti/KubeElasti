---
title: "KubeElasti prerequisites - cluster, tools, and observability"
description: "Requirements before installing KubeElasti: Kubernetes, kubectl, Helm, Prometheus, an ingress controller, and optional KEDA or HPA."
keywords:
  - KubeElasti prerequisites
  - Kubernetes cluster requirements
  - Prometheus for KubeElasti
  - ingress controller setup
  - KEDA optional
  - Helm kubectl
icon: lucide/clipboard-check
hide:
  - toc
---

# Prerequisites

- **Kubernetes Cluster:** You should have a running Kubernetes cluster. You can use any cloud-based or on-premises Kubernetes distribution.
- **kubectl:** Installed and configured to interact with your Kubernetes cluster.
- **Helm:** Installed for managing Kubernetes applications.
- **Prometheus:** You should have a prometheus installed in your cluster.
??? example "Installing Prometheus"
    We will setup a sample prometheus to read metrics from the ingress controller.

    ```bash
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
    helm repo update
    helm install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
      --namespace monitoring \
      --create-namespace \
      --set alertmanager.enabled=false \
      --set grafana.enabled=false \
      --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false
    ```
- **Ingress / Gateway:** Install one edge integration so traffic reaches the resolver with the correct routing header. See [Gateway and ingress integrations](../documentation/integrations/index.md):
    - [NGINX Ingress](../documentation/integrations/nginx.md#install-nginx-ingress)
    - [Istio](../documentation/integrations/istio.md#install-istio)
    - [Envoy Gateway](../documentation/integrations/envoy-gateway.md#install-envoy-gateway)

- **KEDA:** [Optional] You can have a KEDA installed in your cluster, else HPA can be used.
??? example "Installing KEDA"
    We will setup a sample KEDA to scale the target deployment.

    ```bash
    helm repo add kedacore https://kedacore.github.io/charts
    helm repo update
    helm upgrade --install keda kedacore/keda --namespace keda --create-namespace --wait --timeout 180s
    ```