// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Single source of truth for the e2e hostnames the suite resolves to
// 127.0.0.1 (so no /etc/hosts edit is needed). Consumed by both the
// Playwright config (test browser launch) and global-setup.ts (the
// storage-state minting browser) — keep them resolving identical domains.
export const E2E_HOSTS = [
  'openchoreo.e2e-cp.local',
  'thunder.e2e-cp.local',
  'dex.e2e-cp.local',
  'api.e2e-cp.local',
  'observer.e2e-op.local',
];

// Chromium --host-resolver-rules value mapping every e2e host to loopback.
export const hostResolverRules = E2E_HOSTS.map(h => `MAP ${h} 127.0.0.1`).join(
  ', ',
);
