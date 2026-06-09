// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { spawnSync, type SpawnSyncOptionsWithStringEncoding } from 'node:child_process';

// Default to the e2e cluster context. Override with E2E_KUBECONTEXT in CI.
export const KUBE_CONTEXT =
  process.env.E2E_KUBECONTEXT ?? 'k3d-openchoreo-e2e';

export interface KubectlResult {
  status: number;
  stdout: string;
  stderr: string;
}

export interface KubectlOptions {
  context?: string;
  input?: string;
  // Throw on non-zero exit. Defaults to true.
  check?: boolean;
  // Optional override of the default 30s spawn timeout.
  timeoutMs?: number;
}

// Thin synchronous wrapper around `kubectl`. Specs use this to cross-check
// what the UI claims (e.g. "Active" status pill) against what the API has.
//
// Sync was deliberate: assertions are simple "kubectl get x -o json | jq ..."
// snapshots, the suite runs serially (workers: 1), and async kubectl adds
// no parallelism. expect.poll handles the few flows that need retrying.
export function kubectl(args: string[], opts: KubectlOptions = {}): KubectlResult {
  const ctx = opts.context ?? KUBE_CONTEXT;
  const spawnOpts: SpawnSyncOptionsWithStringEncoding = {
    encoding: 'utf8',
    timeout: opts.timeoutMs ?? 30_000,
    input: opts.input,
  };
  const result = spawnSync('kubectl', ['--context', ctx, ...args], spawnOpts);

  if (result.error) {
    throw new Error(`kubectl spawn failed: ${result.error.message}`);
  }
  const out: KubectlResult = {
    status: result.status ?? 1,
    stdout: result.stdout ?? '',
    stderr: result.stderr ?? '',
  };
  if (opts.check !== false && out.status !== 0) {
    throw new Error(
      `kubectl ${args.join(' ')} exited ${out.status}: ${out.stderr || out.stdout}`,
    );
  }
  return out;
}

// kubectl get <kind> <name> -n <ns> -o json, parsed.
export function kGetJSON<T = unknown>(
  kind: string,
  name: string,
  namespace = 'default',
  opts: KubectlOptions = {},
): T {
  const { stdout } = kubectl(
    ['get', kind, name, '-n', namespace, '-o', 'json'],
    opts,
  );
  return JSON.parse(stdout) as T;
}

// True when `kubectl get` returns NotFound. Use to assert post-delete state.
export function kNotFound(
  kind: string,
  name: string,
  namespace = 'default',
): boolean {
  const r = kubectl(
    ['get', kind, name, '-n', namespace, '--ignore-not-found', '-o', 'name'],
    { check: false },
  );
  if (r.status !== 0) return /NotFound/i.test(r.stderr);
  return r.stdout.trim() === '';
}

// kubectl get <kind> <name> -o json, with scope-aware namespace handling.
// Pass namespace="" for cluster-scoped resources to omit the -n flag.
export function kGetJSONScoped<T = unknown>(
  kind: string,
  name: string,
  namespace = 'default',
  opts: KubectlOptions = {},
): T {
  const args = ['get', kind, name, '-o', 'json'];
  if (namespace) args.splice(3, 0, '-n', namespace);
  const { stdout } = kubectl(args, opts);
  return JSON.parse(stdout) as T;
}

// True when `kubectl get` returns NotFound. Scope-aware variant of kNotFound.
// Pass namespace="" for cluster-scoped resources to omit the -n flag.
export function kNotFoundScoped(
  kind: string,
  name: string,
  namespace = 'default',
): boolean {
  const args = ['get', kind, name, '--ignore-not-found', '-o', 'name'];
  if (namespace) args.splice(3, 0, '-n', namespace);
  const r = kubectl(args, { check: false });
  if (r.status !== 0) return /NotFound/i.test(r.stderr);
  return r.stdout.trim() === '';
}

// True when `kubectl get` succeeds. Counterpart to kNotFoundScoped.
// Pass namespace="" for cluster-scoped resources to omit the -n flag.
export function kExists(
  kind: string,
  name: string,
  namespace = 'default',
): boolean {
  const args = ['get', kind, name, '--ignore-not-found', '-o', 'name'];
  if (namespace) args.splice(3, 0, '-n', namespace);
  const r = kubectl(args, { check: false });
  return r.status === 0 && r.stdout.trim() !== '';
}

// kubectl apply -f - with the supplied YAML body. Idempotent.
export function kApplyYAML(yaml: string): KubectlResult {
  return kubectl(['apply', '-f', '-'], { input: yaml });
}

// kubectl delete with --ignore-not-found so teardown stays idempotent.
// Pass namespace="" for cluster-scoped resources (Namespace,
// ClusterAuthzRoleBinding, etc.) to omit the -n flag.
export function kDelete(
  kind: string,
  name: string,
  namespace = 'default',
): KubectlResult {
  const args = ['delete', kind, name, '--ignore-not-found', '--wait=false'];
  if (namespace) args.splice(3, 0, '-n', namespace);
  try {
    return kubectl(args);
  } catch (err) {
    // --ignore-not-found only covers the resource itself; a missing namespace
    // or CRD still exits non-zero. Treat those as already-deleted, but surface
    // anything else (auth failures, timeouts, wrong context).
    if (err instanceof Error && /not ?found/i.test(err.message)) {
      return { status: 0, stdout: '', stderr: err.message };
    }
    throw err;
  }
}
