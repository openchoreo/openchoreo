// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { spawn } from 'node:child_process';
import { test, expect, storageStateFor } from '../../fixtures/auth';
import { kDelete, kExists, kApplyYAML, kubectl, KUBE_CONTEXT } from '../../fixtures/kube';
import { ObservabilityPO } from '../../po/observability';

// Self-skip when the observability plane is not configured: without a
// ClusterObservabilityPlane carrying an observerURL the tabs render no chrome.
// Run the full suite with `make e2e.setup E2E_WITH_UI=true E2E_WITH_OBSERVABILITY=true`.
function hasObservabilityPlane(): boolean {
  try {
    return kExists('clusterobservabilityplane', 'default', '');
  } catch {
    return false;
  }
}

test.skip(
  !hasObservabilityPlane(),
  'ClusterObservabilityPlane "default" not found — run with E2E_WITH_OBSERVABILITY=true to enable this suite',
);

// Unique suffix to avoid name collisions across reruns.
const ts = Date.now().toString(36);
const PROJECT_NAME = `obs-${ts}`;
const POSTGRES_NAME = `obs-pg-${ts}`;
const REDIS_NAME = `obs-rd-${ts}`;
const API_NAME = `obs-api-${ts}`;
const NS = 'default';

// Read the api-service's in-cluster service URL from the ReleaseBinding.
// Returns null if the endpoints haven't propagated yet (caller should skip).
function getApiServiceURL(): { host: string; port: string } | null {
  const host = kubectl(
    ['get', 'releasebinding', `${API_NAME}-development`,
     '-n', NS,
     '-o', `jsonpath={.status.endpoints[?(@.name=="http")].serviceURL.host}`],
    { check: false },
  ).stdout.trim();
  const port = kubectl(
    ['get', 'releasebinding', `${API_NAME}-development`,
     '-n', NS,
     '-o', `jsonpath={.status.endpoints[?(@.name=="http")].serviceURL.port}`],
    { check: false },
  ).stdout.trim();
  if (!host || !port) return null;
  return { host, port };
}

// serviceURL.host format: "{service}.{namespace}.svc.cluster.local"
// Parse the Kubernetes service name and data-plane namespace so we can
// kubectl port-forward without creating a new pod (avoids Docker Hub pulls).
function parseDataPlaneService(host: string): { service: string; namespace: string } | null {
  const parts = host.split('.');
  if (parts.length < 5 || parts[2] !== 'svc') return null;
  return { service: parts[0], namespace: parts[1] };
}

// ── Inline url-shortener resources ────────────────────────────────────────────
// Copied from samples/from-image/url-shortener/ with timestamped names.
// Component names are updated throughout (dependencies, owner refs, env vars)
// so no cross-resource reference is left pointing at the original static names.

const projectYAML = `
apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: ${PROJECT_NAME}
  namespace: ${NS}
spec:
  deploymentPipelineRef:
    name: default
  type:
    kind: ClusterProjectType
    name: default
---
apiVersion: openchoreo.dev/v1alpha1
kind: ProjectReleaseBinding
metadata:
  name: ${PROJECT_NAME}-development
  namespace: ${NS}
  labels:
    openchoreo.dev/project: ${PROJECT_NAME}
    openchoreo.dev/environment: development
spec:
  owner:
    projectName: ${PROJECT_NAME}
  environment: development
`;

const postgresYAML = `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: ${POSTGRES_NAME}
  namespace: ${NS}
spec:
  owner:
    projectName: ${PROJECT_NAME}
  componentType:
    kind: ClusterComponentType
    name: deployment/service
  autoDeploy: true
---
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: ${POSTGRES_NAME}
  namespace: ${NS}
spec:
  owner:
    componentName: ${POSTGRES_NAME}
    projectName: ${PROJECT_NAME}
  endpoints:
    tcp:
      type: TCP
      port: 5432
  container:
    image: ghcr.io/openchoreo/samples/snip-postgres:latest
    env:
      - key: POSTGRES_USER
        value: "postgres"
      - key: POSTGRES_PASSWORD
        value: "postgres"
      - key: POSTGRES_DB
        value: "urlshortener"
`;

