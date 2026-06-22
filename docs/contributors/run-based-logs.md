# Run-Based Logs for Scheduled Tasks — Contributor Guide

This guide documents how the scheduled-task observability feature (Runs tab in Backstage) is implemented on top of the upstream observability events pipeline introduced in v1.2.0-m.1.

## Quick architecture

```
OpenChoreo data-plane Kubernetes Events
        │
        ▼
observability-events-otel-collector (upstream, OTel-based)
  • k8s_events receiver  → watches Events cluster-wide
  • k8seventenrich       → attaches involvedObject labels (openchoreo.dev/* keys preserved)
  • opensearch exporter  → writes to k8s-events-YYYY-MM-DD
        │
        ▼
OpenSearch  (k8s-events-* index, managed by observability-logs-opensearch chart)
        │
        ▼
logs-adapter  (HTTP service, ghcr.io/openchoreo/observability-logs-opensearch-adapter)
  • POST /api/v1/events/query  → returns flat events list with metadata.objectKind/Name/uids
        │
        ▼
observer  (this repo, cmd/observer)
  • POST /api/v1/scheduled-tasks/runs/query
  • POST /api/v1/scheduled-tasks/runs/{jobName}/retries/query
  • POST /api/v1/logs/query  (with podName filter for per-retry logs)
        │
        ▼
Backstage  (Runs tab in plugins/openchoreo-observability, scheduled-task entity page)
```

The observer **never queries OpenSearch directly**; it consumes the `observability.EventsAdapter` Go interface (HTTP backed). The "runs" / "retries" shape is purely a server-side grouping of the same flat events the upstream `/api/v1/events/query` exposes.

## Key files

### Observer service

| File | Purpose |
|------|---------|
| `internal/observer/service/runs.go` | `RunsService` — events fetch + in-memory grouping by `objectKind` (Job / Pod), status derivation, pagination. |
| `internal/observer/service/runs_authz.go` | Authz wrapper, reuses `ActionViewEvents`. |
| `internal/observer/service/runs_test.go` | Unit tests for grouping + status derivation helpers. |
| `internal/observer/api/handlers/runs.go` | HTTP handlers + request validation. |
| `internal/observer/authz/helpers.go` | `RunsScopeAuthz` reuses the events authz path. |
| `internal/observer/types/runs.go` | Request/response types. `RetriesQueryRequest` carries optional `startTime`/`endTime`. |
| `cmd/observer/main.go` | Constructs `RunsService` and registers routes. |

### API contract

- `openapi/observer-runs-api.yaml` — runs/retries OpenAPI spec (importable into Postman).
- `openapi/observability-logs-adapter-api.yaml` — upstream adapter contract we consume.

### Backstage UI (lives in the [backstage-plugins repo](https://github.com/openchoreo/backstage-plugins), not here)

- `plugins/openchoreo-observability/src/components/Runs/` — runs table, run row, retry row, page wrapper.
- `plugins/openchoreo-observability/src/hooks/` — `useRuns`, `useRetries`, `usePodLogs`, `useUrlFiltersForRuns`.
- `plugins/openchoreo-observability/src/api/ObservabilityApi.ts` — `getRuns`, `getRetries`, `getPodLogs`.

## How status is derived

### Run status (per Job)

| Reason present in Job's events | Status |
|----|----|
| `Completed` | `succeeded` |
| `BackoffLimitExceeded` / `DeadlineExceeded` / `FailedCreate` | `failed` (+ `failureReason`) |
| Only `SuccessfulCreate` | `running` |
| Otherwise | `unknown` |

### Retry status (per Pod)

Best-effort from kubelet/scheduler events (`Started`, `OOMKilled`, …) then **overridden** by `applyRunStatusOverride` using the parent Job's status — because native K8s does **not** emit a pod-level event when a container exits.

