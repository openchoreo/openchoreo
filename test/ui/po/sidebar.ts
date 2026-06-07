// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { type Page } from '@playwright/test';

// Backstage left-rail navigation. Items are rendered by core-components
// SidebarItem as <a> / <button> with the visible text as the accessible name,
// so getByRole resolves every entry without any data-testid escape hatch.
//
// The real sidebar only exposes Home / Catalog / Platform / APIs / Create…
// at the top level — there is no "Projects" or "Components" item. Per-kind
// views live under /catalog filtered by `kind`.
export class SidebarPO {
  constructor(private readonly page: Page) {}

  async goHome(): Promise<void> {
    await this.page.getByRole('link', { name: 'Home', exact: true }).first().click();
  }

  async goCatalog(): Promise<void> {
    await this.page.getByRole('link', { name: 'Catalog', exact: true }).first().click();
  }

  async goPlatform(): Promise<void> {
    await this.page
      .getByRole('link', { name: 'Platform', exact: true })
      .first()
      .click();
  }

  async goCreate(): Promise<void> {
    await this.page
      .getByRole('link', { name: /^Create/i })
      .first()
      .click();
  }

  // Sign Out is a direct SidebarItem rendered as a button — no user-menu
  // intermediate in the real Backstage shell.
  async signOut(): Promise<void> {
    await this.page.getByRole('button', { name: 'Sign Out', exact: true }).click();
  }
}
