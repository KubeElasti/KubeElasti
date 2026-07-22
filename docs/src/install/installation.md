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


??? note "Verify the release (optional)"

    **New to this?** Every KubeElasti release is keyless-signed with
    [cosign](https://github.com/sigstore/cosign)/[Sigstore](https://www.sigstore.dev/). Verifying
    proves an artifact genuinely came from KubeElasti's CI and wasn't tampered with. No keys
    needed, just [install cosign](https://docs.sigstore.dev/cosign/system_config/installation/).

    Replace `<version>` (e.g. `0.1.25`) and run:

    ```bash
    ISSUER='https://token.actions.githubusercontent.com'
    CHART_ID='^https://github.com/KubeElasti/KubeElasti/\.github/workflows/release\.yaml@refs/tags/.*'
    IMAGE_ID='^https://github.com/truefoundry/github-workflows-public/\.github/workflows/build\.yml@.*'

    # Helm chart
    cosign verify --certificate-oidc-issuer "$ISSUER" --certificate-identity-regexp "$CHART_ID" \
      ghcr.io/kubeelasti/charts/elasti:<version>

    # Resolver image
    cosign verify --certificate-oidc-issuer "$ISSUER" --certificate-identity-regexp "$IMAGE_ID" \
      ghcr.io/kubeelasti/elasti-resolver:<version>

    # Controller image
    cosign verify --certificate-oidc-issuer "$ISSUER" --certificate-identity-regexp "$IMAGE_ID" \
      ghcr.io/kubeelasti/elasti-operator:<version>
    ```

    A pass prints a `The following checks were performed ...` block. If you see
    **`Error: no matching signatures`**, the artifact is unsigned, altered, or from an unexpected
    source. **Do not install it** ([report it](https://github.com/KubeElasti/KubeElasti/blob/main/SECURITY.md)).
    Verifying the files attached to a release (SBOMs, offline `.sig` bundles) is covered in the
    [release doc](https://github.com/KubeElasti/KubeElasti/blob/main/RELEASE.md#verifying-a-release).


## Uninstall

To uninstall Elasti, **you will need to remove all the installed ElastiServices first.** Then, simply delete the installation file.

```bash
kubectl delete elastiservices --all
helm uninstall elasti -n elasti
kubectl delete namespace elasti
```
