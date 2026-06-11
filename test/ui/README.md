# test/ui — OpenChoreo Backstage UI tests

Playwright tests that drive the Backstage portal end-to-end in Chromium against a running OpenChoreo cluster — sign-in, catalog, component lifecycle, and role-based access, asserted through the real UI.

## Structure

```
ui/
├── playwright.config.ts   Runner config; maps *.e2e-cp.local hostnames to 127.0.0.1
│                          via Chromium host-resolver rules (no /etc/hosts edit needed)
├── specs/                 One folder per suite (auth, catalog, config, lifecycle, dev-ops, pe-ops, abac-ui)
├── po/                    Page objects — intent-named methods over semantic locators
│                          (getByRole/getByLabel; no data-testid hooks needed)
├── fixtures/              Playwright test.extend fixtures: per-role auth storage state,
│                          a thin kubectl wrapper for cross-checking the cluster
├── k3d/                   Helm value overlay that enables Backstage on the e2e cluster
├── scripts/               seed-idp-users.sh — re-seeds Thunder IdP identities on an
│                          already-installed cluster
└── global-setup.ts        Pre-flight checks before the suite runs
```

## How to run

```sh
make e2e.setup E2E_WITH_UI=true   # e2e cluster with Backstage enabled (or `make e2e.setup-ui` to add UI to an existing cluster)
cd test/ui
npm ci
npx playwright install --with-deps chromium
UI_BASE_URL=http://openchoreo.e2e-cp.local:28080 npm test
```

For a fresh install the test identities (PE, Dev, ABAC-restricted) are seeded automatically by Thunder's bootstrap; on an already-installed cluster run `scripts/seed-idp-users.sh` (Thunder is briefly down while it re-runs the setup Job). To watch a run, use `PWSLOWMO=1000 npx playwright test --headed`.

## Test suites

| Suite | What it covers |
|---|---|
| `auth/` | Thunder OIDC sign-in lands in the portal; `occ login` completes a PKCE round-trip driven through the browser consent form |
| `catalog/` | A Component applied with kubectl appears in the Backstage catalog |
| `lifecycle/` | Full UI-driven journey: create project + component → deploy → delete, verified against the cluster at each step |
| `config/` | Component configuration edits through the UI: env vars, secret env vars, file mounts, and per-environment overrides, including form validation |
| `dev-ops/` | Developer role: can create and view Components; platform-level actions (ComponentType create) are denied server-side |
| `pe-ops/` | Platform-engineer role: full CRUD (create, update, delete) of PE-managed CRDs via a representative subset — ComponentType and Trait (namespace-scoped) + ClusterComponentType and ClusterTrait (cluster-scoped) via the YAML editor scaffolder flow, and Environment + DeploymentPipeline via the FormWithYaml scaffolder flow. The remaining CRDs (Workflow, ResourceType, ClusterWorkflow, ClusterResourceType) follow the identical YAML editor UI path and are covered implicitly. Updates are tested via the Definition tab YAML editor; ComponentType and ClusterComponentType also test invalid-edit rejection. |
| `abac-ui/` | Environment-conditioned access: deploy/promote allowed up to staging, the production Promote button renders permission-disabled, and the shape survives relogin |
| `observability/` | Observability panels render their UI chrome: component Logs (`/runtime-logs`), component Metrics (`/metrics`), project Logs (`/logs`), and project Traces (`/traces`). Deploys the url-shortener sample (snip-postgres + snip-redis + snip-api-service with OTEL enabled) via kubectl, waits for Active, then asserts each panel mounts its filter controls. Self-skips when `ClusterObservabilityPlane` is absent — enable with `E2E_WITH_OBSERVABILITY=true`. |

The `pkce-login` and `abac-ui` specs self-skip when their prerequisites are missing (host DNS entries for `occ`, the seeded ABAC identity), so the rest of the suite is unaffected. The `observability/` suite also self-skips when `ClusterObservabilityPlane "default"` is not present.

## Test flow

Specs sign in through the real Thunder OIDC redirect (the shared helper in `fixtures/auth.ts` mints per-role `storageState`, so each suite starts authenticated as the role it tests). They then interact with the portal exclusively through page objects in `po/`, and cross-check every UI action against the cluster with the `kubectl` fixture — a create in the UI must produce the right resource shape, and a delete must remove it. Role suites assert both sides: what the UI permits and what the server actually enforces.

### pkce-login host prerequisites

The `auth/pkce-login` spec spawns `occ` as a host process, so the e2e hostnames must resolve via real DNS (Chromium's resolver rules don't apply):

```sh
sudo sh -c 'echo "127.0.0.1 openchoreo.e2e-cp.local api.e2e-cp.local thunder.e2e-cp.local observer.e2e-op.local" >> /etc/hosts'
```

