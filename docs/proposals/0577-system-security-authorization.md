# Openchoreo System Security - Authorization 

**Authors**:  
@binoyPeries
@mevan-karu 

**Reviewers**:  
@lakwarus 
@sameerajayasoma 
@tishan89 
@manjulaRathnayaka 
@Mirage20 
@binura-g

**Created Date**:  
2025-10-30

**Status**:  
Submitted

**Related Issues/PRs**:  
- Proposal Discussion: [#569](https://github.com/openchoreo/openchoreo/discussions/569)
- Epic: [#577](https://github.com/openchoreo/openchoreo/issues/577)

## Summary

OpenChoreo needs a consistent, hierarchy‑aware authorization (AuthZ) model that answers:

- Can subject U perform action X on resource Y?

- Which resource instances of type T can U access?

- What are U’s effective permissions?

We will introduce an AuthZ module inside the OpenChoreo API Server (PEP) that acts as a Policy Decision Point (PDP) behind a stable interface. The module is intentionally designed with clean abstractions so it can be extracted to a separate service later if needed. A pluggable policy/relationship engine (e.g., OPA, Casbin, OpenFGA) provides decision logic.

## Motivation

- Core limitation: Kubernetes‑native authorization is not sufficient for OpenChoreo’s domain.

  - K8s RBAC  — limitation: Flat relative to OpenChoreo’s org→sub‑org→project→env→component hierarchy; cannot express effective scope or filter reads/lists server‑side without role/binding explosion.

  - K8s Webhook authorizer mode — limitation: SAR lacks a place to carry OpenChoreo’s domain context; results can’t be used to filter responses consistently.
- Personas need scoped roles: Users need roles/permissions bound at different scopes with predictable least‑privilege behavior.
- Client UX needs server‑side filtering: Backstage and API consumers require consistent answers (e.g., “which projects/components can I view/deploy?”).
- Cross‑plane consistency: Control plane, build plane, and data plane should share the same authorization semantics and results.
---

## Goals

- Provide a uniform contract for authorization decisions used by all CP clients (e.g., Backstage UI) and internal services.

- Support hierarchical scoping (org, sub‑org, project, component).

- Enable role/permission modeling and group→role mapping from external IdPs.

- Ship an OOB default role set (PE, Developer, Promoter, Admin, etc.).

---

## Non-Goals

- Replacing Kubernetes RBAC at the cluster level.
- Authentication mechanisms (handled by  IdP integrations).

## Impact

- CP/API clients : No changes required. Existing CP API endpoints will enforce authorization and return server-side filtered results based on effective scope.
- Control Plane API: Must be updated to invoke AuthZ before executing any request handler logic (both read and write). 

### Backward compatibility

- Existing API paths remain; behavior change is that results are now authorization-filtered.
- Clients do not need to add new calls or headers beyond current AuthN; remove any redundant client-side filtering to avoid double-filtering.
- Cluster-level Kubernetes RBAC remains unchanged; this proposal governs OpenChoreo domain authorization.

### Performance considerations

- Additional decision step adds small latency; can be reduced with caching in the future if needed.
- List endpoints may fan out internally but will avoid N+1 client calls.


## Design

### Overview
- API server–owned enforcement: The API layer is responsible for AuthZ. It invokes the AuthZ service via a  interface (implemented as an in‑process package in v1, with a clean seam to move out-of-process later).

- Pre-execution check: Each API endpoint calls AuthZ before executing business logic or reaching out to Kubernetes — for both reads and writes.

- All AuthZ checks occur behind existing API endpoints; no client→ AuthZ service access and no client changes.


## Appendix (optional)

_Any extra context, links to discussions, references, etc._
