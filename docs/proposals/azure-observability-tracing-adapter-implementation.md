# Azure Observability Tracing Adapter — Implementation Plan

Status: IMPLEMENTED and verified E2E on `oc-obs-dev-aks` (2026-06-12).
Module lives in `community-modules/observability-tracing-azure-appinsights`;
adapter image published as
`janakasandaruwan/observability-tracing-azure-appinsights-adapter:0.1.1`
(move to ghcr.io/openchoreo via module CI on upstreaming). All three
endpoints verified against live App Insights data, including cross-tenant
isolation and injection rejection. Two implementation findings beyond the
Phase 0 results: (1) `totimespan(strcat(ms, "ms"))` returns null for
fractional milliseconds — use `TimeGenerated + (todouble(DurationMs) * 1ms)`;
(2) the collector subchart names a cluster-scoped ClusterRole after its
fullname, so `fullnameOverride` must be unique per tracing module
(`otel-collector-azure`) to coexist with the OpenSearch module.
Author: janakas@wso2.com
Sibling docs:
- `azure-observability-logs-module-setup.md` (Azure infra + AKS bring-up)
- `azure-observability-logs-adapter-implementation.md` (logs adapter; this
  plan reuses its Azure client, auth, and Helm identity patterns)
- `azure-observability-metrics-adapter-implementation.md` (metrics adapter)

## Goal

Build a new OpenChoreo tracing adapter that satisfies the published
Observability Tracing Adapter API (the contract implemented by
`observability-tracing-opensearch`) and queries distributed-trace data stored
in workspace-based Application Insights tables (`AppRequests`,
`AppDependencies`) inside the same Log Analytics workspace the logs and
metrics adapters already query.

Ingestion uses the OpenTelemetry Collector (contrib distribution) with the
`azuremonitor` exporter; the existing `k8sattributes` enrichment and
`tail_sampling` pipeline from the OSS tracing modules is kept unchanged.

The module lives in `community-modules/observability-tracing-azure-appinsights`
as a sibling to the existing OpenSearch, OpenObserve, and AWS X-Ray tracing
adapters.

## Non-goals

- Custom Log Analytics table for spans (`OTelTraces_CL` via Logs Ingestion
  API). Rejected: no off-the-shelf collector exporter exists for
  traces → Logs Ingestion, so it requires a custom OTLP→rows bridge service
  we would own forever. Revisit only if a hard requirement (span-kind
  fidelity, ns precision, no-App-Insights-resource policy) emerges.
- Azure native OTLP ingestion. Still in preview (as of June 2026) and uses
  Entra ID + DCR auth instead of the connection string. Tracked as future
  simplification; the query layer is unaffected because it targets the same
  `App*` tables.
- AMA-based trace collection. AMA cannot create spans (traces originate in
  app SDKs) and its preview OTLP receiver does not enrich spans with pod
  labels, which breaks the `openchoreo.dev/*-uid` tenancy filters.
- Jaeger/Zipkin receivers. All tracing modules are OTLP-only; adding legacy
  receivers would be a deliberate cross-module change, not an Azure quirk.
- Alert endpoints. Tracing adapter API has none.

## Reference architecture

```
            ┌────────────────────────────────────────────────────────────┐
            │ AKS cluster (data plane / observability plane)             │
            │                                                            │
            │  app pods (OTel SDK)                                       │
            │     │ OTLP gRPC :4317 / HTTP :4318                         │
            │     ▼                                                      │
            │  OTel Collector (CONTRIB image)                            │
            │    processors:                                             │
            │      k8sattributes  ← copies ALL pod labels onto spans,    │
            │                       incl. openchoreo.dev/*-uid           │
            │      tail_sampling  ← rate limiting                        │
            │    exporters:                                              │
            │      azuremonitor   ← connection string from Secret        │
            │     │                                                      │
            │  ┌──────────────────────────────────────────────────────┐  │
            │  │ adapter Deployment (this module) :9100               │  │
            │  │   ServiceAccount: tracing-adapter                    │  │
            │  │     azure.workload.identity/client-id → UAMI         │  │
            │  │   Pod label: azure.workload.identity/use=true        │  │
            │  └──────────────┬───────────────────────────────────────┘  │
            │                 │ azlogs.Client.QueryWorkspace (KQL)        │
            │                 │ Bearer via azidentity.DefaultAzureCred    │
            └─────────────────┼──────────────────────────────────────────┘
                              ▼
            ┌────────────────────────────────────────────────────────────┐
            │ Application Insights resource (workspace-based, free)      │
            │   connection string = ingestion endpoint only              │
            │           │ rows stored in ▼                               │
            │ Log Analytics workspace (same one as logs/metrics)         │
            │   AppRequests      ← SERVER / CONSUMER spans               │
            │   AppDependencies  ← CLIENT / PRODUCER / INTERNAL spans    │
            │   Properties["openchoreo.dev/*"] = tenancy filters         │
            └────────────────────────────────────────────────────────────┘

            Observer ─POST /api/v1alpha1/traces/query─▶ adapter ─KQL─▶ workspace
```

