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
