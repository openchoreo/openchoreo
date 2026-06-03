// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { defineConfig, devices } from '@playwright/test';
// Mapped into Chromium via --host-resolver-rules below so the suite runs
// without /etc/hosts edits. Shared with global-setup.ts.
import { hostResolverRules } from './fixtures/hosts';

export default defineConfig({
  testDir: './specs',
  // globalSetup mints per-role storage-state files in .auth/ before any
  // worker starts — test.use({ storageState }) only resolves after the
  // files exist on disk, so this can't live in a beforeAll hook.
  globalSetup: './global-setup.ts',
  timeout: 60_000,
  expect: { timeout: 10_000 },

  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,

  reporter: [
    ['list'],
    ['html', { open: 'never', outputFolder: '_report' }],
  ],

  use: {
    baseURL: process.env.UI_BASE_URL ?? 'http://openchoreo.e2e-cp.local:28080',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    actionTimeout: 15_000,
    navigationTimeout: 30_000,
  },

  // Backstage's frontend bundle calls window.crypto.randomUUID() which is
  // only exposed in a "secure context". The e2e portal is plain HTTP, so a
  // polyfill is injected via an init script — see fixtures/auth.ts (test
  // contexts) and global-setup.ts (sign-in mint context).

  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        launchOptions: {
          args: [`--host-resolver-rules=${hostResolverRules}`],
          ...(process.env.PWSLOWMO && { slowMo: Number(process.env.PWSLOWMO) }),
        },
      },
    },
  ],

  outputDir: '_test-results',
});
