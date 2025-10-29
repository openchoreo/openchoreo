# Security Scanner Implementation Plan

## Overview
Implement a cluster-wide security scanner that watches Kubernetes resources (starting with Pods), resolves parent controllers (Pod -> ReplicaSet -> Deployment), runs Checkov policy checks, and stores findings. The scanner operates independently on any Kubernetes cluster without OpenChoreo-specific metadata, supports label-based querying, and avoids redundant scans via resourceVersion tracking.

## Architecture Summary
- Watch Pods created in cluster
- Resolve parent controller chain (Pod -> ReplicaSet -> Deployment)
- Run Checkov policies against resolved parent controller
- Store findings with resource metadata and labels
- Track scanned resourceVersion to skip redundant scans
- Expose REST API for querying findings by labels
- Future: Expand to ClusterRoleBinding, Ingress, etc.

## Data Model

### Schema Design Philosophy

The scanner will support multiple scan types (posture, supply chain, network, runtime) running in parallel. Each scan type:
- Tracks scanned resources independently (avoid rescanning unchanged resources)
- Stores findings with scan-specific metadata
- Shares common resource metadata and labels

### Naming Convention
- Scan tracking tables: `<scan_type>_scanned_resources` (e.g., `posture_scanned_resources`, `supply_chain_scanned_resources`)
- Findings tables: `<scan_type>_findings` (e.g., `posture_findings`, `supply_chain_findings`)
- Shared tables: `resources`, `resource_labels`

### Core Shared Tables

#### resources
Central registry of all Kubernetes resources seen by any scanner. Provides single source of truth for resource metadata.

```sql
CREATE TABLE resources (
  id INTEGER PRIMARY KEY,
  resource_type TEXT NOT NULL,
  resource_namespace TEXT NOT NULL,
  resource_name TEXT NOT NULL,
  resource_uid TEXT NOT NULL,
  resource_version TEXT NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(resource_type, resource_namespace, resource_name)
);
CREATE INDEX idx_resources_lookup ON resources(resource_type, resource_namespace, resource_name);
CREATE INDEX idx_resources_uid ON resources(resource_uid);
```

#### resource_labels
Stores labels for all resources. Shared across scan types for label-based querying.

```sql
CREATE TABLE resource_labels (
  id INTEGER PRIMARY KEY,
  resource_id INTEGER NOT NULL,
  label_key TEXT NOT NULL,
  label_value TEXT NOT NULL,
  UNIQUE(resource_id, label_key),
  FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
);
CREATE INDEX idx_resource_labels_lookup ON resource_labels(label_key, label_value);
CREATE INDEX idx_resource_labels_resource_id ON resource_labels(resource_id);
```

### Posture Scan Tables

#### posture_scanned_resources
Tracks which resources have been scanned by posture scanner and their scan metadata.

```sql
CREATE TABLE posture_scanned_resources (
  id INTEGER PRIMARY KEY,
  resource_id INTEGER NOT NULL,
  resource_version TEXT NOT NULL,
  scan_duration_ms INTEGER,
  scanned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(resource_id),
  FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
);
CREATE INDEX idx_posture_scanned_resources_resource_id ON posture_scanned_resources(resource_id);
```

#### posture_findings
Stores Checkov policy violations from posture scanning.

```sql
CREATE TABLE posture_findings (
  id INTEGER PRIMARY KEY,
  resource_id INTEGER NOT NULL,
  check_id TEXT NOT NULL,
  check_name TEXT NOT NULL,
  severity TEXT NOT NULL,
  category TEXT,
  description TEXT,
  remediation TEXT,
  resource_version TEXT NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
);
CREATE INDEX idx_posture_findings_resource_id ON posture_findings(resource_id);
CREATE INDEX idx_posture_findings_severity ON posture_findings(severity);
CREATE INDEX idx_posture_findings_check_id ON posture_findings(check_id);
CREATE INDEX idx_posture_findings_category ON posture_findings(category);
```

### Query Patterns

#### Upsert resource and get ID
```sql
INSERT INTO resources (resource_type, resource_namespace, resource_name, resource_uid, resource_version)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(resource_type, resource_namespace, resource_name) 
DO UPDATE SET resource_version = ?, resource_uid = ?, updated_at = CURRENT_TIMESTAMP
RETURNING id;
```

#### Check if posture scan needed
```sql
SELECT psr.resource_version 
FROM posture_scanned_resources psr
JOIN resources r ON psr.resource_id = r.id
WHERE r.resource_type = ? AND r.resource_namespace = ? AND r.resource_name = ?;
```

#### Insert posture finding
```sql
INSERT INTO posture_findings (resource_id, check_id, check_name, severity, category, description, remediation, resource_version) 
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
```

