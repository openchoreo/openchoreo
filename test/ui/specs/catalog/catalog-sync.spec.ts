// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test, expect, storageStateFor } from '../../fixtures/auth';
import { kApplyYAML, kDelete } from '../../fixtures/kube';
import { CatalogTablePO } from '../../po/catalogTable';

const ts = Date.now().toString(36);
const PROJECT_NAME = `ui-catsync-${ts}`;
const COMPONENT_NAME = `kubectl-component-${ts}`;
const PRE_BUILT_IMAGE = 'ghcr.io/openchoreo/sample-greeter:latest';

// Both the Project and its Components live in the same OpenChoreo API
// namespace (`default` here). The OpenChoreo catalog provider lists
// components via GET /api/v1/namespaces/{projectNamespace}/components?project=
// (catalog-backend-module-openchoreo OpenChoreoEntityProvider), so a
// kubectl-seeded Component must sit in the project's namespace with
// spec.owner.projectName — NOT in a separate per-project namespace, or the
// provider's query never returns it.
const seedYAML = `
apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: ${PROJECT_NAME}
  namespace: default
spec:
  deploymentPipelineRef:
    name: default
---
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: ${COMPONENT_NAME}
  namespace: default
spec:
  owner:
    projectName: ${PROJECT_NAME}
  componentType:
    kind: ComponentType
    name: deployment/web-app
  parameters:
    image: ${PRE_BUILT_IMAGE}
`;

test.describe('catalog-sync: kubectl-applied component appears in Backstage catalog', () => {
  test.beforeAll(async ({ mintAuthState }) => {
    await mintAuthState('pe');
  });
  test.use({ storageState: storageStateFor('pe') });

  test.afterAll(async () => {
    kDelete('component', COMPONENT_NAME, 'default');
    kDelete('project', PROJECT_NAME, 'default');
  });

  test('component applied via kubectl shows up after catalog refresh', async ({
    page,
  }) => {
    // The OpenChoreo catalog provider syncs on a scheduled poll whose default
    // frequency is 300s (catalog-backend-module-openchoreo/src/module.ts:96).
    // A kubectl-created component only appears after the next cycle, so the
    // poll has to span a full interval plus slack — and the test timeout must
    // exceed the poll. Lower `openchoreo.schedule.frequency` in app-config to
    // make this faster in a dedicated UI-test install.
    test.setTimeout(420_000);
    kApplyYAML(seedYAML);

    const catalog = new CatalogTablePO(page);

    // Open the catalog and filter to Components via the Kind picker.
    await catalog.openKind('component');

    // Reload on each attempt to re-query until the kubectl-created row appears
    // — give it longer than one provider cycle (300s) plus slack.
    await expect
      .poll(
        async () => {
          await catalog.reload();
          return page
            .getByRole('link', { name: COMPONENT_NAME, exact: true })
            .count();
        },
        { timeout: 360_000, intervals: [10_000, 15_000, 30_000] },
      )
      .toBeGreaterThan(0);

    await catalog.openByName(COMPONENT_NAME);
    await expect(
      page.getByRole('heading', { name: COMPONENT_NAME, exact: false }).first(),
    ).toBeVisible();
  });
});
