// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, type Page } from '@playwright/test';
import { CreatePO } from './create';

export interface CreateComponentInput {
  name: string;
  project: string; // project metadata name to select in the form's Project picker
  // Template card label on the "Browse component templates" page. Restricted
  // to the endpoint-bearing templates: create() drives the endpoint sub-form
  // ("Add Endpoint" / "Apply changes") unconditionally, which non-HTTP
  // templates (e.g. "Worker") don't render. Defaults to "Web Application".
  template?: 'Web Application' | 'Service';
  image: string; // container image reference (Container Image deployment source)
  // Web Application / Service component types require at least one HTTP
  // endpoint at render time, so creation adds one. Defaults to 8080.
  endpointPort?: number;
  displayName?: string;
  description?: string;
}

export interface DeployOptions {
  // Container args applied to the release workload before deploying, e.g.
  // ['--port', '9090']. The create wizard has no args field — args can only
  // be set when snapshotting a release in the Deploy tab.
  args?: string[];
}

// Component creation is a 4-step Backstage Scaffolder wizard reached by
// browsing /create -> "Component" card -> per-ComponentType template (there is
// no fixed create-openchoreo-component template URL). The steps are:
//   1. Component Metadata  (Namespace/Project pickers + name)
//   2. Build & Deploy      (Deployment Source cards + image)
//   3. Web Application Details (Endpoints / env / mounts)  -> "Review"
//   4. Review              -> "Create"
//
// Namespace/Project are MUI v4 Selects (role=button, aria-haspopup=listbox),
// not comboboxes — both render the current value ("default") as their
// accessible name, so they are disambiguated by document order (namespace
// first, project second).
export class ComponentPO {
  constructor(private readonly page: Page) {}

  async openCreateForm(template = 'Web Application'): Promise<void> {
    await new CreatePO(this.page).chooseComponentTemplate(template);
    await this.page
      .getByRole('textbox', { name: 'Component Name' })
      .waitFor({ state: 'visible', timeout: 30_000 });
  }

  // Step 1: Component Metadata.
  private async fillMetadata(input: CreateComponentInput): Promise<void> {
    await this.selectProject(input.project);
    await this.page
      .getByRole('textbox', { name: 'Component Name' })
      .fill(input.name);
    if (input.displayName) {
      await this.page
        .getByRole('textbox', { name: 'Display Name' })
        .fill(input.displayName);
    }
    if (input.description) {
      await this.page
        .getByRole('textbox', { name: 'Description' })
        .fill(input.description);
    }
  }

  // The Project picker (2nd MUI Select; namespace is the 1st — both read
  // "default" pre-selection) is backed by the Backstage catalog, which syncs a
  // freshly-created project on a delay. Reload the form to re-fetch until the
  // project shows up, then select it. `default` is preselected so no-op.
  private async selectProject(project: string): Promise<void> {
    if (!project || project === 'default') return;
    await expect
      .poll(
        async () => {
          await this.page.getByRole('button', { name: 'default' }).nth(1).click();
          const option = this.page.getByRole('option', {
            name: project,
            exact: true,
          });
          if (await option.isVisible({ timeout: 2_000 }).catch(() => false)) {
            await option.click();
            return true;
          }
          // Not synced yet — close the menu and reload the form to re-query.
          await this.page.keyboard.press('Escape').catch(() => undefined);
          await this.page.reload();
          await this.page
            .getByRole('textbox', { name: 'Component Name' })
            .waitFor({ state: 'visible', timeout: 30_000 });
          return false;
        },
        { timeout: 150_000, intervals: [3_000] },
      )
      .toBe(true);
  }

  // Step 2: Build & Deploy — pick the Container Image source and fill the
  // image. The source radios have no accessible name, so the card heading is
  // the click target.
  private async fillBuildAndDeploy(input: CreateComponentInput): Promise<void> {
    await this.page
      .getByRole('heading', { name: 'Container Image', level: 6 })
      .click();
    await this.page
      .getByRole('textbox', { name: /ghcr\.io\/org\/app/i })
      .fill(input.image);
  }

  // Step 3: Web Application Details — add one HTTP endpoint (required by the
  // web-application / service component types).
  private async fillDetails(input: CreateComponentInput): Promise<void> {
    await this.page
      .getByRole('button', { name: 'Add Endpoint', exact: true })
      .click();
    // The endpoint sub-form defaults to name "endpoint-1", type HTTP, port
    // 8080. Only the port needs overriding when the image listens elsewhere.
    const port = input.endpointPort ?? 8080;
    const portField = this.page.getByRole('spinbutton').first();
    await portField.fill(String(port));
    // Commit the endpoint item — the wizard refuses to advance to Review while
    // an endpoint is still in edit mode ("Save or cancel the item you are
    // currently editing before proceeding.").
    await this.page
      .getByRole('button', { name: 'Apply changes', exact: true })
      .click();
  }

