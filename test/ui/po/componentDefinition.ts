// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, type Page } from '@playwright/test';

// Page object for the Component Definition tab.
//
// The Definition tab renders the component's full Kubernetes CR as a
// CodeMirror v6 editor (contenteditable="true", role="textbox"). The user
// can replace the YAML content and click the Save button; the portal calls
// the OpenChoreo API to patch the resource and shows a success banner once
// the call returns.
//
// Locator notes:
//   - Editor: role="textbox" (single contenteditable textbox on the page).
//   - Save:   aria-label="Save changes" (floppy-disk icon button).
//   - Discard: aria-label="Discard changes" (undo icon button).
//   - Success: text "Resource saved successfully" in a snackbar banner.
//
// fill() on a CodeMirror contenteditable replaces all content atomically
// without triggering the editor's auto-indent/auto-complete machinery,
// which makes it the reliable choice over keyboard.type().
export class ComponentDefinitionPO {
  constructor(private readonly page: Page) {}

  // Navigate to the Definition tab for the named component. Retries with
  // reload until the CodeMirror editor is visible — the Backstage catalog
  // entity may not have synced yet on a fresh install.
  async open(componentName: string): Promise<void> {
    await expect
      .poll(
        async () => {
          await this.page.goto(
            `/catalog/default/component/${componentName}/definition`,
          );
          try {
            await this.page
              .getByRole('textbox')
              .first()
              .waitFor({ state: 'visible', timeout: 8_000 });
            return true;
          } catch {
            return false;
          }
        },
        { timeout: 360_000, intervals: [3_000] },
      )
      .toBe(true);
  }

  // Replace the editor's entire content with the supplied YAML string.
  // fill() triggers CodeMirror's input event so the Save button activates.
  async setYAML(yaml: string): Promise<void> {
    const editor = this.page.getByRole('textbox').first();
    await editor.waitFor({ state: 'visible', timeout: 10_000 });
    await editor.fill(yaml);
  }

  // Click the Save button and wait for the "Resource saved successfully"
  // success banner. The banner dismisses automatically; catch it before
  // it vanishes.
  async saveChanges(): Promise<void> {
    const save = this.page.getByRole('button', { name: 'Save changes' });
    await expect(save).toBeEnabled({ timeout: 10_000 });
    await save.click();
    await expect(
      this.page.getByText('Resource saved successfully'),
    ).toBeVisible({ timeout: 30_000 });
  }

  // Click the Discard button to revert unsaved edits.
  async discardChanges(): Promise<void> {
    const discard = this.page.getByRole('button', { name: 'Discard changes' });
    await expect(discard).toBeEnabled({ timeout: 10_000 });
    await discard.click();
  }
}
