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
[Sigstore](https://www.sigstore.dev/) signing — no long-lived keys, trust is anchored to the
GitHub Actions OIDC identity that produced the release (via Fulcio and the Rekor transparency log).

What gets signed and where the signature lives:

| Artifact | Registry signature (`cosign verify`) | Release asset (offline `cosign verify-blob`) | Signing identity |
| -------- | ------------------------------------ | -------------------------------------------- | ---------------- |
| Operator image | `ghcr.io/kubeelasti/elasti-operator` | `elasti-operator-<tag>.sig` / `.pem` / `.payload` | `build.yml` in `truefoundry/github-workflows-public` |
| Resolver image | `ghcr.io/kubeelasti/elasti-resolver` | `elasti-resolver-<tag>.sig` / `.pem` / `.payload` | `build.yml` in `truefoundry/github-workflows-public` |
| Image SBOMs | — | `elasti-*-<tag>.spdx.json` + `.spdx.json.sigstore.json` | `build.yml` in `truefoundry/github-workflows-public` |
| Helm chart | `ghcr.io/kubeelasti/charts/elasti` | `elasti-<version>.tgz` + `.tgz.sigstore.json` | `release.yaml` in this repo |
| Chart SBOM | — | `elasti-<version>.sbom.spdx.json` + `.sigstore.json` | `release.yaml` in this repo |

The `.sig`/`.pem`/`.payload` and `*.sigstore.json` release assets are what the
[OpenSSF Signed-Releases](https://github.com/ossf/scorecard/blob/main/docs/checks.md#signed-releases)
check looks for.

## Verifying a release

Install [cosign](https://docs.sigstore.dev/cosign/system_config/installation/), then set:

```bash
ISSUER='https://token.actions.githubusercontent.com'
# Chart + chart SBOM are signed by this repo's release workflow (on the tag):
CHART_ID_RE='^https://github.com/KubeElasti/KubeElasti/\.github/workflows/release\.yaml@refs/tags/.*'
# Images + image SBOMs are signed by the shared build workflow:
IMAGE_ID_RE='^https://github.com/truefoundry/github-workflows-public/\.github/workflows/build\.yml@.*'
```

**Helm chart (from the registry — recommended):**

```bash
cosign verify --certificate-oidc-issuer "$ISSUER" --certificate-identity-regexp "$CHART_ID_RE" \
  ghcr.io/kubeelasti/charts/elasti:<version>
```

**Container images (from the registry — recommended):**

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
cosign verify-blob --bundle elasti-operator-<tag>.spdx.json.sigstore.json \
  --certificate-oidc-issuer "$ISSUER" --certificate-identity-regexp "$IMAGE_ID_RE" \
  elasti-operator-<tag>.spdx.json

# Raw image signature (repeat for elasti-resolver)
cosign verify-blob --signature elasti-operator-<tag>.sig --certificate elasti-operator-<tag>.pem \
  --certificate-oidc-issuer "$ISSUER" --certificate-identity-regexp "$IMAGE_ID_RE" \
  elasti-operator-<tag>.payload
```


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