#### Query findings by labels
```sql
SELECT f.* 
FROM posture_findings f
JOIN resources r ON f.resource_id = r.id
JOIN resource_labels l ON r.id = l.resource_id
WHERE l.label_key = ? AND l.label_value = ?;
```

#### Query findings by resource
```sql
SELECT f.* 
FROM posture_findings f
JOIN resources r ON f.resource_id = r.id
WHERE r.resource_type = ? AND r.resource_namespace = ? AND r.resource_name = ?;
```

#### Delete old findings before reinserting
```sql
DELETE FROM posture_findings 
WHERE resource_id = (
  SELECT id FROM resources 
  WHERE resource_type = ? AND resource_namespace = ? AND resource_name = ?
);
```

### Future Scan Types (Examples)

When adding supply chain scanning:
```sql
CREATE TABLE supply_chain_scanned_resources (
  id INTEGER PRIMARY KEY,
  resource_id INTEGER NOT NULL,
  image_digest TEXT NOT NULL,
  scan_duration_ms INTEGER,
  scanned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(resource_id, image_digest),
  FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
);

CREATE TABLE supply_chain_findings (
  id INTEGER PRIMARY KEY,
  resource_id INTEGER NOT NULL,
  image_name TEXT NOT NULL,
  vulnerability_id TEXT NOT NULL,
  severity TEXT NOT NULL,
  package_name TEXT,
  fixed_version TEXT,
  description TEXT,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
);
```

When adding network scanning:
```sql
CREATE TABLE network_scanned_resources (
  id INTEGER PRIMARY KEY,
  resource_id INTEGER NOT NULL,
  network_policy_version TEXT NOT NULL,
  scanned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(resource_id),
  FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
);

CREATE TABLE network_findings (
  id INTEGER PRIMARY KEY,
  resource_id INTEGER NOT NULL,
  issue_type TEXT NOT NULL,
  severity TEXT NOT NULL,
  source_resource TEXT,
  destination_resource TEXT,
  description TEXT,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
);
```

### Benefits of This Design

- **Shared resource registry**: All scanners reference same `resources` table, avoiding duplication
- **Shared labels**: Query any scan type by labels without duplicating label storage
- **Independent scan tracking**: Each scanner tracks resourceVersion/digest independently
- **Parallel scanning**: Different scan types can run simultaneously without conflicts
- **Consistent patterns**: Adding new scan types follows same pattern (scanned_resources + findings tables)
- **Cascade deletes**: When resource deleted, all related scan data and labels cleanup automatically
- **Flexible queries**: Join through `resources` table to correlate findings across scan types

## Phase 1: Database Schema and Storage Layer

### Objective
Create database schema with proper tables, indexes, and SQLC queries for storing scanned resources, findings, and labels.

### Deliverables
- Migration files for SQLite and PostgreSQL
- SQLC query files for both backends
- Generated Go code and adapters

### Implementation Steps
- Create migration files:
  - `internal/security-scanner/db/migrations/sqlite/00001_create_core_tables.sql` (resources, resource_labels)
  - `internal/security-scanner/db/migrations/sqlite/00002_create_posture_tables.sql` (posture_scanned_resources, posture_findings)
  - `internal/security-scanner/db/migrations/postgres/00001_create_core_tables.sql`
  - `internal/security-scanner/db/migrations/postgres/00002_create_posture_tables.sql`
- Create SQLC query files:
  - `internal/security-scanner/db/queries/sqlite/resources.sql` (core resource operations)
  - `internal/security-scanner/db/queries/sqlite/posture.sql` (posture scan operations)
  - `internal/security-scanner/db/queries/postgres/resources.sql`
  - `internal/security-scanner/db/queries/postgres/posture.sql`
- Define queries:
  - UpsertResource (returns resource_id)
  - GetResource
  - UpsertResourceLabels
  - DeleteResourceLabels
  - GetPostureScannedResource
  - UpsertPostureScannedResource
  - InsertPostureFinding
  - DeletePostureFindingsByResourceId
  - ListPostureFindings (with pagination)
  - ListPostureFindingsByLabels
  - GetPostureFindingsSummary (aggregate by severity)
- Update `internal/security-scanner/db/backend/models.go` with new types
- Run `make sqlc-generate` to generate code
- Update `internal/security-scanner/db/backend/adapter.go` to expose new methods

## Phase 2: Parent Controller Resolution

### Objective
Implement logic to traverse owner references from Pod to parent controller (Deployment/StatefulSet/DaemonSet/Job/CronJob).

### Deliverables
- Controller resolution utility
- Support for Pod -> ReplicaSet -> Deployment chain
- Generic resolution for StatefulSet, DaemonSet, Job, CronJob

### Implementation Steps
- Create `internal/security-scanner/resolver/resolver.go`
- Implement `ResolveParentController(ctx, client, pod)` function:
  - Check Pod's ownerReferences
  - If owner is ReplicaSet, traverse to Deployment
  - If owner is StatefulSet/DaemonSet/Job/CronJob, return that
  - Return resolved controller + type
