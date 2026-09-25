// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test, expect, storageStateFor, ROLES } from '../../fixtures/auth';
import { kDelete, kExists, kNotFound, kubectl } from '../../fixtures/kube';
import { AuditLogsPO } from '../../po/auditLogs';
import { CatalogTablePO } from '../../po/catalogTable';
import { DeletePO } from '../../po/delete';
import { ProjectPO } from '../../po/project';

// Audit records are routed and stored by the observability plane, and the
// e2e setup only enables audit on openchoreo-api, the observer and the logs
// module when that plane is installed (make/e2e.mk). Run the full suite with
// `make e2e.setup E2E_WITH_UI=true E2E_WITH_OBSERVABILITY=true`.
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

const ts = Date.now().toString(36);
const PROJECT_NAME = `ui-audit-${ts}`;
const NS = 'default';
const PE = ROLES.pe.username;

test.describe.configure({ mode: 'serial' });

test.describe('audit: portal actions publish audit records the Audit Logs page shows', () => {
  test.beforeAll(async ({ mintAuthState }) => {
    await mintAuthState('pe');
  });

  test.use({ storageState: storageStateFor('pe') });

  test.afterAll(async () => {
    kDelete('project', PROJECT_NAME, NS);
  });

  test('Audit Logs page renders its query chrome', async ({ page }) => {
    const audit = new AuditLogsPO(page);
    await audit.goto();
    await audit.expectReady();

    await expect(page.getByLabel('Time Range')).toHaveValue('Last 7 days');
    await expect(
      page.getByRole('button', { name: 'Oldest First', exact: true }),
    ).toBeVisible();
    for (const column of ['Time', 'Actor', 'Action', 'Resource', 'Result']) {
      await expect(
        audit.grid.getByRole('columnheader', { name: column, exact: true }),
      ).toBeVisible();
    }
  });

  test('creating a project in the portal publishes create_project as the signed-in user', async ({
    page,
  }) => {
    // Scaffolder create + ingestion through Fluent Bit and OpenSearch.
    test.setTimeout(360_000);

    await page.goto('/');
    await new ProjectPO(page).create({
      name: PROJECT_NAME,
      namespace: NS,
      displayName: PROJECT_NAME,
      description: 'Audit logs UI spec',
    });
    await expect
      .poll(
        () => kubectl(['get', 'project', PROJECT_NAME, '-n', NS], { check: false }).status,
        { timeout: 30_000 },
      )
      .toBe(0);

    const audit = new AuditLogsPO(page);
    await audit.goto({
      filters: { action: 'create_project', 'resource.name': PROJECT_NAME },
      timeRange: '1h',
      sort: 'desc',
    });
    await audit.expectReady();

    // The scaffolder forwards the portal user's token, so the actor must be
    // the human who clicked Create, not a Backstage service account.
    const row = await audit.waitForRow(PROJECT_NAME);
    await expect(row).toContainText(PE);
    await expect(row).toContainText('create_project');
    await expect(row).toContainText('success');
    await expect(row).toContainText('rest');
    await expect(audit.grid.getByRole('row').filter({ hasText: PROJECT_NAME })).toHaveCount(1);
  });

  test('record drawer shows the published create_project record and filters by its values', async ({
    page,
  }) => {
    test.setTimeout(240_000);

    const audit = new AuditLogsPO(page);
    await audit.goto({
      filters: { action: 'create_project', 'resource.name': PROJECT_NAME },
      timeRange: '1h',
      sort: 'desc',
    });
    const row = await audit.waitForRow(PROJECT_NAME);
    await audit.openRecord(row);

    const drawer = audit.drawer;
    await expect(drawer.getByText('via the REST API')).toBeVisible();
    await expect(drawer.getByRole('button', { name: PE, exact: true })).toBeVisible();
    await expect(drawer.getByRole('button', { name: 'management', exact: true })).toBeVisible();
    await expect(
      drawer.getByRole('button', { name: 'openchoreo-api', exact: true }),
    ).toBeVisible();
    await expect(drawer.getByText(`/namespaces/${NS}/projects`).first()).toBeVisible();

    // Drilling into the actor adds it to the query alongside the deep-linked
    // filters, and the record still matches.
    await audit.filterFromRecord(PE);
    await audit.expectFilterChip('actor.id', PE);
    await audit.expectFilterChip('action', 'create_project');
    await expect(audit.rowContaining(PROJECT_NAME)).toHaveCount(1, { timeout: 30_000 });
  });

  test('deleting the project in the portal publishes delete_project', async ({ page }) => {
    test.setTimeout(360_000);

    await new CatalogTablePO(page).openEntity('system', PROJECT_NAME);
    const del = new DeletePO(page);
    await del.openOverflowAndDelete('Project');
    await del.confirm();
    await expect
      .poll(() => kNotFound('project', PROJECT_NAME, NS), { timeout: 60_000 })
      .toBe(true);

    const audit = new AuditLogsPO(page);
    await audit.goto({
      filters: { action: 'delete_project', 'resource.name': PROJECT_NAME },
      timeRange: '1h',
      sort: 'desc',
    });
    const row = await audit.waitForRow(PROJECT_NAME);
    await expect(row).toContainText(PE);
    await expect(row).toContainText('success');
  });
});

test.describe('audit: roles without auditlogs:view', () => {
  test.beforeAll(async ({ mintAuthState }) => {
    await mintAuthState('dev');
  });

  test.use({ storageState: storageStateFor('dev') });

  test('developer sees Insufficient Permissions on the Audit Logs page', async ({ page }) => {
    const audit = new AuditLogsPO(page);
    await audit.goto();
    await expect(
      page.getByRole('heading', { name: 'Audit Logs', exact: true }),
    ).toBeVisible({ timeout: 30_000 });
    await audit.expectForbidden();
  });
});
