---
title: "KubeElasti installation - Helm, verify, and first ElastiService"
description: "Install KubeElasti with Helm, verify the operator and resolver, deploy a sample app, create an ElastiService, test scale-to-zero, and uninstall safely."
keywords:
  - KubeElasti Helm install
  - ElastiService tutorial
  - KubeElasti operator installation
  - Kubernetes scale to zero
  - ElastiService apply
icon: lucide/rocket
hide:
  - toc
---

# Installation

## Install

Use Helm to install KubeElasti into your Kubernetes cluster. Check out [values.yaml](https://github.com/KubeElasti/KubeElasti/blob/main/charts/elasti/values.yaml) to see configuration options in the helm value file.

```bash
helm install elasti oci://tfy.jfrog.io/tfy-helm/elasti --namespace elasti --create-namespace
```


## Uninstall

To uninstall Elasti, **you will need to remove all the installed ElastiServices first.** Then, simply delete the installation file.

```bash
kubectl delete elastiservices --all
helm uninstall elasti -n elasti
kubectl delete namespace elasti
```
