# test/e2e — OpenChoreo end-to-end tests

Go (Ginkgo) suites that exercise OpenChoreo features against a real Kubernetes cluster running all OpenChoreo planes — the same install path a user gets, on a local k3d cluster.

## Structure

```text
e2e/
├── e2e_suite_test.go   Suite entry point; verifies the cluster is reachable (--e2e.kubecontext)
├── e2e_test.go         Platform health checks: all control/data plane pods running
├── suites/             One Go package per feature area (see table below)
├── framework/          Shared helpers: kubectl/occ/helm wrappers, Eventually-style waits,
│                       gateway invocation, port-forwarding, Gitea repos, MCP client
├── fixtures/           Shared test data (alert rules, build source projects)
├── cmd/tier3-fixtures/ One-shot binary that seeds Gitea build sources before tier-3 runs
└── k3d/                Cluster bring-up: k3d config, Helm value overlays per plane,
                        plane registration CRs, CoreDNS and secret-store manifests
```

## How to run

```sh
make e2e                                  # full lifecycle: setup → test → teardown (diagnostics on failure)
make e2e.setup                            # create the openchoreo-e2e k3d cluster and install all planes
make e2e.test                             # run suites against the existing cluster
make e2e.test E2E_LABEL_FILTER='tier1'    # scope to a tier (Ginkgo label expression, e.g. 'tier1 || tier2')
make e2e.down                             # delete the cluster
```

Useful knobs: `E2E_WITH_BUILD=true` / `E2E_WITH_OBSERVABILITY=true` add the workflow and observability planes (required for tier 3); `E2E_WITH_UI=true` enables Backstage for the UI suite (`test/ui/`); `E2E_KEEP_RESOURCES=true` skips per-suite cleanup for debugging. `make e2e.status` and `make e2e.diagnostics` help inspect a broken cluster.

## Test suites

Suites are labeled by tier on their top-level `Describe`, so CI can shard them and developers can run subsets.

| Tier | Suite | What it covers |
|---|---|---|
| 1 | Platform Health | All control-plane and data-plane pods reach Running |
| 1 | Workload Type Matrix | Service, web-app, and scheduled-task components deploy and respond |
| 1 | Connection Resolution | Component-to-component connections resolve and route traffic |
| 1 | NetworkPolicy Enforcement | Cross-project/namespace traffic is isolated as declared |
| 1 | GCP Microservices Demo | A realistic multi-service application deploys end to end |
| 2 | Authorization | Roles, bindings, namespace scoping, deny overrides, CEL conditions, propagation |
| 2 | Gateway Routing | HTTP routing of deployed endpoints through the gateway |
| 2 | OpenChoreo API | The control-plane REST API |
| 2 | OCC CLI | `occ` commands against the control plane |
| 2 | MCP Server | MCP tools served by the control plane |
| 2 | Secrets and External Secrets | Secret references resolved via ESO/OpenBao into workloads |
| 3 | Build From Source Matrix | Source-to-image builds via the workflow plane (sources seeded by `cmd/tier3-fixtures`) |
| 3 | GitOps with Flux | Deploying OpenChoreo resources through a Flux-managed Git repo |
| 3 | Observability Signals | Logs, metrics, and traces flow into the observability plane |
| 3 | Observability Alerts | Alert rules fire and reach notification channels |

Tiers 1–2 run on the default setup; tier 3 additionally needs the workflow and observability planes. Tier 5 is the Backstage UI suite, which lives separately in [`test/ui/`](../ui/README.md).

## Test flow

Each suite is an `Ordered` Ginkgo `Describe` that follows the same shape:

1. `BeforeAll` applies fixtures with kubectl — namespaces, environments, projects, components — and polls (`Eventually`) until the controllers reconcile them (e.g. the data-plane namespace appears, pods become Ready).
2. Specs assert the outcome: resource state via kubectl, and live behavior by invoking the deployed workload through the gateway (from an in-cluster tester pod or a port-forward).
3. `AfterAll` deletes the suite's namespaces, which cascades cleanup to the data plane.

Tests never create the cluster themselves — they target whatever `--e2e.kubecontext` points at, so a single setup is reused across suites and runs. New suites go under `suites/<area>/` as their own package, reuse `framework/` helpers, and pick the lowest tier whose plane requirements they fit.

## Test walkthroughs

The exact steps each suite performs, mirroring the `By(...)` steps in the code. Click a suite to expand; the source link points at its top-level `Describe`.

