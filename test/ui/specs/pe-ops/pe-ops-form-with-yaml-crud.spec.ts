// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test, expect, storageStateFor } from '../../fixtures/auth';
import {
  kGetJSONScoped,
  kNotFoundScoped,
  kExists,
  kDelete,
} from '../../fixtures/kube';
import { cmGetContent, cmSetContent } from '../../fixtures/codemirror';
import { DeletePO } from '../../po/delete';
import { CreatePO } from '../../po/create';
import { CatalogTablePO } from '../../po/catalogTable';
import { ScaffolderWizardPO } from '../../po/scaffolderWizard';
import { DefinitionTabPO } from '../../po/definitionTab';

// PE CRUD for CRDs that use the FormWithYaml scaffolder pattern:
//   - A single step with Form/YAML toggle
//   - Form fields map to specific CRD spec fields
//   - Switching between form and YAML preserves state
//
// Environment is tested first because DeploymentPipeline references
// environments in its promotion paths.

const ts = Date.now().toString(36);

const ENV_NAME = `peops-env-${ts}`;
const ENV_DESC = 'e2e form-with-yaml environment';
const DP_NAME = `peops-dp-${ts}`;
const DP_DESC = 'e2e form-with-yaml deployment pipeline';

test.describe.configure({ mode: 'serial' });

