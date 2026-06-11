// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, type Locator, type Page } from '@playwright/test';

export interface EnvVarInput {
  name: string;
  value?: string;
  secretRef?: { name: string; key: string };
}

export interface FileMountInput {
  fileName: string;
  mountPath: string;
  content?: string;
  secretRef?: { name: string; key: string };
}

// Page object for the WorkloadConfigPage.
//
// Depends on aria-label attributes added to MUI TextFields in
// EnvVarEditor, FileVarEditor, and ContainerContent (Phase 0 selector
// readiness). With those labels, fields are addressed via getByLabel.
export class WorkloadConfigPO {
  constructor(private readonly page: Page) {}

  async open(componentName: string): Promise<void> {
    await expect
      .poll(
        async () => {
          await this.page.goto(
            `/catalog/default/component/${componentName}/environments/workload-config`,
          );
          try {
            await this.page
              .getByRole('heading', { name: 'Environment Variables' })
              .first()
              .waitFor({ state: 'visible', timeout: 10_000 });
            return true;
          } catch {
            return false;
          }
        },
        { timeout: 60_000, intervals: [3_000] },
      )
      .toBe(true);
  }

  // ── Env var operations ──────────────────────────────────────────────

  async addPlainEnv(input: EnvVarInput): Promise<void> {
    await this.clickAddEnvVar();
    await this.page.getByLabel('Name', { exact: true }).last().fill(input.name);
    await this.page.getByLabel('Value', { exact: true }).last().fill(input.value ?? '');
    await this.clickApply();
  }

  async addSecretEnv(input: EnvVarInput): Promise<void> {
    await this.cancelAnyOpenEditor();
    await this.clickAddEnvVar();
    await this.page.getByLabel('Name', { exact: true }).last().fill(input.name);
    await this.page
      .getByRole('button', { name: /switch to secret/i })
      .last()
      .click();
    await this.fillSecretRef(input.secretRef!);
    await this.clickApply();
  }

  async editPlainEnv(currentName: string, next: EnvVarInput): Promise<void> {
    await this.clickEditOnEnvRow(currentName);
    if (next.name !== currentName) {
      const nameField = this.page.getByLabel('Name', { exact: true }).last();
      await nameField.clear();
      await nameField.fill(next.name);
    }
    const valueField = this.page.getByLabel('Value', { exact: true }).last();
    await valueField.clear();
    await valueField.fill(next.value ?? '');
    await this.clickApply();
  }

  async deleteEnv(name: string): Promise<void> {
    const card = this.envCard(name);
    await card
      .getByRole('button', { name: 'Remove environment variable' })
      .click();
  }

  // ── File mount operations ───────────────────────────────────────────

  async addPlainFile(input: FileMountInput): Promise<void> {
    await this.page.reload();
    await this.page
      .getByRole('heading', { name: 'Environment Variables' })
      .first()
      .waitFor({ state: 'visible', timeout: 10_000 });
    await this.cancelAnyOpenEditor();
    await this.clickAddFileMount();
    await this.page.getByLabel('File Name', { exact: true }).last().fill(input.fileName);
    await this.page.getByLabel('Mount Path', { exact: true }).last().fill(input.mountPath);
    if (input.content) {
      const content = this.page.getByLabel(/^(Edit )?Content$/).last();
      await content.scrollIntoViewIfNeeded();
      await content.fill(input.content);
    }
    await this.clickApply();
  }

  async addSecretFile(input: FileMountInput): Promise<void> {
    await this.cancelAnyOpenEditor();
    await this.clickAddFileMount();
    await this.page.getByLabel('File Name', { exact: true }).last().fill(input.fileName);
    await this.page.getByLabel('Mount Path', { exact: true }).last().fill(input.mountPath);
    await this.page
      .getByRole('button', { name: /switch to secret/i })
      .last()
      .click();
    await this.fillSecretRef(input.secretRef!);
    await this.clickApply();
  }

  async editPlainFile(fileName: string, next: FileMountInput): Promise<void> {
    await this.clickEditOnFileRow(fileName);
    // Truthy check, not `!== undefined`: empty string means "skip" (clear would fail MUI validation).
    if (next.mountPath) {
      const mountField = this.page.getByLabel('Mount Path', { exact: true }).last();
      await mountField.clear();
      await mountField.fill(next.mountPath);
    }
    if (next.content !== undefined) {
      const expandBtn = this.page.getByRole('button', {
        name: /expand content/i,
      });
      if (await expandBtn.isVisible({ timeout: 2_000 }).catch(() => false)) {
        await expandBtn.click();
      }
      const content = this.page.getByLabel(/content/i).last();
      await content.waitFor({ state: 'visible', timeout: 5_000 });
      await content.clear();
      await content.fill(next.content);
    }
    await this.clickApply();
  }

  async deleteFile(fileName: string): Promise<void> {
    const card = this.fileCard(fileName);
    await card
      .getByRole('button', { name: 'Remove file mount' })
      .click();
  }

  // ── Assertions ──────────────────────────────────────────────────────

