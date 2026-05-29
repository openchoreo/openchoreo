// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test, expect, storageStateFor } from '../../fixtures/auth';
import { kApplyYAML, kDelete, kubectl } from '../../fixtures/kube';
import { ComponentPO } from '../../po/component';
import { CreatePO } from '../../po/create';

const ts = Date.now().toString(36);
const PROJECT_NAME = `ui-dev-${ts}`;
const COMPONENT_NAME = `dev-comp-${ts}`;
// Name attempted (and expected to be denied) by the authz-gating test.
const COMPONENT_TYPE_NAME = `zz-ct-dev-${ts}`;
// Same pinned greeter sample the lifecycle spec + e2e Ginkgo suites use.
const PRE_BUILT_IMAGE =
  'ghcr.io/openchoreo/samples/greeter-service@sha256:5c67732c99ac3505dbab14c7ec92c33be57904420d62812694c64b56c5f92d40';
// Components live in the project's namespace (`default`), linked by
// spec.owner.projectName — not a namespace named after the project.
const NS = 'default';

// The Dev identity needs a project to operate against. PE-only resources are
// pre-applied via kubectl; the spec then asserts that the Dev identity can
// drive Component creation through the scaffolder.
const seedProjectYAML = `
apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: ${PROJECT_NAME}
  namespace: default
spec:
  deploymentPipelineRef:
    name: default
`;

test.describe.configure({ mode: 'serial' });

test.describe('dev-ops: Dev CRUD inside a PE-seeded project', () => {
  test.beforeAll(async ({ mintAuthState }) => {
    kApplyYAML(seedProjectYAML);
    await mintAuthState('dev');
  });

  test.use({ storageState: storageStateFor('dev') });

  test.afterAll(async () => {
    kDelete('component', COMPONENT_NAME, NS);
    kDelete('project', PROJECT_NAME, NS);
    // Defensive: the authz test expects this to never exist, but clean up if
    // the policy ever changes and the create unexpectedly succeeds.
    kDelete('componenttype', COMPONENT_TYPE_NAME, NS);
  });

  test('creates and views Component + Workload as Dev', async ({ page }) => {
    const component = new ComponentPO(page);

    await component.create({
      name: COMPONENT_NAME,
      project: PROJECT_NAME,
      template: 'Web Application',
      image: PRE_BUILT_IMAGE,
      endpointPort: 9090,
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

    // Creating a component also renders its Workload CR (`<name>-workload`).
    await expect
      .poll(
        () =>
          kubectl(
            ['get', 'workload', `${COMPONENT_NAME}-workload`, '-n', NS],
            { check: false },
          ).status,
        { timeout: 30_000 },
      )
      .toBe(0);

    // The component overview opens for the Dev identity.
    await component.openByName(COMPONENT_NAME);
    await expect(
      page.getByRole('heading', { name: COMPONENT_NAME, level: 1 }),
    ).toBeVisible();
  });

  test('ComponentType create is denied for Dev (server-side authz)', async ({
    page,
  }) => {
    // The developer role has `componenttype:view` but not
    // `componenttype:create`. The scaffolder template renders fully for Dev —
    // gating is enforced server-side when the create action calls the API — so
    // the assertion drives the form to submit and expects the deny on the task
    // page, plus no ComponentType CR.
    await new CreatePO(page).chooseTemplate('ComponentType');
    await page
      .getByRole('textbox', { name: /ComponentType Name/i })
      .waitFor({ state: 'visible', timeout: 30_000 });
    await page
      .getByRole('textbox', { name: /ComponentType Name/i })
      .fill(COMPONENT_TYPE_NAME);
    await page.getByRole('button', { name: 'Next', exact: true }).click();
    await page.getByRole('button', { name: 'Review', exact: true }).click();
    await page.getByRole('button', { name: 'Create', exact: true }).click();

    // The scaffolder task surfaces the API's NotAllowedError.
    await expect(
      page
        .getByText(/do not have permission|not ?allowed|forbidden/i)
        .first(),
    ).toBeVisible({ timeout: 30_000 });

    // The denied create must not have produced a ComponentType.
    await expect
      .poll(
        () =>
          kubectl(
            [
              'get',
              'componenttype',
              COMPONENT_TYPE_NAME,
              '-n',
              NS,
              '--ignore-not-found',
              '-o',
              'name',
            ],
            { check: false },
          ).stdout.trim(),
        { timeout: 10_000 },
      )
      .toBe('');
  });
});