It also needs the `occ` binary (`make go.build.occ`, auto-resolved from `bin/dist/`, override with `OCC_BIN`) and uses `OCC_CONTROL_PLANE_URL` (default `http://api.e2e-cp.local:28080`) for the API — distinct from `UI_BASE_URL`, which is the portal.

## Test walkthroughs

The exact steps each spec performs. Click a spec to expand; the source link points at its `test.describe`.

<details>
<summary><b>sign-in</b> — navigate home → OIDC redirect to Thunder → credentials → post-login sidebar</summary>

Source: [`specs/auth/sign-in.spec.ts:14`](specs/auth/sign-in.spec.ts#L14)

1. Navigate to `/` and click the on-page `Sign In` button (OpenChoreo OIDC provider)
2. Fill the Thunder login gate with the platform-engineer credentials and submit
3. Wait for the `Home` link in the Backstage sidebar — proof the OIDC redirect round-tripped and a session exists
4. Assert the page title matches the portal

</details>

<details>
<summary><b>pkce-login</b> — `occ login` emits auth URL → browser drives consent → token persisted → `occ` API call succeeds</summary>

Source: [`specs/auth/pkce-login.spec.ts:70`](specs/auth/pkce-login.spec.ts#L70)

1. Self-skips unless `api.e2e-cp.local` resolves on the host (see prerequisites above)
2. Bootstrap an isolated `occ` home, add the e2e control-plane context, and spawn `occ login` with a noop browser shim
3. Extract the OAuth2 authorization URL from `occ` stdout, navigate Playwright to it, and submit the PE credentials on Thunder's consent form
4. Assert `occ login` exits 0 after the callback round-trips
5. Cross-check the persisted token by running `occ component list` — exits 0

</details>

<details>
<summary><b>catalog-sync</b> — kubectl apply a Component → catalog provider polls → entity appears in Backstage</summary>

Source: [`specs/catalog/catalog-sync.spec.ts:45`](specs/catalog/catalog-sync.spec.ts#L45)

1. As PE, apply a Project + Component to the cluster with kubectl — no UI interaction creates them
2. Open the catalog filtered to the Component kind and poll (reloading) for up to 6 min until the applied Component shows up as a link
3. Open the entity page and assert the Component heading renders

</details>

<details>
<summary><b>full-lifecycle</b> — create project → create component → deploy to dev → kubectl agrees at each step → delete both via UI</summary>

Source: [`specs/lifecycle/full-lifecycle.spec.ts:32`](specs/lifecycle/full-lifecycle.spec.ts#L32)

1. Create a Project through the UI form; poll `kubectl get project` until the CR exists
2. Create a Component (Web Application template, pre-built greeter image, port 9090); verify the Component CR and its `componentType` via kubectl
3. Deploy to the `development` environment through the UI, setting workload args; wait for the release to show Active and cross-check the ReleaseBinding's Ready condition with kubectl
4. Delete the Component via the entity overflow menu; poll kubectl until the CR is gone
5. Delete the Project the same way; poll kubectl until it is gone

</details>

<details>
<summary><b>config</b> — deploy a component → edit env vars, secrets, and file mounts → per-environment overrides → kubectl verifies every save</summary>

Source: [`specs/config/component-config-edits.spec.ts:193`](specs/config/component-config-edits.spec.ts#L193)

1. As PE, create a project + component and deploy to development; seed a SecretReference and ABAC bindings via kubectl (serial spec, ~14 tests)
2. Component-level config: add a plain env var and a secret env var (from the SecretReference), then a plain and a secret file mount; edit values; each save creates a release and kubectl confirms the Workload carries the change
3. Form validation: an empty env var name or file name disables Apply
4. Per-environment overrides: add development-only env vars and file mounts, override inherited component values (name fields locked), edit and then delete all overrides — each step verified against `ReleaseBinding.spec.workloadOverrides` via kubectl
5. Edit replicas on the component tab → `componentTypeEnvironmentConfigs.replicas` updates
6. As the ABAC-restricted user, open the production overrides page → the UI signals denial and kubectl confirms the production binding never mutated
7. Remove all config through the UI, then delete the component and project via their overflow menus; kubectl confirms everything is gone

</details>

<details>
<summary><b>dev-ops</b> — sign in as Dev → create component succeeds → ComponentType create denied server-side</summary>

Source: [`specs/dev-ops/dev-ops-component.spec.ts:36`](specs/dev-ops/dev-ops-component.spec.ts#L36)

1. As the Dev identity, with a PE-seeded Project (kubectl), create a Component through the scaffolder (Web Application template, pre-built image)
2. kubectl confirms both the Component CR and its generated Workload CR exist; the component overview page renders
3. Navigate directly to the ComponentType scaffolder URL (bypassing the client-side disabled card) and submit the form
4. Assert the server rejects it: the task page shows a permission-denied error and kubectl confirms no ComponentType CR was created — the deny is enforced server-side, not just hidden in the UI

</details>

<details>
<summary><b>pe-ops (YAML editor)</b> — sign in as PE → create, update, and delete PE CRDs that use the YAML editor scaffolder flow</summary>

Source: [`specs/pe-ops/pe-ops-yaml-editor-crud.spec.ts`](specs/pe-ops/pe-ops-yaml-editor-crud.spec.ts)

For a representative subset — ComponentType and Trait (namespace-scoped), ClusterComponentType and ClusterTrait (cluster-scoped) — covering both scope variants and the invalid-edit branch:

1. Open the Create page and fill the matching Scaffolder template (name + description), advance to the YAML editor step, then submit
2. Poll kubectl until the CR exists and assert its description and kind round-tripped
3. Wait (up to 6 min) for the catalog provider to ingest the entity, then open it from the catalog table
4. Navigate to the Definition tab via the Edit icon, modify the description annotation in the YAML editor, save, and poll kubectl until the change is reflected
5. Navigate back to the entity page and delete via the overflow menu; poll kubectl until the CR is not-found

</details>

<details>
<summary><b>pe-ops (FormWithYaml)</b> — sign in as PE → create, update, and delete Environment and DeploymentPipeline via FormWithYaml scaffolder</summary>

Source: [`specs/pe-ops/pe-ops-form-with-yaml-crud.spec.ts`](specs/pe-ops/pe-ops-form-with-yaml-crud.spec.ts)

1. Create an Environment via the form (name, description, auto-selected namespace + dataplane), verify form→YAML→form round-trip preserves values, then submit
2. Poll kubectl until the Environment CR exists; verify spec shape (isProduction, dataPlaneRef)
3. Update the description via the Definition tab YAML editor; poll kubectl until reflected
4. Delete via the overflow menu; poll kubectl until not-found
5. Create a DeploymentPipeline via the form (name, description), submit, verify shape, update, and delete via the same flow

</details>

<details>
<summary><b>pe-ops (legacy)</b> — sign in as PE → create ComponentType and Trait via Scaffolder → kubectl shape check → delete via catalog menu</summary>

Source: [`specs/pe-ops/pe-ops-pipeline.spec.ts:59`](specs/pe-ops/pe-ops-pipeline.spec.ts#L59)

1. For each of ComponentType and Trait (serial): open the Create page and fill the matching Scaffolder template (name + description), walking the wizard to Create
2. Poll kubectl until the CR exists and assert its description field round-tripped into the spec
3. Wait (up to 6 min) for the catalog provider to ingest the entity, then open it from the catalog table
4. Delete it via the entity overflow menu and confirm in the modal; poll kubectl until the CR is not-found

</details>

<details>
<summary><b>abac-ui</b> — env-conditioned bindings → deploy dev ✓ → promote staging ✓ → promote production denied → relogin keeps the deny</summary>

Source: [`specs/abac-ui/abac-env-restriction.spec.ts:126`](specs/abac-ui/abac-env-restriction.spec.ts#L126)

1. Self-skips unless the ABAC identity is seeded (see the Thunder bootstrap note above); seeds a ClusterAuthzRole + allow (dev/staging) and deny (production) bindings via kubectl
2. As the ABAC identity, create a component and deploy to development; wait for the release to go Active
3. Promote to staging — allowed; kubectl confirms exactly one staging ReleaseBinding exists
4. Attempt promote to production — the Promote button renders permission-disabled with a tooltip, and kubectl confirms no production ReleaseBinding was created
5. Sign out and re-authenticate interactively (forcing a Casbin cache eviction), then assert the production Promote button is still denied — regression guard for backstage-plugins#549

</details>

<details>
<summary><b>observability</b> — deploy url-shortener sample → wait Active → assert Logs / Metrics / Traces panels render</summary>

Source: [`specs/observability/observability-panels.spec.ts`](specs/observability/observability-panels.spec.ts)

1. Self-skips unless `ClusterObservabilityPlane "default"` exists; requires `make e2e.setup E2E_WITH_UI=true E2E_WITH_OBSERVABILITY=true`
2. Applies the url-shortener project + snip-postgres + snip-redis + snip-api-service (OTEL-instrumented) inline via kubectl with timestamped names
3. Polls until the api-service ReleaseBinding reaches Ready in the development environment (`autoDeploy: true`)
4. Navigates to the component entity's `/runtime-logs` tab; asserts the "Search Logs..." filter and "Refresh" button are visible
5. Navigates to the component entity's `/metrics` tab; asserts "CPU Usage" and "Memory Usage" card titles plus the "Refresh" button
6. Navigates to the project (system) entity's `/logs` tab; asserts the logs filter chrome renders
7. Navigates to the project entity's `/traces` tab; asserts the "Search Trace ID" input and "Refresh" button render

</details>
