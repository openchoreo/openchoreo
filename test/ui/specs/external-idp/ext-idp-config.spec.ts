// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// External IdP configuration spec.
// Validates that OpenChoreo can be configured to use an external OIDC provider
// (Dex) instead of the built-in Thunder IdP, and that a user with the "admins"
// group from that provider receives admin-level access.
//
// Requires E2E_WITH_EXT_IDP=true when spinning up the cluster:
//   make e2e.setup E2E_WITH_EXT_IDP=true
//
// Dex is installed into the "dex" namespace.  The admin user is provisioned via
// staticPasswords with groups: [admins], matching the default ClusterAuthzRoleBinding.

import { test, expect, type Page } from '@playwright/test';
import { cryptoUUIDPolyfill } from '../../fixtures/auth';
import { CatalogTablePO } from '../../po/catalogTable';

const DEX_ADMIN_EMAIL = 'admin@openchoreo.dev';
const DEX_ADMIN_PASSWORD = 'Admin@123';

// Drive sign-in through Dex's local login page.
// Dex renders different form placeholders from Thunder:
//   "Email Address" and "Password" instead of Thunder's "Enter your username" / "Enter your password".
async function signInViaDex(page: Page, email: string, password: string): Promise<void> {
  await page.goto('/');

  // The Backstage pre-login layout shows a "Sign In" button for the configured provider.
  await page.getByRole('button', { name: 'Sign In', exact: true }).first().click();

  // Wait for Dex's login form (redirected from Backstage).
  const emailField = page.getByPlaceholder('Email Address');
  await emailField.waitFor({ state: 'visible', timeout: 30_000 });
  await emailField.fill(email);
  await page.getByPlaceholder('Password').fill(password);
  await page.getByRole('button', { name: 'Login' }).click();

  // Wait for the redirect back to Backstage; the Home link in the sidebar is the
  // readiness signal (same pattern as fixtures/auth.ts signIn).
  await page.getByRole('link', { name: 'Home' }).first()
    .waitFor({ state: 'visible', timeout: 60_000 });
}

test.describe('external identity provider (Dex)', () => {
  test.describe.configure({ mode: 'serial' });

  test.beforeEach(async ({ context }) => {
    // Backstage uses window.crypto.randomUUID() which requires a secure context.
    // Inject the same polyfill used by the other UI suites.
    await context.addInitScript(cryptoUUIDPolyfill);
  });

  // page.goto goes through Chromium (respecting --host-resolver-rules) with
  // no CORS constraints. response.text() gives the raw body, bypassing
  // Chrome's JSON viewer which changes the DOM structure.
  test('Dex OIDC discovery endpoint is reachable', async ({ page }) => {
    const response = await page.goto(
      'http://dex.e2e-cp.local:28080/dex/.well-known/openid-configuration',
    );
    expect(response?.status()).toBe(200);
    const body = JSON.parse((await response?.text()) ?? '{}') as Record<string, string>;
    expect(body.issuer).toBe('http://dex.e2e-cp.local:28080/dex');
    expect(body.authorization_endpoint).toContain('/dex/auth');
    expect(body.token_endpoint).toContain('/dex/token');
    expect(body.jwks_uri).toContain('/dex/keys');
  });

  test('admin user signs in via Dex and lands on Backstage', async ({ page }) => {
    await signInViaDex(page, DEX_ADMIN_EMAIL, DEX_ADMIN_PASSWORD);

    // Verify we are on Backstage, not on an error page.
    await expect(page.getByRole('link', { name: 'Home' }).first()).toBeVisible();
    await expect(page).not.toHaveURL(/dex\.e2e-cp\.local/);
  });

  test('signed-in admin is not redirected back to the login page', async ({ page }) => {
    await signInViaDex(page, DEX_ADMIN_EMAIL, DEX_ADMIN_PASSWORD);

    // Navigate to a page that requires authentication (Backstage Settings).
    // A user without a valid session is redirected back to "/" which shows
    // the "Sign In" button. If we stay on /settings the session is valid.
    await page.goto('/settings');
    await page.waitForURL(/settings/, { timeout: 15_000 });

    // There must be no Sign In button — that would mean the session was not recognised.
    await expect(page.getByRole('button', { name: 'Sign In', exact: true })).not.toBeVisible();
  });

  test('default project is listed in the catalog', async ({ page }) => {
    await signInViaDex(page, DEX_ADMIN_EMAIL, DEX_ADMIN_PASSWORD);

    const catalog = new CatalogTablePO(page);
    // The catalog mounts on System (Project) by default; gotoKind reinforces that.
    await catalog.gotoKind('system');
    // The seeded default project has metadata.title = "Default Project".
    await catalog.expectListed('Default Project');
  });

  test('cluster data planes are listed in the catalog', async ({ page }) => {
    await signInViaDex(page, DEX_ADMIN_EMAIL, DEX_ADMIN_PASSWORD);

    const catalog = new CatalogTablePO(page);
    await catalog.gotoKind('clusterdataplane');
    await catalog.expectListed('default');
    await catalog.expectListed('e2e-shared');
  });
});