  async create(input: CreateComponentInput): Promise<void> {
    await this.openCreateForm(input.template ?? 'Web Application');
    await this.fillMetadata(input);
    await this.clickStep('Next'); // -> Build & Deploy
    await this.fillBuildAndDeploy(input);
    await this.clickStep('Next'); // -> Web Application Details
    await this.fillDetails(input);
    await this.clickStep('Review'); // -> Review
    await this.clickStep('Create'); // submit
    // The scaffolder lands on a task page; the component is inserted into the
    // catalog immediately (no provider-sync wait).
    await this.page
      .getByRole('button', { name: 'View Component', exact: true })
      .waitFor({ state: 'visible', timeout: 60_000 });
  }

  // Click a wizard advance button and wait for it to disappear/re-render. Each
  // step's button differs (Next / Review / Create); waitFor rides out the
  // picker auto-select + re-render gap that isVisible() can't (its timeout arg
  // is a no-op).
  private async clickStep(label: string): Promise<void> {
    const btn = this.page.getByRole('button', { name: label, exact: true });
    await btn.waitFor({ state: 'visible', timeout: 15_000 });
    await btn.click();
  }

  async openByName(name: string): Promise<void> {
    await this.gotoComponentRoute(name);
  }

  // Navigate to a component's catalog entity route directly. This deliberately
  // uses a URL rather than click-through the catalog: the Backstage entity
  // route resolves as soon as the component is created, whereas the Kind picker
  // only lists the "Component" kind once at least one component has synced into
  // the catalog list — so the filtered-list click path can't reach a
  // freshly-created component. Reload-retry rides out the brief post-create
  // "Entity not found" window.
  private async gotoComponentRoute(name: string, suffix = ''): Promise<void> {
    await expect
      .poll(
        async () => {
          await this.page.goto(`/catalog/default/component/${name}${suffix}`);
          const notFound = await this.page
            .getByText(/Entity not found/i)
            .isVisible({ timeout: 8_000 })
            .catch(() => false);
          return !notFound;
        },
        { timeout: 90_000, intervals: [3_000] },
      )
      .toBe(true);
  }

  // Deploy tab is a graph canvas, not a "Deploy" button. The flow is:
  //   Set up node -> Create release (opens workload-config) -> [set args] ->
  //   Continue -> confirm "Create release" dialog -> Set up -> Deploy (panel)
  //   -> overrides page -> Deploy (confirm).
  async deployTo(
    componentName: string,
    environment: string,
    opts: DeployOptions = {},
  ): Promise<void> {
    // Tolerate the post-create catalog-entity race before the graph renders.
    await this.gotoComponentRoute(componentName, '/environments');
    await this.openSetupPanel();

    // --- Create a release from the current workload ---
    await this.page
      .getByRole('button', { name: 'Create release', exact: true })
      .click();
    if (opts.args?.length) {
      // Container tab is selected by default in the workload-config panel.
      await this.page
        .getByRole('textbox', { name: 'Comma-separated arguments' })
        .fill(opts.args.join(','));
    }
    // The advance button reads "Continue" when nothing changed and
    // "Save & continue" once the workload was edited (e.g. args set above).
    await this.page
      .getByRole('button', { name: /continue/i })
      .first()
      .click();
    // An edited workload first surfaces a "Confirm Save Changes" dialog whose
    // primary action is "Save & Continue"; an unedited one goes straight to
    // the release-name dialog.
    const confirmSave = this.page
      .getByRole('dialog')
      .getByRole('button', { name: /save & continue/i });
    if (await confirmSave.isVisible({ timeout: 5_000 }).catch(() => false)) {
      await confirmSave.click();
    }
    // Release-name dialog (name optional) — confirm to snapshot the release.
    await this.page
      .getByRole('dialog')
      .getByRole('button', { name: 'Create release', exact: true })
      .click();
    // A dialog stuck open means the release was not snapshotted — fail here
    // with a precise error rather than letting the Deploy step time out.
    // waitFor(hidden) also resolves when the dialog detaches entirely.
    await this.page
      .getByRole('dialog')
      .waitFor({ state: 'hidden', timeout: 30_000 });

    // --- Deploy the release to the environment ---
    await this.openSetupPanel();
    const panelDeploy = this.page
      .getByRole('button', { name: 'Deploy', exact: true })
      .first();
    await expect(panelDeploy).toBeEnabled({ timeout: 30_000 });
    await panelDeploy.click();
    await this.page.waitForURL(
      new RegExp(`overrides/${environment}`),
      { timeout: 15_000 },
    );
    // Confirm on the per-environment overrides page.
    await this.page
      .getByRole('button', { name: 'Deploy', exact: true })
      .first()
      .click();
  }

  // Open the "Set up" node's detail panel if it isn't already open. Clicking an
  // already-pressed node would toggle it closed, so guard on aria-pressed.
  private async openSetupPanel(): Promise<void> {
    const setup = this.page.getByRole('button', { name: /Select setup/i });
    await setup.waitFor({ state: 'visible', timeout: 30_000 });
    if ((await setup.getAttribute('aria-pressed')) !== 'true') {
      await setup.click();
    }
  }

  async promoteTo(environment: string): Promise<void> {
    await this.page.getByRole('tab', { name: /Deploy/i }).click();
    await this.page
      .getByRole('button', { name: `Actions for ${environment}`, exact: true })
      .click();
    await this.page.getByRole('menuitem', { name: /Promote/i }).click();
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
