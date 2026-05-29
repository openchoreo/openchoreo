// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, type Page } from '@playwright/test';
import { CreatePO } from './create';

export interface CreateProjectInput {
  name: string;
  namespace?: string; // defaults to "default"; preselected via ?namespace query.
  displayName?: string;
  description?: string;
  pipeline?: string; // matches a Deployment Pipeline entity name.
}

// Project creation flows through the Backstage Scaffolder template
// `create-openchoreo-project`, reached by clicking the "Project" card on the
// Create page (CreatePO). The NamespaceEntityPicker auto-selects the `default`
// namespace when no preselection is supplied, so the click flow needs no
// `?namespace=` query.
//
// Field titles come from the template YAML: "Namespace Name", "Project Name",
// "Display Name", "Description", "Deployment Pipeline". MUI labels are wired
// through aria-labelledby, so getByLabel resolves each.
export class ProjectPO {
  constructor(private readonly page: Page) {}

  // `namespace` is accepted for API symmetry; only the auto-selected `default`
  // namespace is exercised. A non-default namespace would need the
  // NamespaceEntityPicker driven explicitly here.
  async openCreateForm(namespace = 'default'): Promise<void> {
    void namespace;
    await new CreatePO(this.page).chooseTemplate('Project');
  }

  async fillCreateForm(input: CreateProjectInput): Promise<void> {
    await this.page.getByLabel('Project Name', { exact: false }).fill(input.name);
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
    // The DeploymentPipelinePicker (packages/app/src/scaffolder/
    // DeploymentPipelinePicker/DeploymentPipelinePickerExtension.tsx)
    // auto-selects the `default` pipeline when the namespace's pipelines
    // load, so an explicit `pipeline` is informational only. The picker is
    // a MUI v4 TextField with `select` — it does NOT expose a combobox role
    // (that's MUI v5+) — so we deliberately do not drive it. If a non-default
    // pipeline ever needs to be tested, swap in a `.locator()` query against
    // the underlying <input> by its name attribute.
    void input.pipeline;
  }

  async submitCreate(): Promise<void> {
    // Scaffolder uses a multi-step layout. The Project template surfaces
    // step 1 with a "Review" button (advances to step 2) and step 2 with a
    // "Create" button (submits). Some templates also intersperse "Next" on
    // wider steps. We walk the labels in order and *wait* for each before
    // clicking — isVisible() doesn't accept a timeout argument, so we use
    // waitFor with a tolerant timeout to ride out the picker auto-select +
    // re-render gap.
    for (const label of ['Next', 'Review', 'Create']) {
      const btn = this.page.getByRole('button', { name: label, exact: true });
      try {
        await btn.waitFor({ state: 'visible', timeout: 10_000 });
      } catch {
        continue; // this label is not part of this template's flow
      }
      await btn.click();
    }
  }

  async create(input: CreateProjectInput): Promise<void> {
    await this.openCreateForm(input.namespace);
    await this.fillCreateForm(input);
    await this.submitCreate();
  }

  // Catalog row navigates to /catalog/<namespace>/system/<name>.
  async openByName(name: string): Promise<void> {
    await this.page.getByRole('link', { name, exact: true }).first().click();
  }

  async expectListed(name: string): Promise<void> {
    await expect(
      this.page.getByRole('link', { name, exact: true }).first(),
    ).toBeVisible();
  }

  async expectNotListed(name: string): Promise<void> {
    await expect(this.page.getByRole('link', { name, exact: true })).toHaveCount(0);
  }
}