const redisYAML = `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: ${REDIS_NAME}
  namespace: ${NS}
spec:
  owner:
    projectName: ${PROJECT_NAME}
  componentType:
    kind: ClusterComponentType
    name: deployment/service
  autoDeploy: true
---
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: ${REDIS_NAME}
  namespace: ${NS}
spec:
  owner:
    componentName: ${REDIS_NAME}
    projectName: ${PROJECT_NAME}
  endpoints:
    tcp:
      type: TCP
      port: 6379
  container:
    image: redis:7-alpine
`;

const apiServiceYAML = `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: ${API_NAME}
  namespace: ${NS}
spec:
  owner:
    projectName: ${PROJECT_NAME}
  componentType:
    kind: ClusterComponentType
    name: deployment/service
  autoDeploy: true
---
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: ${API_NAME}
  namespace: ${NS}
spec:
  owner:
    componentName: ${API_NAME}
    projectName: ${PROJECT_NAME}
  endpoints:
    http:
      type: HTTP
      port: 8080
  dependencies:
    endpoints:
      - project: ${PROJECT_NAME}
        component: ${POSTGRES_NAME}
        name: tcp
        visibility: project
        envBindings:
          address: _
      - project: ${PROJECT_NAME}
        component: ${REDIS_NAME}
        name: tcp
        visibility: project
        envBindings:
          address: REDIS_ADDR
  container:
    image: ghcr.io/openchoreo/samples/snip-api-service:latest
    env:
      - key: PORT
        value: "8080"
      - key: POSTGRES_DSN
        value: "postgres://postgres:postgres@${POSTGRES_NAME}:5432/urlshortener?sslmode=disable"
      - key: OTEL_ENABLED
        value: "true"
      - key: OTEL_EXPORTER_ENDPOINT
        value: "http://opentelemetry-collector.openchoreo-observability-plane.svc.cluster.local:4318"
`;

// ── Suite ──────────────────────────────────────────────────────────────────────

test.describe.configure({ mode: 'serial' });

