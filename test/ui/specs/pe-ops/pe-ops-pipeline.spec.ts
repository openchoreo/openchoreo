// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test, expect, storageStateFor } from '../../fixtures/auth';
import { kGetJSON, kNotFound, kDelete, kubectl } from '../../fixtures/kube';
import { DeletePO } from '../../po/delete';
import { CreatePO } from '../../po/create';
import { CatalogTablePO } from '../../po/catalogTable';

// PE CRUD across the Phase 1 CRD families. The plan called for five kinds
// (ComponentType, Trait, Workflow, DeploymentPipeline, Environment) but the
// real Backstage UI gates each behind a Scaffolder template with a bespoke
// form shape:
//
//   - ComponentType / Trait / Workflow → name + description + YAML editor
//   - DeploymentPipeline / Environment → single FormWithYaml field
//
// Driving the YAML editor (Monaco) reliably from Playwright is out of Phase 1
// scope. The spec therefore exercises the two families that have plain name +
// description fields and leaves a YAML default — full CRUD across all five
// kinds lands in Phase 2 alongside the YAML-editor selector contract.
interface CrdRow {
  kind: string; // kubectl Kind
  catalogKind: string; // Backstage catalog kind (for the Kind picker)
  cardTitle: string; // Create-page template card title ("Use template <this>")
  formNameLabel: string; // accessible label for the name field
  kindDisplayName: string; // "Delete ${this}" menu item label
  name: string; // generated resource name
  description: string;
}

const ts = Date.now().toString(36);

const rows: CrdRow[] = [
  {
    kind: 'componenttype',
    catalogKind: 'componenttype',
    cardTitle: 'ComponentType',
    formNameLabel: 'ComponentType Name',
    kindDisplayName: 'Component Type',
    name: `peops-ct-${ts}`,
    description: 'tier5 ui spec component type',
  },
  {
    kind: 'trait',
    // The catalog ingests Trait CRs as kind "TraitType" (kubectl kind and
    // catalog kind diverge for traits only).
    catalogKind: 'traittype',
    cardTitle: 'Trait',
    formNameLabel: 'Trait Name',
    kindDisplayName: 'Trait Type',
    name: `peops-trait-${ts}`,
    description: 'tier5 ui spec trait',
  },
];

test.describe.configure({ mode: 'serial' });

test.describe('pe-ops: PE CRUD through the Scaffolder templates', () => {
  test.beforeAll(async ({ mintAuthState }) => {
    await mintAuthState('pe');
  });
  test.use({ storageState: storageStateFor('pe') });

  test.afterAll(async () => {
    for (const row of rows) kDelete(row.kind, row.name, 'default');
  });

  for (const row of rows) {
    test(`${row.kind}: create via scaffolder → kubectl shape → delete`, async ({
      page,
    }) => {
      // Budget for a full default catalog-sync cycle (300s) plus the create
      // and delete flows around it — same pattern as full-lifecycle.
      test.setTimeout(600_000);
      const del = new DeletePO(page);
      const catalog = new CatalogTablePO(page);

      await page.goto('/');
      await new CreatePO(page).chooseTemplate(row.cardTitle);

      await page
        .getByLabel(row.formNameLabel, { exact: false })
        .fill(row.name);
      await page
        .getByLabel('Description', { exact: false })
        .fill(row.description);

      // Walk the wizard to submission. The advance label differs per step
      // (Next → … → Review → Create), a slow step-render can race a
      // fixed label sequence, and a click can be swallowed while the step
      // is still validating — so re-resolve whatever advance affordance is
      // present and re-click until Create has been pressed. (isVisible's
      // timeout arg is a no-op, so a fixed label walk silently skips steps.)
      for (let i = 0; ; i++) {
        expect(i, 'wizard did not reach Create').toBeLessThan(8);
        const advance = page
          .getByRole('button', { name: /^(Next|Review|Create)$/ })
          .first();
        await advance.waitFor({ state: 'visible', timeout: 15_000 });
        const label = (await advance.textContent())?.trim();
        await expect(advance).toBeEnabled({ timeout: 15_000 });
        await advance.click();
        if (label === 'Create') break;
      }

      await expect
        .poll(
          () =>
            kubectl(['get', row.kind, row.name, '-n', 'default'], {
              check: false,
            }).status,
          { timeout: 60_000 },
        )
        .toBe(0);

      const created = kGetJSON<unknown>(row.kind, row.name, 'default');
      expect(JSON.stringify(created)).toContain(row.description);

      // Delete via the catalog entity overflow menu. The freshly-created
      // entity reaches the catalog on the provider's periodic full sync
      // (default 300s; the UI-test install lowers it via
      // backstage.catalogSync.frequency) — size the timeout to survive a
      // full default cycle so the spec also passes against a stock install.
      await catalog.openEntity(row.catalogKind, row.name, 360_000);
      await del.openOverflowAndDelete(row.kindDisplayName);
      await del.confirm();

      await expect
        .poll(() => kNotFound(row.kind, row.name, 'default'), {
          timeout: 60_000,
        })
        .toBe(true);
    });
  }
});
