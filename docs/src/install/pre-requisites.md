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
- **Ingress Controller:** You should have an ingress controller installed in your cluster.
??? example "Installing Ingress Controller"
    
    === "NGINX"
        ```bash
          helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
          helm repo update
          helm upgrade --install nginx-ingress ingress-nginx/ingress-nginx \
            --namespace nginx \
            --set controller.metrics.enabled=true \
            --set controller.metrics.serviceMonitor.enabled=true \
            --create-namespace
        ```

    === "Istio"
        ```shell
        # Download the latest Istio release from the official Istio website.
        curl -L https://istio.io/downloadIstio | sh -
        # Move it to home directory
        mv istio-x.xx.x ~/.istioctl
        export PATH=$HOME/.istioctl/bin:$PATH

        istioctl install --set profile=default -y

        # Label the namespace where you want to deploy your application to enable Istio sidecar Injection
        kubectl create namespace <NAMESPACE>
        kubectl label namespace <NAMESPACE> istio-injection=enabled

        # Create a gateway
        kubectl apply -f ./playground/config/gateway.yaml -n <NAMESPACE>
        ```

- **KEDA:** [Optional] You can have a KEDA installed in your cluster, else HPA can be used.
??? example "Installing KEDA"
    We will setup a sample KEDA to scale the target deployment.

    ```bash
    helm repo add kedacore https://kedacore.github.io/charts
    helm repo update
    helm upgrade --install keda kedacore/keda --namespace keda --create-namespace --wait --timeout 180s
    ```