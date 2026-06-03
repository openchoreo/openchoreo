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

// Page object for the EnvironmentOverridesPage.
// Depends on aria-label attributes added in Phase 0 selector readiness.
export class OverridesPO {
  constructor(private readonly page: Page) {}

  async open(
    componentName: string,
    environment = 'development',
  ): Promise<void> {
    await expect
      .poll(
        async () => {
          await this.page.goto(
            `/catalog/default/component/${componentName}/environments/overrides/${environment}`,
          );
          try {
            await this.page
              .getByText(/overrides|configure/i)
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

  async openWorkloadTab(): Promise<void> {
    const tab = this.page.getByRole('tab', { name: /workload/i });
    if (await tab.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await tab.click();
    }
  }

  async openComponentTab(): Promise<void> {
    const tab = this.page.getByRole('tab', { name: /component/i });
    await tab.click();
  }

  // ── Override inherited entries ──────────────────────────────────────

  async overrideInheritedEnv(
    name: string,
    newValue: string,
  ): Promise<void> {
    const card = this.envCard(name);
    await card
      .getByRole('button', { name: 'Override', exact: true })
      .click();
    const valueField = this.page.getByLabel('Value', { exact: true }).last();
    await valueField.clear();
    await valueField.fill(newValue);
    await this.clickApply();
  }

  async overrideInheritedFile(
    fileName: string,
    newContent: string,
  ): Promise<void> {
    await this.cancelAnyOpenEditor();
    const card = this.fileCard(fileName);
    await card
      .getByRole('button', { name: 'Override', exact: true })
      .click();
    const content = this.page.getByLabel(/^(Edit )?Content$/).last();
    await content.scrollIntoViewIfNeeded();
    await content.clear();
    await content.fill(newContent);
    await this.clickApply();
  }

  // ── Add new override entries ───────────────────────────────────────

  async addPlainEnv(input: EnvVarInput): Promise<void> {
    await this.clickAddEnvVar();
    await this.page.getByLabel('Name', { exact: true }).last().fill(input.name);
    await this.page
      .getByLabel('Value', { exact: true })
      .last()
      .fill(input.value ?? '');
    await this.clickApply();
  }

  async addSecretEnv(input: EnvVarInput): Promise<void> {
    await this.clickAddEnvVar();
    await this.page.getByLabel('Name', { exact: true }).last().fill(input.name);
    await this.page
      .getByRole('button', { name: /switch to secret/i })
      .last()
      .click();
    await this.fillSecretRef(input.secretRef!);
    await this.clickApply();
  }

  async addPlainFile(input: FileMountInput): Promise<void> {
    await this.clickAddFileMount();
    await this.page
      .getByLabel('File Name', { exact: true })
      .last()
      .fill(input.fileName);
    await this.page
      .getByLabel('Mount Path', { exact: true })
      .last()
      .fill(input.mountPath);
    if (input.content) {
      const content = this.page.getByLabel(/^(Edit )?Content$/).last();
      await content.scrollIntoViewIfNeeded();
      await content.fill(input.content);
    }
    await this.clickApply();
  }

  async addSecretFile(input: FileMountInput): Promise<void> {
    await this.clickAddFileMount();
    await this.page
      .getByLabel('File Name', { exact: true })
      .last()
      .fill(input.fileName);
    await this.page
      .getByLabel('Mount Path', { exact: true })
      .last()
      .fill(input.mountPath);
    await this.page
      .getByRole('button', { name: /switch to secret/i })
      .last()
      .click();
    await this.fillSecretRef(input.secretRef!);
    await this.clickApply();
  }

  // ── Edit existing override entries ─────────────────────────────────

  async editOverrideEnv(name: string, newValue: string): Promise<void> {
    await this.cancelAnyOpenEditor();
    const card = this.envCard(name);
    await card
      .getByRole('button', { name: 'Edit', exact: true })
      .first()
      .click();
    const valueField = this.page.getByLabel('Value', { exact: true }).last();
    await valueField.clear();
    await valueField.fill(newValue);
    await this.clickApply();
  }

  async editOverrideFile(
    fileName: string,
    newContent: string,
  ): Promise<void> {
    await this.cancelAnyOpenEditor();
    const card = this.fileCard(fileName);
    await card.scrollIntoViewIfNeeded();
    await card
      .getByRole('button', { name: 'Edit', exact: true })
      .first()
      .click();
    await this.page
      .getByRole('button', { name: 'Apply changes' })
      .waitFor({ state: 'visible', timeout: 5_000 });
    const expandBtn = this.page.getByRole('button', {
      name: /expand content/i,
    });
    if (await expandBtn.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await expandBtn.click();
    }
    const content = this.page.getByLabel(/^(Edit )?Content$/).last();
    await content.waitFor({ state: 'visible', timeout: 5_000 });
    await content.scrollIntoViewIfNeeded();
    await content.clear();
    await content.fill(newContent);
    await this.clickApply();
  }

  // ── Delete override entries ────────────────────────────────────────

  async deleteOverrideEnv(name: string): Promise<void> {
    const card = this.envCard(name);
    await card
      .getByRole('button', { name: 'Remove environment variable' })
      .click();
  }

  async deleteOverrideFile(fileName: string): Promise<void> {
    const card = this.fileCard(fileName);
    await card
      .getByRole('button', { name: 'Remove file mount' })
      .click();
  }

  // ── Save flow ──────────────────────────────────────────────────────

  async saveOverrides(): Promise<void> {
    await this.page
      .getByRole('button', { name: 'Save Overrides', exact: true })
      .click();
    await this.confirmIfShown();
  }

  async saveAndDeploy(): Promise<void> {
    await this.page
      .getByRole('button', { name: /save & deploy/i })
      .first()
      .click();
    await this.confirmIfShown();
  }

  // ── Assertions ─────────────────────────────────────────────────────

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

  // ── Private helpers ────────────────────────────────────────────────

  private envCard(name: string): Locator {
    return this.page
      .getByText(name, { exact: true })
      .locator('xpath=ancestor::div[.//button]')
      .filter({
        has: this.page.getByRole('button', {
          name: /^(edit|override|remove environment variable)$/i,
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
          name: /^(edit|override|remove file mount)$/i,
        }),
      })
      .last();
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

  private async confirmIfShown(): Promise<void> {
    const dialog = this.page.getByRole('dialog');
    if (await dialog.isVisible({ timeout: 5_000 }).catch(() => false)) {
      const confirm = dialog.getByRole('button', {
        name: /save|confirm|deploy|promote/i,
      });
      if (await confirm.isVisible({ timeout: 3_000 }).catch(() => false)) {
        await confirm.click();
        await dialog.waitFor({ state: 'hidden', timeout: 30_000 });
      }
    }
  }
}
