# Phase 1: Database Schema and Storage Layer - COMPLETED

## Migration Files Created
- `internal/security-scanner/db/migrations/sqlite/00001_create_core_tables.sql` - Core tables for resources and labels
- `internal/security-scanner/db/migrations/sqlite/00002_create_posture_tables.sql` - Posture scanning tables
- `internal/security-scanner/db/migrations/postgres/00001_create_core_tables.sql` - PostgreSQL core tables
- `internal/security-scanner/db/migrations/postgres/00002_create_posture_tables.sql` - PostgreSQL posture tables

## SQLC Query Files Created and Simplified
- `internal/security-scanner/db/queries/sqlite/resources.sql` - Simplified to 3 essential resource queries (UpsertResource, InsertResourceLabel, DeleteResourceLabels)
- `internal/security-scanner/db/queries/sqlite/posture.sql` - Simplified to 5 essential posture queries
- `internal/security-scanner/db/queries/postgres/resources.sql` - Simplified PostgreSQL resource queries
- `internal/security-scanner/db/queries/postgres/posture.sql` - Simplified PostgreSQL posture queries
- Removed 15+ unnecessary query methods (GetResource, ListResourcesByLabel, various summary/count queries)
- Kept only 8 essential methods needed for Phase 1-4 implementation

## Generated Code Structure
- `internal/security-scanner/db/backend/sqlite/` - Generated SQLite querier code from simplified queries
- `internal/security-scanner/db/backend/postgres/` - Generated PostgreSQL querier code from simplified queries
- All code regenerated with `make sqlc-generate` after query simplification

## Adapter Implementation - Simplified and Fixed
- `internal/security-scanner/db/backend/adapter.go` - Rewritten from 790 lines to ~300 lines
- Unified Querier interface with only 8 essential methods
- Fixed type conversions for sql.NullTime to time.Time (ScannedAt, CreatedAt fields)
- Fixed PostureFindingWithResource struct literal to properly use embedded PostureFinding
- SQLite uses int64 for IDs, PostgreSQL uses int32, adapters handle conversion
- Added time package import for zero time handling

## Models
- `internal/security-scanner/db/backend/models.go` - Shared types (Resource, ResourceLabel, PostureScannedResource, PostureFinding, PostureFindingWithResource)

## Database Connection
- `internal/security-scanner/db/connection.go` - Fixed to use NewSQLiteAdapter and NewPostgresAdapter

## Controller Updates
- `internal/security-scanner/controller/pod_controller.go` - Updated to use new schema with UpsertResource and label operations
- Still needs Phase 2 fix: Should resolve Pod → Deployment and check resourceVersion before scanning

## Test Files Updated
- `internal/security-scanner/db/example_test.go` - Updated to use new query interface (UpsertResource, InsertResourceLabel, GetPostureScannedResource)

## Code Generation
- Updated `Makefile` with sqlc-generate target
- Successfully generated all querier code from migrations and queries
- All code compiles successfully with no errors across entire security-scanner package

## Key Improvements from Feedback
- Simplified query interface from 20+ methods to 8 essential methods
- Removed unused queries that would be added back in Phase 5 if needed
- Fixed adapter type conversions for sql.NullTime fields
- Verified compilation succeeds across entire security-scanner package

## Phase 1 Complete
Phase 1 database layer is complete and compiles successfully.

# Phase 2: Parent Controller Resolution - COMPLETED

## Resolver Package Created
- `internal/security-scanner/resolver/resolver.go` - Implements ResolveParentController function with full traversal logic
- Defines ControllerType enum (Deployment, StatefulSet, DaemonSet, Job, CronJob, ReplicaSet, Pod)
- Defines ResolvedController struct (Type, Object, Namespace, Name, UID, ResourceVersion, Labels)

## Controller Resolution Logic
- Pod → ReplicaSet → Deployment traversal
- Direct Pod → StatefulSet resolution
- Direct Pod → DaemonSet resolution
- Pod → Job → CronJob traversal
- Direct Pod → Job resolution (standalone jobs)
- Standalone Pod resolution (orphaned/static pods like kube-controller-manager)
- Handles missing owners with proper error messages

## Unit Tests
- `internal/security-scanner/resolver/resolver_test.go` - Comprehensive test suite with 9 test cases
- Tests nil pod handling
- Tests orphaned pod handling (returns ControllerTypePod)
- Tests Pod → Deployment chain
- Tests Pod → StatefulSet direct
- Tests Pod → DaemonSet direct
- Tests Pod → CronJob chain
- Tests Pod → Job direct
- Tests standalone ReplicaSet
- Tests missing owner error handling
- All tests pass using fake Kubernetes client