  async expectEnvVisible(name: string): Promise<void> {
    await expect(this.page.getByText(name, { exact: true }).first()).toBeVisible();
  }

  async expectFileVisible(fileName: string): Promise<void> {
    await expect(this.page.getByText(fileName, { exact: true }).first()).toBeVisible();
  }

  async expectApplyDisabled(): Promise<void> {
    await expect(
      this.page.getByRole('button', { name: 'Apply changes' }),
    ).toBeDisabled();
  }

  async cancelEditing(): Promise<void> {
    await this.page
      .getByRole('button', { name: 'Cancel editing' })
      .click();
  }

  // ── Component tab: trait operations ────────────────────────────────

  async openComponentTab(): Promise<void> {
    await this.page.getByRole('tab', { name: 'Component' }).click();
    await this.page
      .getByRole('button', { name: 'Add Trait' })
      .waitFor({ state: 'visible', timeout: 10_000 });
  }

  // Opens the "Add Trait" dialog, selects the given trait by display name
  // (e.g. "observability-alert-rule (Cluster)"), optionally overrides the
  // instance name, then switches to FORM mode and fills required parameters.
  // Does NOT click Continue — call saveAndCreateRelease() afterwards.
  //
  // The default YAML editor uses an imperatively-set aria-label on a
  // CodeMirror contenteditable, which Playwright cannot reliably locate via
  // scoped locators. FORM mode uses RJSF which generates proper
  // <label for="id"> associations, so fields are addressable by label text.
  //
  // `options.description` fills the RJSF Description field (defaults to
  // 'e2e test alert'). `options.sourceType` selects the source.type enum
  // option (defaults to 'metric'). Pass explicit values to test variants.
  async addTrait(
    traitDisplayName: string,
    instanceName?: string,
    options?: { description?: string; sourceType?: string },
  ): Promise<void> {
    const cancelBtn = this.page.getByRole('button', {
      name: 'Cancel',
      exact: true,
    });

    // Close any lingering dialog before opening a fresh one.
    if (await cancelBtn.isVisible({ timeout: 500 }).catch(() => false)) {
      const listboxOpen = await this.page
        .locator('[role="listbox"]')
        .isVisible({ timeout: 300 })
        .catch(() => false);
      if (listboxOpen) {
        await this.page
          .locator('[role="button"][aria-haspopup="listbox"]')
          .last()
          .click();
        await this.page
          .locator('[role="listbox"]')
          .waitFor({ state: 'hidden', timeout: 3_000 })
          .catch(() => undefined);
      }
      await cancelBtn.click();
      await cancelBtn.waitFor({ state: 'hidden', timeout: 5_000 });
    }

    await this.page
      .getByRole('button', { name: 'Add Trait' })
      .first()
      .click();
    await cancelBtn.waitFor({ state: 'visible', timeout: 5_000 });

    // Open the Select dropdown and choose the trait.
    await this.page
      .locator('[role="button"][aria-haspopup="listbox"]')
      .last()
      .click();
    await this.page
      .getByRole('option', { name: traitDisplayName })
      .waitFor({ state: 'visible', timeout: 5_000 });
    await this.page.getByRole('option', { name: traitDisplayName }).click();

    // Wait for schema to load. The helper text appears only after the schema
    // fetch completes and the Instance Name + parameters section is rendered.
    await this.page
      .getByText('A unique name to identify this trait instance')
      .waitFor({ state: 'visible', timeout: 15_000 });

    // Override the default instance name if provided.
    if (instanceName) {
      const instanceInput = this.page
        .getByText('A unique name to identify this trait instance', {
          exact: true,
        })
        .locator('xpath=ancestor::div[contains(@class,"MuiFormControl")][1]')
        .locator('input');
      await instanceInput.fill(instanceName);
    }

    // Switch to FORM mode (FormYamlToggle button text is exactly "Form").
    await this.page.getByRole('button', { name: 'Form', exact: true }).click();

    // Fill required Description field (RJSF label: sanitizeLabel('description') = 'Description').
    const description = options?.description ?? 'e2e test alert';
    const descField = this.page.getByLabel('Description');
    await descField.waitFor({ state: 'visible', timeout: 10_000 });
    await descField.fill(description);

    // Fill required source.type field. RJSF with the MUI theme renders enum
    // fields as a MUI v4 Select (div[role="button"][aria-haspopup="listbox"]).
    // Click to open the dropdown, then pick the desired option.
    const sourceType = options?.sourceType ?? 'metric';
    const typeField = this.page.getByLabel('Type');
    await typeField.waitFor({ state: 'visible', timeout: 5_000 });
    await typeField.click();
    await this.page
      .getByRole('option', { name: sourceType, exact: true })
      .waitFor({ state: 'visible', timeout: 5_000 });
    await this.page.getByRole('option', { name: sourceType, exact: true }).click();

    // The dialog portal is appended to <body> after the main page content,
    // so .last() resolves to the dialog's confirm button rather than the
    // "Add Trait" button that originally opened the dialog.
    const confirmBtn = this.page
      .getByRole('button', { name: 'Add Trait', exact: true })
      .last();
    await expect(confirmBtn).toBeEnabled({ timeout: 15_000 });
    await confirmBtn.click();
    await cancelBtn.waitFor({ state: 'hidden', timeout: 10_000 });
  }

