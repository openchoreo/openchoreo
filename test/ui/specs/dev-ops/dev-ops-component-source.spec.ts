// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test, expect, storageStateFor } from '../../fixtures/auth';
import { kApplyYAML, kDelete, kGetJSON, kLogs } from '../../fixtures/kube';
import { ComponentPO } from '../../po/component';

// Regression guard for the backstage-plugins 1.2.1 silent-401 class (fixed by
// backstage-plugins#725): with the default auth policy enforced (this suite's
// cluster runs auth ENABLED), the scaffolder's create-component action made
// unauthenticated internal catalog calls. Every failure was swallowed as a
// warning, so creation "succeeded" while
//   (a) the Workflow-entity lookup died -> the Component CR lost its entire
//       spec.workflow.parameters.repository block and builds hung forever in
//       WorkflowPending ("no such key: repository"), and
//   (b) the duplicate-component-name check silently stopped rejecting.
// These tests assert on exactly those signals so any recurrence fails loudly.

const ts = Date.now().toString(36);
const PROJECT_NAME = `ui-src-${ts}`;
const COMPONENT_NAME = `src-comp-${ts}`;
// Components live in the project's namespace (`default`), linked by
// spec.owner.projectName — not a namespace named after the project.
const NS = 'default';
const CP_NS = 'openchoreo-control-plane';

// Same public sample the build e2e suites clone; the build itself is never
// run here — the assertions are on the created Component CR's spec shape.
const REPO_URL = 'https://github.com/openchoreo/sample-workloads';
const BRANCH = 'main';
const APP_PATH = '/service-go-greeter';
const WORKFLOW = 'dockerfile-builder';

const seedProjectYAML = `
apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: ${PROJECT_NAME}
  namespace: default
spec:
  deploymentPipelineRef:
    name: default
  type:
    kind: ClusterProjectType
    name: default
`;

interface ComponentWorkflowSpec {
  metadata: { uid: string };
  spec: {
    workflow?: {
      kind?: string;
      name?: string;
      parameters?: {
        repository?: {
          url?: string;
          appPath?: string;
          revision?: { branch?: string };
        };
      };
    };
  };
}

test.describe.configure({ mode: 'serial' });

test.describe('dev-ops: build-from-source create keeps its auth-dependent guarantees', () => {
  // Backend log scans are bounded to this suite's own runtime.
  const suiteStart = new Date();
  let createdUid = '';

  test.beforeAll(async ({ mintAuthState }) => {
    kApplyYAML(seedProjectYAML);
    await mintAuthState('dev');
  });

  test.use({ storageState: storageStateFor('dev') });

  test.beforeEach(async ({ page }) => {
    await page.goto('/');
  });

  test.afterAll(async () => {
    kDelete('component', COMPONENT_NAME, NS);
    kDelete('project', PROJECT_NAME, NS);
  });

  test('wizard create maps git source into spec.workflow.parameters.repository', async ({
    page,
  }) => {
    // Project catalog sync (60s frequency) + wizard steps.
    test.setTimeout(300_000);

    const component = new ComponentPO(page);
    await component.create({
      name: COMPONENT_NAME,
      project: PROJECT_NAME,
      template: 'Service',
      source: {
        repoUrl: REPO_URL,
        branch: BRANCH,
        appPath: APP_PATH,
        workflow: WORKFLOW,
      },
    });

    const cr = kGetJSON<ComponentWorkflowSpec>('component', COMPONENT_NAME, NS);
    createdUid = cr.metadata.uid;
    expect(cr.spec.workflow?.kind).toBe('ClusterWorkflow');
    expect(cr.spec.workflow?.name).toBe(WORKFLOW);
    // The load-bearing assertion: this block is produced from the Workflow
    // entity's parameter-mapping annotation, which the scaffolder reads from
    // the catalog over an AUTHENTICATED internal call. Under the 1.2.1 bug the
    // lookup 401'd, the mapping was silently dropped, and repository was
    // absent — builds then hung in WorkflowPending.
    const repository = cr.spec.workflow?.parameters?.repository;
    expect(repository, 'spec.workflow.parameters.repository must exist').toBeTruthy();
    expect(repository?.url).toBe(REPO_URL);
    expect(repository?.revision?.branch).toBe(BRANCH);
    expect(repository?.appPath).toBe(APP_PATH);
  });

  test('re-creating the same component name is rejected by the scaffolder', async ({
    page,
  }) => {
    test.setTimeout(300_000);

    const component = new ComponentPO(page);
    // Under the 1.2.1 bug the duplicate check's catalog read 401'd and the
    // action logged a warning and proceeded — the task completed. The guard
    // here is the task page failing with the duplicate error.
    await component.createExpectingTaskError(
      {
        name: COMPONENT_NAME,
        project: PROJECT_NAME,
        template: 'Service',
        source: { repoUrl: REPO_URL, branch: BRANCH, appPath: APP_PATH },
      },
      /already exists/i,
    );

    // And the original CR is untouched (same uid — not deleted/recreated).
    const cr = kGetJSON<ComponentWorkflowSpec>('component', COMPONENT_NAME, NS);
    expect(cr.metadata.uid).toBe(createdUid);
  });

  test('backend swallowed no auth failures during creation (log canary)', async () => {
    // The scaffolder action downgrades internal-call failures to warnings, so
    // a UI-green run can still hide the regression. The suite runs serially
    // (workers: 1), so a since-time-bounded scan of the backend log is safe.
    const logs = kLogs(CP_NS, 'deploy/backstage', { sinceTime: suiteStart });
    expect(logs).not.toContain('Failed to resolve project namespace');
    expect(logs).not.toContain('Failed to check for duplicate component name');
    expect(logs).not.toContain('Failed to fetch Workflow entity');
  });
});
