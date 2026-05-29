// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, type Page } from '@playwright/test';
import { SidebarPO } from './sidebar';

// Maps a kubectl/catalog kind to the label the Kind picker renders for it
// (kindDisplayNames in the OpenChoreo Backstage app). Used to pick the matching
// option when driving the picker dropdown.
const KIND_DISPLAY: Record<string, string> = {
  system: 'Project',
  component: 'Component',
  componenttype: 'Component Type',
  trait: 'Trait',
  api: 'API',
  resource: 'Resource',
  environment: 'Environment',
  deploymentpipeline: 'Deployment Pipeline',
  domain: 'Namespace',
};

// The catalog route (App.tsx) mounts CustomCatalogPage with initialKind="system",
// so opening the catalog with no further interaction lands on the Project list.
const DEFAULT_KIND = 'system';

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

  // Open the catalog (via the sidebar) filtered to a given entity kind by
  // driving the Kind picker dropdown — a MUI v4 Select rendered as a
  // role=button with aria-haspopup="listbox", whose menu items are role=option.
  // The catalog opens on the System (Project) kind, so for projects no dropdown
  // interaction is needed. Changing the kind pushes it into the URL query, so a
  // later reload() preserves the filter.
  async openKind(kind: string): Promise<void> {
    await new SidebarPO(this.page).goCatalog();
    if (kind.toLowerCase() === DEFAULT_KIND) return;
    const display = KIND_DISPLAY[kind.toLowerCase()] ?? kind;
    await this.page.locator('[aria-haspopup="listbox"]').first().click();
    await this.page
      .getByRole('option', { name: display, exact: true })
      .click();
  }

  // Filter to `kind`, wait for the (eventually-consistent) row to sync in —
  // reloading to re-query — then open the entity by clicking its catalog link.
  // Tolerates the brief "Entity not found" window after a fresh create by
  // re-clicking on the next iteration.
  async openEntity(
    kind: string,
    name: string,
    timeoutMs = 90_000,
  ): Promise<void> {
    await this.openKind(kind);
    await expect
      .poll(
        async () => {
          const link = this.page
            .getByRole('link', { name, exact: true })
            .first();
          if (await link.isVisible({ timeout: 3_000 }).catch(() => false)) {
            await link.click();
            const notFound = await this.page
              .getByText(/Entity not found/i)
              .isVisible({ timeout: 6_000 })
              .catch(() => false);
            if (!notFound) return true;
          }
          await this.reload();
          return false;
        },
        { timeout: timeoutMs, intervals: [3_000] },
      )
      .toBe(true);
  }
}