  // Remove an attached trait by instance name.
  // The TraitAccordion renders a trash icon with title="Delete trait".
  // Waits for the accordion row to disappear to avoid racing the next step.
  async removeTrait(instanceName: string): Promise<void> {
    const row = this.page
      .getByRole('button')
      .filter({ has: this.page.getByText(instanceName) })
      .filter({ has: this.page.getByRole('button', { name: 'Delete trait' }) });
    const deleteBtn = row.getByRole('button', { name: 'Delete trait' });
    await deleteBtn.click();
    await deleteBtn.waitFor({ state: 'hidden', timeout: 5_000 });
  }

  // ── Save flow ───────────────────────────────────────────────────────

  async saveAndCreateRelease(): Promise<void> {
    const saveBtn = this.page
      .getByRole('button', { name: /continue/i })
      .first();
    await saveBtn.click();

    const confirmSave = this.page
      .getByRole('dialog')
      .getByRole('button', { name: /save & continue/i });
    if (await confirmSave.isVisible({ timeout: 5_000 }).catch(() => false)) {
      await confirmSave.click();
    }

    await this.page
      .getByRole('dialog')
      .getByRole('button', { name: 'Create release', exact: true })
      .click();
    await this.page
      .getByRole('dialog')
      .waitFor({ state: 'hidden', timeout: 30_000 });
  }

  // ── Private helpers ─────────────────────────────────────────────────

  private envCard(name: string): Locator {
    return this.page
      .getByText(name, { exact: true })
      .locator('xpath=ancestor::div[.//button]')
      .filter({
        has: this.page.getByRole('button', {
          name: /^(edit|remove environment variable)$/i,
        }),
      })
      .last();
  }

  private fileCard(fileName: string): Locator {
    return this.page
      .getByText(fileName, { exact: true })
      .locator('xpath=ancestor::div[.//button]')
      .filter({
        has: this.page.getByRole('button', {
          name: /^(edit|remove file mount)$/i,
        }),
      })
      .last();
  }

  private async clickEditOnEnvRow(name: string): Promise<void> {
    await this.cancelAnyOpenEditor();
    const card = this.envCard(name);
    await card.getByRole('button', { name: 'Edit', exact: true }).first().click();
  }

  private async clickEditOnFileRow(fileName: string): Promise<void> {
    await this.cancelAnyOpenEditor();
    const card = this.fileCard(fileName);
    await card.scrollIntoViewIfNeeded();
    await card.getByRole('button', { name: 'Edit', exact: true }).first().click();
    await this.page
      .getByRole('button', { name: 'Apply changes' })
      .waitFor({ state: 'visible', timeout: 5_000 });
  }

  private async cancelAnyOpenEditor(): Promise<void> {
    const cancelBtn = this.page.getByRole('button', { name: 'Cancel editing' });
    if (await cancelBtn.count() > 0) {
      await cancelBtn.first().scrollIntoViewIfNeeded();
      await cancelBtn.first().click();
      await cancelBtn.first().waitFor({ state: 'hidden', timeout: 5_000 });
    }
  }

  private async clickAddEnvVar(): Promise<void> {
    await this.page
      .getByRole('button', { name: 'Add Environment Variable', exact: true })
      .click();
    await this.page
      .getByRole('button', { name: 'Apply changes' })
      .first()
      .waitFor({ state: 'visible', timeout: 10_000 });
  }

  private async clickAddFileMount(): Promise<void> {
    await this.page
      .getByRole('button', { name: 'Add File Mount', exact: true })
      .click();
    await this.page
      .getByRole('button', { name: 'Apply changes' })
      .last()
      .waitFor({ state: 'visible', timeout: 10_000 });
  }

  private async clickApply(): Promise<void> {
    await this.page
      .getByRole('button', { name: 'Apply changes' })
      .click();
  }

  private async fillSecretRef(ref: {
    name: string;
    key: string;
  }): Promise<void> {
    // MUI v4 Select without id props doesn't get an accessible name from
    // InputLabel. Locate the FormControl containing the label text, then
    // find the [role=button] Select trigger inside it.
    const nameControl = this.page
      .getByText('Secret Reference Name', { exact: true })
      .last()
      .locator('xpath=ancestor::div[contains(@class,"MuiFormControl")]');
    await nameControl.locator('[role="button"]').click();
    await this.page
      .getByRole('option', { name: ref.name, exact: true })
      .click();

    const keyControl = this.page
      .getByText('Secret Reference Key', { exact: true })
      .last()
      .locator('xpath=ancestor::div[contains(@class,"MuiFormControl")]');
    const keyTrigger = keyControl.locator('[role="button"]');
    await expect(keyTrigger).toBeEnabled({ timeout: 5_000 });
    await keyTrigger.click();
    await this.page
      .getByRole('option', { name: ref.key, exact: true })
      .click();
  }
}
