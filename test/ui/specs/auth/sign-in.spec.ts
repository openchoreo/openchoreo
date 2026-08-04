// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Importing from ../../fixtures/auth (not @playwright/test) gets us the
// context override that injects the crypto.randomUUID polyfill before any
// page script — required because Backstage's frontend uses
// window.crypto.randomUUID() and the e2e portal isn't a secure context.
import { test, expect, ROLES } from '../../fixtures/auth';

// Pull the PE credentials from the shared role catalogue so credential
// changes stay centralized in fixtures/auth.ts.
const { username: PE_USERNAME, password: PE_PASSWORD } = ROLES.pe;

test.describe('backstage sign-in', () => {
  test('signs in via Thunder OIDC and lands on the post-login layout', async ({
    page,
  }) => {
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
  });
});
