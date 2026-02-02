# OpenChoreo Governance

This document describes the governance model for the OpenChoreo project. OpenChoreo is a Cloud Native Computing Foundation (CNCF) Sandbox project and follows CNCF project governance principles and the CNCF Code of Conduct.

## Mission

OpenChoreo is an open-source, Kubernetes-native Developer Platform focused on providing clear architectural and development abstractions that reduce developer cognitive load while preserving strong governance, extensibility, and operational clarity for platform teams.

The project aims to:

- Provide composable, Kubernetes-native abstractions for building developer platforms
- Encourage open collaboration and vendor-neutral governance
- Foster a welcoming, inclusive, and sustainable community

## CNCF Alignment

OpenChoreo is a CNCF Sandbox project and operates in alignment with:

- CNCF Code of Conduct
- CNCF Trademark Guidelines
- CNCF Project Lifecycle and Governance guidelines

While OpenChoreo was initially created by contributors from WSO2, it is a vendor-neutral open source project. Governance decisions are made collectively by the maintainer group and are not controlled by any single organization.

## Roles and Responsibilities

### Maintainers

Maintainers have write access to the OpenChoreo GitHub repositories and are responsible for the overall health, direction, and sustainability of the project. A list of current maintainers is maintained in the [MAINTAINERS.md](./MAINTAINERS.md) file.

Maintainer responsibilities include:

- Defining and evolving the project roadmap and architecture
- Reviewing and approving code and documentation changes
- Ensuring consistency with architectural and design principles
- Managing releases and versioning
- Upholding the CNCF Code of Conduct
- Supporting and mentoring contributors

Being a maintainer is a privilege that comes with responsibility. Maintainers are expected to act in the best interests of the project and its community.

### Contributors

Contributors are anyone who participates in the project through code contributions, documentation, issue reporting, reviews, design discussions, or community support.

All contributors are encouraged to:

- Submit issues and pull requests
- Participate in technical and design discussions
- Review code and documentation
- Help other community members

No formal approval is required to become a contributor.

## Decision Making

OpenChoreo follows a consensus-driven decision-making model inspired by other CNCF projects.

### Lazy Consensus

Most day-to-day decisions are made using lazy consensus. Proposals are considered accepted if no substantive objections are raised within a reasonable time period (typically 3–5 business days).

### Voting

Formal votes are used when consensus cannot be reached or when required for specific governance decisions.

Voting rules:

- One vote per maintainer
- Votes may be conducted asynchronously via GitHub issues, pull requests, or mailing lists

A formal vote is required for:

- Adding new maintainers
- Removing maintainers
- Major architectural or governance changes

## Becoming a Maintainer

New maintainers are selected based on demonstrated merit and sustained contributions to the project. Typical criteria include:

- Sustained and meaningful contributions over time (code, documentation, design discussions, or reviews)
  - perform reviews for at least 10 non-trivial PRs
  - contribute at least 20 non-trivial PRs and have them merged
- Demonstrated technical leadership and architectural understanding
- Active participation in project discussions and reviews
- Familiarity with project workflows, contribution guidelines, and quality standards
- A track record of constructive collaboration within the community

A new maintainer must be nominated by an existing maintainer and approved by a **supermajority (at least two-thirds)** of the current maintainers.

## Removing a Maintainer

Maintainers may step down voluntarily at any time.

A maintainer may be removed due to:

- Prolonged inactivity (typically six months or more without meaningful participation, unless a planned return is communicated)
- Repeated failure to meet maintainer responsibilities
- Violation of the CNCF Code of Conduct
- Actions detrimental to the project or community

Removal requires approval by a **supermajority** of the remaining maintainers.

## Meetings

Where feasible, maintainers are encouraged to participate in public community meetings, office hours, or maintainer syncs.

Closed meetings may be held to address security issues or Code of Conduct matters. All maintainers, except those directly involved in a reported Code of Conduct issue, must be invited to such meetings.

## Code of Conduct

OpenChoreo adopts the CNCF Code of Conduct. All participants are expected to adhere to it in all project spaces.

## Security

Security vulnerabilities should be reported privately following the process described in [SECURITY.md](./SECURITY.md).

## Amendments

Changes to this governance document require:

- A pull request proposing the change
- Approval by a **supermajority** of maintainers

## Acknowledgements

This governance model is inspired by governance documents from other CNCF Sandbox and Incubating projects and is intentionally lightweight to support the project’s current stage while allowing room to evolve as the community grows.
