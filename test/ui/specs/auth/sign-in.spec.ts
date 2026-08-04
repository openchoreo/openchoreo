// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Importing from ../../fixtures/auth (not @playwright/test) gets us the
// context override that injects the crypto.randomUUID polyfill before any
// page script — required because Backstage's frontend uses
// window.crypto.randomUUID() and the e2e portal isn't a secure context.
import { test, expect, ROLES } from '../../fixtures/auth';
import { kLogs } from '../../fixtures/kube';

// Pull the PE credentials from the shared role catalogue so credential
// changes stay centralized in fixtures/auth.ts.
const { username: PE_USERNAME, password: PE_PASSWORD } = ROLES.pe;

const CP_NS = 'openchoreo-control-plane';

test.describe('backstage sign-in', () => {
  test('signs in via Thunder OIDC and lands on the post-login layout', async ({
    page,
  }) => {
    // Bound the backend log scan below to this sign-in only.
    const testStart = new Date();

    await page.goto('/');

    // Pre-login layout exposes one Sign In affordance per provider; this
    // install only has the OpenChoreo provider, so .first() is safe.
    await page.getByRole('button', { name: 'Sign In', exact: true }).first().click();

    // Thunder's gate page uses these placeholder strings — pinning to them
    // keeps us off the toggle-visibility icon button that getByLabel matches.
    await page.getByPlaceholder('Enter your username').fill(PE_USERNAME);
    await page.getByPlaceholder('Enter your password').fill(PE_PASSWORD);
    await page.getByRole('button', { name: 'Sign In', exact: true }).click();

    // Post-login: Home link appears in the Backstage sidebar.
    await expect(page.getByRole('link', { name: 'Home' }).first()).toBeVisible({
      timeout: 60_000,
    });
    await expect(page).toHaveTitle(/openchoreo|backstage/i);

    // Regression guard (backstage-plugins#725): the resolver's capability
    // pre-cache is a backend->backend POST to the permission plugin —
    // invisible to the browser, and its failure is swallowed as a warning
    // while sign-in still succeeds. Under the 1.2.1 bug it 401'd on every
    // sign-in. The backend log is the only observable; poll briefly to ride
    // out the log-flush gap after the UI turns interactive.
    await expect
      .poll(() => kLogs(CP_NS, 'deploy/backstage', { sinceTime: testStart }), {
        timeout: 30_000,
        intervals: [2_000],
      })
      .toContain('Pre-cached capabilities for');
    const logs = kLogs(CP_NS, 'deploy/backstage', { sinceTime: testStart });
    expect(logs).not.toContain('Failed to pre-cache capabilities');
  });
});
