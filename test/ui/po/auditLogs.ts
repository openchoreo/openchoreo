// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, type Locator, type Page } from '@playwright/test';

// Time-range preset keys the page reads from `?timeRange=` (TIME_RANGE_OPTIONS
// in the openchoreo-react TimeRangeFilter). An unknown key is dropped silently
// and the page falls back to the 7-day default.
export type AuditTimeRange = '10m' | '30m' | '1h' | '24h' | '7d' | '14d' | '30d';

export interface AuditQuery {
  // Filter tokens as `path:value` pairs, e.g. { action: 'create_project' }.
  // A path is any field the filter bar offers (actor.id, resource.name, ...).
  filters?: Record<string, string>;
  timeRange?: AuditTimeRange;
  sort?: 'asc' | 'desc';
}

// Top-level /audit-logs page (openchoreo-observability plugin). The page
// queries the observer straight from the browser, so observer.e2e-op.local
// must resolve through the Chromium host-resolver rules (fixtures/hosts.ts).
//
// The page keeps its whole query in the URL (`f=path:value`, `timeRange`,
// `sort`), so specs deep-link to a filtered view instead of driving the
// Autocomplete filter bar token by token.
export class AuditLogsPO {
  constructor(private readonly page: Page) {}

  async goto(query: AuditQuery = {}): Promise<void> {
    const params = new URLSearchParams();
    for (const [path, value] of Object.entries(query.filters ?? {})) {
      params.append('f', `${path}:${value}`);
    }
    if (query.timeRange) params.set('timeRange', query.timeRange);
    if (query.sort === 'desc') params.set('sort', 'desc');
    const qs = params.toString();
    await this.page.goto(`/audit-logs${qs ? `?${qs}` : ''}`);
  }

  get grid(): Locator {
    return this.page.getByRole('grid', { name: 'Audit logs' });
  }

  // The header row also has role=row, so match data rows by their text.
  rowContaining(text: string): Locator {
    return this.grid.getByRole('row').filter({ hasText: text });
  }

  get drawer(): Locator {
    return this.page.getByLabel('Audit event detail');
  }

  async expectReady(): Promise<void> {
    await expect(
      this.page.getByRole('heading', { name: 'Audit Logs', exact: true }),
    ).toBeVisible({ timeout: 30_000 });
    await expect(this.page.getByLabel('Filter audit records')).toBeVisible({
      timeout: 30_000,
    });
    await expect(this.grid).toBeVisible({ timeout: 30_000 });
  }

  async expectForbidden(): Promise<void> {
    await expect(
      this.page.getByText('Insufficient Permissions', { exact: true }),
    ).toBeVisible({ timeout: 30_000 });
    await expect(
      this.page.getByText('You do not have permission to view audit logs.'),
    ).toBeVisible();
    await expect(this.grid).toHaveCount(0);
  }

  async refresh(): Promise<void> {
    await this.page.getByRole('button', { name: 'Refresh', exact: true }).click();
  }

  // Records reach the page through Fluent Bit tailing the producer's stdout
  // and an OpenSearch index refresh, so a fresh event takes a while to become
  // queryable. Refresh re-resolves a relative window against the clock, so
  // polling it (rather than reloading) is enough to pick up new records.
  async waitForRow(text: string, timeoutMs = 180_000): Promise<Locator> {
    const row = this.rowContaining(text).first();
    await expect
      .poll(
        async () => {
          if (await row.isVisible().catch(() => false)) return true;
          await this.refresh();
          return row
            .waitFor({ state: 'visible', timeout: 5_000 })
            .then(() => true)
            .catch(() => false);
        },
        { timeout: timeoutMs, intervals: [5_000] },
      )
      .toBe(true);
    return row;
  }

  async openRecord(row: Locator): Promise<void> {
    await row.click();
    await expect(this.drawer.getByText('Audit event', { exact: true })).toBeVisible();
  }

  // Drawer values are buttons that add `path:value` to the query and close
  // the drawer.
  async filterFromRecord(value: string): Promise<void> {
    await this.drawer.getByRole('button', { name: value, exact: true }).first().click();
    await expect(this.drawer).toBeHidden();
  }

  // Chips render the path and value as separate spans, so their accessible
  // name reads `path : value`.
  async expectFilterChip(path: string, value: string): Promise<void> {
    await expect(
      this.page.getByRole('button', { name: `${path} : ${value}`, exact: true }),
    ).toBeVisible();
  }
}
