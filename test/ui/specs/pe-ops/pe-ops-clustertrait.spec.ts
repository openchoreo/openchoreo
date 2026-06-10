// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { randomUUID } from 'node:crypto';

import { test, expect, storageStateFor } from '../../fixtures/auth';
import { kubectl, kNotFoundScoped } from '../../fixtures/kube';
import { CreatePO } from '../../po/create';
import { DeletePO } from '../../po/delete';

// Combine a millisecond timestamp with a UUID fragment so concurrent CI runs
// on the same cluster do not produce AlreadyExists collisions.
const ts = `${Date.now().toString(36)}-${randomUUID().slice(0, 8)}`;
// ClusterTrait names must be valid Kubernetes resource names.
const CLUSTER_TRAIT_NAME = `ui-ct-${ts}`;

// ── Test suite ─────────────────────────────────────────────────────────────

test.describe.configure({ mode: 'serial' });

test.describe('pe-ops: ClusterTrait lifecycle via scaffolder', () => {
  test.beforeAll(async ({ mintAuthState }) => {
    await mintAuthState('pe');
  });

  test.use({ storageState: storageStateFor('pe') });

  test.afterAll(async () => {
    // Best-effort fallback in case the UI deletion test did not run.
    // ClusterTrait is cluster-scoped — omit -n flag.
    kubectl(['delete', 'clustertrait', CLUSTER_TRAIT_NAME, '--ignore-not-found', '--wait=false'], { check: false });
  });

  // ── 1. Create ClusterTrait via scaffolder ───────────────────────────────
  test('creates ClusterTrait via scaffolder and verifies kubectl', async ({ page }) => {
    test.setTimeout(120_000);

    await page.goto('/');
    await new CreatePO(page).chooseTemplate('ClusterTrait');

    // ── Step 1: ClusterTrait Metadata ──
    await page
      .getByLabel('ClusterTrait Name', { exact: false })
      .waitFor({ state: 'visible', timeout: 30_000 });
    await page.getByLabel('ClusterTrait Name', { exact: false }).fill(CLUSTER_TRAIT_NAME);
    await page.getByLabel('Display Name', { exact: false }).fill(CLUSTER_TRAIT_NAME);
    await page.getByLabel('Description', { exact: false }).fill('UI e2e test ClusterTrait');

    // Advance through wizard (Next → Review → Create).
    for (let i = 0; ; i++) {
      expect(i, 'scaffolder wizard did not reach Create within 8 steps').toBeLessThan(8);
      const advance = page
        .getByRole('button', { name: /^(Next|Review|Create)$/ })
        .first();
      await advance.waitFor({ state: 'visible', timeout: 15_000 });
      const label = (await advance.textContent())?.trim();
      await expect(advance).toBeEnabled({ timeout: 15_000 });
      await advance.click();
      if (label === 'Create') break;
    }

    // Wait for the scaffolder task to complete (View link appears).
    await expect(
      page.getByRole('button', { name: /view.*clustertrait/i }).first(),
    ).toBeVisible({ timeout: 60_000 });

    // Cross-check: ClusterTrait CR must exist in the cluster.
    await expect
      .poll(
        () =>
          kubectl(
            ['get', 'clustertrait', CLUSTER_TRAIT_NAME, '--ignore-not-found', '-o', 'name'],
            { check: false },
          ).stdout.trim(),
        { timeout: 30_000, intervals: [2_000] },
      )
      .toBe(`clustertrait.openchoreo.dev/${CLUSTER_TRAIT_NAME}`);
  });

  // ── 2. View ClusterTrait in catalog ────────────────────────────────────
  test('ClusterTrait entity appears in catalog under openchoreo-cluster namespace', async ({
    page,
  }) => {
    test.setTimeout(120_000);

    // The scaffolder action registers ClusterTraits as kind=ClusterTraitType in
    // the "openchoreo-cluster" namespace (entityRef format:
    // clustertraittype:openchoreo-cluster/{name}).
    // Poll until the catalog provider syncs the new entity.
    await expect
      .poll(
        async () => {
          await page.goto(
            `/catalog/openchoreo-cluster/clustertraittype/${CLUSTER_TRAIT_NAME}`,
          );
          return page
            .getByRole('heading', { name: CLUSTER_TRAIT_NAME })
            .first()
            .isVisible({ timeout: 5_000 })
            .catch(() => false);
        },
        { timeout: 120_000, intervals: [5_000] },
      )
      .toBe(true);
  });

  // ── 3. Delete ClusterTrait via UI ──────────────────────────────────────
  test('deletes ClusterTrait via catalog overflow menu and verifies kubectl', async ({
    page,
  }) => {
    test.setTimeout(120_000);

    // Navigate to the entity page (already confirmed to exist in test 2).
    await page.goto(
      `/catalog/openchoreo-cluster/clustertraittype/${CLUSTER_TRAIT_NAME}`,
    );
    await page
      .getByRole('heading', { name: CLUSTER_TRAIT_NAME })
      .first()
      .waitFor({ state: 'visible', timeout: 30_000 });

    // Delete via the entity overflow menu. The menu item rendered by
    // Backstage EntityContextMenu reads "Delete Cluster Trait Type" for
    // ClusterTraitType entities (useDeleteEntityMenuItems displayTitle).
    const del = new DeletePO(page);
    await del.openOverflowAndDelete('Cluster Trait Type');
    await del.confirm();

    // Cross-check: the ClusterTrait CR must be gone from the cluster.
    await expect
      .poll(
        () => kNotFoundScoped('clustertrait', CLUSTER_TRAIT_NAME, ''),
        { timeout: 60_000, intervals: [3_000] },
      )
      .toBe(true);
  });
});