<details>
<summary><b>Platform Health</b> (tier 1) — control/data plane pods Running → CRDs registered → defaults present → agent connected</summary>

Source: [`e2e_test.go:16`](e2e_test.go#L16)

1. Assert all pods in the control-plane and data-plane namespaces reach Running
2. Assert the OpenChoreo CRDs are registered (Project, Component, ComponentType, Trait, Environment, DeploymentPipeline, ComponentRelease, ReleaseBinding, Workload, Workflow, …)
3. Assert the default namespace carries the out-of-box resources: Project `default`, DeploymentPipeline, the development/staging/production Environments, the built-in ClusterComponentTypes and builder ClusterWorkflows
4. Assert the ClusterDataPlane `default` reports its agent as connected, and the cluster-agent pod logs confirm the connection

</details>

<details>
<summary><b>Workload Type Matrix</b> (tier 1) — apply platform fixtures → deploy 3 component types → reconcile → invoke → delete → verify drain</summary>

Source: [`suites/workloadtypes/workloadtypes_test.go:30`](suites/workloadtypes/workloadtypes_test.go#L30)

1. Apply DeploymentPipeline, Environment, Project via kubectl
2. Apply three Components with pre-built images: a service (greeter), a web application (http-echo), a scheduled task
3. Wait for the controller to create the data-plane namespace and start a tester pod in it
4. Per type, assert: ReleaseBinding `Ready` → pod Running → endpoint TCP-reachable from the tester pod (for the CronJob: `lastScheduleTime` populated instead)
5. Invoke the service and web-app public URLs through the kgateway external listener (e.g. `/greeter/greet` returns 200)
6. Delete one Component → its ReleaseBinding and Deployment drain while the sibling stays Ready; delete the Project → the whole DP namespace drains

</details>

<details>
<summary><b>Connection Resolution</b> (tier 1) — deploy providers → deploy consumer with connections → resolve → env vars rendered → bad connection blocks Ready</summary>

Source: [`suites/connections/connections_test.go:23`](suites/connections/connections_test.go#L23)

1. Apply platform fixtures, then deploy two provider Components exposing HTTP endpoints with different visibilities
2. Assert provider ReleaseBindings reach Ready with `ConnectionsResolved=True` (reason `NoConnections`)
3. Deploy a consumer Component declaring connections to both providers; assert its ReleaseBinding reaches `ConnectionsResolved=True` (reason `AllConnectionsResolved`) and Ready
4. Assert the rendered Deployment carries the connection env vars (`PROVIDER_A_URL`, `PROVIDER_B_URL`) with the correct service URLs, and the ReleaseBinding status lists both resolved connections
5. Deploy a consumer pointing at a nonexistent component → `ConnectionsResolved=False` (reason `ConnectionsPending`) and Ready=False; remove the connections → status recovers and clears

</details>

<details>
<summary><b>NetworkPolicy Enforcement</b> (tier 1) — deploy components across namespaces/projects/envs → NetworkPolicies rendered → probe 11 allow/block scenarios</summary>

Source: [`suites/networkpolicy/networkpolicy_test.go:37`](suites/networkpolicy/networkpolicy_test.go#L37)

1. Create two OpenChoreo namespaces with multiple Projects, and deploy Components with project / namespace / internal / external endpoint visibilities plus client pods in each project
2. Promote one component to staging so a second environment's data-plane namespace exists
3. Assert per-component NetworkPolicies are rendered in every data-plane namespace and enforcement is active
4. Probe 11 connectivity scenarios from in-cluster client pods: intra-project allowed, cross-project to project-only blocked, cross-namespace blocked, cross-environment blocked, gateway → external-visible allowed, gateway → project-only blocked, and so on
5. Assert cluster DNS still resolves while the egress policies are in place

</details>

<details>
<summary><b>GCP Microservices Demo</b> (tier 1) — apply sample manifests → promote redis → all 11 services Ready → browse the shop through the gateway</summary>

Source: [`suites/microservicesdemo/microservicesdemo_test.go:50`](suites/microservicesdemo/microservicesdemo_test.go#L50)

1. `kubectl apply -R` the in-repo `samples/gcp-microservices-demo` (10 Components + a redis Resource)
2. Promote the redis ResourceReleaseBinding to its latest release (the manual step the sample README asks of users)
3. Wait for redis then all 10 component ReleaseBindings to reach Ready, and every pod to be Running
4. Through the public gateway URL, GET `/` (proves frontend → productcatalog gRPC), `/product/<id>` (product + currency RPCs), and `/cart` (cart RPC) — all must return 200

</details>

<details>
<summary><b>Authorization</b> (tier 2) — mint tokens per role → create roles/bindings → call the API → assert allow/deny</summary>

Source: [`suites/authz/authz_test.go:34`](suites/authz/authz_test.go#L34)

1. Mint an OIDC subject token and build authenticated + unauthenticated API clients
2. Per scenario: apply AuthzRole / ClusterAuthzRole and bindings via kubectl, wait for policy propagation, call the API operation under test, assert the HTTP status matches the expected allow/deny
3. Scenarios covered: unauthenticated (401), no binding (403), admin full access, developer partial access, namespace scoping, deny overriding an allow, binding create/delete propagation, cluster binding scoped to one namespace, role update propagation, server-side list filtering, who may manage authz resources themselves, CEL-conditioned allow, CEL-conditioned deny

</details>

<details>
<summary><b>Gateway Routing</b> (tier 2) — deploy components with different endpoint visibilities → HTTPRoutes rendered per visibility → invoke external/internal URLs → env gateway override</summary>

Source: [`suites/gateway/gateway_test.go:17`](suites/gateway/gateway_test.go#L17)

1. Deploy five Components whose endpoints differ in visibility (external, project-only, internal, multi-endpoint, custom basePath); promote one to staging
2. External visibility: HTTPRoute exists and is Accepted, the external URL returns 200
3. Project-only visibility: no external HTTPRoute is rendered and the ReleaseBinding has no `externalURLs`
4. Internal visibility: internal HTTPRoute only, reachable from the in-cluster tester pod
5. Multi-endpoint component: one external HTTPRoute per endpoint, each with its own URL; basePath component: the route rewrites the prefix (`replacePrefixMatch`) and stays routable
6. Staging: the rendered HTTPRoute parent switches to the environment's gateway override and hostname

</details>

<details>
<summary><b>OpenChoreo API</b> (tier 2) — get OAuth2 token → CRUD namespaces/projects/components over REST → CRs appear on the cluster → delete cleans up</summary>

Source: [`suites/openchoreoapi/openchoreoapi_test.go:20`](suites/openchoreoapi/openchoreoapi_test.go#L20)

1. Reach the API through kgateway, obtain an OAuth2 token from the Thunder IdP; assert unauthenticated requests get 401 and `/health`, `/ready`, `/version` return 200
2. Create a namespace via the API → the Kubernetes namespace exists with the control-plane label; list mixes API-created and kubectl-created namespaces
3. Create a project via the API → the Project CR exists; list ComponentTypes with cursor-based pagination
4. Create a component + workload via the API → Component CR, ComponentRelease, and ReleaseBinding appear, and the binding reaches Ready
5. Delete the component then the project via the API → Component, Workload, ComponentRelease, ReleaseBinding, and Project CRs are all removed

</details>

<details>
<summary><b>OCC CLI</b> (tier 2) — occ apply/get/create/delete against the control plane → resources reconcile → negative paths rejected</summary>

Source: [`suites/occ/occ_test.go:32`](suites/occ/occ_test.go#L32)

1. Smoke: `occ` connects and basic commands answer
2. `occ apply` YAML fixtures create and configure resources
3. Resource commands CRUD namespaces, projects, components, workloads, and release bindings
4. Config, secret, and login command groups each get their own pass (context config, SecretReference management, auth flows)
5. Negative scenarios: invalid input, missing fields, and authorization errors are rejected with useful errors
6. Delete commands remove what was created; suite cleanup drains the data-plane namespaces

</details>

<details>
<summary><b>MCP Server</b> (tier 2) — get token → connect MCP client → list/filter tools → create namespace/project/component/workload via tools → release chain appears</summary>

Source: [`suites/mcp/mcp_test.go:33`](suites/mcp/mcp_test.go#L33)

1. Fetch an OAuth2 token; assert an unauthenticated `POST /mcp` gets 401 with a `WWW-Authenticate: Bearer` challenge
2. Connect an MCP client and `tools/list`: core tools are present, and filtering by toolset narrows the list (e.g. `namespace` toolset exposes only namespace tools)
3. Call `create_namespace` → the namespace exists on the cluster and shows up in `list_namespaces`
4. Call `create_project`, then `create_component` (auto-deploy) and `create_workload` with image + endpoint → the corresponding CRs exist
5. The release chain follows: ComponentRelease appears in the component status, the ReleaseBinding is created, and `list_components` returns the new component

</details>

<details>
<summary><b>Secrets and External Secrets</b> (tier 2) — create SecretReferences → deploy workload using them → ESO syncs → pod sees real values → update/delete propagate</summary>

Source: [`suites/secrets/secrets_test.go:22`](suites/secrets/secrets_test.go#L22)

1. Apply platform fixtures, two SecretReferences (one env var, one file mount) backed by a fake secret store, and a Component + Workload that consumes both
2. Wait for ReleaseBinding Ready, then find the two ExternalSecrets the controller rendered in the DP namespace
3. Assert each ExternalSecret reaches Ready and its target K8s Secret holds the expected decoded value
4. `kubectl exec` into the workload pod: `printenv APP_USERNAME` and `cat password.txt` return the store values — the full chain from CR to process environment
5. Update the env SecretReference to a different store key → a new ExternalSecret with an updated content hash rolls the pod, which now reads the new value (the file secret is untouched)
6. Delete the Component → ExternalSecrets and their K8s Secrets cascade-delete from the DP namespace and no pods remain

</details>

<details>
<summary><b>Build From Source Matrix</b> (tier 3) — trigger all builds concurrently → each workflow succeeds → release deploys → invoke</summary>

Source: [`suites/build/build_test.go:90`](suites/build/build_test.go#L90)

1. Seed Gitea with sample source repos (shared tier-3 fixture)
2. Up front, apply a Component + WorkflowRun for each of the 5 builder cases — dockerfile (service + react web-app), GCP buildpacks, Paketo buildpacks, Ballerina buildpack — so the builds run concurrently on the workflow plane
3. Per builder, wait up to 20 min for its WorkflowRun (Argo Workflow) to succeed; for the dockerfile case, also assert live build-pod logs are streamable
4. Assert the build produced a ComponentRelease, the ReleaseBinding reaches Ready, and the workload pod runs the built image
5. Probe the rendered Service for TCP reachability from a tester pod
6. Solo specs: a build without `workload.yaml` auto-generates the Workload CR; SecretReference values resolve into the rendered Argo Workflow via CEL

</details>

<details>
<summary><b>GitOps with Flux</b> (tier 3) — push manifests to Git → Flux reconciles → deployed → push image bump → rollout</summary>

Source: [`suites/gitops/gitops_test.go:17`](suites/gitops/gitops_test.go#L17)

1. Install Flux and an in-cluster Gitea, create a gitops repo, push the platform manifests (namespace, pipeline, environments, project)
2. Point a Flux GitRepository + Kustomization at the repo and wait for it to reconcile
3. Push a Component + Workload doc → Flux applies it → ReleaseBinding Ready, pod Running — no kubectl apply of app resources anywhere
4. Push a commit bumping the image tag → the rendered Deployment rolls to the new image
5. Push 3 components in one commit → all 3 reconcile and run (bulk promote)

</details>

<details>
<summary><b>Observability Signals</b> (tier 3) — deploy workload → generate traffic → query observer API for logs and metrics</summary>

Source: [`suites/observability/observability_test.go:44`](suites/observability/observability_test.go#L44)

1. Deploy the greeter Component and wait for its ReleaseBinding Ready and pod Running; start a curl tester pod
2. Generate ~45s of HTTP traffic at 5 RPS against the greeter's ClusterIP with a marker query string
3. Query the observer's `POST /api/v1/logs/query` for the greeter's startup log line, scoped by component/project/environment
4. Query `POST /api/v1/metrics/query` for CPU/memory series (kube-state-metrics + cadvisor) of the workload
5. Probe the traces query endpoint best-effort (the sample app isn't OTel-instrumented, so empty results are acceptable)

</details>

<details>
<summary><b>Observability Alerts</b> (tier 3) — register webhook channel → apply alert rules → trigger conditions → notifications arrive at the receiver</summary>

Source: [`suites/alerts/alerts_test.go:40`](suites/alerts/alerts_test.go#L40)

1. Deploy an in-cluster webhook receiver and register it as a NotificationChannel
2. Apply the observability-alert-rule ClusterTrait and a Component carrying two alert rules: a metric rule (CPU threshold) and a log rule (pattern match)
3. Wait for the rendered ObservabilityAlertRule to land on the observability plane
4. Trigger the log rule by emitting the search phrase from inside the pod; poll the webhook receiver for the metric and log notifications (delivery asserted best-effort, rule presence strictly)
5. Run a build, delete its WorkflowRun, then query the observer for the deleted run's logs — build logs stay queryable after CR cleanup

</details>
