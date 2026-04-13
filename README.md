<div align="center">
  <br />
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./docs/images/openchoreo-horizontal-white.png">
    <img alt="OpenChoreo Logo" src="./docs/images/openchoreo-horizontal-color.png" width="450">
  </picture>

  <h1>
    A complete, open-source developer platform for Kubernetes
  </h1>
  <h2>
    Ready to use on day one, built to integrate with your stack
  </h2>

<!-- License & Community -->
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fopenchoreo%2Fopenchoreo.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fopenchoreo%2Fopenchoreo?ref=badge_shield)
[![CNCF Sandbox](https://img.shields.io/badge/CNCF-Sandbox-00ADD8?logo=cloud-native-computing-foundation&logoColor=white)](https://www.cncf.io/projects/openchoreo/)
[![Twitter Follow](https://img.shields.io/twitter/follow/openchoreo?style=social)](https://x.com/openchoreo)
[![Slack](https://img.shields.io/badge/slack-openchoreo-blue?logo=slack)](https://cloud-native.slack.com/archives/C0ABYRG1MND)

<!-- Security & Compliance -->
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/11819/badge)](https://www.bestpractices.dev/projects/11819)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/openchoreo/openchoreo/badge)](https://scorecard.dev/viewer/?uri=github.com/openchoreo/openchoreo)
[![CLOMonitor](https://img.shields.io/endpoint?url=https://clomonitor.io/api/projects/cncf/openchoreo/badge)](https://clomonitor.io/projects/cncf/openchoreo)
[![LFX Health Score](https://insights.linuxfoundation.org/api/badge/health-score?project=openchoreo)](https://insights.linuxfoundation.org/project/openchoreo)

<!-- Build, Quality & Project Info -->
[![Build and Test](https://github.com/openchoreo/openchoreo/actions/workflows/build-and-test.yml/badge.svg?branch=main)](https://github.com/openchoreo/openchoreo/actions/workflows/build-and-test.yml)
[![Codecov](https://codecov.io/gh/openchoreo/openchoreo/branch/main/graph/badge.svg)](https://codecov.io/gh/openchoreo/openchoreo)
[![Go Report Card](https://goreportcard.com/badge/github.com/openchoreo/openchoreo)](https://goreportcard.com/report/github.com/openchoreo/openchoreo)
[![GitHub Release](https://img.shields.io/github/v/release/openchoreo/openchoreo)](https://github.com/openchoreo/openchoreo/releases/latest)
[![GitHub last commit](https://img.shields.io/github/last-commit/openchoreo/openchoreo.svg)](https://github.com/openchoreo/openchoreo/commits/main)
[![GitHub issues](https://img.shields.io/github/issues/openchoreo/openchoreo.svg)](https://github.com/openchoreo/openchoreo/issues)

</div>

## What is OpenChoreo?

OpenChoreo is a developer platform for Kubernetes offering development and architecture abstractions, a Backstage-powered developer portal, application CI/CD, GitOps, RBAC and observability.

OpenChoreo orchestrates Kubernetes and other CNCF and open-source projects as a domain-driven, API-first platform to give platform teams a strong head start. You can use it as-is, or tailor it to fit your own Internal Developer Platform (IDP) vision.

<picture>
  <img src="./docs/images/openchoreo-architecture-diagram.png"
  alt="OpenChoreo architecture"/>
</picture>

## Key features

- **Modular, multi-plane platform architecture**

  Independently deployable control, data, build, and observability planes separate concerns with clear boundaries and flexible deployment topologies, from a single Kubernetes cluster to massively distributed fleets.

- **Platform abstractions (APIs) as building blocks**
  
  Core platform concepts are exposed as declarative APIs (environments, gateways, pipelines/workflows, component types, modules, etc.), so topology and delivery behavior can be standardized across an organization.

- **Programmable developer abstractions**

  Developers use higher-level, extensible Kubernetes-native abstractions (projects, components, endpoints, dependencies) and golden paths to ship without dealing with the full surface area of the Kubernetes API.

- **Intelligent, integrated observability**

  Unified access to distributed logs, metrics, traces, and alerts and exposed via APIs. A unified platform model enriched with observability data allows for faster debugging and operational actions for humans and AI.

- **Built-in agents**

  Agents are first-class platform citizens.
  Includes an SRE agent for root cause analysis and remediation, a FinOps agent for cost optimization, and more.

- **AI-assisted/driven engineering and operations**

  A controlled agent interface with MCP servers, skills, and the CLI lets AI assistants and agents participate in development, delivery, and operations, without bypassing guardrails.

- **GitOps: Declarative platform + app state**

  Platform and application state are reconciled from Git for auditability and drift resistance, with GUI and CLI support for imperative actions when speed matters (or if that's what you prefer).

- **Multi-tenancy and access controls**

  Built-in tenancy boundaries and role-based access control enable safe self-service across teams, projects, and environments with least-privilege access.

- **Modules catalog**

  Integrate external tools into OpenChoreo's unified platform experience using community-driven marketplace modules, or build your own.

## Documentation

OpenChoreo's documentation is available at [openchoreo.dev](https://openchoreo.dev).

## Getting Started

The easiest way to try OpenChoreo is by following the **[Quick Start Guide](https://openchoreo.dev/docs/getting-started/quick-start-guide/)**. 

Visit the **[Installation Guides](https://openchoreo.dev/docs/category/try-it-out/)** to learn more about installing OpenChoreo on Kubernetes for further evaluation and production use.

For a deeper understanding of OpenChoreo's architecture, see **[OpenChoreo Architecture](https://openchoreo.dev/docs/overview/architecture)** and **[OpenChoreo Concepts](https://openchoreo.dev/docs/category/concepts/)**.

## Join the Community & Contribute

We’d love for you to be part of OpenChoreo’s journey! 
Whether you’re fixing a bug, improving documentation, or suggesting new features, every contribution counts.

- **[Contributor Guide](./docs/contributors/README.md)** – Learn how to get started.
- **[Report an Issue](https://github.com/openchoreo/openchoreo/issues)** – Help us improve Choreo.
- **[Join our Slack](https://cloud-native.slack.com/archives/C0ABYRG1MND)** – Be part of the community.

We’re excited to have you onboard!

## Roadmap

We maintain an OpenChoreo Roadmap as a GitHub project board to share what we’re building and when we expect to deliver it.

See the [OpenChoreo Roadmap](https://github.com/orgs/openchoreo/projects/5/views/2)

### How It Works

- **Backlog**: New tasks are added to the [Backlog](https://github.com/orgs/openchoreo/projects/5/views/1) and are prioritized by the OpenChoreo team
- **GitHub discussions**: OpenChoreo is designed in the open. [GitHub discussions](https://github.com/openchoreo/openchoreo/discussions) are used to discuss and review new features and improvements
- **Release management**: Epics that track the progress of a high-level task is assigned to a planned release in the [roadmap](https://github.com/orgs/openchoreo/projects/5/views/2)

## License
OpenChoreo is licensed under Apache 2.0. See the **[LICENSE](./LICENSE)** file for full details.

[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fopenchoreo%2Fopenchoreo.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2Fopenchoreo%2Fopenchoreo?ref=badge_large)

---

<div align="center">
  <h2>OpenChoreo is a <a href="https://www.cncf.io/">CNCF</a> Sandbox Project</h2>
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./docs/images/cncf-logo-white.png">
    <img src="./docs/images/cncf-logo.svg" width="400" alt="CNCF Logo"/>
  </picture>
</div>
