# Distributed Tracing in OpenChoreo — From Zero

This document explains how distributed tracing is implemented across the OpenChoreo
codebases: the tracing community modules (`community-modules/observability-tracing-*`)
and the Observer in the main repo (`cmd/observer/`). It assumes no prior knowledge
of tracing.

---

## Part 0: What is tracing, and why does it exist?

Imagine a user clicks "Checkout" in an online store. Behind the scenes, that one
click might touch five different services:

```
Browser → API Gateway → Order Service → Payment Service → Database
                              ↓
                        Inventory Service
```

Now the user reports "checkout is slow." Where is it slow? Logs alone can't easily
tell you, because each service writes its own logs independently and there's no
thread connecting them.

**Distributed tracing** solves this by giving each request a unique ID and recording
how long it spent in each service. The vocabulary:

- **Trace** — the complete journey of one request through the whole system.
  Identified by a `traceId` (a random hex string like `4bf92f3577b34da6a3ce929d0e0e4736`).
- **Span** — one unit of work inside that journey (e.g., "Order Service handled
  POST /orders", or "DB query ran"). Each span has a `spanId`, a start time, an end
  time, and a status (ok/error).
- **Parent-child relationship** — spans nest. The Order Service span is the *parent*
  of the Payment Service span it triggered. Each span records its `parentSpanId`.
  The span with **no parent** is the **root span** — the entry point of the request.

A trace is therefore a tree of spans:

```
TRACE 4bf92f35...  (total: 850ms)
│
└── SPAN A: "POST /checkout"  [API Gateway]          0ms ──────────────── 850ms   ← root span (parentSpanId = "")
    │
    └── SPAN B: "create order"  [Order Service]        20ms ───────────── 830ms   (parent = A)
        │
        ├── SPAN C: "charge card"  [Payment Service]     50ms ──── 700ms          (parent = B)
        │   └── SPAN D: "SQL INSERT"  [Database]           60ms ─ 120ms           (parent = C)
        │
        └── SPAN E: "reserve stock"  [Inventory]          55ms ── 200ms           (parent = B)
```

Looking at this you instantly see: the 850ms is mostly inside "charge card."
That's the value of tracing.

**How does the traceId travel between services?** When Service A calls Service B
over HTTP, it adds a header (`traceparent: 00-<traceId>-<spanId>-01`). Service B
reads it and tags its own spans with the same traceId. This is called *context
propagation* and it's handled by the OpenTelemetry SDK inside each app — not by
the platform.

**OpenTelemetry (OTel)** is the industry-standard toolkit for all of this: SDKs
that apps use to create spans, a wire protocol (**OTLP**) for shipping them, and
a **Collector** for receiving/processing/forwarding them.

---

## Part 1: The big picture in OpenChoreo

OpenChoreo's tracing system is split across two codebases with a clean dividing line:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                                                                             │
│   DATA PLANE (where user workloads run)                                    │
│   ┌──────────────┐                                                         │
│   │  Your app    │  Layer 1: instrumentation (OTel SDK in the app)         │
│   │  (OTel SDK)  │                                                         │
│   └──────┬───────┘                                                         │
│          │ OTLP (spans)                                                    │
│          ▼                                                                 │
│   ┌──────────────────────┐                                                 │
│   │ OpenTelemetry        │  Layer 2: collection                            │
│   │ Collector            │  (receive, enrich, sample, export)              │
│   └──────┬───────────────┘                                                 │
│          │                                                                 │
├──────────┼──────────────────────────────────────────────────────────────────┤
│          ▼                                                                 │
│   OBSERVABILITY PLANE                                                      │
│   ┌──────────────────────┐                                                 │
│   │ Storage backend      │  Layer 3: storage                               │
│   │ (OpenSearch / X-Ray  │  (indices, retention)                           │
│   │  / OpenObserve)      │                                                 │
│   └──────▲───────────────┘                                                 │
│          │ backend-specific queries                                        │
│   ┌──────┴───────────────┐                                                 │
│   │ Tracing Adapter      │  Layer 4: query adapter      ← community-modules│
│   │ :9100                │  (uniform REST API over any backend)            │
│   └──────▲───────────────┘                                                 │
│          │ HTTP (uniform API)                                              │
│   ┌──────┴───────────────┐                                                 │
│   │ Observer :9097       │  Layer 5: platform API       ← choreov3 repo    │
│   │ (JWT, authz,         │  (security + abstraction)                       │
│   │  name→UID)           │                                                 │
│   └──────▲───────────────┘                                                 │
│          │                                                                 │
├──────────┼──────────────────────────────────────────────────────────────────┤
│          │ HTTPS + JWT                                                     │
│   ┌──────┴───────────────┐                                                 │
│   │ UI / Backstage /     │  Layer 6: consumers                             │
│   │ control plane        │                                                 │
│   └──────────────────────┘                                                 │
└─────────────────────────────────────────────────────────────────────────────┘
```

Layers 2–4 live in `community-modules` (one module per storage backend:
`observability-tracing-opensearch`, `observability-tracing-openobserve`,
`observability-tracing-aws-xray`). Layer 5 lives in the main repo (`cmd/observer/`).
The contract between layers 4 and 5 is a shared OpenAPI spec, which is what makes
backends swappable.

---

## Part 2: Layer 1 — Instrumentation (inside the app)

This layer is the developer's responsibility, not the platform's. The app embeds
an OpenTelemetry SDK (available for Go, Java, Node, Python, etc.), which:

1. Creates a span when a request arrives, ends it when the response is sent.
2. Propagates the `traceparent` header on outgoing calls.
3. Batches finished spans and pushes them over **OTLP** to a collector endpoint,
   typically configured by one env var:

```
OTEL_EXPORTER_OTLP_ENDPOINT=http://opentelemetry-collector:4318
```

The platform's only job here is to make sure a collector endpoint exists for the
app to send to. Everything from here down is platform-owned.

---

## Part 3: Layer 2 — The OpenTelemetry Collector (collection & enrichment)

**Where:** Helm templates in each community module, e.g.
`observability-tracing-opensearch/helm/templates/opentelemetry-collector/configMap.yaml`.
Uses the upstream `opentelemetry-collector` chart (v0.146.1).

The Collector is a pipeline with three stages — think of it as a mail sorting facility:

```
                 ┌────────────────── OTel Collector ──────────────────┐
                 │                                                    │
  apps ───OTLP──▶│  RECEIVERS          PROCESSORS         EXPORTERS   │───▶ storage
                 │  ┌───────────┐     ┌──────────────┐   ┌─────────┐  │
                 │  │ otlp/grpc │     │ k8sattributes│   │opensearch│ │
                 │  │  :4317    │ ──▶ │              │──▶│   or     │ │
                 │  │ otlp/http │     │ tail_sampling│   │ awsxray  │ │
                 │  │  :4318    │     └──────────────┘   │   or     │ │
                 │  └───────────┘                        │openobserve│ │
                 │                                       └─────────┘  │
                 └────────────────────────────────────────────────────┘
```

**Receivers** open the OTLP ports (gRPC 4317, HTTP 4318) that apps send spans to.

**Processors** modify spans in flight:

- **`k8sattributes`** — when a span arrives, the collector asks the Kubernetes API
  "which pod sent this?" (matching the span's source IP to a pod IP) and copies that
  pod's labels onto the span. OpenChoreo puts identity labels on every workload pod
  it deploys, so every span gets stamped with:

  ```
  openchoreo.dev/namespace          ← which tenant namespace
  openchoreo.dev/project-uid        ← which project
  openchoreo.dev/component-uid      ← which component
  openchoreo.dev/environment-uid    ← which environment (dev/staging/prod)
  ```

  This is how a span from "some pod" becomes a span from "component X of project Y
  in prod." The app developer never has to set these. Every query later in the
  stack filters on these fields. (See Part 10 for exactly where this is configured
  and where the pod labels come from.)

- **`tail_sampling`** — rate limiting (default 10 spans/sec, configurable) so a
  chatty app can't flood storage.

**Exporters** write the processed spans to the backend. This is the only part that
differs between the three modules (OpenSearch exporter / X-Ray exporter /
OpenObserve HTTP exporter).

### Multi-cluster topologies

OpenChoreo separates the data plane (where apps run) from the observability plane
(where storage runs), possibly in different clusters. The chart supports three
modes via `global.installationMode`:

```
singleCluster:                    multiCluster:

┌─ one cluster ─────────┐         ┌─ data plane cluster ──┐   ┌─ observability cluster ─┐
│ apps → collector →    │         │ apps → collector      │   │  collector (receiver)   │
│        storage        │         │        (exporter) ────┼──▶│  exposed via HTTPRoute  │
└───────────────────────┘         │        forwards OTLP  │   │     │                   │
                                  └───────────────────────┘   │     ▼                   │
                                                              │  storage                │
                                                              └─────────────────────────┘
```

In multi-cluster mode, each data plane runs a lightweight collector
(`multiClusterExporter`) that just forwards OTLP to a central collector
(`multiClusterReceiver`) exposed through a Gateway API `HTTPRoute` in the
observability plane.

---

## Part 4: Layer 3 — Storage (OpenSearch as the reference)

**Where:** `observability-tracing-opensearch/init/setup-opensearch.sh` (index
template + retention policy), provisioned by a Helm setup job.

OpenSearch is a search/analytics database (an Elasticsearch fork). Spans land in
**daily indices** — one per calendar day:

```
otel-traces-2026-06-08
otel-traces-2026-06-09
otel-traces-2026-06-10   ← today's spans go here
```

Daily indices make time-range queries fast (only relevant days are touched) and
make retention trivial: an **ISM policy** (Index State Management) deletes whole
indices after 30 days (configurable via `OTEL_TRACES_MIN_INDEX_AGE`). No
per-record cleanup needed.

Each document in an index is **one span**, with a fixed schema (`dynamic: false`
means unknown fields aren't indexed — protects against schema explosion):

```json
{
  "traceId":       "4bf92f3577b34da6...",
  "spanId":        "00f067aa0ba902b7",
  "parentSpanId":  "",
  "name":          "POST /checkout",
  "kind":          "SERVER",
  "startTime":     "2026-06-10T10:00:00.000000001Z",
  "endTime":       "2026-06-10T10:00:00.850000000Z",
  "status":        { "code": "ok" },
  "resource": {
    "k8s.pod.name":                    "checkout-7d9f...",
    "service.name":                    "checkout",
    "openchoreo.dev/namespace":        "acme-corp-ns",
    "openchoreo.dev/project-uid":      "f47ac10b-...",
    "openchoreo.dev/component-uid":    "d4e5f6a7-...",
    "openchoreo.dev/environment-uid":  "7a8b9c0d-..."
  },
  "attributes": { "http.method": "POST", "http.status_code": 200 }
}
```

Field notes:

- `traceId`, `spanId`, `parentSpanId`, `kind`, `status.code`, and the
  `resource.openchoreo.dev/*` fields are `keyword` (exact-match) fields.
- `startTime`/`endTime` are `date_nanos` (nanosecond precision).
- `parentSpanId == ""` marks the root span.
- Span `kind` is one of `SERVER`, `CLIENT`, `INTERNAL`, `PRODUCER`, `CONSUMER`.

Important: storage knows nothing about "traces" as objects. **It only stores
spans.** A "trace" is reconstructed at query time by grouping spans that share a
`traceId` — that's the next layer's job.

---

## Part 5: Layer 4 — The query adapter (community module)

**Where:** `observability-tracing-opensearch/internal/` — `handlers.go:45-174`,
`opensearch/queries.go`, `opensearch/process.go:17-104`. A small Go service,
port 9100. The OpenObserve and X-Ray modules implement the identical API against
their own backends.

This is the **translation layer**: it speaks a simple, uniform REST API upward,
and the backend's native query language downward.

```
            uniform REST API (same for all 3 backends)
                          │
            ┌─────────────▼──────────────┐
            │   tracing-adapter :9100    │
            │                            │
            │  POST /api/v1alpha1/traces/query                 → list traces
            │  POST /api/v1alpha1/traces/{traceId}/spans/query → spans of one trace
            │  GET  /api/v1alpha1/traces/{traceId}/spans/{spanId} → one span's detail
            │  GET  /healthz                                   │
            └─────────────┬──────────────┘
                          │ backend-native queries
                          ▼
              OpenSearch DSL / X-Ray SDK / OpenObserve API
```

The API is defined as an OpenAPI spec, and both server stubs and the client are
**code-generated** (oapi-codegen) — so the adapter and its callers can never drift
apart on request/response shapes.

### Endpoint 1: "list traces"

A request looks like:

```json
POST /api/v1alpha1/traces/query
{
  "startTime": "2026-06-10T00:00:00Z",
  "endTime":   "2026-06-10T23:59:59Z",
  "limit": 20,
  "sortOrder": "desc",
  "searchScope": {
    "namespace":   "acme-corp-ns",
    "project":     "f47ac10b-...",
    "component":   "d4e5f6a7-...",
    "environment": "7a8b9c0d-..."
  }
}
```

`namespace` is required (tenant isolation); the UID filters are optional.

Remember: storage only has individual spans. To return a list of *traces*, the
adapter uses an OpenSearch **terms aggregation** (`queries.go:111-260`) — "group
all matching spans by traceId, and for each group compute summaries":

```
   spans matching filters (time range + searchScope terms)
        │
        ▼  GROUP BY traceId
   ┌─────────────────────────────────────────────────┐
   │ for each traceId bucket, compute:               │
   │   • earliest_span   (min startTime → trace start)│
   │   • latest_span     (max endTime  → trace end)   │
   │   • root_span       (the span where              │
   │                      parentSpanId == "")         │
   │   • error_span_count (spans with status=error)   │
   │   • span count                                   │
   └─────────────────────────────────────────────────┘
        │
        ▼
   one summary row per trace:
   { traceId, traceName, spanCount, rootSpanName,
     startTime, endTime, durationNs, hasErrors }
```

So one OpenSearch query reconstructs trace summaries from raw spans, entirely
server-side. The `hasErrors` flag is what lets a UI paint failed traces red.

### Endpoint 2: "spans of a trace"

Once a user clicks a trace, the UI calls `POST /traces/{traceId}/spans/query`.
This is a plain filter query (`queries.go:21-107`): `traceId == X` plus the same
scope filters, returning every span document. The caller rebuilds the tree using
each span's `parentSpanId` to draw the waterfall view from Part 0.

### Endpoint 3: "span detail"

`GET /traces/{traceId}/spans/{spanId}` fetches a single span *with* full
`attributes` and `resourceAttributes`. (The list endpoints omit attributes by
default — `includeAttributes: false` — to keep payloads small.)

### Tenant safety at this layer

Every query the adapter builds includes a `term` filter on
`resource.openchoreo.dev/namespace` — `namespace` is a required field in
`searchScope`. The adapter physically cannot produce a query that spans tenants.
But note what it does **not** do: it doesn't check *who is asking*. It trusts its
caller. That's deliberate, and it's why the next layer exists.

---

## Part 6: Layer 5 — The Observer (main repo)

**Where:** `cmd/observer/main.go` (entry, trace routes at lines 281-283),
`internal/observer/api/handlers/traces.go`, `internal/observer/service/traces.go`,
`internal/observer/service/tracing_adapter.go`,
`internal/observer/adaptor/traces_default.go`, interface in
`pkg/observability/traces.go:12-19`.

The Observer is the **front door of the observability plane** — one API server
(port 9097) for logs, metrics, traces, and alerts. For tracing it exposes the
*same three endpoints* as the adapter, but adds everything a multi-tenant platform
needs that the adapter deliberately skipped:

```
        UI / control plane
              │  POST /api/v1alpha1/traces/query
              │  Authorization: Bearer <JWT>
              │  searchScope: { project: "my-project", component: "checkout", ... }  ← NAMES
              ▼
   ┌────────────────────────── Observer :9097 ──────────────────────────┐
   │                                                                    │
   │  ① JWT middleware      — is this token valid? who is the caller?   │
   │                                                                    │
   │  ② Authz wrapper       — may THIS caller see THIS scope?           │
   │     (NewTracesServiceWithAuthz, main.go:226-232 →                  │
   │      calls authz service /api/v1/authz/evaluates)                  │
   │                                                                    │
   │  ③ Name → UID resolution (resolveSearchScope, traces.go:109-140)   │
   │     "my-project" ──▶ openchoreo-api (OAuth2) ──▶ "f47ac10b-..."    │
   │     because storage indexes UIDs, not names                        │
   │                                                                    │
   │  ④ Route to a backend, via the TracingAdapter interface:           │
   │       GetTraces / GetSpans / GetSpanDetails                        │
   │            │                                                       │
   │            ├── TRACING_ADAPTER_ENABLED=true (default)              │
   │            │     → HTTP client to http://tracing-adapter:9100      │
   │            │       (the Layer-4 community module)                  │
   │            │                                                       │
   │            └── TRACING_ADAPTER_ENABLED=false                       │
   │                  → DefaultTracesAdaptor: query OpenSearch          │
   │                    otel-traces-* directly (built-in fallback,      │
   │                    same aggregation logic, queries.go:665-914)     │
   └────────────────────────────────────────────────────────────────────┘
```

Each step exists for a reason:

**① Authentication (JWT).** The adapter trusts anyone who can reach it; the
Observer doesn't. Every request must carry a valid JWT.

**② Authorization.** Being authenticated isn't enough — a developer in project A
must not read project B's traces. The traces service is wrapped in an authz
decorator that asks a central authorization service "may subject S query scope X?"
*before* any backend call happens.

**③ Name→UID resolution.** Humans and UIs use names ("checkout"); the spans in
storage are stamped with stable UIDs (names can change, UIDs can't). The Observer's
`ResourceUIDResolver` calls openchoreo-api (authenticating with OAuth2 client
credentials) to translate, then forwards UIDs downstream.

**④ Pluggability.** Both backends implement one Go interface:

```go
// pkg/observability/traces.go:12-19
type TracingAdapter interface {
    GetTraces(ctx, params)           (*TracesQueryResult, error)
    GetSpans(ctx, traceID, params)   (*SpansResult, error)
    GetSpanDetails(ctx, traceID, spanID) (*SpanDetail, error)
}
```

The handler and service code never know which one is behind it. The same pattern
is used for logs (`LOGS_ADAPTER_ENABLED`, adapter on :9098) and metrics (always an
external adapter on :9099) — tracing just follows the house convention.

Configuration is all env vars:

| Variable | Default | Meaning |
|---|---|---|
| `TRACING_ADAPTER_ENABLED` | `true` | use external adapter vs built-in OpenSearch fallback |
| `TRACING_ADAPTER_URL` | `http://tracing-adapter:9100` | where the Layer-4 module lives |
| `OPENSEARCH_ADDRESS` | `http://localhost:9200` | used by the fallback path |
| `AUTHZ_SERVICE_URL` | — | authorization service |
| `UID_RESOLVER_OPENCHOREO_API_URL` | — | name→UID resolution |

---

## Part 7: Layer 6 — Consumers

The control plane discovers an Observer through a CRD: `ObservabilityPlane`
(namespaced) or `ClusterObservabilityPlane` (cluster-scoped), whose spec carries
an `observerURL` (`api/v1alpha1/observabilityplane_types.go`). An `Environment`
is tied to planes, so when a UI asks "show traces for component X in prod," the
control plane knows *which* Observer instance to call and does so over HTTPS with
a JWT.

---

## Part 8: One request, end to end

**Write path** (happens continuously, milliseconds per span):

```
1. User hits POST /checkout on the "checkout" component in prod.
2. The app's OTel SDK creates spans (root + children), propagates the
   traceparent header to downstream services it calls.
3. SDK batches spans → OTLP → OTel Collector :4318.
4. Collector's k8sattributes processor looks up the sending pod and stamps:
   openchoreo.dev/namespace, project-uid, component-uid, environment-uid.
5. tail_sampling rate-limits; the OpenSearch exporter writes each span
   as a document into otel-traces-2026-06-10.
6. 30 days later, the ISM policy deletes that whole index.
```

**Read path** (when a developer opens the tracing UI):

```
1. UI → Observer:9097  POST /api/v1alpha1/traces/query
        JWT + { searchScope: { namespace, project: "shop", component: "checkout" } }
2. Observer validates the JWT, asks authz "may this user query this scope?" → yes
3. Observer resolves "shop"/"checkout" names → UIDs via openchoreo-api
4. Observer → tracing-adapter:9100 with UIDs
5. Adapter builds a terms-aggregation query (group spans by traceId,
   compute root span / duration / error count) → OpenSearch
6. Trace summaries flow back up; UI shows a sortable list, errors flagged
7. Developer clicks a trace → /traces/{traceId}/spans/query → all spans
   → UI rebuilds the parent/child tree → waterfall diagram → "ah, the
   slowness is in 'charge card'"
```

---

## Part 9: Why it's layered this way

The architecture comes down to three separations:

1. **Collection vs. query** — the Collector and the adapter share nothing at
   runtime; their only contract is the storage schema (the index template). You
   can scale or replace either independently.
2. **Backend-specific vs. backend-agnostic** — everything that knows "OpenSearch"
   (or X-Ray, or OpenObserve) lives in one community module. The Observer, authz,
   UID resolution, and UI are written once against the OpenAPI contract.
   **Adding a new backend = writing one new community module** (collector exporter
   config + the three query endpoints); zero changes in the main repo.
3. **Trust boundary** — the adapter enforces *what* can be queried (namespace
   filter is structurally required), the Observer enforces *who* may query it
   (JWT + authz). Keeping the security layer in the platform core means a
   community-contributed adapter can't weaken tenant isolation.

---

## Part 10: Where the k8sattributes enrichment comes from

Two separate places work together: the **processor configuration** lives in the
community module's Helm chart, and the **pod labels it extracts** are stamped by
the OpenChoreo control plane at deploy time.

### 10.1 The processor configuration

`observability-tracing-opensearch/helm/templates/opentelemetry-collector/configMap.yaml`:

```yaml
processors:
  k8sattributes:
    auth_type: "serviceAccount"   # use the collector pod's own SA to call the k8s API
    passthrough: false            # actually resolve metadata here (don't defer)
    extract:
      labels:
        - tag_name: $$1           # Helm-escaped $1 → the regex capture group
          key_regex: (.*)         # match EVERY pod label
          from: pod
      metadata:
        - k8s.pod.name
        - k8s.pod.uid
        - k8s.deployment.name
        - k8s.namespace.name
        - k8s.node.name
```

Two things to notice:

- **It copies all pod labels, not just OpenChoreo's.** `key_regex: (.*)` with
  `tag_name: $$1` means "take every label on the pod and attach it to the span as
  a resource attribute, with the same name." So `openchoreo.dev/component-uid: d4e5...`
  on the pod becomes `resource.openchoreo.dev/component-uid` on the span. There is
  no OpenChoreo-specific list in the collector config — the filtering happens
  later, because the OpenSearch index template (`dynamic: false`) only indexes the
  specific `resource.openchoreo.dev/*` fields it has mappings for. Everything else
  is carried but not searchable.

- **It's only enabled where pods are local.** The Helm template gates the
  processor on installation mode: it runs in `singleCluster` and
  `multiClusterExporter` modes, but **not** in `multiClusterReceiver`. Enrichment
  must happen in the cluster where the workload pods actually live, because the
  processor identifies the sender by matching the span's **source IP to a pod IP**
  via the local Kubernetes API (hence `auth_type: serviceAccount` — the collector's
  service account needs RBAC to list/watch pods, provided by the upstream chart).
  The central receiver in another cluster cannot resolve a foreign pod IP, so
  spans arrive there already enriched.

```
multiClusterExporter (data plane)              multiClusterReceiver (obs plane)
┌────────────────────────────────┐             ┌─────────────────────────────┐
│ app pod ──OTLP──▶ collector    │             │ collector                   │
│   ▲              [k8sattributes│──OTLP──────▶│ [NO k8sattributes]          │
│   │               + sampling]  │  (already   │ ──▶ opensearch exporter     │
│   └─ "which pod has this IP?"  │   enriched) │                             │
│      via local k8s API         │             │                             │
└────────────────────────────────┘             └─────────────────────────────┘
```

### 10.2 Where the pod labels themselves come from

The labels exist on the pod because the **ReleaseBinding controller** puts them
there when it renders the workload. In
`internal/controller/releasebinding/controller.go:289-309`, it builds the standard
label set from the actual Kubernetes object UIDs:

```go
standardLabels := map[string]string{
    labels.LabelKeyComponentUID:   componentUID,    // string(component.UID)
    labels.LabelKeyEnvironmentUID: environmentUID,  // string(environment.UID)
    labels.LabelKeyProjectUID:     projectUID,      // string(project.UID)
    // plus the human-readable *-name labels
}
```

The constants are defined in `internal/labels/labels.go:23-25`
(`openchoreo.dev/project-uid`, `openchoreo.dev/component-uid`,
`openchoreo.dev/environment-uid`). These go into the `MetadataContext`
(`controller.go:311-326`) — both as `Labels` and `PodSelectors` — which feeds the
ComponentType rendering pipeline, so the generated Deployment's pod template
carries them. The RenderedRelease then ships those manifests to the data plane.

### 10.3 Full provenance of one field

```
Component CR created → k8s assigns metadata.uid
        │
ReleaseBinding controller: standardLabels[openchoreo.dev/component-uid] = component.UID
        │  (releasebinding/controller.go:295)
        ▼
rendering pipeline → Deployment podTemplate.metadata.labels → pod gets the label
        │
        ▼
app sends span → collector matches source IP → pod → k8sattributes copies
all pod labels onto the span as resource attributes
        │
        ▼
OpenSearch doc: resource."openchoreo.dev/component-uid" (indexed as keyword)
        │
        ▼
adapter query: term filter on that field
```

The nice property: the app, the OTel SDK, and the collector config all have zero
knowledge of OpenChoreo identity. The control plane stamps identity onto pods
once, and the generic "copy all labels" rule carries it through to every span
automatically.
