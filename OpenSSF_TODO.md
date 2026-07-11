# OpenSSF Baseline — Progress & TODO

Tracking for the [OpenSSF Best Practices project #12491](https://www.bestpractices.dev/en/projects/12491).

- **OSPS Baseline track:** 57% (13/64 met, 51 remaining) — primary target.
- **Classic "passing" badge:** 0% (all criteria unanswered) — overlaps heavily with Baseline; fill the form once Baseline work lands.

Legend: `[x]` done · `[ ]` to do · `[~]` partially done / needs verification

---

## ✅ Already Met (13)

- [x] OSPS-AC-01.01 — MFA for sensitive actions
- [x] OSPS-BR-03.01 / BR-03.02 — Releases over encrypted channels
- [x] OSPS-DO-01.01 — User guide / docs site
- [x] OSPS-DO-02.01 — Docs for each released version
- [x] OSPS-GV-02.01 — Enforceable code of conduct (CODE_OF_CONDUCT.md)
- [x] OSPS-GV-03.01 — Contribution process exists (CONTRIBUTING.md)
- [x] OSPS-LE-02.01 / LE-02.02 — License present & OSI-approved (MIT)
- [x] OSPS-LE-03.01 / LE-03.02 — License in standard location / SPDX
- [x] OSPS-QA-01.01 / QA-01.02 — Public VCS with change history

---

## Workstream 1 — GitHub settings (no commits; repo/org UI or `gh api`)

- [ ] OSPS-AC-02.01 — New collaborators default to lowest (Read) base permission
- [ ] OSPS-AC-03.01 — Branch protection on `main`: block direct pushes
- [ ] OSPS-AC-03.02 — Branch protection on `main`: block deletion
- [ ] OSPS-AC-04.01 — Settings → Actions → default `GITHUB_TOKEN` = read-only
- [ ] OSPS-QA-03.01 — Require status checks to pass before merge
- [ ] OSPS-QA-07.01 — Require ≥1 non-author approval before merge

## Workstream 2 — CI/CD workflow commits (`.github/workflows/`)

- [x] OSPS-AC-04.02 — Added explicit least-priv `permissions: contents: read` to `lint-and-test.yaml` & `e2e-kuttl-test.yaml`
- [ ] OSPS-BR-01.01 / BR-01.04 — Sanitize untrusted/collaborator inputs (no `${{ github.event.* }}` inline in `run:`)
- [ ] OSPS-BR-01.03 — Ensure fork/PR workflows cannot access secrets (`pull_request` vs `pull_request_target`)
- [ ] OSPS-BR-06.01 + QA-02.02 — Sign release artifacts (cosign) + generate SBOM (syft) + signed checksums manifest
- [ ] OSPS-LE-01.01 — DCO enforcement (app or signoff-check workflow) + document `git commit -s`
- [ ] OSPS-VM-05.03 — SCA status check on PRs (govulncheck / dependency-review), required
- [ ] OSPS-VM-06.02 — Make CodeQL (`codescan.yaml`) a required check
- [x] ~~OSPS-BR-01.02~~ — Retired upstream (N/A)

## Workstream 3 — New / updated docs & policy files (commits)

- [ ] OSPS-DO-04.01 / DO-05.01 — New `SUPPORT.md` (support scope + duration, security-update EOL)
- [ ] OSPS-VM-01.01 — Add explicit CVD response timeframe to SECURITY.md
- [~] OSPS-VM-02.01 — Security contacts in SECURITY.md (present; verify & mark)
- [ ] OSPS-VM-03.01 — Enable GitHub Private Vulnerability Reporting + document
- [ ] OSPS-BR-07.02 — Secrets-management policy (SECURITY.md section)
- [ ] OSPS-DO-06.01 — Dependency management policy doc
- [ ] OSPS-VM-05.01 / VM-05.02 — Dependency remediation thresholds + block-before-release policy
- [ ] OSPS-VM-06.01 — SAST remediation threshold policy
- [ ] OSPS-DO-03.01 / DO-03.02 — Provenance verification instructions (depends on BR-06.01 signing)
- [ ] OSPS-GV-03.02 — Contribution acceptance requirements in CONTRIBUTING.md
- [ ] OSPS-DO-07.01 — Build-from-source instructions (CONTRIBUTING/DEVELOPMENT)
- [ ] OSPS-QA-06.02 — Document how/when tests run
- [ ] OSPS-QA-06.03 — Test policy for major changes
- [~] OSPS-GV-01.01 / GV-01.02 — Roles & members (MAINTAINERS + GOVERNANCE; verify & mark)
- [ ] OSPS-GV-04.01 — Policy: review collaborators before escalating permissions (GOVERNANCE.md)
- [ ] OSPS-QA-04.01 — Document monorepo codebases (operator/resolver/pkg)
- [ ] OSPS-SA-01.01 — Design doc: actors & trust boundaries
- [ ] OSPS-SA-02.01 — External interface descriptions (CRD + HTTP APIs)
- [ ] OSPS-SA-03.01 / SA-03.02 — Security assessment + threat model
- [ ] OSPS-VM-04.01 — Publish discovered vulnerabilities (GHSA)
- [ ] OSPS-VM-04.02 — VEX feed (heavy lift)

## Workstream 4 — Hygiene + "already satisfied, just mark met"

- [x] OSPS-BR-07.01 — gitignore `.env`/`*.log`/keys, removed `tests/load/k6_logs.log`, added gitleaks CI scan (`.github/workflows/gitleaks.yml` + `.gitleaks.toml` + `.gitleaksignore`). `resolver/.env` kept (non-secret tuning config used by `resolver/Makefile`)
- [~] OSPS-QA-05.01 / QA-05.02 — No executables/unreviewable binaries (only `.xcf` image sources — allowed) → mark met
- [~] OSPS-BR-02.01 / BR-02.02 — SemVer tags + release assets → mark met
- [~] OSPS-BR-05.01 / QA-02.01 — Go modules dependency management → mark met
- [~] OSPS-QA-06.01 — Tests run in CI on PR → mark met
- [~] OSPS-BR-04.01 — CHANGELOG attached per release under `## Changelog` header → verify & mark

## Workstream 5 — Dependency automation (commit)

- [ ] Add `renovate.json` or `.github/dependabot.yml` (supports VM-05.x + DO-06.01)

---

## Heaviest lifts
- OSPS-BR-06.01 — Release signing + SBOM + checksums
- OSPS-VM-04.02 — VEX feed
