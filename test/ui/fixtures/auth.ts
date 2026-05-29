// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test as base, type BrowserContext, type Page } from '@playwright/test';
import { existsSync, mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';

// Polyfill injected before any page script. Backstage uses
// window.crypto.randomUUID(), which is gated by "secure context" (HTTPS or
// localhost). The e2e portal serves over plain HTTP at openchoreo.e2e-cp.local
// so the upstream code throws; we replace it with a v4-shaped stub built on
// crypto.getRandomValues (always available). Not cryptographically meaningful
// — Backstage only uses these as cache / DOM keys.
export const cryptoUUIDPolyfill = `
(() => {
  if (typeof window === 'undefined' || !window.crypto) return;
  if (typeof window.crypto.randomUUID === 'function') return;
  window.crypto.randomUUID = function randomUUID() {
    const buf = new Uint8Array(16);
    window.crypto.getRandomValues(buf);
    buf[6] = (buf[6] & 0x0f) | 0x40;
    buf[8] = (buf[8] & 0x3f) | 0x80;
    const hex = Array.from(buf, b => b.toString(16).padStart(2, '0'));
    return (
      hex.slice(0, 4).join('') + '-' +
      hex.slice(4, 6).join('') + '-' +
      hex.slice(6, 8).join('') + '-' +
      hex.slice(8, 10).join('') + '-' +
      hex.slice(10, 16).join('')
    );
  };
})();
`;

// Identity catalogue. PE + Dev are seeded by Thunder's bootstrap script
// (install/k3d/common/values-thunder.yaml → 50-user-schema-and-users.sh).
// The ABAC identity is provisioned by test/ui/scripts/seed-idp-users.sh
// because the bootstrap script does not know about it.
export type Role = 'pe' | 'dev' | 'abac';

export interface RoleCreds {
  username: string;
  password: string;
  group: string;
}

export const ROLES: Record<Role, RoleCreds> = {
  pe: {
    username: 'platform-engineer@openchoreo.dev',
    password: 'PE@123',
    group: 'platform-engineers',
  },
  dev: {
    username: 'developer@openchoreo.dev',
    password: 'Dev@123',
    group: 'developers',
  },
  abac: {
    username: 'abac-dev@openchoreo.dev',
    password: 'Abac@123',
    group: 'abac-developers',
  },
};

// Persisted Backstage auth state per role. Specs opt in with
//   test.use({ storageState: storageStateFor('pe') })
// after the suite has invoked signInAs(role) once to mint it.
export function storageStateFor(role: Role): string {
  const out = resolve(__dirname, '..', '.auth', `${role}.json`);
  return out;
}

// Drive the Thunder OIDC consent page via the on-page Sign In button. The
// current Backstage configuration uses a same-page redirect flow rather than
// a popup, so the spec just clicks the Sign In affordance Backstage renders
// pre-login and fills the form on the resulting Thunder consent page.
export async function signIn(
  page: Page,
  _context: BrowserContext,
  creds: RoleCreds,
): Promise<void> {
  await page.goto('/');

  // Backstage's pre-login layout shows one or more Sign In buttons (one per
  // provider). Click the first one — there is only the OpenChoreo provider
  // configured in this install.
  await page.getByRole('button', { name: 'Sign In', exact: true }).first().click();

  // Wait for Thunder's gate to render. The placeholder strings are stable
  // across the deployment because they come from the Thunder gate template.
  const usernameField = page.getByPlaceholder('Enter your username');
  await usernameField.waitFor({ state: 'visible', timeout: 30_000 });
  await usernameField.fill(creds.username);
  await page.getByPlaceholder('Enter your password').fill(creds.password);
  await page.getByRole('button', { name: 'Sign In', exact: true }).click();

  // Wait for the redirect back to Backstage. The post-login shell exposes
  // the Home link in the sidebar — wait for that as the readiness signal.
  // Match what specs/auth/sign-in.spec.ts does (no exact:true) so the same
  // call site succeeds whether the Home affordance comes from the sidebar
  // logo (<a aria-label="Home">) or the SidebarItem (text="Home").
  await page.getByRole('link', { name: 'Home' }).first()
    .waitFor({ state: 'visible', timeout: 60_000 });
}

// Mint a storageState file for a role by driving sign-in once and saving the
// browser context. Cached on disk so subsequent specs in the run reuse it.
async function mintStorageState(
  context: BrowserContext,
  role: Role,
): Promise<string> {
  const path = storageStateFor(role);
  if (existsSync(path)) return path;

  const page = await context.newPage();
  await signIn(page, context, ROLES[role]);
  mkdirSync(dirname(path), { recursive: true });
  await context.storageState({ path });
  await page.close();
  return path;
}

export interface AuthFixtures {
  signInAs: (role: Role) => Promise<{ page: Page }>;
  mintAuthState: (role: Role) => Promise<string>;
}

export const test = base.extend<AuthFixtures>({
  // Inject the crypto.randomUUID polyfill into every page created in this
  // context before any spec code runs. Without it, the Backstage scaffolder
  // pickers throw and the form stalls.
  context: async ({ context }, use) => {
    await context.addInitScript(cryptoUUIDPolyfill);
    await use(context);
  },

  // Drive a fresh sign-in in the current browser context. Use this when a spec
  // needs to assert sign-in behavior itself (the auth/sign-in spec).
  signInAs: async ({ page, context }, use) => {
    await use(async (role: Role) => {
      await signIn(page, context, ROLES[role]);
      return { page };
    });
  },

  // Mint (or reuse) a persisted storageState file for a role. Specs that just
  // want a logged-in session should call this in a beforeAll and then
  // test.use({ storageState }) rather than signing in for every test.
  mintAuthState: async ({ browser }, use) => {
    await use(async (role: Role) => {
      const ctx = await browser.newContext();
      const path = await mintStorageState(ctx, role);
      await ctx.close();
      return path;
    });
  },
});

export { expect } from '@playwright/test';
