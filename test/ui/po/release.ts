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

  // Ensure we're on the Deploy/environments graph for the component. Uses the
  // entity route directly (see ComponentPO.gotoComponentRoute) — the filtered
  // catalog list can't reach a component until it has synced.
  async openDeployTab(componentName: string): Promise<void> {
    await this.page.goto(
      `/catalog/default/component/${encodeURIComponent(componentName)}/environments`,
    );
  }

  async expectActive(
    componentName: string,
    environment: string,
    timeoutMs = 120_000,
  ): Promise<void> {
    await this.openDeployTab(componentName);
    const envRe = new RegExp(environment, 'i');
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
          // accessibility tree preserves the combined text node. Each
          // environment renders its own article node, so scope the check to
          // the article that names the requested environment rather than
          // assuming it comes first in document order.
          const articles = this.page.locator('article');
          const count = await articles.count().catch(() => 0);
          for (let i = 0; i < count; i++) {
            const aria = await articles.nth(i).ariaSnapshot().catch(() => '');
            if (envRe.test(aria) && ACTIVE.test(aria)) return true;
          }
          // The graph has no in-page refresh; reload to re-poll the binding.
          await this.page.reload();
          return false;
        },
        { timeout: timeoutMs, intervals: [4_000] },
      )
      .toBe(true);
  }
}
