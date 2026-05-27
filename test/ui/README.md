# test/ui — OpenChoreo Backstage UI tests

Playwright tests that drive the Backstage portal end-to-end against a running
OpenChoreo cluster.

## Quickstart against the local `openchoreo-e2e` k3d cluster

1. Bring up the e2e cluster with Backstage turned on. The default
   `test/e2e/k3d/values-cp.yaml` keeps Backstage off — apply the
   `test/ui/k3d/values-cp-ui.yaml` overlay on top to flip it on:

   ```sh
   kubectl --context k3d-openchoreo-e2e -n openchoreo-control-plane \
     create secret generic backstage-secrets \
     --from-literal=backend-secret="$(head -c 32 /dev/urandom | base64)" \
     --from-literal=client-secret="backstage-portal-secret" \
     --from-literal=jenkins-api-key="placeholder-not-in-use"

   helm --kube-context k3d-openchoreo-e2e upgrade openchoreo-control-plane \
     install/helm/openchoreo-control-plane \
     --namespace openchoreo-control-plane \
     --values test/e2e/k3d/values-cp.yaml \
     --values test/ui/k3d/values-cp-ui.yaml
   ```

2. Install dependencies and run the suite:

   ```sh
   cd test/ui
   npm install
   npx playwright install --with-deps chromium
   npm test
   ```

The Playwright config maps the `*.e2e-cp.local` hostnames to `127.0.0.1` via
Chromium's `--host-resolver-rules`, so no `/etc/hosts` edit is needed.

## Layout

- `playwright.config.ts` — runner config.
- `specs/` — one folder per suite (`auth/`, `lifecycle/`, …).
- `po/` — page objects (intent-named methods, semantic locators).
- `fixtures/` — Playwright `test.extend` fixtures (auth state, kubectl helpers).
- `k3d/` — Helm value overlays that turn the e2e cluster into a UI-test target.

## Sign-in spec — pre-warmed popup pattern

`specs/auth/sign-in.spec.ts` signs in as `platform-engineer@openchoreo.dev`
(seeded by Thunder's `50-user-schema-and-users.sh`).

Backstage's OpenChoreo-auth provider opens the consent popup during the
initial `/api/auth/openchoreo-auth/refresh` probe — before the on-page
`Sign In` button renders. The spec arms `context.waitForEvent('page')`
*before* `page.goto('/')` and fills the form in that pre-warmed popup
directly.

**Do not click the on-page Sign In button.** It kicks off a second
`/oauth2/authorize` call against the same `flowId` / `authId`, which races
the warm popup for Thunder's SQLite store and fails non-deterministically
with `SQLITE_BUSY → server_error: Failed to process authorization request`.

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