The override:
- Job failed → all retries `Failed`.
- Job succeeded → last retry `Succeeded`, earlier ones `Failed`.
- Job running with N≥2 retries → first N−1 `Failed`, last keeps its derived status.
- Job unknown → keep derived.

## Local dev workflow

1. **Bring up the cluster.** Use the upstream quick-start container with observability enabled:
   ```bash
   docker run -it --rm -v /var/run/docker.sock:/var/run/docker.sock --network=host \
     ghcr.io/openchoreo/quick-start:v1.2.0-m.1
   # inside:
   ./install.sh --version v1.2.0-m.1 --with-observability
   ```
   This installs the OTel events collector, the logs-adapter, and the observer (release image).

2. **Rebuild & redeploy just the observer** after editing code:
   ```bash
   ./hack/redeploy-observer.sh
   ```
   This cross-compiles, builds the docker image (`ghcr.io/openchoreo/observer:latest-dev`), imports into k3d, and rolls out.

3. **Run Backstage locally** (so you can iterate on UI too) from the backstage-plugins repo:
   ```bash
   cd ../backstage-plugins
   nvm use v22.22.0
   yarn install
   yarn start
   ```
   The local Backstage runs on `http://localhost:3000` (frontend) + `http://localhost:7007` (backend).

   To let it call the cluster's observer, allowlist its origin once:
   ```bash
   ./hack/patch-observer-cors-for-local-dev.sh
   ```

4. **Seed a scheduled-task component** to generate events:
   ```bash
   kubectl apply -f samples/from-image/issue-reporter-schedule-task/github-issue-reporter.yaml
   ```
   It schedules every 5 minutes.

## Sanity-checking the pipeline

```bash
OS_PASS=$(kubectl get secret opensearch-admin-credentials -n openchoreo-observability-plane \
  -o jsonpath='{.data.password}' | base64 -d)

# Confirm events are landing
kubectl exec opensearch-master-0 -n openchoreo-observability-plane -c opensearch -- \
  curl -sk -u "admin:$OS_PASS" 'https://localhost:9200/k8s-events-*/_count'

# Sample one Job-kind document
kubectl exec opensearch-master-0 -n openchoreo-observability-plane -c opensearch -- \
  curl -sk -u "admin:$OS_PASS" -X POST 'https://localhost:9200/k8s-events-*/_search?size=1&pretty' \
  -H 'Content-Type: application/json' \
  -d '{"query":{"term":{"resource.k8s.object.kind":"Job"}}}'

# Hit the logs-adapter directly (bypasses observer auth/scope-resolution)
kubectl run curl-test --rm -i --restart=Never --image=curlimages/curl --timeout=20s \
  -n openchoreo-observability-plane -- \
  curl -s -X POST 'http://logs-adapter.openchoreo-observability-plane:9098/api/v1/events/query' \
  -H 'Content-Type: application/json' \
  -d '{"startTime":"...","endTime":"...","searchScope":{"namespace":"default","componentUid":"<uid>","environmentUid":"<uid>","projectUid":"<uid>"}}'
```

## Known limitations

1. **Pod-level success/failure events do not exist natively.** The `applyRunStatusOverride` heuristic keeps the UI correct for finished runs but can't always tell a running retry inside a running Job from a completed retry. Fix would be upstream collector support for synthetic pod-phase events.

2. **`restartPolicy: OnFailure` collapses retries into one pod.** The Job controller restarts the container in place rather than spawning a new pod, so the retries sub-table shows one row even after multiple container restarts. Workaround: default scheduled-task components to `restartPolicy: Never`.

3. **Adapter caps at 1000 events per call.** For very high-frequency CronJobs the retries fetch can truncate if the window is wide. Mitigation: the UI should pass `startTime`/`endTime` on the retries request to scope the window to the specific run's lifetime.

## Cluster pause/resume (saves resources during dev)

```bash
k3d cluster stop openchoreo-quick-start    # preserves volumes
k3d cluster start openchoreo-quick-start   # resume; OpenSearch + observer take ~30–60s
```
