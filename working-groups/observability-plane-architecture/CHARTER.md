# Working Group Charter: Observability Architecture

| | |
|---|---|
| **Status** | Draft — v0.1 |
| **Created** | June 2026 |
| **Last Updated** | June 2026 |
| **WG Lead** | [@sameerajayasoma](https://github.com/sameerajayasoma) |
| **Co-Lead(s)** | [@nilushancosta](https://github.com/nilushancosta), [@akila-i](https://github.com/sameerajayasoma)  |
| **Core Members** | [@lakwarus](https://github.com/lakwarus), [@binura-g](https://github.com/binura-g), [@tishan89](https://github.com/tishan89), [@Mirage20](https://github.com/Mirage20), [@LakshanSS](https://github.com/LakshanSS) |
| **Slack** | `#openchoreo-dev` on [CNCF Slack](https://cloud-native.slack.com) |
| **Meetings** | Biweekly — see [WG community calendar](https://zoom-lfx.platform.linuxfoundation.org/meetings/openchoreo?committee=6f05c460-a211-488b-8176-010c0cf1bcba&view=week) |
| **GitHub Label** | [`wg/observability-architecture`](https://github.com/openchoreo/openchoreo/labels/wg%2Fobservability-architecture) |

---

## Background and Motivation

OpenChoreo's observability plane provides a well-defined foundation for collecting and querying logs, metrics, and distributed traces across workflow and data planes. By default it ships with
three capabilities: logs (OpenSearch), metrics (Prometheus), and tracing (OTEL collector + OpenSearch) — along with an adapter pattern that allows organizations to plug in external observability systems such as Datadog, Grafana Cloud, or cloud-provider-native solutions instead.

The plane exposes an Observer API (OpenAPI-v3) and its own MCP server, keeping observability data out of the control plane's critical path and enabling direct querying from the experience plane and AI agents.

As the project grows, new capabilities are being planned that stretch beyond this original three-pillar model:

- **DORA Metrics** — engineering effectiveness signals derived from CI/CD pipeline events in the workflow plane, cross-referenced with deployment events from the control plane.
- **Cost Insights** — resource consumption data cross-referenced with cloud billing APIs, supporting the built-in FinOps agent.
- **Kubernetes Event Support** — the K8s event stream as a first-class observability signal for platform-level visibility.
- **AI Platform Agents** — the SRE, FinOps, and Architect agents all require domain-enriched observability data via the Observer MCP.

These new capabilities introduce architectural questions that the existing module model does not
answer:

- **Data model** — DORA and cost are not raw signals like logs or metrics; they are derived insights computed across multiple signal types. The plane has no classification or extension model that distinguishes these two classes.
- **Infrastructure fit** — the existing backends (OpenSearch, Prometheus) are well-suited for raw signals but may not be the right fit for cross-signal aggregation and computation required by derived insights. It is not yet clear whether new infrastructure components are needed or whether the existing backends can be extended.
- **External system behaviour** — when OpenChoreo is configured to use an external observability system (Datadog, CloudWatch, Google Cloud Monitoring, Azure Monitor, etc.), the adapter pattern works well for raw signal querying, but the behaviour for derived insights is undefined. Some derived insights (e.g., cost) may actually be better sourced from the cloud provider directly, while others (e.g., DORA) require local computation regardless of where raw signals are stored.

Without a consistent architectural framework addressing all three of these questions, each new feature risks being implemented as a one-off addition with its own collection agent, storage model, computation logic, and Observer API surface. The result would be a bloated, inconsistent observability plane that is hard to reason about, hard to extend, and hard to operate.

This working group is formed to define that consistent framework before the proliferation takes hold.

---

## Mission

The Observability Architecture Working Group exists to define a principled, extensible architecture for the OpenChoreo observability plane, one that can accommodate the current and future set of
observability capabilities without becoming an ad-hoc collection of one-off integrations, and that behaves consistently whether OpenChoreo is using its default module stack or an external observability system.

The WG produces architectural specifications and design guidelines, not feature implementations. Implementation work follows after the architecture is agreed, carried out by contributors under the project's standard development process.

---

## Scope

### In Scope

- **Observability Data Model** — classification of all data types the observability plane must handle, distinguishing raw infrastructure signals (logs, metrics, traces, K8s events) from derived domain insights (DORA metrics, cost insights). Each class has different collection,computation, storage, and query characteristics that the architecture must explicitly acknowledge and address consistently.

- **Infrastructure Assessment** — for each data class, determine whether existing backends (OpenSearch, Prometheus) are sufficient, require extension, or whether a new infrastructure component is needed. Define the minimum viable infrastructure footprint that supports both default and external-system configurations without unnecessary bloat.

- **Module Extension Contract** — a formal interface specification that any new observability module must satisfy, covering: collection agent contract, Observer API adapter interface, MCP surface contribution, computation/aggregation contract for derived insights, lifecycle hooks, and schema registration.

- **Observer API Design Principles** — resource model, versioning strategy, domain-enrichment conventions, query pattern standards, and backward compatibility guarantees. Applies to both the OpenAPI-v3 REST interface and the MCP server surface.

- **Domain Insights Layer Architecture** — how derived insights (DORA, cost) are composed over raw signals, including: aggregation and computation model, storage strategy, query path, and the compute-vs-query split that determines what must run locally vs. what can be delegated to an external system.

- **External System Behaviour** — for each new capability, define how it behaves when OpenChoreo is configured with an external observability system (Datadog, Splunk, CloudWatch, Google Cloud Monitoring, Azure Monitor, etc.). This includes identifying which capabilities require a minimal local compute component regardless of external configuration, and which can be fully sourced from or delegated to the external system.

- **Collection Agent Extension Model** — how new signal types are collected from data and workflow planes, enriched with domain metadata (plane, namespace, project, component), and forwarded to the observability plane without bypassing existing enrichment pipelines.

- **AI Agent Integration Standards** — how the SRE, FinOps, and Architect agents query observability data via the Observer MCP, and what new observability features must provide to be agent-queryable.

- **Implementation Roadmap** — a prioritised, architecture-aligned backlog for DORA, cost insights, and K8s event features, ready for handoff to feature development teams.

### Out of Scope

- Implementation of any individual feature (DORA, cost insights, K8s events). These are downstream of the WG's architecture work.
- Changes to the control plane, data plane, or workflow plane architecture, except where observability collection requirements necessitate a cross-plane contract change.
- UI/UX design for observability features in the Backstage portal.
- Selection of specific backend technologies where the existing module model already handles this (e.g., swapping OpenSearch for Loki). Technology choices within the bounds of the extension contract remain the responsibility of module maintainers.
- Governance of the broader OpenChoreo project outside the observability plane.

---

## Deliverables

All deliverables are published as GitHub Discussions for community RFC review, and once agreed, merged as Markdown documents to `docs/architecture/observability/` via a standard PR.

| ID | Deliverable | Target | Description |
|---|---|---|---|
| D1 | Observability Data Model | Month 1 | Classification of all data types the observability plane must handle — raw signals (logs, metrics, traces, K8s events) vs. derived domain insights (DORA, cost) — with the implications of each class for collection, computation, storage, and querying. |
| D2 | Infrastructure Assessment | Month 1 | For each data class, an assessment of whether existing backends (OpenSearch, Prometheus) are sufficient or require extension, and whether any new infrastructure components are needed. Covers both default module stack and external system configurations. |
| D3 | Module Extension Contract | Month 1 | Specification defining the interface any new observability module must implement: collection agent contract, Observer API adapter interface, computation/aggregation contract for derived insights, MCP surface requirements, and lifecycle hooks. |
| D4 | Observer API Design Principles | Month 1 | Versioned resource model, query pattern conventions, domain-enrichment standards, and guidance on how new features expose themselves via the Observer API and MCP server. |
| D5 | Domain Insights Layer Architecture | Month 2 | Architecture RFC covering how derived insights (DORA, cost) are computed and composed over raw signals — including the compute-vs-query split, aggregation model, storage strategy, query path, and behaviour under external system configurations. |
| D6 | AI Agent Integration Guidelines | Month 2 | Documentation on how the SRE, FinOps, and Architect agents consume observability data via the Observer MCP, and standards for new agent-facing observability capabilities. |
| D7 | Implementation Roadmap | Month 2 | Prioritised backlog of features (DORA, cost, K8s events) structured according to the agreed architecture, ready for handoff to feature development teams. |

---

## Timeline

The WG is chartered for a **2-month period**, targeting completion of all deliverables by end of August 2026. Upon completion the WG formally dissolves and ownership of the architecture documents transfers to the project's maintainer group.

| Month | Milestone | Key Activities |
|---|---|---|
| Month 1 | Kickoff + Data Model + Infra Assessment (D1, D2) | First WG meeting, agree on scope, produce data model classification, begin infrastructure assessment and module contract draft. |
| Month 1,2 | Architecture RFCs (D3, D4, D5) | Open GitHub Discussions for each RFC; community review and iteration; present at community call. |
| Month 2 | Finalise + Handoff (D6, D7) | AI agent integration guidelines, consolidated implementation roadmap, formal close or transition to maintainer ownership. |

> If the WG determines that ongoing architectural governance of the observability plane is needed
> beyond this charter period, it may propose conversion to a permanent SIG via a PR to
> [`GOVERNANCE.md`](../../GOVERNANCE.md).

---

## Working Mode

### Meetings

- Biweekly video call, 60 minutes, listed on the CNCF community calendar.
- Meeting notes published to the WG's GitHub Discussion thread within 48 hours.
- Meetings are open to all community members. Core members are expected to attend or send a representative.
- Recordings optionally posted to the OpenChoreo YouTube channel when available.

### Communication

- **Async:** `#openchoreo-dev` on [CNCF Slack](https://cloud-native.slack.com). Tag your message
  with `[wg-observability-architecture]` for visibility.
- **RFCs and proposals:** GitHub Discussions in `openchoreo/openchoreo`, tagged
  [`wg/observability-architecture`](https://github.com/openchoreo/openchoreo/labels/wg%2Fobservability-architecture).
- **Issues and PRs:** labelled
  [`wg/observability-architecture`](https://github.com/openchoreo/openchoreo/labels/wg%2Fobservability-architecture).

### Decision Making

The WG uses **lazy consensus** as its default decision model. A proposal posted to GitHub Discussions is considered accepted after 3 business days with no blocking objection from a core member or maintainer.

For contentious decisions where lazy consensus cannot be reached, the WG Lead calls a vote among core members. A simple majority carries the decision, with the WG Lead holding a casting vote in the event of a tie.

Decisions that modify the module extension contract, Observer API design principles, or introduce new infrastructure components require review by at least one OpenChoreo maintainer outside the WG before acceptance.

### RFC Process

- RFCs are opened as GitHub Discussions using the structured template: Problem, Proposed Solution, Alternatives Considered, Open Questions.
- Minimum 7-calendar-day community review period before a WG meeting vote.
- Accepted RFCs are merged to `docs/architecture/observability/` via a PR requiring two approvals from WG core members.
- RFCs that change cross-plane contracts or introduce new infrastructure components also require approval from a maintainer of the affected plane or component area.

---

## Relationship to Project Governance

This working group operates under OpenChoreo's governance model as defined in [`GOVERNANCE.md`](../../GOVERNANCE.md). The WG Lead reports to the maintainer group and presents a progress update at the monthly community call.

The WG does not have authority to merge code or release features. Its outputs are architectural specifications that inform and constrain subsequent implementation PRs, which follow the standard contribution and review process.

Any proposed change to the Observer API's public surface, the module extension contract, or the introduction of new infrastructure components must be reviewed by the project's maintainers before the WG adopts it as a standard.

This charter may be amended by a PR to this file, subject to lazy consensus from WG core members and approval from one maintainer.

---

## Success Criteria

The WG is considered successful when:

1. All seven deliverables (D1–D7) are merged to the repository and acknowledged by the maintainer group.
2. The infrastructure assessment (D2) provides a clear decision on whether new components are needed, with that decision reflected in the implementation roadmap.
3. At least two of the planned features (DORA, cost insights, K8s events) have implementation proposals that explicitly reference and conform to the WG's architecture documents.
4. The module extension contract (D3) is used as the review checklist for at least one new observability module PR before the WG dissolves.
5. At least three organisations outside WSO2 have participated in WG discussions or RFC reviews.

---

## Reference Documents

- [OpenChoreo Architecture Overview](https://openchoreo.dev/docs/overview/architecture/)
- [OpenChoreo Observability](https://openchoreo.dev/explore/observability/)
- [OpenChoreo GOVERNANCE.md](../../GOVERNANCE.md)
- [CNCF Working Group Guidelines](https://github.com/cncf/toc/blob/main/workinggroups/)
- [OpenChoreo GitHub Discussions](https://github.com/openchoreo/openchoreo/discussions)
- [CNCF Slack](https://cloud-native.slack.com)

---

*OpenChoreo is a CNCF Sandbox project. Copyright © 2026 OpenChoreo Project Authors. Apache 2.0 License.*
