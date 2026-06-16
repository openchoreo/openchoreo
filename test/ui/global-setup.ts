// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { chromium, type FullConfig } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { dirname } from 'node:path';
import { ROLES, cryptoUUIDPolyfill, signIn, storageStateFor, type Role } from './fixtures/auth';
import { hostResolverRules } from './fixtures/hosts';

// Mint per-role storage-state files once, before any test workers spin up.
// `test.use({ storageState })` is resolved when Playwright builds a browser
// context for the test, which happens *before* beforeAll runs — so minting
// inside beforeAll is too late. globalSetup is the right hook.
//
// Idempotent: if the file already exists from a previous run, we skip
// re-signing in. Delete .auth/ to force a fresh run.
export default async function globalSetup(config: FullConfig): Promise<void> {
  // In ext-idp mode Thunder is replaced by Dex. No Thunder role storage
  // states are needed — the ext-idp spec drives sign-in directly.
  if (process.env.E2E_WITH_EXT_IDP === 'true') return;

  const baseURL =
    config.projects[0]?.use.baseURL ??
    process.env.UI_BASE_URL ??
    'http://openchoreo.e2e-cp.local:28080';

  const roles: Role[] = ['pe', 'dev', 'abac'];

  for (const role of roles) {
    const path = storageStateFor(role);
    try {
      // Fast-path: don't re-sign-in if the file is already on disk.
      // eslint-disable-next-line @typescript-eslint/no-require-imports
      const { existsSync } = require('node:fs') as typeof import('node:fs');
      if (existsSync(path)) continue;
    } catch {
      /* fall through */
    }

    const browser = await chromium.launch({
      args: [`--host-resolver-rules=${hostResolverRules}`],
    });
    const ctx = await browser.newContext({ baseURL });
    await ctx.addInitScript(cryptoUUIDPolyfill);
    const page = await ctx.newPage();
    try {
      await signIn(page, ctx, ROLES[role]);
      mkdirSync(dirname(path), { recursive: true });
      await ctx.storageState({ path });
    } catch (err) {
      // Capture diagnostics so this is debuggable without re-running headed.
      try {
        mkdirSync(dirname(path), { recursive: true });
        const dbgBase = `${dirname(path)}/${role}-failed`;
        await page.screenshot({ path: `${dbgBase}.png`, fullPage: true });
        const html = await page.content();
        // eslint-disable-next-line @typescript-eslint/no-require-imports
        const { writeFileSync } = require('node:fs') as typeof import('node:fs');
        writeFileSync(`${dbgBase}.html`, html);
        writeFileSync(
          `${dbgBase}.url.txt`,
          `${page.url()}\n${(err as Error).message}\n`,
        );
        // eslint-disable-next-line no-console
        console.log(`[global-setup] dumped diagnostics to ${dbgBase}.{png,html,url.txt}`);
      } catch {
        /* best-effort dump */
      }
      // The ABAC identity has no provisioning path yet — Thunder's admin API
      // is auth-gated, so it isn't part of the bootstrap-seeded users. Don't
      // fail the whole run when only that identity is missing — the abac-ui
      // spec self-skips when its storage state file isn't on disk.
      if (role === 'abac') {
        // eslint-disable-next-line no-console
        console.warn(
          '[global-setup] abac sign-in failed; abac-ui specs will self-skip',
        );
      } else {
        throw err;
      }
    } finally {
      await page.close().catch(() => undefined);
      await ctx.close().catch(() => undefined);
      await browser.close().catch(() => undefined);
    }
  }
}
