// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, type Page } from '@playwright/test';

// Backstage catalog table. The name column renders a Link, so the row is
// uniquely identifiable by getByRole('link', { name }) — no testid needed.
export class CatalogTablePO {
  constructor(private readonly page: Page) {}

  async openByName(name: string): Promise<void> {
    await this.page.getByRole('link', { name, exact: true }).first().click();
  }

  async expectListed(name: string, timeoutMs = 60_000): Promise<void> {
    await expect(
      this.page.getByRole('link', { name, exact: true }).first(),
    ).toBeVisible({ timeout: timeoutMs });
  }

  // Poll until the row appears (catalog sync is eventually consistent).
  async waitForRow(name: string, timeoutMs = 60_000): Promise<void> {
    await expect
      .poll(
        async () =>
          this.page.getByRole('link', { name, exact: true }).count(),
        { timeout: timeoutMs, intervals: [1_000, 2_000, 5_000] },
      )
      .toBeGreaterThan(0);
  }

  // The OpenChoreo catalog (CustomCatalogPage) has no manual refresh control —
  // the processor backfills entities on its own poll cycle. To re-query, we
  // reload the page; the ChoreoEntityKindPicker restores the kind from the
  // URL query parameter, so the filter survives the reload.
  async reload(): Promise<void> {
    await this.page.reload({ waitUntil: 'domcontentloaded' });
    // The catalog list fetches asynchronously after the DOM mounts. Wait for
    // the Kind picker label to render so we don't count rows on a half-painted
    // page (which would report 0 spuriously).
    await this.page
      .getByText('Kind', { exact: true })
      .first()
      .waitFor({ state: 'visible', timeout: 15_000 })
      .catch(() => undefined);
    await this.page.waitForLoadState('networkidle').catch(() => undefined);
  }

  // Navigate straight to the catalog filtered to a given entity kind. The
  // kind picker reads the `kind` query parameter on mount
  // (ChoreoEntityKindPicker.useEntityKindFilter), so this selects it without
  // driving the MUI Select dropdown.
  async gotoKind(kind: string): Promise<void> {
    await this.page.goto(
      `/catalog?filters%5Bkind%5D=${encodeURIComponent(kind.toLowerCase())}`,
    );
  }
}
