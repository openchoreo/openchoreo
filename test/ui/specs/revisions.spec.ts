// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test, expect, storageStateFor } from '../fixtures/auth';
import { kApplyYAML, kDelete, kGetJSON, kubectl } from '../fixtures/kube';
import { RevisionsPO } from '../po/revisions';

// Release binding revision history and rollback through the Deploy tab's
// environment side panel. The history is seeded with kubectl so the spec
// only drives what it tests:
//   r1  the initial auto-deploy                     (cause Deploy)
//   r2  a workload env change, auto-deployed        (cause Deploy, active)
// then rolls back to r1 in the UI. No revision is created: r1 becomes active
// again, and the binding's status.currentRevision points at it.

const IMAGE =
  'ghcr.io/openchoreo/samples/greeter-service@sha256:5c67732c99ac3505dbab14c7ec92c33be57904420d62812694c64b56c5f92d40';
const PORT = 9090;
const NS = 'default';
const ENV = 'development';
const ENV_DISPLAY = 'Development';

// Names are generated per execution in beforeAll, not at module load: a
// retry or --repeat-each reuses the loaded module, and reusing names would
// meet the previous execution's binding and revisions while their delete is
// still in progress.
let PROJECT_NAME = '';
let COMPONENT_NAME = '';
let BINDING_NAME = '';
// The env value that differs between r1 and r2; the diff must show both.
let V1 = '';
let V2 = '';

function newRun(): void {
  const id = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 5)}`;
  PROJECT_NAME = `ui-rev-${id}`;
  COMPONENT_NAME = `frontend-${id}`;
  BINDING_NAME = `${COMPONENT_NAME}-${ENV}`;
  V1 = `rev-one-${id}`;
  V2 = `rev-two-${id}`;
}

const projectYAML = () => `
apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: ${PROJECT_NAME}
  namespace: ${NS}
spec:
  deploymentPipelineRef:
    name: default
  type:
    kind: ClusterProjectType
    name: default
---
apiVersion: openchoreo.dev/v1alpha1
kind: ProjectReleaseBinding
metadata:
  name: ${PROJECT_NAME}-${ENV}
  namespace: ${NS}
  labels:
    openchoreo.dev/project: ${PROJECT_NAME}
    openchoreo.dev/environment: ${ENV}
spec:
  owner:
    projectName: ${PROJECT_NAME}
  environment: ${ENV}
`;

const componentYAML = (version: string) => `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: ${COMPONENT_NAME}
  namespace: ${NS}
spec:
  owner:
    projectName: ${PROJECT_NAME}
  componentType:
    kind: ClusterComponentType
    name: deployment/service
  autoDeploy: true
---
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: ${COMPONENT_NAME}
  namespace: ${NS}
spec:
  owner:
    componentName: ${COMPONENT_NAME}
    projectName: ${PROJECT_NAME}
  endpoints:
    http:
      type: HTTP
      port: ${PORT}
  container:
    image: ${IMAGE}
    args: ["--port", "${PORT}"]
    env:
      - key: REVISION_MARKER
        value: "${version}"
