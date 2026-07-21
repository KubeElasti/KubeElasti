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
helm install elasti oci://ghcr.io/kubeelasti/charts/elasti --namespace elasti --create-namespace
```


## Verify the release (optional)

Every KubeElasti release is signed with [cosign](https://github.com/sigstore/cosign) using
keyless [Sigstore](https://www.sigstore.dev/) signing — there are no long-lived keys, trust is
anchored to the GitHub Actions OIDC identity that built the release (verified through Fulcio and
the Rekor transparency log). The Helm chart, both container images, and their SBOMs are all signed.

Install cosign (`brew install cosign`, or see the [install docs](https://docs.sigstore.dev/cosign/system_config/installation/)), then:

### Verify the Helm chart

The chart is signed in this repository's release workflow, so the signing identity is
`release.yaml` on the release tag:

```bash
CHART_ID_RE='^https://github.com/KubeElasti/KubeElasti/\.github/workflows/release\.yaml@refs/tags/.*'
ISSUER='https://token.actions.githubusercontent.com'

cosign verify \
  --certificate-oidc-issuer "$ISSUER" \
  --certificate-identity-regexp "$CHART_ID_RE" \
  ghcr.io/kubeelasti/charts/elasti:<version>
```

### Verify the container images

The images and their SBOMs are signed by the shared build workflow, so the signing identity is
`build.yml` in `truefoundry/github-workflows-public`:

```bash
IMAGE_ID_RE='^https://github.com/truefoundry/github-workflows-public/\.github/workflows/build\.yml@.*'
ISSUER='https://token.actions.githubusercontent.com'

for img in elasti-operator elasti-resolver; do
  cosign verify \
    --certificate-oidc-issuer "$ISSUER" \
    --certificate-identity-regexp "$IMAGE_ID_RE" \
    ghcr.io/kubeelasti/${img}:<version>
done
```

For verifying the SBOMs and offline signature bundles attached to the GitHub release, see
[the release process doc](https://github.com/KubeElasti/KubeElasti/blob/main/RELEASE.md#verifying-a-release).


## Uninstall

To uninstall Elasti, **you will need to remove all the installed ElastiServices first.** Then, simply delete the installation file.

```bash
kubectl delete elastiservices --all
helm uninstall elasti -n elasti
kubectl delete namespace elasti
```
