// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, type Page } from '@playwright/test';
import { CreatePO } from './create';

// Template card title for the default cluster-scoped ClusterProjectType; each
// (Cluster)ProjectType yields one card titled by its display name.
export const DEFAULT_PROJECT_TYPE_TEMPLATE = 'Default Project Type';

export interface CreateProjectInput {
  name: string;
  namespace?: string; // defaults to "default"; preselected via ?namespace query.
  displayName?: string;
  description?: string;
  pipeline?: string; // matches a Deployment Pipeline entity name.
  projectType?: string; // template card title; defaults to DEFAULT_PROJECT_TYPE_TEMPLATE.
}

// Project creation flows through a per-(Cluster)ProjectType Scaffolder template,
// reached via the "Project" navigation card on the Create page (CreatePO) then
// the type's template card. The NamespaceEntityPicker auto-selects the `default`
// namespace, so the click flow needs no `?namespace=` query.
//
// Field titles: "Namespace", "Project Name", "Display Name", "Description",
// "Deployment Pipeline". MUI labels are wired through aria-labelledby, so
// getByLabel resolves each.
export class ProjectPO {
  constructor(private readonly page: Page) {}

  // Only the auto-selected `default` namespace is supported here; a non-default
  // namespace needs the NamespaceEntityPicker driven explicitly, so reject it
  // rather than silently creating the project under `default`.
  async openCreateForm(
    namespace = 'default',
    projectType = DEFAULT_PROJECT_TYPE_TEMPLATE,
  ): Promise<void> {
    if (namespace !== 'default') {
      throw new Error(
        `ProjectPO.openCreateForm: unsupported namespace "${namespace}"; drive the NamespaceEntityPicker explicitly for non-default namespaces.`,
      );
    }
    await new CreatePO(this.page).chooseProjectTemplate(projectType);
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
    // Multi-step scaffolder: "Next" advances optional intermediate steps;
    // "Review" then "Create" submit. "Review"/"Create" are mandatory — a
    // timeout there is a slow/broken render and must fail loudly, not be
    // skipped.
    for (const { label, required } of [
      { label: 'Next', required: false },
      { label: 'Review', required: true },
      { label: 'Create', required: true },
    ]) {
      const btn = this.page.getByRole('button', { name: label, exact: true });
      try {
        await btn.waitFor({ state: 'visible', timeout: required ? 30_000 : 10_000 });
      } catch (err) {
        if (required) throw err;
        continue; // 'Next' is genuinely not part of this template's flow
      }
      await btn.click();
    }
  }

  async create(input: CreateProjectInput): Promise<void> {
    await this.openCreateForm(input.namespace, input.projectType);
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
