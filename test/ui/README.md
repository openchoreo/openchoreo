# test/ui — OpenChoreo Backstage UI tests

Playwright tests that drive the Backstage portal end-to-end against a running
OpenChoreo cluster.

## Quickstart against the local `openchoreo-e2e` k3d cluster

The cleanest path is to let the Makefile do the bring-up:

```sh
make e2e.setup E2E_WITH_UI=true
cd test/ui
npm ci
npx playwright install --with-deps chromium
UI_BASE_URL=http://openchoreo.e2e-cp.local:28080 npm test
```

`make e2e.setup E2E_WITH_UI=true` extends the default e2e setup with the
`test/ui/k3d/values-cp-ui.yaml` overlay (turns Backstage on) and waits for the
Backstage deployment to become Ready. The PE and Dev identities come from
Thunder's own bootstrap (`50-user-schema-and-users.sh`).

To bolt UI onto an existing cluster without re-running the full setup:

```sh
make e2e.setup-ui
```

The Playwright config maps the `*.e2e-cp.local` hostnames to `127.0.0.1` via
Chromium's `--host-resolver-rules`, so no `/etc/hosts` edit is needed.

## Layout

- `playwright.config.ts` — runner config.
- `specs/` — one folder per suite (`auth/`, `catalog/`, `lifecycle/`,
  `dev-ops/`, `pe-ops/`, `abac-ui/`).
- `po/` — page objects (intent-named methods, semantic locators). Every
  affordance currently resolves via `getByRole` + accessible name or
  `getByLabel`; no `data-testid` escape hatches are required against the
  current Backstage UI surface.
- `fixtures/` — Playwright `test.extend` fixtures (per-role storage state via
  `mintAuthState` + `storageStateFor`, a thin `kubectl` wrapper).
- `k3d/` — Helm value overlays that turn the e2e cluster into a UI-test target.
- `scripts/` — `seed-idp-users.sh`, the Thunder setup-Job re-run for
  already-installed clusters (see below).

## Specs

| Suite | Spec | What it asserts |
|---|---|---|
| `auth/` | sign-in | Thunder OIDC sign-in lands on the post-login Backstage layout |
| `auth/` | pkce-login | `occ login` PKCE round-trip: Playwright drives the consent form, token persists, `occ get components` succeeds (self-skips unless the e2e hostnames resolve on the host — see below) |
| `catalog/` | catalog-sync | kubectl-applied Component shows up in the Backstage catalog within polling timeout |
| `lifecycle/` | full-lifecycle | UI-driven project + component create → deploy → delete, kubectl agreement on each step |
| `dev-ops/` | dev-ops-component | Dev creates + views a Component; ComponentType create is denied server-side |
| `pe-ops/` | pe-ops-pipeline | PE CRUD via the Scaffolder templates (ComponentType, Trait): create → kubectl shape → delete via the entity overflow menu |
| `abac-ui/` | abac-env-restriction | Env-conditioned ClusterAuthzRoleBinding: deploy-to-dev + promote-to-staging allowed, promote-to-production renders the Promote button permission-disabled; relogin keeps the same permission shape (regression guard for backstage-plugins#549). Self-skips unless the ABAC identity is seeded — see below |

## Sign-in spec

`specs/auth/sign-in.spec.ts` signs in as `platform-engineer@openchoreo.dev`
(seeded by Thunder's `50-user-schema-and-users.sh`).

Sign-in is a same-page OIDC redirect: click the on-page `Sign In` button →
fill the Thunder gate (`Enter your username` / `Enter your password`) → submit →
wait for the post-login sidebar `Home` link. The shared helper in
`fixtures/auth.ts` drives the same flow to mint per-role `storageState`.

## Thunder bootstrap overlay (redirect URI + ABAC identity)

`test/e2e/k3d/values-thunder.yaml` overlays two bootstrap scripts onto the
Thunder chart:

- `51-backstage-app.sh` — adds
  `http://openchoreo.e2e-cp.local:28080/api/auth/openchoreo-auth/handler/frame`
  alongside the single-cluster default redirect URI.
- `52-abac-user.sh` — provisions the ABAC-restricted identity
  (`abac-dev@openchoreo.dev` in group `abac-developers`) the `abac-ui` suite
  signs in as. Thunder's admin API rejects every request (401) once the
  server is up — even from loopback inside the pod — so identities can only
  be seeded from the bootstrap setup Job, which runs against the SQLite
  store directly.

The Thunder helm chart only runs the bootstrap on `helm install`
(`helm.sh/hook: pre-install`, `hook-delete-policy: hook-succeeded`), so
fresh installs (`make e2e.setup E2E_WITH_UI=true`) get both automatically.
For an already-installed cluster, re-run the setup Job with:

```sh
test/ui/scripts/seed-idp-users.sh
```

The script re-renders the bootstrap ConfigMap with the e2e overlay, scales
Thunder to zero (the setup Job needs the RWO SQLite PVC), re-applies the
setup Job (renamed, helm hooks stripped — the bootstrap scripts are
idempotent), and scales Thunder back up. Sign-ins fail during the ~1–2
minute window while Thunder is down. Requires `helm`, `kubectl`, `yq`.

## pkce-login prerequisites

The `auth/pkce-login` spec spawns `occ` as a host process — unlike the
browser specs it cannot use Chromium's `--host-resolver-rules`, so the e2e
hostnames must resolve via real DNS on the host:

```sh
sudo sh -c 'echo "127.0.0.1 openchoreo.e2e-cp.local api.e2e-cp.local thunder.e2e-cp.local" >> /etc/hosts'
```

The spec self-skips when the control-plane hostname doesn't resolve, so the
rest of the suite is unaffected by a missing entry. It also needs the `occ`
binary — `make go.build.occ` places it under `bin/dist/<os>/<arch>/occ`,
which the spec resolves automatically (override with `OCC_BIN`). The
control-plane API URL defaults to `http://api.e2e-cp.local:28080`; override
with `OCC_CONTROL_PLANE_URL` (deliberately not `UI_BASE_URL`, which is the
Backstage portal, not the API).

## Headed runs

`launchOptions.slowMo` is parked behind the `PWSLOWMO` env var (currently
commented out in `playwright.config.ts`). Re-enable when you want to watch
the click stream:

```sh
PWSLOWMO=1000 npx playwright test --headed
```
