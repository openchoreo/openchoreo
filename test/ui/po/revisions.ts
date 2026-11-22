// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, type Locator, type Page } from '@playwright/test';
import { ComponentPO } from './component';

// One revision in the Revision Browser's list, as read from its accessible
// name. The row's visible text is split across badges, so the UI spells it
// out in aria-label, starting with the revision's name, {component}-r{N}:
//   "frontend-r3, Healthy, cause Deploy, active, known good"
export interface RevisionRow {
  name: string;
  revision: number;
  /** The component (or resource) the name starts with. */
  owner: string;
  health: string;
  cause: string;
  active: boolean;
  knownGood: boolean;
}

const ROW_NAME = /^((\S+)-r(\d+)), (\w+), cause (\w+)((?:, [\w ]+)*)$/;

export function parseRevisionRow(name: string): RevisionRow {
  const m = ROW_NAME.exec(name);
  if (!m) throw new Error(`unexpected revision row name: ${name}`);
  const flags = m[6].split(', ').filter(Boolean);
  return {
    name: m[1],
    owner: m[2],
    revision: Number(m[3]),
    health: m[4],
    cause: m[5],
    active: flags.includes('active'),
    knownGood: flags.includes('known good'),
  };
}

// Matches a revision name, {component}-r{N}, by its number.
const named = (revision: number) => `\\S+-r${revision}`;

// Revision history in the Deploy tab's environment side panel: the active
// revision's name, the Roll back action that opens the Revision Browser on
// it, the browser (a list of revisions newest first, the highlighted one's
// details with View and Compare), and the dry-run rollback confirmation.
export class RevisionsPO {
  constructor(private readonly page: Page) {}

  // Open the component's entity page and switch to the Deploy tab through
  // the tab bar. ComponentPO.openByName rides out the catalog-sync window of
  // a kubectl-seeded component.
  async openDeployTab(componentName: string): Promise<void> {
    await new ComponentPO(this.page).openByName(componentName);
    await this.page.getByRole('tab', { name: /^deploy$/i }).click();
    await this.page
      .getByRole('button', { name: /Select environment/i })
      .first()
      .waitFor({ state: 'visible', timeout: 30_000 });
  }

  // Select the environment node so its detail panel opens. The node toggles,
  // so only click when it is not already selected.
  async selectEnvironment(displayName: string): Promise<void> {
    const node = this.page.getByRole('button', {
      name: `Select environment ${displayName}`,
      exact: true,
    });
    if ((await node.getAttribute('aria-pressed')) !== 'true') {
      await node.click();
    }
    await expect(this.rollBackAction()).toBeVisible({ timeout: 30_000 });
  }

  // The panel's Actions entry that opens the Revision Browser.
  rollBackAction(): Locator {
    return this.page.getByRole('button', { name: 'Roll back', exact: true });
  }

  // The active revision's name, as the panel's Revision row shows it.
  activeRevisionName(): Locator {
    return this.page.getByTestId('active-revision-name');
  }

  // ── Revision browser ────────────────────────────────────────────────

  browser(): Locator {
    return this.page.getByRole('dialog', { name: /^Select revision/ });
  }

  // Roll back opens the Revision Browser on the active revision.
  async openBrowser(): Promise<void> {
    if (await this.browser().isVisible()) return;
    await this.rollBackAction().click();
    await expect(this.browser()).toBeVisible();
  }

  async closeBrowser(): Promise<void> {
    if (!(await this.browser().isVisible())) return;
    await this.browser().getByRole('button', { name: 'Close' }).first().click();
    await expect(this.browser()).toBeHidden();
  }

  row(revision: number): Locator {
    return this.browser().getByTestId(`browser-revision-${revision}`);
  }

  // The revisions as the browser lists them, top to bottom.
  async readTimeline(): Promise<RevisionRow[]> {
    await this.openBrowser();
    const revisionRows = this.browser().locator('[data-testid^="browser-revision-"]');
    // all() reads only the rows already rendered: wait for the list to load.
    await expect(revisionRows.first()).toBeVisible();
    const rows = await revisionRows.all();
    const labels = await Promise.all(
      rows.map(async r => (await r.getAttribute('aria-label')) ?? ''),
    );
    await this.closeBrowser();
    return labels.map(parseRevisionRow);
  }

  async rowState(revision: number): Promise<RevisionRow> {
    await this.openBrowser();
    const label = (await this.row(revision).getAttribute('aria-label')) ?? '';
    await this.closeBrowser();
    return parseRevisionRow(label);
  }

  // Highlight `target` in the list, switch to Compare, and compare it with
  // `base` picked in "Compare with". The diff shows the two full
  // ReleaseBindingRevision objects: base on the left, target on the right.
  async compare(base: number, target: number): Promise<void> {
    await this.openBrowser();
    await this.row(target).click();
    await this.browser()
      .getByRole('button', { name: 'Compare with another revision' })
      .click();
    const picker = this.browser().getByRole('textbox', { name: 'Compare with' });
    await picker.click();
    await this.page
      .getByRole('listbox')
      .getByRole('option', { name: new RegExp(`-r${base}\\b`) })
      .click();
  }

  // The diff's column headers name the two revisions: "<base> (<age>)" and
  // "<target>".
  async expectDiffBetween(base: number, target: number): Promise<void> {
    await expect(
      this.browser().getByText(new RegExp(`^${named(base)} \\(`)).last(),
    ).toBeVisible();
    await expect(
      this.browser().getByText(new RegExp(`^${named(target)}$`)).last(),
    ).toBeVisible();
  }

  async expectDiffShows(text: string): Promise<void> {
    await expect(this.browser().getByText(text).first()).toBeVisible();
  }

  // ── Rollback ────────────────────────────────────────────────────────

  confirmDialog(revision: number): Locator {
    return this.page.getByRole('dialog', {
      name: new RegExp(`^Roll back .* to ${named(revision)}\\?$`),
    });
  }

  // Highlight a revision in the browser and open its rollback confirmation,
  // which replaces the browser and runs a dry run on open.
  async openRollback(revision: number): Promise<Locator> {
    await this.openBrowser();
    await this.row(revision).click();
    await this.browser()
      .getByRole('button', { name: `Roll back to r${revision}`, exact: true })
      .click();
    const dialog = this.confirmDialog(revision);
    await expect(dialog).toBeVisible();
    return dialog;
  }

  // Open the confirmation and wait for its dry run; returns the warnings it
  // shows so the spec can assert them before confirming.
  async startRollback(revision: number): Promise<string[]> {
    const dialog = await this.openRollback(revision);
    // The confirm button only enables once the dry run returned.
    await expect(
      dialog.getByRole('button', { name: 'Roll back', exact: true }),
    ).toBeEnabled({ timeout: 30_000 });
    return dialog.getByRole('alert').allTextContents();
  }

  async confirmRollback(revision: number, reason?: string): Promise<void> {
    const dialog = this.confirmDialog(revision);
    if (reason) {
      await dialog.getByRole('textbox', { name: 'Reason (optional)' }).fill(reason);
    }
    await dialog.getByRole('button', { name: 'Roll back', exact: true }).click();
    await expect(dialog).toBeHidden({ timeout: 30_000 });
  }
}
