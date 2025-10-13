# Modular architecture for OpenChoreo

**author**:
@lakwarus

**Reviewers**: 
@sameerajayasoma
@binura-g
@manjulaRathnayaka

**status**: 
Accepted

**created**: 
2025-09-29

**updated**
2025-10-13

**issue**: 
[Issue #0482 – openchoreo/openchoreo](https://github.com/openchoreo/openchoreo/discussions/482)

---

# Summary

This proposal introduces a **modular architecture for OpenChoreo**, enabling adopters to start with a **secure, production-ready Kubernetes foundation** and incrementally add modules as their needs grow.  

The design provides two entry points:
- **OpenChoreo Secure Core (Cilium Edition):** Full Zero Trust security, traffic encryption, and support for all add-on modules.  
- **OpenChoreo Secure Core (Generic CNI Edition):** Basic Kubernetes networking with limited add-on support, providing a lower-friction entry with a natural upgrade path to the Cilium Edition.  

This modular approach lowers adoption friction, supports production use cases from day one, and creates a **land-and-expand path** toward a full Internal Developer Platform (IDP).

---

# Motivation

Adopters of Internal Developer Platforms (IDPs) typically seek two things:
1. **Low-friction entry:** Start simple, prove value quickly.  
2. **Scalable growth path:** Add features as the organization matures.  

With earlier approaches (e.g., monolithic designs), challenges arose:
- Complex architecture → slow feature delivery and high infra cost.  
- Adoption required multiple teams to align, leading to long, difficult cycles.  

A modular OpenChoreo addresses these issues by:
- Delivering a **default Secure Core** that is immediately valuable.  
- Allowing adopters to **add modules incrementally**, instead of an “all-or-nothing” platform.  
- Creating a **clear upgrade path** from initial adoption → enterprise-grade platform without replatforming.  

---

# Goals

- Provide a **modular open-source architecture** that adopters can consume incrementally.  
- Offer a **secure Kubernetes foundation** with built-in abstractions (Org, Projects, Components, Environments).  
- Support both **Cilium** and **generic CNIs** to meet adopters where they are.  
- Deliver **tangible benefits** to Developers (simplified onboarding) and Platform Engineers (Zero Trust, control, governance).  
- Establish a **progressive adoption path**: Secure Core → Core Modules (CD + Observe) → Expansion (CI, API, Elastic) → Enterprise (Guard, Vault, AI).  

---

# Non-Goals

- Targeting **individual hobbyist developers**.  
- Replacing existing CI/CD pipelines; instead, OpenChoreo integrates with them.  

---

# Impact

### Positive
- **Lower entry barrier** → easy for teams to adopt.  
- **Progressive expansion path** → natural growth into advanced modules.  
- **Clearer messaging** → modular story is easier for the community to understand.  
- **Improved adoption experience** → Devs and PEs get immediate value.  

### Negative / Risks
- Operating two Secure Core flavors (Cilium vs Generic) adds **support overhead**.  
- Migration tooling will be required for adopters moving from Generic → Cilium.  
- If adopters stay on Generic Edition, they miss advanced features (e.g., governance).  

---

# Design

## OpenChoreo Secure Core (Default Platform)

### Description
- Zero Trust–enabled Kubernetes baseline.  
- Supports key abstractions: **Org, Project, Components, Environments**.  
- Provides network isolation, CLI, UI, and AI-assisted lifecycle management.  
- Minimal operational footprint.  

### Technology
- Kubernetes  
- CNI: Cilium (Cilium Edition) or Generic (Calico, Flannel, cloud-provider CNI)  
- Ingress Gateway (NGINX IC)  
- External Secrets Controller ([external-secrets.io](https://external-secrets.io/latest/))  
- Thunder + Envoy GW (for CP APIs)  
- Cert-Manager  
- CLI / UI / AI Webapp  

### Topology
- Single cluster, single environment by default.  
- OpenChoreo CRDs support **multi-environment definitions** within the same cluster.  
- No built-in GitOps (external GitOps can be plugged in).  
- No CI/CD, Observability, or API Gateway in the base.  

### Benefits
**For Developers**  
- Secure, isolated projects without Kubernetes complexity.  
- CLI + UI to create/manage projects & components.  
- Reduced friction compared to raw Kubernetes.  

**For Platform Engineers**  
- Enforce Zero Trust.  
- Simplified cluster onboarding.  
- Higher-level controls vs vanilla Kubernetes.  

---

## Secure Core Flavors

### Secure Core (Cilium Edition)
- Cilium is the mandatory CNI.  
- Enables full Zero Trust networking with traffic encryption.  
- Supports **all add-on modules**, including advanced observability and governance.  
- Target: Enterprises and advanced teams building toward a full IDP.  

### Secure Core (Generic CNI Edition)
- Runs with generic CNI (Calico, Flannel, cloud-provider).  
- Provides basic network isolation (no encryption).  
- Limited add-on compatibility:  
  - Observability: basic logs & metrics only.  
  - Governance & Security: not supported.  
- Other add-ons (CD, CI, API Gateway, Elastic, Automate) supported.  
- Target: Teams that want a quick start with existing network stacks.  

---

### Why Choreo Secure Core Matters

The Secure Core is designed as the **lowest barrier to entry** into the OpenChoreo ecosystem.  
It offers a **better-than-vanilla Kubernetes experience** — secure by default, modular by design — without forcing adoption of CI/CD or multi-environment pipelines on day one.

#### Strategic Rationale
- **Minimize Adoption Friction:**  
  Start with a secure, production-ready cluster that integrates with existing pipelines.  
- **Support Real Production Use Cases:**  
  Secure Core can run production workloads out of the box.  
- **Progressive Expansion:**  
  Add advanced modules (Observability, Governance, Security) as maturity grows.  

---

## Planes and Deployment Model

### Motivation for Planes
As OpenChoreo evolves into a modular and scalable architecture, it needs a flexible deployment model that can adapt to different organizational needs and scales.  

To achieve this, OpenChoreo introduces the concept of **Planes** — logical deployment boundaries that allow modules to be deployed:
- On the same Kubernetes cluster (co-located),
- On separate Kubernetes clusters (isolated),
- Or shared across multiple clusters.

This provides flexibility in balancing **security**, **cost**, and **operational scalability**, without complicating the user experience.

---

### What Is a Plane
A **Plane** is a named grouping for one or more modules that share similar operational characteristics — such as scaling behavior, resource isolation, or control boundaries.  

Typical planes include:
- **Control Plane** – Runs the OpenChoreo API, registry, and control services.  
- **Data Plane** – Hosts user workloads, ingress gateways, and runtime components.  
- **Observability Plane** – Runs backend observability services (log collection, metrics aggregation, tracing, indexing).  

Each plane may correspond to a separate Kubernetes cluster or be co-located depending on the deployment footprint and scale.

---

### Plane-Aware Module Deployment

Each module declares a `plane` property indicating where it should be deployed.  
Operators can override or customize this placement to suit their topology.

**Examples:**

- **Choreo Observe (Observability Plane)**  
  - Runs all backend services for logs, metrics, and tracing aggregation.  
  - Dashboards may run elsewhere, but raw telemetry data is stored and processed within the Observability Plane.  
  - A single Observability Plane can serve multiple Data Planes — a **cost-effective pattern** compared to running separate observability stacks per Data Plane.

- **Choreo Build (CI Plane)**  
  - Can run on the **Data Plane** or a **dedicated CI Plane** for isolation or scale.  
  - Example: Enterprises can deploy CI pipelines in a separate CI Plane for resource isolation.

- **Choreo Elastic (Autoscaling Plane)**  
  - Must run within the **Data Plane**, since it directly interacts with runtime workloads and scaling controllers.

---

### Deployment Flexibility

| **Pattern** | **Description** |
|--------------|-----------------|
| **Single-Plane (Simple)** | All modules run on one Data Plane cluster — ideal for minimal setups. |
| **Multi-Plane (Intermediate)** | Separate Control and Data Plane clusters, with optional Observability Plane. |
| **Shared-Observability (Advanced)** | One Observability Plane serves multiple Data Planes to reduce cost and centralize telemetry. |
| **Fully Isolated (Enterprise)** | Each plane (Control, Data, CI, Observability) runs independently for scale, compliance, and security. |

---

### Operator Benefits
- **Flexibility:** Deploy modules where they fit best (shared or isolated).  
- **Cost Efficiency:** Share Observability Plane across multiple Data Planes.  
- **Security & Isolation:** Separate build, runtime, and monitoring concerns.  
- **Progressive Adoption:** Start with a single-plane setup, scale to multi-plane later.

---

### Summary
The **Plane** abstraction makes OpenChoreo topology-aware and future-ready.  
By defining clear deployment boundaries (Control, Data, CI, Observability, etc.), OpenChoreo can scale horizontally across clusters while maintaining operational simplicity.  
This aligns perfectly with the modular philosophy — **start minimal, expand progressively, deploy intelligently.**

---

## Add-On Modules

| **Module** | **Description** | **Tech Stack** | **Cilium Edition** | **Generic Edition** |
|-------------|-----------------|----------------|--------------------|---------------------|
| **Choreo CD** | GitOps-driven continuous delivery | Argo CD | ✅ | ✅ |
| **Choreo Build (CI)** | CI pipeline integration (builds & scans) | Argo Workflows, Argo Events | ✅ | ✅ |
| **Choreo Observe** | Unified metrics, logs, and traces | FluentBit, OpenSearch, Prometheus, Thanos, Velero | ✅ (Advanced) | ⚠️ (Common) |
| **Choreo Edge (API Gateway)** | External + internal API management | Envoy, Choreo API Mgmt | ✅ | ✅ |
| **Choreo Automate** | General-purpose pipelines | Argo Workflows + Events | ✅ | ✅ |
| **Choreo Elastic** | Autoscaling & scale-to-zero | KEDA | ✅ | ✅ |
| **Choreo Guard** | Governance & compliance | Cilium policies, workflows | ✅ | ❌ |
| **Choreo Registry** | Container registry | Harbor / Distribution | ✅ | ✅ |
| **Choreo Vault** | Secret management | HashiCorp Vault / OpenBao | ✅ | ✅ |
| **Choreo AI Gateway** | Secure LLM/AI inference APIs | WSO2 AI GW | ✅ | ✅ |
| **Choreo Intelligence** | AI-powered developer & ops insights | (Future) | ✅ | ✅ |

---

## Adoption Path

1. **Step 1 – Start with Secure Core**  
   - Cilium Edition: Full Zero Trust, encryption, add-ons supported.  
   - Generic Edition: Basic isolation, limited add-ons.  

2. **Step 2 – Add Core Modules (CD + Observe)**  
   - GitOps delivery + environment promotion.  
   - Advanced observability (Cilium) or basic logs/metrics (Generic).  

3. **Step 3 – Expand (Build, API, Elastic)**  
   - Add CI, API Gateways, autoscaling.  

4. **Step 4 – Enterprise (Guard, Vault, AI)**  
   - Governance, secrets, AI modules.  
   - Only Cilium Edition supports full governance.  

---

# Industry Use Cases

- **FinTech:** Secure Core (Cilium) + CD + Guard + Observe → Compliance & Zero Trust.  
- **SaaS Startup:** Secure Core (Generic) + Elastic + Build → Cost-optimized, quick start.  
- **Healthcare:** Secure Core (Cilium) + Guard + Vault + Observe → HIPAA-ready.  
- **AI Platforms:** Secure Core + API + AI Gateway + Intelligence → Secure AI delivery.  

---

# Next Steps
- Gather community feedback on modular design.  
- Align roadmap with **OpenChoreo 1.0 release**.  
- Define packaging for Secure Core + optional modules.  
