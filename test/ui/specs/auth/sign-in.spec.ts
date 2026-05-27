// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test, expect } from '@playwright/test';

const PE_USERNAME = 'platform-engineer@openchoreo.dev';
const PE_PASSWORD = 'PE@123';

test.describe('backstage sign-in', () => {
  test('signs in via Thunder OIDC and lands on the post-login layout', async ({
    page,
    context,
  }) => {
    // Backstage opens the consent popup during the initial /refresh probe,
    // before the on-page Sign In button renders. Arm the listener before
    // navigation so we don't miss it. Clicking Sign In afterwards races
    // the warm popup for the same flowId in Thunder's SQLite and fails
    // with SQLITE_BUSY → server_error.
    const popupPromise = context.waitForEvent('page', { timeout: 30_000 });
    await page.goto('/');
    const consent = await popupPromise;
    await consent.waitForLoadState('domcontentloaded');
    expect(consent.url()).toContain('/gate/signin');

    // getByLabel('Password') also matches the toggle-visibility icon
    // button, so pin to the visible placeholder copy.
    await consent.getByPlaceholder('Enter your username').fill(PE_USERNAME);
    await consent.getByPlaceholder('Enter your password').fill(PE_PASSWORD);
    await consent.getByRole('button', { name: 'Sign In', exact: true }).click();

    await consent.waitForEvent('close', { timeout: 30_000 });

    await expect(page.getByRole('button', { name: 'Sign In' })).toBeHidden();
    await expect(page.getByRole('link', { name: 'Home' }).first()).toBeVisible();
    await expect(page).toHaveTitle(/openchoreo|backstage/i);
  });
});
