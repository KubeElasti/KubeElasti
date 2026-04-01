# KubeElasti Governance

## Overview

KubeElasti is an open source project for Kubernetes-native scale-to-zero and scale-from-zero workflows. This document describes how the project is governed, how decisions are made, and how contributors can grow into leadership roles.

The project follows a maintainer-led governance model aligned with CNCF governance guidance. Governance in KubeElasti is centered on people, their responsibilities, and the public processes used to make decisions.

This governance applies to the KubeElasti GitHub organization and repositories, including code, documentation, release processes, issue tracking, and community spaces operated for the project.

## Values

KubeElasti maintainers and contributors are expected to uphold these values:

- Openness: project discussion, design review, and decision-making should happen in public whenever possible.
- Fairness: contributors are evaluated on the quality and consistency of their work, not on employer or background.
- Respect: community interactions must follow the project's Code of Conduct.
- Collaboration: issues, pull requests, and proposals should move forward through constructive review and iteration.
- Sustainability: leadership should be shared and documented so the project can continue to grow beyond any one individual or company.

## Roles

### Contributors

Contributors are anyone who participates in the project, including through code, documentation, testing, issue triage, design discussion, or community support.

Contributors are expected to:

- Follow the processes described in [CONTRIBUTING.md](./CONTRIBUTING.md).
- Abide by the [Code of Conduct](./CODE_OF_CONDUCT.md).
- Work collaboratively with reviewers and maintainers.

### Reviewers

Reviewers are contributors who have demonstrated sustained, high-quality participation and are trusted by maintainers to review contributions in one or more areas of the project.

Reviewers are expected to:

- Provide timely, constructive, and technically sound feedback.
- Help contributors navigate project expectations and quality bars.
- Escalate significant design, release, or governance questions to maintainers when needed.

Reviewers are appointed by maintainers based on sustained contribution quality and ongoing engagement with the project.

### Maintainers

Maintainers are the project's decision-making body and stewards of the overall health of KubeElasti. They are responsible for technical direction, release oversight, project processes, and community continuity.

The current maintainers are listed in [MAINTAINERS](./MAINTAINERS). That file is the source of truth for the active maintainer roster.

Maintainers are expected to:

- Review and approve changes across the project.
- Make and document project decisions in public channels whenever practical.
- Curate releases, roadmap priorities, and project health.
- Mentor contributors and reviewers.
- Enforce project policies, including this governance document.
- Act in accordance with the [Code of Conduct](./CODE_OF_CONDUCT.md) and [Security Policy](./SECURITY.md).

## Decision Making

### General Principle

KubeElasti prefers discussion and consensus over formal process. Most technical and community decisions should be made through public discussion in GitHub issues, pull requests, and discussions.

### Lazy Consensus

The default decision model is lazy consensus. If a proposal is made in a public project channel and no maintainer raises a substantive objection within a reasonable review period, the proposal may proceed.

A substantive objection should include clear reasoning and, where possible, a path to resolution.

### When a Formal Vote Is Required

A formal maintainer vote should be used when:

- There is unresolved disagreement among maintainers.
- The decision is high impact, difficult to reverse, or materially affects project direction.
- This document explicitly requires a vote.

Formal votes should happen in a publicly visible GitHub issue or pull request whenever possible.

### Voting Rules

- Each active maintainer has one vote.
- A simple majority of all active maintainers approves routine formal votes.
- At least half of all active maintainers must participate for the vote to be valid.
- Votes to add or remove a maintainer require a two-thirds majority of all active maintainers.
- In the event of a tie, the proposal does not pass.

## Becoming a Maintainer

New maintainers are selected from active contributors who have demonstrated:

- Sustained, high-quality contributions over time.
- Good judgment in technical and community discussions.
- Reliability in review, follow-through, and collaboration.
- Familiarity with the project's architecture, release needs, and contributor workflows.
- Commitment to the project's values and Code of Conduct.

The process for adding a maintainer is:

1. An existing maintainer nominates the candidate in a public GitHub issue or pull request.
2. Active maintainers discuss the nomination publicly whenever practical.
3. The nomination is approved by a two-thirds vote of all active maintainers.
4. Once approved, the maintainer roster is updated in [MAINTAINERS](./MAINTAINERS) and, if needed, in other project access controls.

Non-code contributions, including documentation, testing, triage, community support, and release operations, are valid paths to maintainership if they demonstrate sustained project leadership.

## Maintainer Inactivity and Removal

Maintainers are expected to remain reasonably active in project review, decision-making, or release and community work.

A maintainer may be moved to inactive status when they have had little or no meaningful participation for approximately three months and do not respond to a check-in from the other maintainers.

Inactive maintainers:

- Are not counted as active maintainers for voting purposes.
- May return to active status after resuming participation and agreement from the active maintainers.

A maintainer may be removed for extended inactivity, repeated failure to meet maintainer responsibilities, or conduct that harms the project or community. Removal requires a two-thirds vote of all active maintainers.

Maintainers may also step down voluntarily at any time by notifying the other maintainers, after which [MAINTAINERS](./MAINTAINERS) should be updated.

## Communications

Official project communication and decision-making channels include:

- GitHub issues: bug reports, proposals, roadmap items, and formal votes.
- GitHub pull requests: code review and change approval.
- GitHub discussions: broader questions, design exploration, and community conversation.
- CNCF Slack `#kubeelasti`: real-time community discussion and support.

Private discussions should be avoided for project decisions except where confidentiality is required, such as security reports or Code of Conduct matters.

## Code of Conduct

KubeElasti follows the [CNCF Code of Conduct](./CODE_OF_CONDUCT.md). All community members, including contributors, reviewers, and maintainers, are expected to uphold it.

Reports of Code of Conduct violations should follow the reporting path documented in [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md). Maintainers may coordinate response and resolution as appropriate.

## Security Response

Security issues must be reported according to [SECURITY.md](./SECURITY.md). Maintainers may handle security reports directly or delegate response coordination to a smaller trusted group as needed.

Security reports must not be raised as public GitHub issues until a coordinated disclosure path allows it.

## Changes to Governance

Changes to this governance document must be proposed through a pull request and approved by a simple majority of all active maintainers.

Material changes should be announced in the pull request description and discussed in public before approval.
