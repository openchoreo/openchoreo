// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { type Page } from '@playwright/test';
import { SidebarPO } from './sidebar';

// The OpenChoreo "Create" page (CustomTemplateListPage). The landing view shows
// a card per scaffolder template (Project, ComponentType, Trait, …) plus two
// navigation cards — "Component" and "Resource" — that drill into their own
// per-type template lists. Each template card renders a
// `<button aria-label="Use template <title>">`, and the navigation cards are
// role=button elements whose accessible name is "<Title> <description>".
//
// Reaching a template form is therefore a pure click flow (sidebar → Create →
// card), so specs never need to deep-link to `/create/templates/...`. The
// NamespaceEntityPicker falls back to the `default` namespace on its own, so the
// `?namespace=` query the old URLs carried is unnecessary.
export class CreatePO {
  constructor(private readonly page: Page) {}

  // Open the Create landing page via the left-rail "Create…" item.
  async open(): Promise<void> {
    await new SidebarPO(this.page).goCreate();
  }

  // Choose a landing-view template card (e.g. "Project", "ComponentType",
  // "Trait") and land on its scaffolder form.
  async chooseTemplate(title: string): Promise<void> {
    await this.open();
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
