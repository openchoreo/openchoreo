// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, type Page } from '@playwright/test';

// Drives the ResourceDefinitionTab for updating PE resource CRDs. Every PE
// entity page has a "Definition" tab that renders a YAML editor (CodeMirror 6)
// with the full CRD. The About card exposes an "Edit" icon button that links
// to this tab.
export class DefinitionTabPO {
  constructor(private readonly page: Page) {}

  async openViaEditIcon(timeoutMs = 60_000): Promise<void> {
    // The entity page may briefly show "Entity not found" before the About
    // card (which hosts the Edit icon) renders. Wait for the button.
    const editBtn = this.page.getByRole('button', { name: 'Edit', exact: true });
    await editBtn.waitFor({ state: 'visible', timeout: timeoutMs });
    await editBtn.click();
    await expect(
      this.page.getByText(/Definition:/, { exact: false }),
    ).toBeVisible({ timeout: 30_000 });
  }

  async openViaTab(): Promise<void> {
    await this.page
      .getByRole('tab', { name: /definition/i })
      .click();
    await expect(
      this.page.getByText(/Definition:/, { exact: false }),
    ).toBeVisible({ timeout: 30_000 });
  }

  async save(): Promise<void> {
    await this.page
      .getByRole('button', { name: 'Save changes', exact: true })
      .click();
  }

  async expectSaveSuccess(timeoutMs = 30_000): Promise<void> {
    await expect(
      this.page.getByText('Resource saved successfully'),
    ).toBeVisible({ timeout: timeoutMs });
  }

  async expectSaveError(timeoutMs = 30_000): Promise<void> {
    await expect(
      this.page.getByRole('alert').filter({ hasText: /./}),
    ).toBeVisible({ timeout: timeoutMs });
  }

  async getSaveErrorText(timeoutMs = 30_000): Promise<string> {
    const alert = this.page.getByRole('alert').filter({ hasText: /./ });
    await expect(alert).toBeVisible({ timeout: timeoutMs });
    return (await alert.textContent()) ?? '';
  }

  async dismissError(): Promise<void> {
    await this.page
      .getByRole('alert')
      .getByRole('button', { name: 'Close' })
      .click();
  }

  async discard(): Promise<void> {
    await this.page
      .getByRole('button', { name: 'Discard changes', exact: true })
      .click();
  }
}
