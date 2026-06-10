// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test, expect, storageStateFor } from '../../fixtures/auth';
import { kubectl, kApplyYAML, kDelete } from '../../fixtures/kube';
import { ProjectPO } from '../../po/project';
import { ComponentPO } from '../../po/component';
import { ReleasePO } from '../../po/release';
import { WorkloadConfigPO } from '../../po/workloadConfig';

// Pre-built greeter sample — same digest used across the e2e suite.
const PRE_BUILT_IMAGE =
  'ghcr.io/openchoreo/samples/greeter-service@sha256:5c67732c99ac3505dbab14c7ec92c33be57904420d62812694c64b56c5f92d40';
const ENDPOINT_PORT = 9090;
const NS = 'default';
const ts = Date.now().toString(36);
const PROJECT_NAME = `ui-ta-${ts}`;
const COMPONENT_NAME = `ta-comp-${ts}`;

// "web-application" ClusterComponentType allows exactly one ClusterTrait:
// "observability-alert-rule". A newly-scaffolded ClusterTrait does not appear
// in this dropdown until added to the ComponentType's allowedTraits — a
// separate PE operation. These tests use the platform-provided trait directly.
const TRAIT_OPTION_LABEL = 'observability-alert-rule (Cluster)';
const TRAIT_INSTANCE_NAME = 'observability-alert-rule-1';

// The observability-alert-rule ClusterTrait CEL validation requires a
// notification channel: either environmentConfigs.actions.notifications.channels
// is non-empty in the ReleaseBinding, or environment.defaultNotificationChannel
// is set. The controller resolves the default channel by finding an
// ObservabilityAlertsNotificationChannel where spec.isEnvDefault=true for the
// environment. We apply one here with a dummy webhook URL so the validation
// passes without requiring real infrastructure.
const NOTIF_CHANNEL_NAME = 'e2e-notif-dev';
const notifChannelYAML = `
apiVersion: openchoreo.dev/v1alpha1
kind: ObservabilityAlertsNotificationChannel
metadata:
  name: ${NOTIF_CHANNEL_NAME}
  namespace: ${NS}
spec:
  environment: development
  isEnvDefault: true
  type: webhook
  webhookConfig:
    url: https://example.com/webhook/e2e-alerts
    headers:
      Content-Type:
        value: "application/json"
`;

// ── kubectl helpers ────────────────────────────────────────────────────────

type ComponentRelease = {
  spec: {
    componentProfile?: {
      traits?: Array<{ kind: string; name: string; instanceName: string }>;
    };
  };
};

function getLatestRelease(): ComponentRelease | null {
  const result = kubectl(
    [
      'get', 'componentrelease',
      '-n', NS,
      '-l', `openchoreo.dev/component=${COMPONENT_NAME}`,
      '--sort-by=.metadata.creationTimestamp',
      '-o', 'json',
    ],
    { check: false },
  );
  // Surface kubectl failures so the poll fails with a real error rather than
  // silently returning [] (which would make the detach check false-pass).
  if (result.status !== 0) {
    throw new Error(`kubectl get componentrelease failed: ${result.stderr || result.stdout}`);
  }
  // JSON parse errors also propagate — only null for the genuine empty-list case.
  const list = JSON.parse(result.stdout) as { items: ComponentRelease[] };
  return list.items.at(-1) ?? null;
}

function getReleaseTraits(): Array<{ kind: string; name: string; instanceName: string }> {
  return getLatestRelease()?.spec?.componentProfile?.traits ?? [];
}

// ── Test suite ─────────────────────────────────────────────────────────────

test.describe.configure({ mode: 'serial' });

