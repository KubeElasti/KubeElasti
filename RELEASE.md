# KubeElasti Release Process

This document outlines the release process for KubeElasti, covering both beta and stable releases.

## Release Workflow Overview

KubeElasti has 3 release types:
1. **Stable Release**: Manually triggered via Github releases
2. **Beta Release**: Manually triggered via Github releases
3. **Beta Releases (Legacy)**: Automated from the `main` branch


## 1. Stable Release

Stable releases require manual preparation and are triggered by creating a GitHub release.

### Preparation Steps

1. Update version information:
   
   - Update `charts/elasti/Chart.yaml` with the new version number:
     ```yaml
     version: X.Y.Z
     appVersion: "X.Y.Z"
     ```
   
   - Update `charts/elasti/values.yaml` to reference the specific commit SHA:
     ```yaml
     elastiController:
       manager:
         image:
           tag: vX.Y.Z
     elastiResolver:
       proxy:
         image:
           tag: vX.Y.Z
     ```

2. Update `CHANGELOG.md` with the new version number:
   ```markdown
   ## vX.Y.Z (YYYY-MM-DD)
   ### New
   - <Where the change was done>: <What was added>
   - <Resolved>: ...
   - <General>: ...

   ### Experimental
   - <Where the change was done>: <What was added>

   ### Improvements
   - <Where the change was done>: <What was improved>

   ### Fixes
   - <Where the change was done>: <What was fixed>

   ### Breaking Changes
   - <Where the change was done>: <What was changed>

   ### Other
   - <Where the change was done>: <What was changed>

   ### New Contributors
   - @rethil made their first contribution in #154
   etc...
   ```

3. Create a pull request with these changes
4. Review and merge the PR to `main`

### Release Steps

1. Create a new GitHub release:
   - Tag format: `vX.Y.Z`
   - Title: `KubeElasti vX.Y.Z`
   - Include release notes detailing changes
        ```markdown
        We are happy to release KubeElasti vX.Y.Z 🎉

        Here are some highlights of this release:
        - Highlight 1
        - Highlight 2

        Here are the breaking changes of this release:
        - Breaking Change 1
        - Breaking Change 2

        Learn how to deploy KubeElasti by reading [our documentation](https://kubeelasti.dev).

        # New
        - <Where the change was done>: <What was added>
        - <Resolved>: ...
        - <General>: ...

        ## Experimental
        - <Where the change was done>: <What was added>

        # Improvements
        - <Where the change was done>: <What was improved>

        # Fixes
        - <Where the change was done>: <What was fixed>

        # Breaking Changes
        - <Where the change was done>: <What was changed>

        # Other
        - <Where the change was done>: <What was changed>

        # New Contributors
        - @rethil made their first contribution in #154
        etc...
        ```
    - Feel free to add more sections as needed, or remove what is not needed.


2. The `.github/workflows/release.yaml` workflow is triggered:
   - Operator and resolver images are built, pushed, and **keyless-signed** with cosign
   - An SPDX SBOM is generated and signed for each image
   - Helm chart is packaged
   - Chart is pushed to GitHub Container Registry (`oci://ghcr.io/kubeelasti/charts`) and **keyless-signed** with cosign
   - An SPDX SBOM is generated and signed for the chart
   - The chart, all SBOMs, and all cosign signature files are attached to the GitHub release

## Supply-chain signing

