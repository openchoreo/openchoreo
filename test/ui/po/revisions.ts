// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { expect, type Locator, type Page } from '@playwright/test';
import { ComponentPO } from './component';

// One row of the environment side-panel's revision timeline, as read from
// its accessible name. The row's visible text is split across badges, so the
// UI spells it out in aria-label, starting with the revision's name,
// {release}-r{N}:
//   "frontend-6f9c2a-r3, Healthy, cause Deploy, active, known good"
export interface RevisionRow {
  name: string;
  revision: number;
  release: string;
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
    release: m[2],
    revision: Number(m[3]),
    health: m[4],
    cause: m[5],
    active: flags.includes('active'),
    knownGood: flags.includes('known good'),
  };
}

// Matches a revision name, {release}-r{N}, by its number.
const named = (revision: number) => `\\S+-r${revision}`;

// Revision history in the Deploy tab's environment side panel: the timeline
// (a listbox named "Revisions" whose options are revisions, newest first),
// the revision browser dialog, and the dry-run rollback confirmation.
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
    await expect(this.picker()).toBeVisible({ timeout: 30_000 });
  }

  // The revision picker, built like the Deploy panel's release picker: the
  // field shows the selected revision (the active one by default) and its
  // dropdown, a listbox named "Revisions", lists the revisions newest first.
  picker(): Locator {
    return this.page.getByRole('textbox', { name: 'Revision', exact: true });
  }

  timeline(): Locator {
    return this.page.getByRole('listbox', { name: 'Revisions', exact: true });
  }

  async openTimeline(): Promise<void> {
    if (!(await this.timeline().isVisible())) {
      await this.picker().click();
    }
    await expect(this.timeline()).toBeVisible();
  }

  async closeTimeline(): Promise<void> {
    if (await this.timeline().isVisible()) {
      await this.picker().press('Escape');
    }
    await expect(this.timeline()).toBeHidden();
  }

  row(revision: number): Locator {
    return this.timeline().getByRole('option', {
      name: new RegExp(`^${named(revision)},`),
    });
  }

  // The accessible label is on each option's content.
  private async label(option: Locator): Promise<string> {
    return (
      (await option.locator('[data-testid^="revision-row-"]').getAttribute('aria-label')) ?? ''
    );
  }

  // The revisions as the dropdown lists them, top to bottom.
  async readTimeline(): Promise<RevisionRow[]> {
    await this.openTimeline();
    const options = await this.timeline().getByRole('option').all();
    const labels = await Promise.all(options.map(o => this.label(o)));
    await this.closeTimeline();
    return labels.map(parseRevisionRow);
  }

  async rowState(revision: number): Promise<RevisionRow> {
    await this.openTimeline();
    const label = await this.label(this.row(revision));
    await this.closeTimeline();
    return parseRevisionRow(label);
  }

  // ── Revision browser ────────────────────────────────────────────────

  // The Revision Browser, built like the Release Browser: a list of
  // revisions on the left, the highlighted revision on the right with View
  // and Compare tabs.
  browser(): Locator {
    return this.page.getByRole('dialog', { name: /^Select revision/ });
  }

  // The Revision Browser opens from the picker dropdown's footer.
  async openBrowser(): Promise<void> {
    await this.openTimeline();
    await this.page.getByText('Open Revision Browser', { exact: true }).click();
    await expect(this.browser()).toBeVisible();
  }

  // Highlight `target` in the list, switch to Compare, and compare it with
  // `base` picked in "Compare with". The diff shows the two full
  // ReleaseBindingRevision objects: base on the left, target on the right.
  async compare(base: number, target: number): Promise<void> {
    await this.browser()
      .getByRole('button', { name: new RegExp(`^${named(target)},`) })
      .click();
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

  async closeBrowser(): Promise<void> {
    await this.browser().getByRole('button', { name: 'Close' }).first().click();
    await expect(this.browser()).toBeHidden();
  }

  // ── Rollback ────────────────────────────────────────────────────────

  confirmDialog(revision: number): Locator {
    return this.page.getByRole('dialog', {
      name: new RegExp(`^Roll back .* to ${named(revision)}\\?$`),
    });
  }

  // Pick a revision in the dropdown and open the rollback confirmation.
  // The dialog runs a dry run on open; returns the warnings it shows so the
  // spec can assert them before confirming.
  async startRollback(revision: number): Promise<string[]> {
    await this.openTimeline();
    await this.row(revision).click();
    await expect(this.picker()).toHaveValue(new RegExp(`-r${revision}$`));
    await this.page
      .getByRole('button', { name: `Roll back to r${revision}`, exact: true })
      .click();
    const dialog = this.confirmDialog(revision);
    await expect(dialog).toBeVisible();
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
