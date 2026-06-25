// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { type Page } from '@playwright/test';
import { SidebarPO } from './sidebar';

// The OpenChoreo "Create" page (CustomTemplateListPage). "Application
// Resources" are three navigation cards — "Project", "Component", "Resource" —
// that drill into per-type template sub-lists; "Platform Resources" are direct
// template cards (Namespace, Environment, ComponentType, Trait, …). Template
// cards render `<button aria-label="Use template <title>">`; navigation cards
// are role=button elements named "<Title> <description>". Reaching a form is a
// pure click flow, so specs never deep-link to `/create/templates/...`.
export class CreatePO {
  constructor(private readonly page: Page) {}

  // Open the Create landing page via the left-rail "Create…" item.
  async open(): Promise<void> {
    await new SidebarPO(this.page).goCreate();
  }

  // Choose a "Platform Resources" template card (e.g. "Namespace",
  // "ComponentType", "Trait") and land on its scaffolder form.
  async chooseTemplate(title: string): Promise<void> {
    await this.open();
    await this.useTemplate(title);
  }

  // Project templates live behind the "Project" navigation card, which opens
  // the per-ProjectType template list; pick the template from there.
  async chooseProjectTemplate(title: string): Promise<void> {
    await this.open();
    await this.page
      .getByRole('button', { name: /Project Browse project templates/i })
      .click();
    await this.useTemplate(title);
  }

  // Component templates live behind the "Component" navigation card, which opens
  // the per-ComponentType template list; pick the template from there.
  async chooseComponentTemplate(title: string): Promise<void> {
    await this.open();
    await this.page
      .getByRole('button', { name: /Component Browse component templates/i })
      .click();
    await this.useTemplate(title);
  }

  private async useTemplate(title: string): Promise<void> {
    await this.page
      .getByRole('button', { name: `Use template ${title}`, exact: true })
      .click();
  }
}