test.describe('pe-ops: trait attach / detach via workload-config Component tab', () => {
  test.beforeAll(async ({ mintAuthState }) => {
    // Apply a default notification channel for the development environment so
    // the observability-alert-rule CEL validation passes during deployment.
    // The controller resolves environment.defaultNotificationChannel from an
    // ObservabilityAlertsNotificationChannel with spec.isEnvDefault=true.
    kApplyYAML(notifChannelYAML);
    await mintAuthState('pe');
  });

  test.use({ storageState: storageStateFor('pe') });

  test.afterAll(async () => {
    kubectl(['delete', 'component', COMPONENT_NAME, '-n', NS, '--ignore-not-found', '--wait=false'], { check: false });
    kubectl(['delete', 'project', PROJECT_NAME, '-n', NS, '--ignore-not-found', '--wait=false'], { check: false });
    kDelete('observabilityalertsnotificationchannel', NOTIF_CHANNEL_NAME, NS);
  });

  // ── 1. Create project ───────────────────────────────────────────────────
  test('creates project via scaffolder and verifies kubectl', async ({ page }) => {
    test.setTimeout(120_000);

    await page.goto('/');
    await new ProjectPO(page).create({
      name: PROJECT_NAME,
      namespace: NS,
      displayName: PROJECT_NAME,
      description: 'UI trait-attach e2e test project',
      pipeline: 'default',
    });

    await expect
      .poll(
        () => kubectl(['get', 'project', PROJECT_NAME, '-n', NS], { check: false }).status,
        { timeout: 30_000 },
      )
      .toBe(0);
  });

  // ── 2. Create component (BYOI / Web Application) ───────────────────────
  test('creates Web Application component via scaffolder (BYOI) and verifies kubectl', async ({
    page,
  }) => {
    test.setTimeout(300_000);

    await page.goto('/');
    await new ComponentPO(page).create({
      name: COMPONENT_NAME,
      project: PROJECT_NAME,
      template: 'Web Application',
      image: PRE_BUILT_IMAGE,
      endpointPort: ENDPOINT_PORT,
      description: 'UI trait-attach e2e test component',
    });

    await expect
      .poll(
        () => kubectl(['get', 'component', COMPONENT_NAME, '-n', NS], { check: false }).status,
        { timeout: 30_000 },
      )
      .toBe(0);
  });

  // ── 3. Attach trait ────────────────────────────────────────────────────
  test('attaches ClusterTrait via workload-config Component tab and creates release', async ({
    page,
  }) => {
    test.setTimeout(120_000);

    const wc = new WorkloadConfigPO(page);
    await wc.open(COMPONENT_NAME);
    await wc.openComponentTab();

    await wc.addTrait(TRAIT_OPTION_LABEL, TRAIT_INSTANCE_NAME);
    await wc.saveAndCreateRelease();

    // Cross-check: the new ComponentRelease snapshot must include the trait.
    await expect
      .poll(() => getReleaseTraits(), { timeout: 30_000, intervals: [2_000] })
      .toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            kind: 'ClusterTrait',
            name: 'observability-alert-rule',
          }),
        ]),
      );
  });

  // ── 4. Deploy to development ───────────────────────────────────────────
  test('deploys latest release (with trait) to development environment', async ({
    page,
  }) => {
    test.setTimeout(300_000);

    await new ComponentPO(page).deployLatestRelease(COMPONENT_NAME, 'development');
  });

  // ── 5. Wait for active deployment ─────────────────────────────────────
  test('deployment with trait reaches Active state', async ({ page }) => {
    test.setTimeout(300_000);

    await new ReleasePO(page).expectActive(COMPONENT_NAME, 'development', 240_000);

    // Cross-check: ReleaseBinding reaches Ready=True.
    await expect
      .poll(
        () =>
          kubectl(
            [
              'get', 'releasebinding',
              '-n', NS,
              '-l', `openchoreo.dev/component=${COMPONENT_NAME}`,
              '-o', 'jsonpath={.items[*].status.conditions[?(@.type=="Ready")].status}',
            ],
            { check: false },
          ).stdout.includes('True'),
        { timeout: 240_000, intervals: [5_000] },
      )
      .toBe(true);
  });

  // ── 6. Detach trait ────────────────────────────────────────────────────
  test('detaches ClusterTrait via workload-config Component tab and verifies release', async ({
    page,
  }) => {
    test.setTimeout(120_000);

    const wc = new WorkloadConfigPO(page);
    await wc.open(COMPONENT_NAME);
    await wc.openComponentTab();

    await wc.removeTrait(TRAIT_INSTANCE_NAME);
    await wc.saveAndCreateRelease();

    await expect
      .poll(() => getReleaseTraits().length, { timeout: 30_000, intervals: [2_000] })
      .toBe(0);
  });
});