- Handle edge cases (no owner, orphaned pods)
- Add unit tests with fake client

## Phase 3: Checkov Integration

### Objective
Integrate Checkov CLI to scan Kubernetes manifests and parse results.

### Deliverables
- Checkov CLI wrapper
- Manifest generator from K8s objects
- Result parser for Checkov JSON output

### Implementation Steps
- Create `internal/security-scanner/checkov/checkov.go`
- Implement `RunCheckov(ctx, manifest []byte) ([]Finding, error)`:
  - Write manifest to temp file
  - Execute `checkov -f <file> --framework kubernetes --output json`
  - Parse JSON output into findings
  - Map Checkov severity to scanner severity levels
- Create `internal/security-scanner/checkov/types.go` for Finding struct
- Add Checkov to Dockerfile (install via pip)
- Add unit tests with sample manifests

## Phase 4: Pod Watcher and Reconciliation Logic

### Objective
Watch Pods, resolve parent controllers, check resourceVersion, run Checkov, store findings.

### Deliverables
- Pod controller with full reconciliation flow
- ResourceVersion-based deduplication
- Label extraction and storage

### Implementation Steps
- Update `internal/security-scanner/controller/pod_controller.go`:
  - On Pod create/update event
  - Resolve parent controller using resolver
  - Upsert resource to `resources` table (get resource_id)
  - Check if parent's resourceVersion already scanned (query `posture_scanned_resources`)
  - If already scanned, skip
  - If not scanned:
    - Generate YAML manifest from parent controller
    - Run Checkov
    - Delete old findings for resource_id
    - Insert new posture findings
    - Extract labels from parent controller
    - Delete old labels for resource_id
    - Insert new labels
    - Upsert `posture_scanned_resources` record
- Add logging for each step
- Handle errors gracefully (don't crash on Checkov failures)

## Phase 5: REST API for Findings Retrieval

### Objective
Expose REST API to query posture findings by resource, labels, severity, category.

### Deliverables
- GET /api/v1/posture/findings endpoint with filters
- GET /api/v1/posture/summary endpoint for aggregates
- GET /api/v1/health endpoint

### Implementation Steps
- Create `internal/security-scanner/api/types.go`:
  - PostureFindingsRequest (filters: namespace, name, labels, severity, category)
  - PostureFindingsResponse (list of findings + pagination)
  - PostureSummaryResponse (counts by severity and category)
- Update `internal/security-scanner/api/handlers.go`:
  - Implement ListPostureFindings handler
  - Implement GetPostureSummary handler
  - Implement Health handler
- Wire up DB Querier in handlers
- Add request validation
- Update `RegisterRoutes` in `cmd/security-scanner/main.go`

## Phase 6: Helm Chart and Deployment

### Objective
Deploy security-scanner as standalone component to any cluster.

### Deliverables
- Helm chart with RBAC, Deployment, Service
- Configuration for Checkov policies
- Documentation

### Implementation Steps
- Update `install/helm/openchoreo-security-scanner/values.yaml`:
  - Add Checkov policy configuration
  - Add resource limits
  - Add database backend options
- Update `install/helm/openchoreo-security-scanner/templates/deployment.yaml`:
  - Ensure Checkov is installed in image
  - Mount DB volume
  - Set environment variables
- Update `install/helm/openchoreo-security-scanner/templates/rbac.yaml`:
  - Add permissions to get/list/watch Pods, ReplicaSets, Deployments, StatefulSets, DaemonSets, Jobs, CronJobs
- Create `docs/security-scanner.md`:
  - Installation instructions
  - API usage examples
  - Label-based querying
  - Adding custom Checkov policies

## Phase 7: Testing and Validation

### Objective
Validate end-to-end flow with sample workloads.

### Deliverables
- Sample workload with known violations
- E2E test scripts
- Verification of findings

### Implementation Steps
- Create `samples/security-scanner/vulnerable-deployment.yaml`:
  - Deployment with privileged container, no resource limits, root user
- Deploy security-scanner to Kind cluster
- Apply sample deployment
- Query API to verify findings appear
- Query by labels
- Verify resourceVersion deduplication (update deployment without changes)
- Verify findings deleted when deployment deleted

## Success Criteria
- Scanner watches Pods and resolves parent controllers
- Checkov runs against parent controller manifests
- Findings stored with labels in database
- ResourceVersion tracking prevents redundant scans
- API returns findings filtered by labels
- Works on any Kubernetes cluster (no OpenChoreo dependencies)
- Documented and deployable via Helm

## Future Enhancements (Out of Scope)
- Support for ClusterRoleBinding, Ingress, NetworkPolicy
- Custom policy engine (beyond Checkov)
- Remediation suggestions
- Integration with OpenChoreo control plane for UI
