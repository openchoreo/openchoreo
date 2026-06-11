// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, type Page } from '@playwright/test';
import { CreatePO } from './create';

export interface CreateNamespaceInput {
  name: string;
  displayName?: string;
  description?: string;
}

// Namespace creation uses the "Namespace" Backstage Scaffolder template
// (reachable via Create → "Use template Namespace"). The template is a PE-only
// operation: it provisions a Kubernetes namespace carrying the
// `openchoreo.dev/control-plane: "true"` label, which the OpenChoreo
// catalog provider uses to ingest it as a `Domain` entity.
//
// Field labels come from the Namespace Scaffolder template YAML. If any label
// does not match the real template, fail loudly (waitFor with timeout) rather
// than silently skip — a mis-labelled field produces an empty-name submission
// that is harder to diagnose than a clear "element not found" failure.
export class NamespacePO {
  constructor(private readonly page: Page) {}

  async openCreateForm(): Promise<void> {
    await new CreatePO(this.page).chooseTemplate('Namespace');
    // Wait for the first required field to confirm the form rendered.
    await this.page
      .getByLabel('Namespace Name', { exact: false })
      .waitFor({ state: 'visible', timeout: 30_000 });
  }

  async fillCreateForm(input: CreateNamespaceInput): Promise<void> {
    await this.page
      .getByLabel('Namespace Name', { exact: false })
      .fill(input.name);
    if (input.displayName) {
      await this.page
        .getByLabel('Display Name', { exact: false })
        .fill(input.displayName);
    }
    if (input.description) {
      await this.page
        .getByLabel('Description', { exact: false })
        .fill(input.description);
    }
  }

  // Walk the scaffolder wizard to submission. Uses the same resilient loop as
  // pe-ops to handle variable step counts and slow step renders — re-resolves
  // the current advance button on every iteration rather than assuming a fixed
  // label sequence.
  async submitCreate(): Promise<void> {
    for (let i = 0; ; i++) {
      expect(i, 'wizard did not reach Create within 8 steps').toBeLessThan(8);
      const advance = this.page
        .getByRole('button', { name: /^(Next|Review|Create)$/ })
        .first();
      await advance.waitFor({ state: 'visible', timeout: 15_000 });
      const label = (await advance.textContent())?.trim();
      await expect(advance).toBeEnabled({ timeout: 15_000 });
      await advance.click();
      if (label === 'Create') break;
    }
  }

  async create(input: CreateNamespaceInput): Promise<void> {
    await this.openCreateForm();
    await this.fillCreateForm(input);
    await this.submitCreate();
  }

  async expectListed(name: string): Promise<void> {
    await expect(
      this.page.getByRole('link', { name, exact: true }).first(),
    ).toBeVisible();
  }

  async expectNotListed(name: string): Promise<void> {
    await expect(
      this.page.getByRole('link', { name, exact: true }),
    ).toHaveCount(0);
  }
}
