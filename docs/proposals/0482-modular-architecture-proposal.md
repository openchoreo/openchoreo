# Modular architecture for OpenChoreo

**author**:
@lakwarus

**Reviewers**: 
@sameerajayasoma
@binura-g
@manjulaRathnayaka

**status**: 
Draft

**created**: 
2025-09-29

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

## Why Not Just Vanilla Kubernetes?

- **Abstraction:**  
  - *Vanilla K8s:* Infrastructure-level primitives (pods, YAML).  
  - *OpenChoreo Secure Core:* Enterprise-ready abstractions (Org, Project, Component, Environment).  

- **Zero Trust Security:**  
  - *Vanilla K8s:* Must configure manually.  
  - *OpenChoreo Secure Core:* Built-in Zero Trust (Cilium Edition).  

- **Multi-Environment Support:**  
  - *Vanilla K8s:* Namespaces + pipelines.  
  - *OpenChoreo Secure Core:* CRDs for structured environments.  

- **Enterprise Support:**  
  - *Vanilla K8s + Cilium:* DIY or expensive vendor contracts.  
  - *OpenChoreo Secure Core:* Community-supported OSS (with commercial support available separately).  

---

## Add-On Modules

| **Module** | **Name** | **Description** | **Cilium Edition** | **Generic Edition** |
|------------|----------|-----------------|---------------------|----------------------|
| CD | OpenChoreo CD Plane | GitOps-driven Continuous Delivery (Argo CD). | ✅ | ✅ |
| CI | OpenChoreo Build Plane | CI pipelines, build packs, container scans. | ✅ | ✅ |
| Observability | OpenChoreo Observe Plane | Logs, metrics, tracing, cell diagrams. | ✅ Full | ⚠️ Basic only |
| API Gateway | OpenChoreo API Gateway Plane | External & internal API management. | ✅ | ✅ |
| Automation | OpenChoreo Automate Plane | General-purpose pipelines. | ✅ | ✅ |
| Elasticity | OpenChoreo Elastic Plane | Autoscaling & scale-to-zero (KEDA). | ✅ | ✅ |
| Governance | OpenChoreo Guard Plane | Compliance, egress control, approvals. | ✅ | ❌ |
| Registry | OpenChoreo Registry Plane | Embedded/external registry (Harbor). | ✅ | ✅ |
| Vault | OpenChoreo Vault Plane | Secret management (Vault/OpenBao). | ✅ | ✅ |
| AI Gateway | OpenChoreo AI Gateway Plane | Secure LLM/AI inference APIs. | ✅ | ✅ |
| AI Intelligence | OpenChoreo Intelligence Plane | AI-powered ops/dev workflows. | ✅ | ✅ |

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
