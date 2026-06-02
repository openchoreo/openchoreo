// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test, expect, ROLES } from '../../fixtures/auth';
import { spawn, spawnSync, type SpawnSyncReturns } from 'node:child_process';
import { chmodSync, existsSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { lookup } from 'node:dns/promises';

// occ runs as a host process — unlike the browser specs, it can't use
// Chromium's --host-resolver-rules, so it needs the e2e control-plane
// hostname to resolve via real DNS (an /etc/hosts entry mapping
// openchoreo.e2e-cp.local / api.e2e-cp.local → 127.0.0.1). When that's
// absent (the suite's default, since the browser specs don't need it), this
// spec self-skips rather than failing — same pattern as abac-ui.
// NOTE: deliberately not falling back to UI_BASE_URL — that is the Backstage
// portal URL, not the OpenChoreo API; occ must talk to the API hostname.
function controlPlaneURL(): string {
  return (
    process.env.OCC_CONTROL_PLANE_URL ?? 'http://api.e2e-cp.local:28080'
  );
}

function controlPlaneHost(): string {
  return new URL(controlPlaneURL()).hostname;
}

async function hostResolvable(host: string): Promise<boolean> {
  try {
    await lookup(host);
    return true;
  } catch {
    return false;
  }
}

// occ does not have a --no-browser flag today; the authorization URL is
// always printed to stdout alongside the (best-effort) browser launch. We
// suppress the real browser launch by prepending a tmp dir to PATH that
// contains no-op `open`/`xdg-open`/`cmd` shims. Playwright drives the URL
// captured from stdout instead.
function noopBrowserPath(): string {
  const dir = mkdtempSync(join(tmpdir(), 'occ-no-browser-'));
  for (const name of ['open', 'xdg-open', 'cmd']) {
    const path = join(dir, name);
    writeFileSync(path, '#!/usr/bin/env bash\nexit 0\n');
    chmodSync(path, 0o755);
  }
  return dir;
}

function resolveOccBinary(): string {
  // Prefer an explicit override, then the `make go.build.occ` output
  // (bin/dist/<os>/<arch>/occ), then a manually-placed bin/occ.
  if (process.env.OCC_BIN) return process.env.OCC_BIN;
  const repoRoot = join(__dirname, '..', '..', '..', '..');
  const os =
    process.platform === 'darwin'
      ? 'darwin'
      : process.platform === 'win32'
        ? 'windows'
        : 'linux';
  const arch = process.arch === 'x64' ? 'amd64' : process.arch;
  const dist = join(repoRoot, 'bin', 'dist', os, arch, 'occ');
  if (existsSync(dist)) return dist;
  return join(repoRoot, 'bin', 'occ');
}

test.describe('pkce-login: occ login drives PKCE through the Backstage browser', () => {
  test('occ login → consent → token persisted → occ get components succeeds', async ({
    page,
  }) => {
    const cpHost = controlPlaneHost();
    test.skip(
      !(await hostResolvable(cpHost)),
      `occ needs ${cpHost} to resolve on the host — add an /etc/hosts entry ` +
        `(${cpHost} 127.0.0.1) to run this spec. The browser specs don't need it.`,
    );

    const occ = resolveOccBinary();
    const noopDir = noopBrowserPath();

    // Isolated HOME so the spec doesn't trample the user's ~/.occ. occ
    // resolves the control-plane URL from its config file (~/.occ), not from
    // env vars, so we bootstrap a context inside this isolated HOME before
    // running `occ login`.
    const env: NodeJS.ProcessEnv = {
      ...process.env,
      PATH: `${noopDir}:${process.env.PATH ?? ''}`,
      HOME: mkdtempSync(join(tmpdir(), 'occ-home-')),
    };

    const cpURL = controlPlaneURL();

    const must = (r: SpawnSyncReturns<string>, label: string) => {
      if (r.status !== 0) {
        throw new Error(
          `${label} exited ${r.status}: ${r.stderr || r.stdout}`,
        );
      }
    };

    must(
      spawnSync(occ, ['config', 'controlplane', 'add', 'e2e', '--url', cpURL], {
        env,
        encoding: 'utf8',
      }),
      'controlplane add',
    );
    must(
      spawnSync(occ, ['config', 'context', 'add', 'e2e', '--controlplane', 'e2e', '--credentials', 'ui-pkce'], {
        env,
        encoding: 'utf8',
      }),
      'context add',
    );
    // Activate the e2e context — otherwise occ falls back to the seeded
    // `default` context whose URL points at localhost:8080.
    must(
      spawnSync(occ, ['config', 'context', 'use', 'e2e'], {
        env,
        encoding: 'utf8',
      }),
      'context use',
    );

    const child = spawn(occ, ['login', '--credential', 'ui-pkce'], { env });

    const authURL = await new Promise<string>((resolve, reject) => {
      let stdout = '';
      let stderr = '';
      const timer = setTimeout(
        () => reject(new Error(`timed out waiting for auth URL on stdout\n--- stdout ---\n${stdout}\n--- stderr ---\n${stderr}`)),
        30_000,
      );
      child.stdout.on('data', (chunk: Buffer) => {
        stdout += chunk.toString('utf8');
        const m = stdout.match(/https?:\/\/[^\s]+\/oauth2\/authorize\S*/);
        if (m) {
          clearTimeout(timer);
          resolve(m[0]);
        }
      });
      child.stderr.on('data', (chunk: Buffer) => {
        stderr += chunk.toString('utf8');
      });
      child.on('error', err => {
        clearTimeout(timer);
        reject(err);
      });
      child.on('close', code => {
        clearTimeout(timer);
        reject(
          new Error(
            `occ login exited ${code} before printing auth URL\n--- stdout ---\n${stdout}\n--- stderr ---\n${stderr}`,
          ),
        );
      });
    });

    // Drive the consent form via Playwright. Reuse the PE identity — the
    // pkce flow only validates the browser leg, not RBAC.
    await page.goto(authURL);
    await page
      .getByPlaceholder('Enter your username')
      .fill(ROLES.pe.username);
    await page
      .getByPlaceholder('Enter your password')
      .fill(ROLES.pe.password);
    await page.getByRole('button', { name: 'Sign In', exact: true }).click();

    // After the consent submission, Thunder redirects to occ's local
    // callback (http://127.0.0.1:<port>/callback). The browser shows the
    // success page rendered by occ's local server; the CLI process should
    // then exit 0.
    const exitCode = await new Promise<number>(resolve => {
      child.on('close', code => resolve(code ?? 1));
    });
    expect(exitCode).toBe(0);

    // Verify a token round-trip via a follow-on occ call (occ has no
    // kubectl-style `get` verb — listing is noun-first and project-scoped;
    // the `default` namespace + project ship with the e2e seed resources).
    const r = spawnSync(
      occ,
      ['component', 'list', '--namespace', 'default', '--project', 'default'],
      { env, encoding: 'utf8' },
    );
    expect(r.status, r.stderr || r.stdout).toBe(0);
  });
});
