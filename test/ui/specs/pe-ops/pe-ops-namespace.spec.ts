// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test, expect, storageStateFor } from '../../fixtures/auth';
import { kubectl, kDelete, kExists } from '../../fixtures/kube';
import { cmGetContent, cmSetContent } from '../../fixtures/codemirror';
import { NamespacePO } from '../../po/namespace';
import { DeletePO } from '../../po/delete';
import { CatalogTablePO } from '../../po/catalogTable';
import { ScaffolderWizardPO } from '../../po/scaffolderWizard';
import { CreatePO } from '../../po/create';
import { DEFAULT_PROJECT_TYPE_TEMPLATE } from '../../po/project';

// Namespace names must be valid Kubernetes namespace names: lowercase
// alphanumeric + '-', no leading/trailing hyphens, max 63 characters.
// Date.now().toString(36) produces ~8 lowercase alphanumeric characters.
const ts = Date.now().toString(36);
const NAMESPACE_NAME = `ui-ns-${ts}`;
const ENV1_NAME = `ui-env1-${ts}`;
const ENV2_NAME = `ui-env2-${ts}`;
const PIPELINE_NAME = `ui-pipe-${ts}`;
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
    // Best-effort teardown in reverse creation order.
    kDelete('project', PROJECT_NAME, NAMESPACE_NAME);
    kDelete('deploymentpipeline', PIPELINE_NAME, NAMESPACE_NAME);
    kDelete('environment', ENV2_NAME, NAMESPACE_NAME);
    kDelete('environment', ENV1_NAME, NAMESPACE_NAME);
    // namespace="" omits the -n flag for the cluster-scoped Kubernetes namespace resource.
    kDelete('namespace', NAMESPACE_NAME, '');
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

  // ── 2. Environments ───────────────────────────────────────────────────────
  test('creates two environments in the namespace', async ({ page }) => {
    // Budget: catalog sync for namespace (360s) + two environment creates.
    test.setTimeout(900_000);

    // The NamespaceEntityPicker loads options from the catalog. Wait for the
    // namespace domain entity to sync before attempting to select it.
    const catalog = new CatalogTablePO(page);
    await catalog.openEntity('domain', NAMESPACE_NAME, 360_000);

    const wizard = new ScaffolderWizardPO(page);

    for (const envName of [ENV1_NAME, ENV2_NAME]) {
      await page.goto('/');
      await new CreatePO(page).chooseTemplate('Environment');

      await wizard.selectMuiOption('Namespace', NAMESPACE_NAME);
      // The Data Plane field auto-populates after namespace selection.
      await wizard.waitForMuiSelectValue('Data Plane');
      await wizard.fillMuiField('Environment Name', envName);

      await wizard.submit();

      await expect(
        page.getByRole('link', { name: /view.*environment/i }).first(),
      ).toBeVisible({ timeout: 60_000 });

      await expect.poll(
        () => kExists('environment', envName, NAMESPACE_NAME),
        { timeout: 30_000 },
      ).toBe(true);
    }
  });

  // ── 3. Deployment Pipeline ────────────────────────────────────────────────
  test('creates deployment pipeline with env1→env2 promotion path', async ({ page }) => {
    test.setTimeout(300_000);

    await page.goto('/');
    const wizard = new ScaffolderWizardPO(page);
    await new CreatePO(page).chooseTemplate('Deployment Pipeline');

    await wizard.selectMuiOption('Namespace', NAMESPACE_NAME);
    await wizard.fillMuiField('Pipeline Name', PIPELINE_NAME);

    // Switch to YAML mode to inject the promotion path referencing the two
    // environments created in the previous test.
    await wizard.switchToYaml();
    const yaml = await cmGetContent(page);

    // The default YAML has an empty promotionPaths list. Replace the spec
    // section to wire env1 → env2.
    const updatedYaml = yaml.replace(
      /spec:.*$/ms,
      `spec:\n  promotionPaths:\n    - sourceEnvironmentRef:\n        name: ${ENV1_NAME}\n      targetEnvironmentRefs:\n        - name: ${ENV2_NAME}`,
    );
    expect(updatedYaml, 'YAML spec replacement must succeed before applying').not.toBe(yaml);
    await cmSetContent(page, updatedYaml);

    await wizard.submit();

    await expect(
      page.getByRole('link', { name: /view.*pipeline/i }).first(),
    ).toBeVisible({ timeout: 60_000 });

    await expect.poll(
      () => kExists('deploymentpipeline', PIPELINE_NAME, NAMESPACE_NAME),
      { timeout: 30_000 },
    ).toBe(true);
  });

  // ── 4. Project ────────────────────────────────────────────────────────────
  test('creates project in the namespace using the new pipeline', async ({ page }) => {
    test.setTimeout(300_000);

    await page.goto('/');
    await new CreatePO(page).chooseProjectTemplate(DEFAULT_PROJECT_TYPE_TEMPLATE);

    const wizard = new ScaffolderWizardPO(page);

    // The NamespaceEntityPicker auto-selects 'default'; override to our namespace.
    await wizard.selectMuiOption('Namespace', NAMESPACE_NAME);

    // The DeploymentPipelinePicker auto-populates once the namespace is set.
    // Wait for it to resolve before filling other fields.
    await wizard.waitForMuiSelectValue('Deployment Pipeline');

    await page.getByLabel('Project Name', { exact: false }).fill(PROJECT_NAME);
    await page.getByLabel('Display Name', { exact: false }).fill(PROJECT_NAME);

    await wizard.submit();

    await expect(
      page.getByRole('link', { name: /view.*project/i }).first(),
    ).toBeVisible({ timeout: 60_000 });

    await expect.poll(
      () => kExists('project', PROJECT_NAME, NAMESPACE_NAME),
      { timeout: 30_000 },
    ).toBe(true);
  });

  // ── 5. View ───────────────────────────────────────────────────────────────
  test('namespace entity page shows project, environments, and pipeline', async ({
    page,
  }) => {
    // Budget covers: catalog sync for all four entities (namespace domain,
    // two environments, pipeline, project) + reload-to-repoll loops.
    test.setTimeout(1_200_000);

    const catalog = new CatalogTablePO(page);

    // openEntity navigates via the URL-based gotoKind so the Kind filter
    // survives reload-to-repoll, then clicks the row once it appears.
    await catalog.openEntity('domain', NAMESPACE_NAME, 360_000);

    // Assert the entity page renders with the correct heading.
    await expect(
      page.getByRole('heading', { name: NAMESPACE_NAME }).first(),
    ).toBeVisible({ timeout: 15_000 });

    // Assert "Has Projects" section is visible.
    await expect(
      page.getByRole('heading', { name: /has projects/i }).first(),
    ).toBeVisible({ timeout: 15_000 });

    // Poll for the project link in "Has Projects". The catalog relation is
    // eventually consistent — reload the page between attempts rather than
    // querying a stale DOM. 30s intervals give the catalog provider time to
    // complete a sync cycle between checks.
    await expect.poll(
      async () => {
        const visible = await page
          .getByRole('link', { name: PROJECT_NAME, exact: true })
          .first()
          .isVisible();
        if (!visible) await catalog.reload();
        return visible;
      },
      { timeout: 360_000, intervals: [30_000] },
    ).toBe(true);

    // Assert the "Other Resources in Namespace" card is visible — this card
    // renders environments and pipelines that have a RELATION_PART_OF pointing
    // to this domain.
    await expect(
      page
        .getByRole('heading', { name: /other resources in namespace/i })
        .first(),
    ).toBeVisible({ timeout: 15_000 });

    // Poll for each environment and pipeline link in "Other Resources in
    // Namespace". Each entity may arrive in a different catalog sync cycle.
    for (const resourceName of [ENV1_NAME, ENV2_NAME, PIPELINE_NAME]) {
      await expect.poll(
        async () => {
          const visible = await page
            .getByRole('link', { name: resourceName, exact: true })
            .first()
            .isVisible();
          if (!visible) await catalog.reload();
          return visible;
        },
        { timeout: 360_000, intervals: [30_000] },
      ).toBe(true);
    }

    // Assert the Create Project button exists on the entity page.
    await expect(
      page.getByRole('button', { name: /create project/i }).first(),
    ).toBeVisible({ timeout: 15_000 });
  });

  // ── 6. Delete ─────────────────────────────────────────────────────────────
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

    // Assert phase, not existence: gone ('') or Terminating both prove the delete
    // fired; full teardown waits on the finalizer cascade we don't gate on. Default
    // check:true so a real kubectl error fails instead of passing as empty output.
    await expect
      .poll(
        () =>
          kubectl([
            'get', 'namespace', NAMESPACE_NAME, '--ignore-not-found', '-o', 'jsonpath={.status.phase}',
          ]).stdout.trim(),
        { timeout: 60_000, intervals: [3_000] },
      )
      .not.toBe('Active');
  });
});
