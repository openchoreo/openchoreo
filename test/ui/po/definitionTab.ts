// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, type Locator, type Page } from '@playwright/test';

// Copy shown by the Definition tab's success Snackbar after a valid save.
// Referenced both to assert success and to exclude that Snackbar when
// locating an error alert (see saveErrorAlert).
const SAVE_SUCCESS_MESSAGE = 'Resource saved successfully';

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
      this.page.getByText(SAVE_SUCCESS_MESSAGE),
    ).toBeVisible({ timeout: timeoutMs });
  }

  // The save error alert, scoped to exclude the success Snackbar. After a
  // valid save the success Snackbar ("Resource saved successfully") auto-hides
  // on a timer, so it can still be mounted as role="alert" when a subsequent
  // save surfaces a validation error. Matching every non-empty alert would
  // then resolve to two elements and trip Playwright's strict-mode check, so
  // we target the error alert as "an alert that is not the success message".
  private saveErrorAlert(): Locator {
    return this.page
      .getByRole('alert')
      .filter({ hasText: /\S/ })
      .filter({ hasNotText: SAVE_SUCCESS_MESSAGE });
  }

  async expectSaveError(timeoutMs = 30_000): Promise<void> {
    await expect(this.saveErrorAlert()).toBeVisible({ timeout: timeoutMs });
  }

  async getSaveErrorText(timeoutMs = 30_000): Promise<string> {
    const alert = this.saveErrorAlert();
    await expect(alert).toBeVisible({ timeout: timeoutMs });
    return (await alert.textContent()) ?? '';
  }

  async dismissError(): Promise<void> {
    await this.saveErrorAlert()
      .getByRole('button', { name: 'Close' })
      .click();
  }

  async discard(): Promise<void> {
    await this.page
      .getByRole('button', { name: 'Discard changes', exact: true })
      .click();
  }
}