Releases are signed with [cosign](https://github.com/sigstore/cosign) using keyless
[Sigstore](https://www.sigstore.dev/) signing. There are no long-lived keys; trust is anchored to the
GitHub Actions OIDC identity that produced the release (via Fulcio and the Rekor transparency log).

What gets signed and where the signature lives:

> `<version>` below is the chart/app version **without** the leading `v` (e.g. `0.1.25`). That is
> what appears in the asset filenames and the image tags. The GitHub release/git tag keeps the `v`
> (e.g. `v0.1.25`).

| Artifact | Registry signature (`cosign verify`) | Release asset (offline `cosign verify-blob`) | Signing identity |
| -------- | ------------------------------------ | -------------------------------------------- | ---------------- |
| Operator image | `ghcr.io/kubeelasti/elasti-operator` | `elasti-operator-<version>.sig` / `.pem` / `.payload` | `build.yml` in `truefoundry/github-workflows-public` |
| Resolver image | `ghcr.io/kubeelasti/elasti-resolver` | `elasti-resolver-<version>.sig` / `.pem` / `.payload` | `build.yml` in `truefoundry/github-workflows-public` |
| Image SBOMs | n/a | `elasti-*-<version>.spdx.json` + `.spdx.json.sigstore.json` | `build.yml` in `truefoundry/github-workflows-public` |
| Helm chart | `ghcr.io/kubeelasti/charts/elasti` | `elasti-<version>.tgz` + `.tgz.sigstore.json` | `release.yaml` in this repo |
| Chart SBOM | n/a | `elasti-<version>.sbom.spdx.json` + `.sigstore.json` | `release.yaml` in this repo |

## Verifying a release

Verifying proves a release artifact was genuinely built by KubeElasti's CI and hasn't been
tampered with. Most **end users only need to verify the chart and images from the registry**.
Those steps, with a plain-English explanation, live in the
[installation guide](https://kubeelasti.dev/src/install/installation/#verify-the-release-optional-but-recommended).
This section additionally covers verifying the **files attached to the GitHub release** (SBOMs and
offline signature bundles), which is what a supply-chain audit or the OpenSSF check would do.

A successful `cosign verify` / `verify-blob` prints `Verified OK` (blobs) or a
`The following checks were performed ...` block (registry). Anything else, especially
`no matching signatures`, means **do not trust the artifact**.

Install [cosign](https://docs.sigstore.dev/cosign/system_config/installation/), then set:

```bash
ISSUER='https://token.actions.githubusercontent.com'
# Chart + chart SBOM are signed by this repo's release workflow (on the tag):
CHART_ID_RE='^https://github.com/KubeElasti/KubeElasti/\.github/workflows/release\.yaml@refs/tags/.*'
# Images + image SBOMs are signed by the shared build workflow:
IMAGE_ID_RE='^https://github.com/truefoundry/github-workflows-public/\.github/workflows/build\.yml@.*'
```

**Helm chart (from the registry, recommended):**

```bash
cosign verify --certificate-oidc-issuer "$ISSUER" --certificate-identity-regexp "$CHART_ID_RE" \
  ghcr.io/kubeelasti/charts/elasti:<version>
```

**Container images (from the registry, recommended):**

```bash
for img in elasti-operator elasti-resolver; do
  cosign verify --certificate-oidc-issuer "$ISSUER" --certificate-identity-regexp "$IMAGE_ID_RE" \
    ghcr.io/kubeelasti/${img}:<version>
done
```

**Release assets (offline, from the GitHub release):** download the assets, then verify the
self-contained Sigstore bundles:

```bash
# Chart tarball
cosign verify-blob --bundle elasti-<version>.tgz.sigstore.json \
  --certificate-oidc-issuer "$ISSUER" --certificate-identity-regexp "$CHART_ID_RE" \
  elasti-<version>.tgz

# Chart SBOM
cosign verify-blob --bundle elasti-<version>.sbom.spdx.json.sigstore.json \
  --certificate-oidc-issuer "$ISSUER" --certificate-identity-regexp "$CHART_ID_RE" \
  elasti-<version>.sbom.spdx.json

# Image SBOM (repeat for elasti-resolver)
cosign verify-blob --bundle elasti-operator-<version>.spdx.json.sigstore.json \
  --certificate-oidc-issuer "$ISSUER" --certificate-identity-regexp "$IMAGE_ID_RE" \
  elasti-operator-<version>.spdx.json

# Raw image signature (repeat for elasti-resolver)
cosign verify-blob --signature elasti-operator-<version>.sig --certificate elasti-operator-<version>.pem \
  --certificate-oidc-issuer "$ISSUER" --certificate-identity-regexp "$IMAGE_ID_RE" \
  elasti-operator-<version>.payload
```

**Verifying an image with the `.sig` file (offline, two steps).** The `.sig`/`.pem`/`.payload`
trio lets you verify an image *without pulling the signature from the registry*. But be aware:
the command above proves the **payload** is authentic, but it does **not**, on its own, tie the
signature to the image you're about to run. The payload is a small JSON document that records the
image **digest** it vouches for, so you must also confirm that digest matches your image:

```bash
# Step 1: verify the signature over the payload (as above) -> "Verified OK"
cosign verify-blob --signature elasti-operator-<version>.sig --certificate elasti-operator-<version>.pem \
  --certificate-oidc-issuer "$ISSUER" --certificate-identity-regexp "$IMAGE_ID_RE" \
  elasti-operator-<version>.payload

# Step 2: read the digest the payload vouches for ...
SIGNED_DIGEST=$(jq -r '.critical.image."docker-manifest-digest"' elasti-operator-<version>.payload)
echo "$SIGNED_DIGEST"

# ... and confirm it matches the actual image (needs crane, or `docker buildx imagetools inspect`)
crane digest ghcr.io/kubeelasti/elasti-operator:<version>   # must equal $SIGNED_DIGEST
```

If both steps pass and the two digests are identical, the `.sig` proves that exact image was
signed by the expected workflow. (Verifying straight from the registry with `cosign verify`, see
above, does both steps for you in one command, which is why it's the recommended path.)

> Newer cosign prints a deprecation note for `--signature`/`--certificate` on `verify-blob`; the
> command still succeeds. The `.sigstore.json` bundles above are the preferred, future-proof form.

All of the commands in this section were confirmed against a real signed release
(`elasti-*-0.1.25` assets), and every check returns `Verified OK`.


## 2. Beta Release

Same steps as stable, we just replace it with `vX.Y.Z-beta`.


## 3.  Beta(Legacy) Release

Beta(Legacy) releases are automatically generated when code is merged to the `main` branch.

### Workflow

1. Code is merged to the `main` branch
2. The `.github/workflows/build-n-publish.yml` workflow is triggered:
   - Docker images are built for both operator and resolver components
   - Images are tagged with the commit SHA
   - Images are pushed to GitHub Container Registry (`ghcr.io/kubeelasti`)
   - The `helm-main` branch is updated with the latest SHA in `values.yaml`

### Using Beta Releases

Beta releases can be used for testing by referencing:
- The specific commit SHA from the Docker images
- The Helm chart from the `helm-main` branch


## Version Numbering

KubeElasti follows semantic versioning (SemVer):
- **X**: Major version for incompatible API changes
- **Y**: Minor version for new functionality in a backward-compatible manner
- **Z**: Patch version for backward-compatible bug fixes


## Rollback Procedure

If issues are discovered in a release:
1. For critical issues, create a hotfix release
2. For stable releases.
   1. Prepare a new patch release with fixes.
   2. Mark the old release as deprecated or bad.