Column mapping (verified against learn.microsoft.com and the
azuremonitorexporter source — see References):

| Adapter API concept | App Insights column |
|---|---|
| traceId | `OperationId` |
| spanId | `Id` |
| parentSpanId | `ParentId` (empty ⇒ root span) |
| span name | `Name` |
| durationNs | `DurationMs` × 1e6 (precision loss: ms → ns) |
| status error | `Success == false` (+ `ResultCode`) |
| service name | `AppRoleName` (from `service.name`) |
| attributes / resource attributes | `Properties` (strings/bools), `Measurements` (numbers) |
| span kind | implied by table; CLIENT/PRODUCER/INTERNAL collapse into AppDependencies — Phase 0 decides the mapping |

Caution: `AppTraces` is the LOG table (legacy .NET "trace" naming). The
tracing adapter never reads it.

## Phase 0 — de-risking spike (blocking)

The design rests on one assumption: spans exported via `azuremonitor` carry
the `openchoreo.dev/*-uid` resource attributes in the `Properties` column.
The exporter source (`applyResourcesToDataProperties` in
`contracts_utils.go`) says yes; prove it on a real cluster before writing
module code.

1. Create a workspace-based Application Insights resource against the
   existing workspace; capture the connection string into a Secret.
2. Hand-deploy an OTel Collector contrib with the OpenSearch module's
   receiver/processor config plus the `azuremonitor` exporter, and one
   OTLP-instrumented sample app on an OpenChoreo-labeled pod.
3. Verify in the workspace:
   - spans land in `AppRequests` / `AppDependencies`;
   - `Properties["openchoreo.dev/component-uid"]` etc. present and
     filterable in `where` clauses;
   - where (if anywhere) span kind survives → decide the SpanKind mapping;
   - App Insights ingestion sampling/throttling does not interact badly
     with `tail_sampling` (expectation: adaptive sampling is SDK-side only);
   - measure ingestion latency (expected: minutes) for the README.
4. Prototype the trace-list KQL and confirm root span, duration, span count,
   and error count come back correct for a known trace.

Exit criteria: a saved KQL query returning correct trace summaries, plus
captured result rows checked in later as mapping-test fixtures.

### Phase 0 results (run 2026-06-11 on oc-obs-dev-aks)

Setup used: App Insights `oc-obs-dev-appinsights` (workspace-based →
`oc-obs-dev-law-v2`), collector contrib v0.153.0 via the upstream Helm chart
(release `otel-spike`, namespace `otel-spike`), telemetrygen sending
10 traces x 4 spans from a pod labeled with the four `openchoreo.dev/*`
test values.

1. PASSED (load-bearing): all four `openchoreo.dev/*` pod labels arrive in
   `Properties` on every row and are filterable with
   `tostring(Properties["openchoreo.dev/..."]) == ...`.
2. PASSED: trace-list `summarize by OperationId` returns correct SpanCount,
   root span, and error count. Cross-tenant negative test returns 0 rows.
3. FINDING — root detection: the azuremonitor exporter sets
   `ParentId = OperationId` for root spans, NOT empty. Root predicate must
   be `ParentId == OperationId or isempty(ParentId)`. (KQL snippets in this
   doc still show the isempty-only form; fix when implementing.)
