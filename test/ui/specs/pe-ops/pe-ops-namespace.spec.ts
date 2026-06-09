// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test, expect, storageStateFor } from '../../fixtures/auth';
import { kubectl, kDelete } from '../../fixtures/kube';
import { NamespacePO } from '../../po/namespace';
import { DeletePO } from '../../po/delete';
import { CatalogTablePO } from '../../po/catalogTable';
import { ProjectPO } from '../../po/project';

// Namespace names must be valid Kubernetes namespace names: lowercase
// alphanumeric + '-', no leading/trailing hyphens, max 63 characters.
// Date.now().toString(36) produces ~8 lowercase alphanumeric characters.
const ts = Date.now().toString(36);
const NAMESPACE_NAME = `ui-ns-${ts}`;
const PROJECT_NAME = `ui-proj-${ts}`;

// Helpers for cluster-scoped kubectl operations. kGetJSON and kNotFound both
// inject `-n <namespace>` unconditionally, which is harmless for Kubernetes
// namespaces (kubectl ignores -n on cluster-scoped resources) but less clear —
// so use kubectl() directly here to make the cluster-scoped intent explicit.
function namespaceExists(): boolean {
  const r = kubectl(
    ['get', 'namespace', NAMESPACE_NAME, '--ignore-not-found', '-o', 'name'],
    { check: false },
  );
  return r.stdout.trim() !== '';
}

function namespaceLabel(): string {
  const r = kubectl(
    [
      'get',
      'namespace',
      NAMESPACE_NAME,
      '-o',
      'jsonpath={.metadata.labels.openchoreo\\.dev/control-plane}',
    ],
    { check: false },
  );
  return r.stdout.trim();
}

test.describe.configure({ mode: 'serial' });

test.describe('pe-ops: Namespace lifecycle through the Backstage UI', () => {
  test.beforeAll(async ({ mintAuthState }) => {
    await mintAuthState('pe');
  });

  test.use({ storageState: storageStateFor('pe') });

  test.afterAll(async () => {
    // Best-effort teardown. namespace="" omits the -n flag for the
    // cluster-scoped Kubernetes namespace resource.
    kDelete('namespace', NAMESPACE_NAME, '');
    // Clean up the project created in the View test (namespaced resource).
    kDelete('project', PROJECT_NAME, NAMESPACE_NAME);
  });

  // ── 1. Create ─────────────────────────────────────────────────────────────
  test('creates namespace via scaffolder and verifies Kubernetes namespace', async ({
    page,
  }) => {
    // Budget covers a full catalog-sync cycle in case the entity-wait step
    // below needs to retry across a default 300s provider poll interval.
    test.setTimeout(600_000);

    await page.goto('/');
    await new NamespacePO(page).create({
      name: NAMESPACE_NAME,
      displayName: NAMESPACE_NAME,
      description: 'Created to test namespace management as a part of UI e2e test',
    });

    // The scaffolder task page shows a "View" affordance once the action
    // completes. Wait for it as the success signal before cross-checking kubectl.
    await expect(
      page.getByRole('button', { name: /view.*namespace/i }).first(),
    ).toBeVisible({ timeout: 60_000 });

    // Cross-check: Kubernetes namespace was created.
    await expect.poll(namespaceExists, { timeout: 30_000 }).toBe(true);

    // Cross-check: the control-plane label is present — the OpenChoreo catalog
    // provider only ingests namespaces that carry this label.
    expect(namespaceLabel()).toBe('true');
  });

  // ── 2. View ───────────────────────────────────────────────────────────────
  test('namespace appears as a Domain entity in the catalog with a project listed', async ({
    page,
  }) => {
    // The catalog provider ingests Kubernetes namespaces with the
    // openchoreo.dev/control-plane label as Backstage Domain entities
    // (kind=domain, displayed as "Namespace" in the Kind picker). Syncing is
    // eventually consistent — size the timeout to survive a full default
    // provider cycle (300s) on installs without the UI-test values overlay.
    // Budget covers two sequential reload-to-repoll waits (namespace + project).
    test.setTimeout(900_000);

    // Create a project under the namespace so the "Has Projects" relation
    // list is non-empty and the listing can be verified.
    await page.goto('/');
    await new ProjectPO(page).create({
      name: PROJECT_NAME,
      displayName: PROJECT_NAME,
      description: 'Created to verify project listing on namespace entity page',
    });
    await expect(
      page.getByRole('button', { name: /view.*project/i }).first(),
    ).toBeVisible({ timeout: 60_000 });

    const catalog = new CatalogTablePO(page);

    // openEntity navigates via the URL-based gotoKind so the Kind filter
    // survives reload-to-repoll, then clicks the row once it appears.
    await catalog.openEntity('domain', NAMESPACE_NAME, 360_000);

    // Assert the entity page renders with the correct heading.
    await expect(
      page.getByRole('heading', { name: NAMESPACE_NAME }).first(),
    ).toBeVisible({ timeout: 15_000 });

    // Assert the "Has Projects" list section is rendered.
    await expect(
      page.getByRole('heading', { name: /has projects/i }).first(),
    ).toBeVisible({ timeout: 15_000 });

    // Assert the created project is listed under "Has Projects". The relation
    // table isn't auto-refetched, so reload to re-query until the project syncs.
    await expect
      .poll(
        async () => {
          const link = page.getByRole('link', { name: PROJECT_NAME }).first();
          if (await link.isVisible({ timeout: 3_000 }).catch(() => false)) {
            return true;
          }
          await page.reload({ waitUntil: 'domcontentloaded' });
          return false;
        },
        { timeout: 360_000, intervals: [5_000] },
      )
      .toBe(true);

    // Assert the Create Project button exists on the entity page.
    await expect(
      page.getByRole('button', { name: /create project/i }).first(),
    ).toBeVisible({ timeout: 15_000 });
  });

  // ── 3. Delete ─────────────────────────────────────────────────────────────
  test('deletes namespace via UI overflow menu and verifies Kubernetes namespace is gone', async ({
    page,
  }) => {
    test.setTimeout(120_000);

    const catalog = new CatalogTablePO(page);
    const del = new DeletePO(page);

    // Re-open the entity page (test isolation — each test starts with a fresh
    // page context from the stored auth state, not the previous test's page).
    await catalog.openEntity('domain', NAMESPACE_NAME, 60_000);

    // Delete via the entity overflow ("more") menu. The menu item rendered by
    // Backstage EntityContextMenu reads "Delete Namespace" for Domain entities
    // whose displayTitle is "Namespace".
    await del.openOverflowAndDelete('Namespace');
    await del.confirm();

    // Cross-check: the Kubernetes namespace must be gone (or terminating).
    await expect
      .poll(
        () =>
          kubectl(
            ['get', 'namespace', NAMESPACE_NAME, '--ignore-not-found', '-o', 'name'],
            { check: false },
          ).stdout.trim(),
        { timeout: 60_000, intervals: [3_000] },
      )
      .toBe('');
  });
});
