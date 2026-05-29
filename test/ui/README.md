# test/ui — OpenChoreo Backstage UI tests

Playwright tests that drive the Backstage portal end-to-end against a running
OpenChoreo cluster.

## Quickstart against the local `openchoreo-e2e` k3d cluster

The cleanest path is to let the Makefile do the bring-up:

```sh
make e2e.setup E2E_WITH_UI=true
cd test/ui
npm install
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
  `dev-ops/`).
- `po/` — page objects (intent-named methods, semantic locators). Every
  affordance currently resolves via `getByRole` + accessible name or
  `getByLabel`; no `data-testid` escape hatches are required against the
  current Backstage UI surface.
- `fixtures/` — Playwright `test.extend` fixtures (per-role storage state via
  `mintAuthState` + `storageStateFor`, a thin `kubectl` wrapper).
- `k3d/` — Helm value overlays that turn the e2e cluster into a UI-test target.

## Specs

| Suite | Spec | What it asserts |
|---|---|---|
| `auth/` | sign-in | Thunder OIDC sign-in lands on the post-login Backstage layout |
| `catalog/` | catalog-sync | kubectl-applied Component shows up in the Backstage catalog within polling timeout |
| `lifecycle/` | full-lifecycle | UI-driven project + component create → deploy → delete, kubectl agreement on each step |
| `dev-ops/` | dev-ops-component | Dev creates + views a Component; ComponentType create is denied server-side |

Further suites (`pe-ops`, `abac-ui`, `auth/pkce-login`) land in follow-up PRs.

## Sign-in spec

`specs/auth/sign-in.spec.ts` signs in as `platform-engineer@openchoreo.dev`
(seeded by Thunder's `50-user-schema-and-users.sh`).

Sign-in is a same-page OIDC redirect: click the on-page `Sign In` button →
fill the Thunder gate (`Enter your username` / `Enter your password`) → submit →
wait for the post-login sidebar `Home` link. The shared helper in
`fixtures/auth.ts` drives the same flow to mint per-role `storageState`.

## Thunder Backstage app must list the e2e redirect URI

The chart's `bootstrap.scripts.51-backstage-app.sh` is overlaid in
`test/e2e/k3d/values-thunder.yaml` to add
`http://openchoreo.e2e-cp.local:28080/api/auth/openchoreo-auth/handler/frame`
alongside the single-cluster default. The Thunder helm chart only runs the
bootstrap on `helm install` (`helm.sh/hook: pre-install`,
`hook-delete-policy: hook-succeeded`), so applying the overlay to an
already-installed cluster needs three steps:

```sh
# Re-render with the e2e overlay and patch the bootstrap ConfigMap in place.
helm --kube-context k3d-openchoreo-e2e template thunder \
  oci://ghcr.io/asgardeo/helm-charts/thunder --namespace thunder --version 0.28.0 \
  --values install/k3d/common/values-thunder.yaml \
  --values test/e2e/k3d/values-thunder.yaml \
  | awk '/^# Source: thunder\/templates\/bootstrap-configmap.yaml/,/^# Source:/' \
  | sed '/^# Source: thunder\/templates\/[^b]/,$d' \
  | kubectl --context k3d-openchoreo-e2e -n thunder apply -f -

# Free the RWO sqlite PVC so the setup Job can mount it.
kubectl --context k3d-openchoreo-e2e -n thunder scale deploy thunder-deployment --replicas=0
kubectl --context k3d-openchoreo-e2e -n thunder wait --for=delete pod \
  -l app.kubernetes.io/name=thunder --timeout=2m

# Re-apply the setup Job manifest (renamed, helm hooks stripped) — bootstrap
# scripts PUT-update each app idempotently against the existing PVC data.
```

## Headed runs

`launchOptions.slowMo` is parked behind the `PWSLOWMO` env var (currently
commented out in `playwright.config.ts`). Re-enable when you want to watch
the click stream:

```sh
PWSLOWMO=1000 npx playwright test --headed
```
