# test/ — OpenChoreo test suites

End-to-end and UI tests that run against a real OpenChoreo cluster (local k3d), plus shared Go test helpers. Note that the unit/integration tests live next to the code they test.

## Structure

```text
test/
├── e2e/        Go (Ginkgo) end-to-end tests — `make e2e`
│   ├── e2e_suite_test.go   Suite entry point; requires --e2e.kubecontext
│   ├── e2e_test.go         Tier-1 platform health checks (all pods running)
│   ├── suites/             One package per feature area (authz, build, gateway, gitops, mcp, observability, occ, secrets, …)
│   ├── framework/          Test helpers: kubectl, occ, gitea, flux, gateway, port-forwarding, waits, MCP client, fixtures
│   ├── fixtures/           Shared test data (alert rules, build sources)
│   ├── cmd/tier3-fixtures/ One-shot setup binary that seeds tier-3 build sources before the suite runs
│   └── k3d/                Cluster bring-up config: k3d config, Helm value overlays, plane registrations, CoreDNS and secret-store manifests
├── ui/         Playwright tests for the Backstage portal (see ui/README.md)
│   ├── specs/              Test suites (auth, catalog, lifecycle, dev-ops, pe-ops, abac-ui, config)
│   ├── po/                 Page objects (semantic locators, intent-named methods)
│   ├── fixtures/           Per-role auth state + kubectl wrapper fixtures
│   ├── k3d/                Helm overlay that enables Backstage on the e2e cluster
│   └── scripts/            IdP user seeding for already-installed clusters
└── utils/      Shared Go helpers (run commands, install/uninstall cert-manager and prometheus-operator for tests)
```

- **`e2e/`** — spins up nothing itself; it asserts against an existing cluster identified by `--e2e.kubecontext`. Suites are labeled `tier1`, `tier2`, `tier3` on their top-level `Describe` so CI can shard them (`E2E_LABEL_FILTER`). `make e2e.setup` creates the local `openchoreo-e2e` k3d cluster from the configs in `e2e/k3d/`, then `make e2e.test` runs the suites.
- **`ui/`** — drives the Backstage portal in Chromium against the same e2e cluster (`make e2e.setup E2E_WITH_UI=true`). Covers sign-in (Thunder OIDC), catalog sync, full component lifecycle, role-based access, and ABAC restrictions. Full details and prerequisites in [`ui/README.md`](ui/README.md).
- **`utils/`** — small Go package used by tests to shell out (`Run`) and to install/uninstall external operators (cert-manager, prometheus-operator).

## Running

```sh
make e2e                # full lifecycle: setup k3d cluster → test → teardown
make e2e.test           # run tests against an existing cluster
cd test/ui && npm test  # UI tests (after make e2e.setup E2E_WITH_UI=true)
```

See `make help` for the full list of e2e targets.
