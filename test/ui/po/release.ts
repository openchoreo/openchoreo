// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, type Page } from '@playwright/test';

// Deployment status on the component's Deploy tab (the "environments" graph).
// Each environment node renders a status string as `status: <state>` plus a
// `deployed: <time> ago` suffix once a release has been deployed. Observed
// states: "Not Deployed", "Failed", and the healthy/active state.
//
// The graph nodes are custom SVG/div elements with hashed class names and no
// per-environment ARIA scoping, so we match on the visible status text. The
// lifecycle spec only deploys to `development`, leaving the other environments
// "Not Deployed", which keeps the healthy-state match unambiguous.
const ACTIVE = /status:\s*(Active|Ready|Healthy|Running|Succeeded)/i;

export class ReleasePO {
  constructor(private readonly page: Page) {}

  // Ensure we're on the Deploy/environments graph for the component. Callers
  // reach this right after deploying, so the component entity (with its tab
  // bar) is already open — click the "Deploy" tab to land on the graph. The
  // `/environments` route maps to the "Deploy" entity tab (EntityPage.tsx).
  async openDeployTab(_componentName: string): Promise<void> {
    await this.page.getByRole('tab', { name: 'Deploy', exact: true }).click();
  }

  async expectActive(
    componentName: string,
    _environment: string,
    timeoutMs = 120_000,
  ): Promise<void> {
    await this.openDeployTab(componentName);
    await expect
      .poll(
        async () => {
          await this.page
            .getByRole('button', { name: /Select environment/i })
            .first()
            .waitFor({ state: 'visible', timeout: 15_000 })
            .catch(() => undefined);
          // The status string ("status: Active deployed: …") is split across
          // CSS-uppercased spans, so innerText can't see it contiguously — the
          // accessibility tree preserves the combined text node.
          const aria = await this.page
            .locator('article')
            .first()
            .ariaSnapshot()
            .catch(() => '');
          if (ACTIVE.test(aria)) return true;
          // The graph has no in-page refresh; reload to re-poll the binding.
          await this.page.reload();
          return false;
        },
        { timeout: timeoutMs, intervals: [4_000] },
      )
      .toBe(true);
  }
}
