// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test, expect, storageStateFor } from '../../fixtures/auth';
import { kGetJSON, kNotFound, kDelete, kubectl } from '../../fixtures/kube';
import { ProjectPO } from '../../po/project';
import { ComponentPO } from '../../po/component';
import { ReleasePO } from '../../po/release';
import { DeletePO } from '../../po/delete';
import { CatalogTablePO } from '../../po/catalogTable';

// Pre-built greeter sample. Pinned to the same digest the e2e Ginkgo suites
// use (test/e2e/suites/observability) so the cluster can already pull it and
// the readiness contract is known: it serves HTTP on 9090 when started with
// `--port 9090`.
const PRE_BUILT_IMAGE =
  'ghcr.io/openchoreo/samples/greeter-service@sha256:5c67732c99ac3505dbab14c7ec92c33be57904420d62812694c64b56c5f92d40';
const ENDPOINT_PORT = 9090;

// Components and ReleaseBindings are namespaced resources that live in the
// project's namespace (`default`) and link to their project via
// spec.owner.projectName — there is no per-project namespace.
const NS = 'default';

// Suffix keeps reruns from colliding when a test container restarts.
const ts = Date.now().toString(36);
const PROJECT_NAME = `ui-lifecycle-${ts}`;
const COMPONENT_NAME = `greeter-${ts}`;

test.describe.configure({ mode: 'serial' });

test.describe('lifecycle: full kubectl-equivalent flow through Backstage UI', () => {
  test.beforeAll(async ({ mintAuthState }) => {
    await mintAuthState('pe');
  });

  test.use({ storageState: storageStateFor('pe') });

  test.afterAll(async () => {
    // Best-effort teardown in case a test bails mid-flight.
    kDelete('component', COMPONENT_NAME, NS);
    kDelete('project', PROJECT_NAME, NS);
  });

  test('creates project + component, deploys, deletes — UI and kubectl agree', async ({
    page,
  }) => {
    // create + multi-step deploy + wait-for-Ready exceeds the global 60s.
    // Sized to absorb a full catalog-provider sync cycle (default 300s)
    // inside the entity-route wait on installs without the UI values overlay.
    test.setTimeout(900_000);
    const project = new ProjectPO(page);
    const component = new ComponentPO(page);
    const release = new ReleasePO(page);
    const del = new DeletePO(page);
    const catalog = new CatalogTablePO(page);

    await page.goto('/');

    // ---- Create project ----
    await project.create({
      name: PROJECT_NAME,
      namespace: NS,
      displayName: PROJECT_NAME,
      description: 'Tier 5 UI lifecycle spec',
      pipeline: 'default',
    });

    await expect
      .poll(
        () =>
          kubectl(['get', 'project', PROJECT_NAME, '-n', NS], {
            check: false,
          }).status,
        { timeout: 30_000 },
      )
      .toBe(0);

    // ---- Create component (Web Application, pre-built image, HTTP :9090) ----
    await component.create({
      name: COMPONENT_NAME,
      project: PROJECT_NAME,
      template: 'Web Application',
      image: PRE_BUILT_IMAGE,
      endpointPort: ENDPOINT_PORT,
      description: 'pre-built greeter sample',
    });

    await expect
      .poll(
        () =>
          kubectl(['get', 'component', COMPONENT_NAME, '-n', NS], {
            check: false,
          }).status,
        { timeout: 30_000 },
      )
      .toBe(0);

    // ---- Deploy to development ----
    // The greeter needs `--port 9090` to listen on its endpoint port; the
    // create wizard has no args field, so it's set on the release workload.
    await component.deployTo(COMPONENT_NAME, 'development', {
      args: ['--port', String(ENDPOINT_PORT)],
    });
    await release.expectActive(COMPONENT_NAME, 'development', 120_000);

    // Cross-check kubectl: ReleaseBinding Ready, componentType matches.
    await expect
      .poll(
        () => {
          const bindings = kubectl(
            [
              'get',
              'releasebinding',
              '-n',
              NS,
              '-l',
              `openchoreo.dev/component=${COMPONENT_NAME}`,
              '-o',
              'jsonpath={.items[*].status.conditions[?(@.type=="Ready")].status}',
            ],
            { check: false },
          ).stdout;
          return bindings.includes('True');
        },
        { timeout: 120_000 },
      )
      .toBe(true);

    const componentJSON = kGetJSON<{
      spec: { componentType: { name: string } };
    }>('component', COMPONENT_NAME, NS);
    expect(componentJSON.spec.componentType.name).toBe(
      'deployment/web-application',
    );

    // ---- Delete component via UI ----
    await component.openByName(COMPONENT_NAME);
    await del.openOverflowAndDelete('Component');
    await del.confirm();

    await expect
      .poll(() => kNotFound('component', COMPONENT_NAME, NS), {
        timeout: 60_000,
      })
      .toBe(true);

    // ---- Delete project via UI ----
    await catalog.openEntity('system', PROJECT_NAME);
    await del.openOverflowAndDelete('Project');
    await del.confirm();

    await expect
      .poll(() => kNotFound('project', PROJECT_NAME, NS), {
        timeout: 60_000,
      })
      .toBe(true);
  });
});
