// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, type Page } from '@playwright/test';

// Walks the Backstage multi-step scaffolder wizard. The wizard steps are
// advanced by clicking whichever button is currently visible among
// "Next", "Review", and "Create". A slow step-render can race a fixed label
// sequence, so we re-resolve on each iteration.
export class ScaffolderWizardPO {
  constructor(private readonly page: Page) {}

  async fillField(label: string, value: string): Promise<void> {
    await this.page.getByLabel(label, { exact: false }).fill(value);
  }

  // Fill a MUI v4 text field where the label lacks a `for` attribute and the
  // input has no `id`. Locates the input via a CSS sibling selector relative
  // to the label element. Use this for custom form components (e.g.
  // FormWithYaml templates) that don't wire up standard accessibility attrs.
  async fillMuiField(label: string, value: string): Promise<void> {
    const input = this.muiFieldLocator(label);
    await input.waitFor({ state: 'visible', timeout: 15_000 });
    await input.fill(value);
  }

  muiFieldLocator(label: string) {
    return this.page
      .locator('label', { hasText: label })
      .locator('+ div input')
      .first();
  }

  async selectMuiOption(label: string, option: string): Promise<void> {
    const dropdown = this.page
      .locator('label', { hasText: label })
      .locator('+ div [role="button"]')
      .first();
    await dropdown.waitFor({ state: 'visible', timeout: 15_000 });
    await dropdown.click();
    await this.page
      .getByRole('option', { name: option, exact: true })
      .click();
  }

  async waitForMuiSelectValue(label: string, timeoutMs = 15_000): Promise<void> {
    const input = this.muiFieldLocator(label);
    await expect(input).not.toHaveValue('', { timeout: timeoutMs });
  }

  async advanceTo(targetLabel = 'Create'): Promise<void> {
    for (let i = 0; ; i++) {
      expect(i, `wizard did not reach ${targetLabel}`).toBeLessThan(10);
      const btn = this.page
        .getByRole('button', { name: /^(Next|Review|Create)$/ })
        .first();
      await btn.waitFor({ state: 'visible', timeout: 15_000 });
      const label = (await btn.textContent())?.trim();
      await expect(btn).toBeEnabled({ timeout: 15_000 });
      await btn.click();
      if (label === targetLabel) break;
    }
  }

  async next(): Promise<void> {
    const btn = this.page
      .getByRole('button', { name: 'Next' })
      .first();
    await expect(btn).toBeEnabled({ timeout: 15_000 });
    await btn.click();
  }

  async submit(): Promise<void> {
    await this.advanceTo('Create');
  }

  async switchToYaml(): Promise<void> {
    await this.page.getByRole('button', { name: 'YAML', exact: true }).click();
    await this.expectMode('yaml');
  }

  async switchToForm(): Promise<void> {
    await this.page.getByRole('button', { name: 'Form', exact: true }).click();
    await this.expectMode('form');
  }

  async expectMode(mode: 'form' | 'yaml'): Promise<void> {
    const label = mode === 'form' ? 'Form' : 'YAML';
    await expect(
      this.page.getByRole('button', { name: label, exact: true }),
    ).toHaveAttribute('aria-pressed', 'true');
  }
}