## Pod Controller Integration
- `internal/security-scanner/controller/pod_controller.go` - Updated to use resolver
- Calls ResolveParentController to get parent controller instead of storing Pod directly
- Upserts resolved controller (not Pod) to resources table
- Checks if resourceVersion already scanned using GetPostureScannedResource
- Skips scanning if resource already scanned at same resourceVersion
- Stores resolved controller labels instead of Pod labels
- Logs detailed information about resolution and scanning decisions

## Bug Fix: Support Pod Type
- Fixed issue where standalone pods were marked as type "None"
- Changed ControllerTypeNone to ControllerTypePod
- Static pods like kube-controller-manager now correctly identified as type "Pod"
- Both orphaned pods and pods with unknown owner types return ControllerTypePod

## Bug Fix: ResourceVersion Deduplication Not Working
- Added missing call to UpsertPostureScannedResource after resource is ready to scan
- Now properly marks resources as scanned with their resourceVersion
- Second reconcile of same pod now triggers "Resource already scanned at this version, skipping" log
- Placeholder implementation (nil scanDurationMs) until Phase 3 adds actual Checkov scanning

## Verification
- All resolver tests pass (9 test cases)
- Entire security-scanner package compiles successfully
- Pod controller correctly integrates resolver and checks resourceVersion before scanning
- Standalone pods like kube-controller-manager-openchoreo-control-plane now show type=Pod in logs

## Phase 2 Complete - Ready for Phase 3
Phase 2 parent controller resolution is complete. Next step is Phase 3: Integrate Checkov CLI to scan Kubernetes manifests and parse results.

# Phase 3: Checkov Integration - COMPLETED

## Checkov Package Created
- `internal/security-scanner/checkov/types.go` - Defines Finding struct and Checkov JSON output types
- `internal/security-scanner/checkov/checkov.go` - Implements RunCheckov function with CLI execution and JSON parsing
- `internal/security-scanner/checkov/checkov_test.go` - Comprehensive test suite with unit and integration tests

## Core Types
- Finding struct with CheckID, CheckName, Severity, Category, Description, Remediation fields
- Severity enum: CRITICAL, HIGH, MEDIUM, LOW, INFO, UNKNOWN
- Checkov JSON parsing structs (checkovOutput, checkovResults, checkovCheck, checkovCheckResult)

## RunCheckov Implementation
- Creates temporary file for Kubernetes manifest
- Executes checkov CLI with flags: -f <file> --framework kubernetes --output json --quiet
- Parses JSON output into Finding structs
- Maps Checkov severity to scanner severity levels (defaults to MEDIUM when null)
- Extracts category from check_class field
- Handles checkov command failures gracefully
- Cleans up temporary files