4. FINDING — kind mapping confirmed: SERVER spans → `AppRequests`, CLIENT
   spans → `AppDependencies` with `DependencyType == "Other"` for non-HTTP
   spans. Kind is not preserved beyond the table split (open decision #1
   stands: AppDependencies rows can't distinguish CLIENT/PRODUCER/INTERNAL).
5. FINDING — ingestion latency: rows queryable in under ~2 minutes,
   better than the expected 2-5 min.
6. FINDING — collector rename: `k8sattributes` is a deprecated alias as of
   contrib v0.153.0; the module's collector config should use
   `k8s_attributes`.
7. `DurationMs` confirmed as float milliseconds (e.g. 0.123).

## Repository layout

```
observability-tracing-azure-appinsights/
├── main.go                          # boot: config, credential, ping, serve
├── Dockerfile                       # multi-stage, distroless/alpine, non-root
├── Makefile                         # gen, build, test, lint
├── module.yaml                      # CI image manifest (adapter image only —
│                                    #   no setup image; schema is Azure-managed)
├── README.md
├── go.mod                           # azcore, azidentity, azlogs, oapi runtime
├── helm/
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
│       ├── adapter/                 # deployment, service(:9100), configmap,
│       │                            #   serviceaccount (WI annotations),
│       │                            #   networkpolicy, validate.yaml
│       └── opentelemetry-collector/ # configMap (contrib), deployment,
│                                    #   httproute (multiClusterReceiver)
└── internal/
    ├── config.go                    # env vars, fail-fast validation
    ├── server.go                    # OpenAPI router (stdlib net/http)
    ├── handlers.go                  # 3 endpoints + /healthz
    ├── api/                         # cfg-server/models/client.yaml + gen/
    │                                #   (copied from tracing-opensearch,
    │                                #    regenerated with oapi-codegen)
    └── appinsights/
        ├── client.go                # azlogs wrapper: timeout, error mapping
        ├── kql.go                   # 3 query builders (parameterized)
        ├── mapping.go               # rows → TraceInfo / SpanInfo
        ├── labels.go                # openchoreo.dev/* Properties keys,
        │                            #   scope → where-clause builder
        └── *_test.go                # golden KQL + fixture mapping tests
```

Donors: API/server/handler skeleton and collector Helm templates from
`observability-tracing-opensearch`; Azure client wrapper, auth, ServiceAccount
identity pattern, validate.yaml, and config conventions from
`observability-logs-azure-loganalytics`.

## Module naming and CI

`module.yaml`:

```yaml
images:
  - name: observability-tracing-azure-appinsights-adapter
    context: .
    dockerfile: Dockerfile
```

No `setup` image. The OpenSearch module needs an init job for index templates
and ISM policies; here the table schema and retention are managed by Azure.

## OpenAPI contract

Identical to `observability-tracing-opensearch` — the Observer's generated
client is the consumer, so the spec is copied verbatim and only the server
implementation differs:

- `POST /api/v1alpha1/traces/query` — list traces (searchScope.namespace
  required; project/component/environment optional UIDs)
- `POST /api/v1alpha1/traces/{traceId}/spans/query` — spans of one trace
- `GET  /api/v1alpha1/traces/{traceId}/spans/{spanId}` — span detail
- `GET  /healthz`

## Configuration (environment variables)

| Var | Required | Default | Purpose |
|---|---|---|---|
| `LOG_ANALYTICS_WORKSPACE_ID` | yes | — | workspace customerId (GUID) for azlogs queries |
| `SERVER_PORT` | no | `9100` | adapter listen port (Observer default) |
| `QUERY_TIMEOUT_SECONDS` | no | `30` | per-query KQL timeout |
| `MAX_QUERY_LIMIT` | no | `10000` | hard cap on `limit` (matches OpenSearch module) |
| `LOG_LEVEL` | no | `INFO` | |

The App Insights connection string is collector-side only (exporter Secret);
the adapter never sees it. Credentials come from
`azidentity.NewDefaultAzureCredential` — Workload Identity in-cluster, az CLI
locally. The UAMI needs `Log Analytics Reader` on the workspace (already
granted for the logs adapter; same workspace covers `App*` tables).

## Implementation phases

### Phase 1 — adapter (query path)

- Skeleton per the layout above; regenerate `internal/api/gen`.
- `internal/appinsights` query layer (detail below).
- Boot sequence mirrors the logs adapter: load config →
  `DefaultAzureCredential` → one near-zero-cost reachability ping
  (`union AppRequests, AppDependencies | take 1`) → serve.
- Unit tests green; `golangci-lint` clean.

### Phase 2 — Helm chart and collector wiring

- Collector templates from the OpenSearch module with these deltas:
  - contrib image (configurable; pinned tag in values);
  - `azuremonitor` exporter with `connection_string` from
    `${env:APPINSIGHTS_CONNECTION_STRING}` (Secret-mounted);
  - receivers (`otlp` 4317/4318), `k8sattributes` (all-labels regex
    extract), `tail_sampling`, and all three `installationMode`s kept
    verbatim. In `multiClusterReceiver`, the central collector runs the
    azuremonitor exporter; HTTPRoute unchanged.
- Adapter templates from the logs module: deployment, service, configmap,
  ServiceAccount with `azure.workload.identity/client-id` annotation +
  `azure.workload.identity/use: "true"` pod label, networkpolicy,
  validate.yaml (fail install on missing workspaceId).
- `values.yaml` keys: `azure.{subscriptionId,resourceGroup,region}`,
  `logAnalytics.workspaceId`, `appInsights.connectionStringSecretRef`,
  `adapter.*` (image, serviceAccount.annotations, queryTimeoutSeconds,
  logLevel), `opentelemetryCollectorCustomizations.*` (image, tailSampling,
  queue), `global.installationMode`.
- Azure prerequisites added to the install skill / Terraform: App Insights
  resource (workspace-based), federated identity credential for the
  `tracing-adapter` ServiceAccount on the existing UAMI.

### Phase 3 — Observer integration and end-to-end validation

- Wire Observer: `TRACING_ADAPTER_ENABLED=true`,
  `TRACING_ADAPTER_URL=http://tracing-adapter.<ns>.svc:9100`.
- E2E: instrumented sample component → traces visible via Observer
  `POST /api/v1alpha1/traces/query` with name-based scope (exercises
  name→UID resolution + authz + adapter + KQL).
- Negative tests: cross-namespace scope returns empty; unknown traceId;
  empty window; limit overflow.

## Query layer detail

### KQL — traces list

Equivalent of the OpenSearch terms aggregation. All user inputs are bound,
never interpolated:

```kusto
union AppRequests, AppDependencies
| where TimeGenerated between (datetime({start}) .. datetime({end}))
| where tostring(Properties["openchoreo.dev/namespace"]) == {namespace}
| where tostring(Properties["openchoreo.dev/component-uid"]) == {componentUid}   // optional
| where tostring(Properties["openchoreo.dev/environment-uid"]) == {environmentUid} // optional
| where tostring(Properties["openchoreo.dev/project-uid"]) == {projectUid}       // optional
| summarize
    SpanCount  = count(),
    StartTime  = min(TimeGenerated),
    EndTime    = max(TimeGenerated + totimespan(strcat(tostring(DurationMs), "ms"))),
    ErrorCount = countif(Success == false),
    RootName   = take_anyif(Name, isempty(ParentId)),
    RootId     = take_anyif(Id, isempty(ParentId))
  by OperationId
| order by StartTime {sortOrder}
| take {limit}
```

`traceName`/`rootSpanName` come from the root-span fields; `hasErrors` is
`ErrorCount > 0`; `durationNs` is `(EndTime - StartTime)` in ns. Traces whose
root span fell outside the window (or was sampled away) have empty Root*
fields — same degraded behavior as the OpenSearch module; fall back to the
earliest span's name.

### KQL — spans of a trace

```kusto
union AppRequests, AppDependencies
| where TimeGenerated between (datetime({start}) .. datetime({end}))
| where OperationId == {traceId}
| <same scope filters>
| project TimeGenerated, Id, ParentId, Name, DurationMs, Success, ResultCode,
          AppRoleName, Type, Target, Properties, Measurements, itemType = $table
| order by TimeGenerated asc
```

`Properties`/`Measurements` are projected only when `includeAttributes=true`.
`$table`/itemType drives the kind mapping (AppRequests → SERVER;
AppDependencies → per Phase 0 decision).

### KQL — span detail

Same union filtered by `OperationId == {traceId} and Id == {spanId}`, full
attribute projection, `take 1`.

### Error mapping

azlogs errors map to the contract's error responses: 401/403 from Entra →
502 with auth detail (misconfigured identity is an operator error, not a
client error); workspace throttling (429) → 429 passthrough; KQL timeout →
504; partial-result flag from azlogs → log warning + serve partial (matches
logs adapter behavior).

## Test strategy

- `kql.go`: golden-string tests per builder × scope permutations (namespace
  only; +component; +environment; +project; all), sort orders, limits.
  Injection attempts (quotes, pipes in traceId) must fail validation, not
  reach KQL.
- `mapping.go`: fixtures captured from the Phase 0 spike rows; assert
  ms→ns conversion, root detection, error flag, kind mapping.
- `handlers.go`: httptest against a mocked client (pattern from the X-Ray
  module's `client_test.go`).
- E2E on the Azure cluster as Phase 3 above.

## Deployment story

### Local dev

```
az login
export LOG_ANALYTICS_WORKSPACE_ID=<customerId>
go run .   # DefaultAzureCredential falls through to AzureCLICredential
# in another terminal:
curl -s localhost:9100/healthz
curl -s -X POST localhost:9100/api/v1alpha1/traces/query -d @sample-query.json
```

### In-cluster

```
helm install tracing-azure ./helm \
  --set logAnalytics.workspaceId=<customerId> \
  --set appInsights.connectionStringSecretRef=appinsights-conn \
  --set adapter.serviceAccount.annotations."azure\.workload\.identity/client-id"=<uami-client-id>
# wire Observer:
#   TRACING_ADAPTER_ENABLED=true
#   TRACING_ADAPTER_URL=http://tracing-adapter.<ns>.svc.cluster.local:9100
```

## Open decisions, deferred until the spike

1. SpanKind mapping for AppDependencies rows (table-only signal vs a
   Properties key, if the exporter preserves one).
2. Whether the spans-of-trace query needs a wider time window than the
   request's (a trace's spans can straddle the window edges; OpenSearch
   module behavior is the reference).
3. Pagination beyond `take {limit}` for very large traces (BatchGetTraces in
   the X-Ray module paginates; KQL `take` truncates silently — may need a
   `serializedLength`-style guard).
4. Whether `multiClusterExporter` data planes are AKS-only or any-K8s (the
   collector side has no Azure dependency; only docs are affected).

## Cost

- Application Insights resource: free; billing is per-GB into the existing
  workspace (Analytics plan), same meter as logs.
- Span volume is bounded by `tail_sampling` (default 10 spans/sec per
  collector) — at that ceiling, worst case is in the low single-digit
  GB/month range per cluster.
- Adapter pod: same footprint as siblings (64–128 Mi, 20–100m CPU).

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Resource attrs not filterable in `Properties` | Phase 0 spike is blocking; exporter source already confirms the mechanism |
| Span kind lost in the two-table split | spike decides mapping; document the fidelity loss vs OpenSearch |
| `summarize by OperationId` slow over long windows | enforce limit/time-range caps (max 10000, default 20) like the OpenSearch module |
| App Insights ingestion latency (minutes) surprises users | measure in spike; document expected freshness in README and Observer-facing docs |
| Contrib collector image drift / exporter breaking change | pin image tag in values; bump deliberately |
| Azure native OTLP ingestion GAs and obsoletes the exporter hop | query layer targets the same `App*` tables either way; keep ingestion and query concerns separated in the chart |

## Acceptance criteria

- [ ] Phase 0 spike passed; KQL prototype + fixture rows captured.
- [ ] All three endpoints return contract-correct responses against live
      App Insights data.
- [ ] Tenancy: a query scoped to namespace A never returns namespace B spans
      (verified with two labeled workloads).
- [ ] Observer E2E with name-based scope works (`TRACING_ADAPTER_URL` wired).
- [ ] Helm install from a clean cluster succeeds with only documented values;
      validate.yaml rejects missing workspaceId.
- [ ] Unit tests + lint green; README documents env vars, prerequisites,
      ingestion latency, and the AppTraces naming trap.

## Effort estimate

- Phase 0 spike: ~1 day (dominated by ingestion-latency round trips).
- Phase 1 adapter: 2–3 days with both donor modules to crib from.
- Phase 2 Helm/collector: 1–2 days.
- Phase 3 integration + validation: 2–3 days.

## References

- Tracing adapter contract + reference implementation:
  `community-modules/observability-tracing-opensearch`
- Azure client/auth/Helm donor:
  `community-modules/observability-logs-azure-loganalytics`
- App Insights workspace tables (AppRequests/AppDependencies mapping):
  https://learn.microsoft.com/en-us/azure/azure-monitor/app/convert-classic-resource
- Telemetry correlation (OperationId/Id/ParentId):
  https://learn.microsoft.com/en-us/azure/azure-monitor/app/correlation
- azuremonitorexporter (contrib; resource attrs → properties):
  https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/exporter/azuremonitorexporter/README.md
- AKS Workload Identity:
  https://learn.microsoft.com/en-us/azure/aks/workload-identity-overview
- azlogs query SDK:
  https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azlogs
- Azure Monitor OTLP ingestion (preview; future simplification):
  https://learn.microsoft.com/en-us/azure/azure-monitor/containers/opentelemetry-protocol-ingestion