test.describe('pe-ops: FormWithYaml CRUD', () => {
  test.beforeAll(async ({ mintAuthState }) => {
    await mintAuthState('pe');
  });
  test.use({ storageState: storageStateFor('pe') });

  test.afterAll(async () => {
    kDelete('deploymentpipeline', DP_NAME, 'default');
    kDelete('environment', ENV_NAME, 'default');
  });

  test('environment: create via form → form↔yaml round-trip → update → delete', async ({
    page,
  }) => {
    test.setTimeout(300_000);
    const catalog = new CatalogTablePO(page);
    const del = new DeletePO(page);
    const wizard = new ScaffolderWizardPO(page);

    // ── CREATE ──
    await page.goto('/');
    await new CreatePO(page).chooseTemplate('Environment');

    // Verify starts in Form mode
    await wizard.expectMode('form');
    // Namespace must be explicitly selected; auto-selection is racy
    await wizard.selectMuiOption('Namespace', 'default');
    // Wait for Data Plane to auto-populate after namespace selection
    await wizard.waitForMuiSelectValue('Data Plane');
    await wizard.fillMuiField('Environment Name', ENV_NAME);
    await wizard.fillMuiField('Description', ENV_DESC);

    // ── Form → YAML round-trip verification ──
    await wizard.switchToYaml();

    // Verify form values appear in YAML
    const yamlContent = await cmGetContent(page);
    expect(yamlContent).toContain(ENV_NAME);
    expect(yamlContent).toContain(ENV_DESC);

    // Switch back to Form — verify fields preserved
    await wizard.switchToForm();
    await expect(wizard.muiFieldLocator('Environment Name')).toHaveValue(ENV_NAME);

    // Submit via the wizard
    await wizard.submit();

    // Poll kubectl until the Environment CR exists
    await expect
      .poll(
        () => kExists('environment', ENV_NAME, 'default'),
        { timeout: 60_000 },
      )
      .toBe(true);

    // Verify shape
    const created = kGetJSONScoped<{
      kind: string;
      spec: { isProduction: boolean; dataPlaneRef?: unknown };
      metadata: { annotations?: Record<string, string> };
    }>('environment', ENV_NAME, 'default');
    expect(created.kind).toBe('Environment');
    // isProduction is omitted (omitempty) when false, so check it's falsy
    expect(created.spec.isProduction).toBeFalsy();
    expect(created.spec.dataPlaneRef).toBeTruthy();

    // ── UPDATE via Definition Tab ──
    await catalog.openEntity('environment', ENV_NAME, 90_000);

    const defTab = new DefinitionTabPO(page);
    await defTab.openViaEditIcon();

    // Modify the description annotation
    const updatedDesc = `${ENV_DESC} updated`;
    const currentYaml = await cmGetContent(page);
    const newYaml = currentYaml.replace(ENV_DESC, updatedDesc);
    await cmSetContent(page, newYaml);

    await defTab.save();
    await defTab.expectSaveSuccess();

    await expect
      .poll(
        () => {
          const obj = kGetJSONScoped<{
            metadata: { annotations?: Record<string, string> };
          }>('environment', ENV_NAME, 'default');
          return obj.metadata.annotations?.['openchoreo.dev/description'];
        },
        { timeout: 60_000 },
      )
      .toBe(updatedDesc);

    // ── DELETE ──
    await catalog.openEntity('environment', ENV_NAME, 60_000);
    await del.openOverflowAndDelete('Environment');
    await del.confirm();

    await expect
      .poll(
        () => kNotFoundScoped('environment', ENV_NAME, 'default'),
        { timeout: 60_000 },
      )
      .toBe(true);
  });

  test('deploymentpipeline: create via form → update → delete', async ({
    page,
  }) => {
    test.setTimeout(300_000);
    const catalog = new CatalogTablePO(page);
    const del = new DeletePO(page);
    const wizard = new ScaffolderWizardPO(page);

    // ── CREATE ──
    await page.goto('/');
    await new CreatePO(page).chooseTemplate('Deployment Pipeline');

    await wizard.expectMode('form');
    // Namespace must be explicitly selected; auto-selection is racy
    await wizard.selectMuiOption('Namespace', 'default');
    await wizard.fillMuiField('Pipeline Name', DP_NAME);
    await wizard.fillMuiField('Description', DP_DESC);

    // ── Form → YAML round-trip verification ──
    await wizard.switchToYaml();
    const yamlContent = await cmGetContent(page);
    expect(yamlContent).toContain(DP_NAME);
    expect(yamlContent).toContain(DP_DESC);

    // Switch back to Form — verify fields preserved
    await wizard.switchToForm();
    await expect(wizard.muiFieldLocator('Pipeline Name')).toHaveValue(DP_NAME);

    // Submit
    await wizard.submit();

    // Poll kubectl until the DeploymentPipeline CR exists
    await expect
      .poll(
        () => kExists('deploymentpipeline', DP_NAME, 'default'),
        { timeout: 60_000 },
      )
      .toBe(true);

    // Verify shape
    const created = kGetJSONScoped<{
      kind: string;
      metadata: { annotations?: Record<string, string> };
    }>('deploymentpipeline', DP_NAME, 'default');
    expect(created.kind).toBe('DeploymentPipeline');

    // ── UPDATE via Definition Tab ──
    await catalog.openEntity('deploymentpipeline', DP_NAME, 90_000);

    const defTab = new DefinitionTabPO(page);
    await defTab.openViaEditIcon();

    const updatedDesc = `${DP_DESC} updated`;
    const currentYaml = await cmGetContent(page);
    const newYaml = currentYaml.replace(DP_DESC, updatedDesc);
    await cmSetContent(page, newYaml);

    await defTab.save();
    await defTab.expectSaveSuccess();

    await expect
      .poll(
        () => {
          const obj = kGetJSONScoped<{
            metadata: { annotations?: Record<string, string> };
          }>('deploymentpipeline', DP_NAME, 'default');
          return obj.metadata.annotations?.['openchoreo.dev/description'];
        },
        { timeout: 60_000 },
      )
      .toBe(updatedDesc);

    // ── DELETE ──
    await catalog.openEntity('deploymentpipeline', DP_NAME, 60_000);
    await del.openOverflowAndDelete('Deployment Pipeline');
    await del.confirm();

    await expect
      .poll(
        () => kNotFoundScoped('deploymentpipeline', DP_NAME, 'default'),
        { timeout: 60_000 },
      )
      .toBe(true);
  });
});