## Severity Mapping
- Checkov severity to scanner Severity enum
- Empty/null severity defaults to MEDIUM (since Checkov often doesn't provide severity)
- Maps CRITICAL, HIGH, MEDIUM, LOW, INFO, INFORMATIONAL

## Category Extraction
- Extracts category from check_class field (e.g., "checkov.kubernetes.SecurityCheck" → "security")
- Uses bc_category if available, otherwise derives from check_class
- Defaults to "security" if no category found

## JSON Parsing
- Handles Checkov JSON structure with check_result as nested object (not string)
- Supports nullable fields (description, short_description, severity, bc_category)
- Uses check_name for CheckName, falls back to short_description if available

## Unit Tests
- TestMapSeverity - Tests severity mapping including empty string → MEDIUM
- TestCategorizeCheck - Tests category extraction from check_class

## Integration Tests
- TestRunCheckov - Tests with vulnerable deployment manifest, verifies findings are returned
- TestRunCheckov_ValidManifest - Tests with secure deployment, expects fewer findings
- Tests skip gracefully if checkov is not installed

## Dockerfile Update
- `cmd/security-scanner/Dockerfile` - Multi-stage build with Alpine downloader and distroless runtime
- Downloads Checkov binary from GitHub releases in Alpine stage
- Supports amd64 (x86_64) and arm64 architectures
- Checkov version pinned to 3.2.331
- Final image uses gcr.io/distroless/static:nonroot for minimal attack surface
- No package manager or shell in final image

## Test Results
- All 4 tests pass (TestMapSeverity, TestCategorizeCheck, TestRunCheckov, TestRunCheckov_ValidManifest)
- Verified Checkov successfully scans Kubernetes manifests and returns findings
- Verified JSON parsing handles actual Checkov output format

## Phase 3 Complete - Ready for Phase 4
Phase 3 Checkov integration is complete. Next step is Phase 4: Update Pod controller to call Checkov, generate YAML manifests, store findings in database.

# Phase 4: Pod Watcher and Reconciliation Logic - COMPLETED

## Pod Controller Updates
- `internal/security-scanner/controller/pod_controller.go` - Updated with complete reconciliation flow
- Added imports for appsv1, batchv1, checkov package, and sigs.k8s.io/yaml
- Added generateManifest method to convert K8s objects to YAML manifests
- Added comprehensive error handling and logging throughout the flow

## Complete Reconciliation Flow
1. **Pod Event Handling**: Controller watches Pod create/update events
2. **Parent Resolution**: Uses resolver to traverse Pod → ReplicaSet → Deployment chain
3. **Resource Storage**: Upserts resolved parent controller to resources table
4. **Deduplication Check**: Queries posture_scanned_resources to avoid redundant scans
5. **Label Management**: Deletes old labels and inserts new labels from parent controller
6. **Manifest Generation**: Converts parent controller object to YAML manifest
7. **Checkov Scanning**: Runs Checkov against generated manifest
8. **Findings Storage**: Deletes old findings and inserts new findings from scan
9. **Scan Tracking**: Upserts posture_scanned_resources with scan duration

## ResourceVersion-Based Deduplication
- Checks GetPostureScannedResource before scanning
- Skips scanning if resourceVersion matches previously scanned version
- Logs "Resource already scanned at this version, skipping" for deduplication
- Prevents redundant Checkov scans on unchanged resources

## Label Extraction and Storage
- Extracts labels from resolved parent controller (not Pod)
- Deletes old labels with DeleteResourceLabels before inserting new ones
- Inserts each label with InsertResourceLabel
- Logs label count in scan preparation logs

## YAML Manifest Generation
- generateManifest method supports all controller types:
  - Deployment, StatefulSet, DaemonSet (apps/v1)
  - Job, CronJob (batch/v1) 
  - Pod (core/v1)
- Preserves TypeMeta (APIVersion, Kind) and ObjectMeta
- Extracts only the Spec field for scanning (removes status, managedFields)
- Uses sigs.k8s.io/yaml for proper K8s YAML serialization

## Checkov Integration
- Calls checkov.RunCheckov with generated manifest
- Handles Checkov failures gracefully with error logging
- Processes findings array and converts to database format
- Maps nullable fields (category, description, remediation) to pointers

## Database Operations
- DeletePostureFindingsByResourceID before inserting new findings
- InsertPostureFinding for each Checkov finding with proper field mapping
- UpsertPostureScannedResource with actual scan duration in milliseconds
- All database operations include comprehensive error handling

## Event Filter Updates
- UpdateFunc now checks resourceVersion changes
- Only processes updates when ResourceVersion actually changed
- Prevents unnecessary reconciliations for status-only updates

## Logging and Error Handling
- Structured logging with slog at each major step
- Error logging includes resource identifiers and operation context
- Info logging for successful operations with timing and counts
- All errors return ctrl.Result{} to trigger requeue if needed

## Performance Tracking
- Measures scan duration with time.Since(scanStartTime).Milliseconds()
- Stores scan duration in posture_scanned_resources table
- Logs scan duration and findings count for monitoring

## Compilation and Testing
- All code compiles successfully with no errors
- Fixed linting issue with error comparison (errors.Is instead of !=)
- Ready for integration testing with actual Kubernetes clusters

## Phase 4 Complete - Ready for Phase 5
Phase 4 Pod watcher and reconciliation logic is complete. The security scanner now:
- Watches Pods and resolves parent controllers
- Generates YAML manifests from K8s objects
- Runs Checkov policy scans
- Stores findings with labels in database
- Tracks resourceVersion to prevent redundant scans
- Provides comprehensive logging and error handling

Next step is Phase 5: REST API for Findings Retrieval.

- Updated ClusterRole to include get/list permissions for apps/v1 resources (Deployments, ReplicaSets, StatefulSets, DaemonSets) and batch/v1 resources (Jobs, CronJobs)