`;

// ── Cluster cross-checks ────────────────────────────────────────────────

type RevisionJSON = {
  metadata: { name: string };
  spec: { revision: number; cause: string };
  status?: { health?: string; active?: boolean };
};

function revisions(): RevisionJSON[] {
  const r = kubectl(
    [
      'get',
      'releasebindingrevisions',
      '-n',
      NS,
      '-l',
      `openchoreo.dev/release-binding=${BINDING_NAME}`,
      '-o',
      'json',
    ],
    { check: false },
  );
  if (r.status !== 0) return [];
  return (JSON.parse(r.stdout).items as RevisionJSON[]).sort(
    (a, b) => b.spec.revision - a.spec.revision,
  );
}

// "<revision>:<health>:<active|->" newest first, for compact polling.
function revisionSummary(): string[] {
  return revisions().map(
    r => `${r.spec.revision}:${r.status?.health ?? ''}:${r.status?.active ? 'active' : '-'}`,
  );
}

// Revision health is observed by the controller, so a revision stuck in
// Progressing usually means the controller was not running, e.g. it lost
// leader election while the API server stalled. Name that in the failure.
function controllerStatus(): string {
  const r = kubectl(
    [
      'get',
      'pods',
      '-n',
      'openchoreo-control-plane',
      '-l',
      'app.kubernetes.io/component=controller-manager',
      '-o',
      'jsonpath={range .items[*]}{.metadata.name} ready={.status.containerStatuses[0].ready} restarts={.status.containerStatuses[0].restartCount} lastExit={.status.containerStatuses[0].lastState.terminated.reason}/{.status.containerStatuses[0].lastState.terminated.finishedAt}{"\\n"}{end}',
    ],
    { check: false },
  );
  return r.stdout.trim() || r.stderr.trim();
}

async function expectRevisions(expected: string[], timeout: number): Promise<void> {
  try {
    await expect
      .poll(revisionSummary, { timeout, intervals: [3_000] })
      .toEqual(expected);
  } catch (err) {
    throw new Error(
      `${(err as Error).message}\ncontroller-manager: ${controllerStatus()}`,
    );
  }
}

function currentRevision(): string {
  return (
    kGetJSON<{ status?: { currentRevision?: string } }>('releasebinding', BINDING_NAME, NS)
      .status?.currentRevision ?? ''
  );
}

function autoDeploy(): boolean {
  return (
    kGetJSON<{ spec?: { autoDeploy?: boolean } }>('component', COMPONENT_NAME, NS)
      .spec?.autoDeploy ?? false
  );
}

// ── Suite ───────────────────────────────────────────────────────────────

test.describe('release binding revisions through the Deploy tab', () => {
  test.describe.configure({ mode: 'serial' });
  test.use({ storageState: storageStateFor('pe') });

  test.beforeAll(async ({ mintAuthState }) => {
    test.setTimeout(420_000);
    newRun();
    await mintAuthState('pe');

    // Fail fast when the controller is down: nothing below can progress
    // without it.
    await expect
      .poll(() => /ready=true/.test(controllerStatus()), {
        timeout: 120_000,
        message: `controller-manager not ready: ${controllerStatus()}`,
      })
      .toBe(true);

    kApplyYAML(projectYAML());
    kApplyYAML(componentYAML(V1));

    // r1 must be Healthy before r2 exists: a revision keeps the health it
    // had when it stopped being active, and the rollback target should be
    // a healthy one.
    await expectRevisions(['1:Healthy:active'], 300_000);

    kApplyYAML(componentYAML(V2));
    await expectRevisions(['2:Healthy:active', '1:Healthy:-'], 300_000);
  });

  test.afterAll(async () => {
    kDelete('component', COMPONENT_NAME, NS);
    kDelete('projectreleasebinding', `${PROJECT_NAME}-${ENV}`, NS);
    kDelete('project', PROJECT_NAME, NS);
  });

  test('shows the timeline, compares two revisions and rolls back', async ({
    page,
  }) => {
    test.setTimeout(420_000);
    const revs = new RevisionsPO(page);

    // ── Open frontend → Deploy → Development ──
    await revs.openDeployTab(COMPONENT_NAME);
    await revs.selectEnvironment(ENV_DISPLAY);

    // ── Timeline: newest first, r2 active ──
    await expect
      .poll(async () => (await revs.readTimeline()).map(r => r.revision))
      .toEqual([2, 1]);
    const [r2, r1] = await revs.readTimeline();
    expect(r2).toMatchObject({ cause: 'Deploy', health: 'Healthy', active: true });
    expect(r1).toMatchObject({ cause: 'Deploy', health: 'Healthy', active: false });
    expect(r1.release).not.toBe(r2.release);

    // ── Compare r1 with r2 in the revision browser ──
    await revs.openBrowser();
    await revs.compare(1, 2);
    await revs.expectDiffBetween(1, 2);
    // Full ReleaseBindingRevision objects: each side binds its own release.
    // (Unchanged lines such as `kind:` may be collapsed in the diff.)
    await revs.expectDiffShows(`componentReleaseName: ${r1.release}`);
    await revs.expectDiffShows(`componentReleaseName: ${r2.release}`);
    await revs.closeBrowser();

    // ── Development is the first environment and the component auto-deploys:
    // the rollback is blocked with a clear message, and nothing changes ──
    await revs.openTimeline();
    await revs.row(1).click();
    await page.getByRole('button', { name: 'Roll back to r1', exact: true }).click();
    const blockedDialog = page.getByRole('dialog');
    await expect(
      blockedDialog.getByText(/^Rollback blocked: auto deploy is enabled for this component/),
    ).toBeVisible({ timeout: 30_000 });
    await expect(
      blockedDialog.getByRole('button', { name: 'Roll back', exact: true }),
    ).toBeDisabled();
    await blockedDialog.getByRole('button', { name: 'Cancel' }).click();
    await expect(blockedDialog).toBeHidden();
    expect(autoDeploy()).toBe(true);
    expect(currentRevision()).toBe(`${BINDING_NAME}-r2`);

    // ── Turn auto deploy off, as the Set Up tab does, then roll back ──
    kubectl(['patch', 'component', COMPONENT_NAME, '-n', NS, '--type', 'merge',
      '-p', '{"spec":{"autoDeploy":false}}']);
    const warnings = await revs.startRollback(1);
    expect(warnings.join('\n')).not.toContain('auto deploy');
    expect(revisionSummary()).toHaveLength(2);

    await revs.confirmRollback(1, 'UI test: back to r1');
    await expect(
      page.getByText(
        new RegExp(`^Rolled back .* to revision 1: ${r1.name} is active again\\.$`),
      ),
    ).toBeVisible();
    // The reason reaches the binding's RevisionReactivated event.
    await expect
      .poll(
        () =>
          kubectl(['get', 'events', '-n', NS, '--field-selector',
            `involvedObject.name=${BINDING_NAME},reason=RevisionReactivated`,
            '-o', 'jsonpath={.items[*].message}']).stdout,
        { timeout: 60_000 },
      )
      .toContain('Message: "UI test: back to r1"');

    // ── No revision is created: r1 is active again and settles Healthy ──
    // No reload: the panel polls until the controller switches to r1, then
    // while r1 is Progressing again.
    await expect
      .poll(() => revs.rowState(1), { timeout: 60_000 })
      .toMatchObject({ cause: 'Deploy', release: r1.release, active: true });
    await expect
      .poll(async () => (await revs.rowState(1)).health, {
        timeout: 240_000,
        intervals: [2_000],
      })
      .toBe('Healthy');

    // ── The previously active revision lost its Active marker ──
    await expect
      .poll(async () => (await revs.rowState(2)).active)
      .toBe(false);
    const timeline = await revs.readTimeline();
    expect(timeline.map(r => r.revision)).toEqual([2, 1]);
    expect(timeline.filter(r => r.active).map(r => r.revision)).toEqual([1]);

    // ── Cluster agrees ──
    expect(revisionSummary()).toEqual(['2:Healthy:-', '1:Healthy:active']);
    // The object is {binding}-r{N}; it is shown as {release}-r{N}.
    expect(currentRevision()).toBe(`${BINDING_NAME}-r1`);
    expect(r1.name).toBe(`${r1.release}-r1`);
    expect(autoDeploy()).toBe(false);
  });
});