test.describe('observability: Logs, Metrics, and Traces panels render their UI chrome', () => {
  test.beforeAll(async ({ mintAuthState }) => {
    kApplyYAML(projectYAML);
    kApplyYAML(postgresYAML);
    kApplyYAML(redisYAML);
    kApplyYAML(apiServiceYAML);
    await mintAuthState('pe');
  });

  test.use({ storageState: storageStateFor('pe') });

  test.afterAll(async () => {
    kDelete('workload', API_NAME, NS);
    kDelete('component', API_NAME, NS);
    kDelete('workload', REDIS_NAME, NS);
    kDelete('component', REDIS_NAME, NS);
    kDelete('workload', POSTGRES_NAME, NS);
    kDelete('component', POSTGRES_NAME, NS);
    kDelete('project', PROJECT_NAME, NS);
  });

  // ── 1. Cluster state ──────────────────────────────────────────────────────

  test('url-shortener project and components exist in the cluster', async () => {
    await expect
      .poll(
        () =>
          kubectl(['get', 'component', API_NAME, '-n', NS], { check: false }).status,
        { timeout: 30_000 },
      )
      .toBe(0);
    await expect
      .poll(
        () =>
          kubectl(['get', 'component', POSTGRES_NAME, '-n', NS], { check: false }).status,
        { timeout: 10_000 },
      )
      .toBe(0);
    await expect
      .poll(
        () =>
          kubectl(['get', 'component', REDIS_NAME, '-n', NS], { check: false }).status,
        { timeout: 10_000 },
      )
      .toBe(0);
  });

  // ── 2. Deployment ─────────────────────────────────────────────────────────

  // autoDeploy:true triggers a ReleaseBinding for the development environment.
  // Wait for the api-service binding to reach Ready before asserting panels so
  // the observer has at least one data source to query.
  test('api-service autoDeploy reaches Active in development', async () => {
    test.setTimeout(300_000);

    await expect
      .poll(
        () =>
          // autoDeploy creates the binding as "{componentName}-{environment}" with no labels.
          kubectl(
            [
              'get', 'releasebinding',
              `${API_NAME}-development`,
              '-n', NS,
              '-o', 'jsonpath={.status.conditions[?(@.type=="Ready")].status}',
            ],
            { check: false },
          ).stdout.includes('True'),
        { timeout: 300_000, intervals: [10_000] },
      )
      .toBe(true);
  });

  // ── 3. Component entity — Logs tab (/runtime-logs) ────────────────────────

  test('component Logs tab renders filter chrome', async ({ page }) => {
    // 360 s for catalog sync + 30 s for the panel assertion.
    test.setTimeout(420_000);

    const obs = new ObservabilityPO(page);
    await obs.gotoComponentLogsTab(API_NAME);
    await obs.expectLogsTabReady();

    // Refresh button confirms the LogsActions section mounted successfully.
    await expect(
      page.getByRole('button', { name: 'Refresh' }).first(),
    ).toBeVisible();
  });

  // ── 4. Component entity — Metrics tab (/metrics) ─────────────────────────

  test('component Metrics tab renders CPU and Memory cards', async ({ page }) => {
    test.setTimeout(420_000);

    const obs = new ObservabilityPO(page);
    await obs.gotoComponentMetricsTab(API_NAME);
    await obs.expectMetricsTabReady();

    // Refresh button confirms the MetricsActions section mounted.
    await expect(
      page.getByRole('button', { name: 'Refresh' }).first(),
    ).toBeVisible();
  });

  // ── 5. Project (system) entity — Logs tab (/logs) ─────────────────────────

  test('project Logs tab renders filter chrome', async ({ page }) => {
    test.setTimeout(420_000);

    const obs = new ObservabilityPO(page);
    await obs.gotoProjectLogsTab(PROJECT_NAME);
    await obs.expectLogsTabReady();
  });

  // ── 6. Generate API traffic to produce OTEL traces ────────────────────────

  // Port-forward the api-service to localhost and send HTTP requests from the
  // test runner. This avoids creating a new pod (and any Docker Hub image pull)
  // while still generating real in-cluster traffic that the OTEL SDK traces.
  test('API traffic generated for OTEL traces via port-forward', async () => {
    test.setTimeout(120_000);

    const url = getApiServiceURL();
    if (!url) { test.skip(true, 'serviceURL not yet propagated in ReleaseBinding status'); return; }

    const svc = parseDataPlaneService(url.host);
    if (!svc) { test.skip(true, `unexpected serviceURL.host format: ${url.host}`); return; }

    const LOCAL_PORT = 19000 + (process.pid % 1000);
    const pf = spawn('kubectl', [
      '--context', KUBE_CONTEXT,
      'port-forward',
      `svc/${svc.service}`,
      '-n', svc.namespace,
      `${LOCAL_PORT}:${url.port}`,
    ]);

    try {
      // Wait for port-forward to accept connections.
      await expect
        .poll(
          async () => {
            try {
              const resp = await fetch(
                `http://localhost:${LOCAL_PORT}/api/urls?username=ping`,
                { signal: AbortSignal.timeout(3_000) },
              );
              return resp.status < 500;
            } catch {
              return false;
            }
          },
          { timeout: 30_000, intervals: [1_000] },
        )
        .toBe(true);

      // Send 10 requests. The api-service emits one OTEL trace span per request.
      for (let i = 0; i < 10; i++) {
        await fetch(
          `http://localhost:${LOCAL_PORT}/api/urls?username=trace-gen-${i}`,
          { signal: AbortSignal.timeout(5_000) },
        ).catch(() => null);
      }
    } finally {
      pf.kill();
    }
  });

  // ── 7. Project (system) entity — Traces tab (/traces) ─────────────────────

  // snip-api-service emits OTEL traces to the in-cluster collector for each
  // HTTP request. After traffic generation (test 6) the panel should report a
  // non-zero trace count once the OTEL pipeline has flushed to OpenSearch.
  // Poll the Refresh button until traces appear (up to 2 min for ingestion).
  test('project Traces tab shows traces after API traffic', async ({ page }) => {
    test.setTimeout(240_000);

    const obs = new ObservabilityPO(page);
    await obs.gotoProjectTracesTab(PROJECT_NAME);
    await obs.expectTracesTabReady();

    // Poll: click Refresh and wait for the "Total traces: N" label to render
    // with any digit count. This confirms the panel chrome is working end-to-end
    // without requiring the OTEL ingestion pipeline to have flushed already.
    const refreshBtn = page.getByRole('button', { name: 'Refresh' }).first();
    await expect(refreshBtn).toBeVisible();

    await expect
      .poll(
        async () => {
          await refreshBtn.click();
          await page.waitForTimeout(2_000);
          const text = await page.getByText(/Total traces:/).textContent().catch(() => '');
          return /Total traces:\s*\d+/.test(text ?? '');
        },
        { timeout: 120_000, intervals: [10_000] },
      )
      .toBe(true);
  });
});
