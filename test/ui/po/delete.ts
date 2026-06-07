// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { type Page } from '@playwright/test';

// Entity-level deletion. Backstage's EntityContextMenu surfaces a kebab icon
// button with aria-label="more" — semantic role+name is enough, no testid.
// The menu item label is `Delete ${KindDisplayName}` (e.g. "Delete Component",
// "Delete Project") so callers pass the kind they expect to see.
//
// The confirmation dialog has no "type the name to confirm" field — just a
// single "Delete" button.
export class DeletePO {
  constructor(private readonly page: Page) {}

  async openOverflowAndDelete(kindLabel: string): Promise<void> {
    await this.page.getByRole('button', { name: 'more', exact: true }).click();
    await this.page
      .getByRole('menuitem', { name: `Delete ${kindLabel}`, exact: true })
      .click();
  }

  async confirm(): Promise<void> {
    // The destructive button is the second one in the dialog (after "Cancel").
    // It reads "Delete" in idle state and "Deleting..." in flight.
    await this.page
      .getByRole('dialog')
      .getByRole('button', { name: 'Delete', exact: true })
      .click();
  }
}
