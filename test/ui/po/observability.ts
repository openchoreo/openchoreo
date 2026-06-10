// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, type Page } from '@playwright/test';

export class ObservabilityPO {
  constructor(private readonly page: Page) {}

  // Navigate to a Backstage entity tab, polling until the entity syncs into
  // the catalog. Mirrors the ComponentPO.gotoComponentRoute pattern: waits for
  // a positive heading signal rather than the absence of "Entity not found" so
  // the poll doesn't pass spuriously on a half-rendered page.
  private async gotoEntityTab(
    kind: 'component' | 'system',
    name: string,
    tabPath: string,
  ): Promise<void> {
    await expect
      .poll(
        async () => {
          await this.page.goto(`/catalog/default/${kind}/${name}${tabPath}`);
          try {
            await this.page
              .getByRole('heading', { name })
              .first()
              .waitFor({ state: 'visible', timeout: 8_000 });
            if (
              await this.page
                .getByText('Entity not found')
                .isVisible({ timeout: 1_000 })
                .catch(() => false)
            )
              return false;
            return true;
          } catch {
            return false;
          }
        },
        { timeout: 360_000, intervals: [3_000] },
      )
      .toBe(true);
  }

  // Component entity page tabs.
  async gotoComponentLogsTab(componentName: string): Promise<void> {
    await this.gotoEntityTab('component', componentName, '/runtime-logs');
  }

  async gotoComponentMetricsTab(componentName: string): Promise<void> {
    await this.gotoEntityTab('component', componentName, '/metrics');
  }

  // Project (system) entity page tabs.
  async gotoProjectLogsTab(projectName: string): Promise<void> {
    await this.gotoEntityTab('system', projectName, '/logs');
  }

  async gotoProjectTracesTab(projectName: string): Promise<void> {
    await this.gotoEntityTab('system', projectName, '/traces');
  }

  // Wait until the Logs panel has rendered its filter chrome. The "Search Logs..."
  // placeholder is mounted on first render (before any data loads), so its
  // visibility proves the FeatureGatedContent gate was passed and the panel is active.
  async expectLogsTabReady(): Promise<void> {
    await this.page
      .getByPlaceholder('Search Logs...')
      .waitFor({ state: 'visible', timeout: 30_000 });
  }

  // Wait until the Metrics panel has rendered both resource-usage cards. The
  // "CPU Usage" and "Memory Usage" CardHeader titles are mounted once the
  // environments API returns, even when there is no time-series data yet.
  async expectMetricsTabReady(): Promise<void> {
    await this.page
      .getByText('CPU Usage')
      .first()
      .waitFor({ state: 'visible', timeout: 30_000 });
    await expect(this.page.getByText('Memory Usage').first()).toBeVisible();
  }

  // Wait until the Traces panel has rendered its actions bar. TracesActions
  // always renders "Total traces: N" once the panel mounts, even when there is
  // no data yet — more reliable than getByLabel on MUI v4's floating TextField
  // whose label element is not semantically associated with the input.
  async expectTracesTabReady(): Promise<void> {
    await this.page
      .getByText(/Total traces:/)
      .waitFor({ state: 'visible', timeout: 30_000 });
  }
}
