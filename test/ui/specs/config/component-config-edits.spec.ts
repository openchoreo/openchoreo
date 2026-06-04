// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test, expect, storageStateFor } from '../../fixtures/auth';
import {
  kApplyYAML,
  kDelete,
  kGetJSON,
  kNotFound,
  kubectl,
} from '../../fixtures/kube';
import { ProjectPO } from '../../po/project';
import { ComponentPO } from '../../po/component';
import { ReleasePO } from '../../po/release';
import { DeletePO } from '../../po/delete';
import { CatalogTablePO } from '../../po/catalogTable';
import { WorkloadConfigPO } from '../../po/workloadConfig';
import { OverridesPO } from '../../po/overrides';

const PRE_BUILT_IMAGE =
  'ghcr.io/openchoreo/samples/greeter-service@sha256:5c67732c99ac3505dbab14c7ec92c33be57904420d62812694c64b56c5f92d40';
const ENDPOINT_PORT = 9090;
const NS = 'default';

const ts = Date.now().toString(36);
const PROJECT_NAME = `ui-config-${ts}`;
const COMPONENT_NAME = `greeter-cfg-${ts}`;
const SECRET_REF_NAME = `ui-cfg-secret-${ts}`;
const ABAC_BINDING_NAME = `ui-cfg-abac-${ts}`;

// ── CRD type helpers ────────────────────────────────────────────────────

type WorkloadJSON = {
  spec?: {
    container?: {
      image?: string;
      args?: string[];
      env?: Array<{
        key: string;
        value?: string;
        valueFrom?: { secretKeyRef?: { name: string; key: string } };
      }>;
      files?: Array<{
        key: string;
        mountPath?: string;
        value?: string;
        valueFrom?: { secretKeyRef?: { name: string; key: string } };
      }>;
    };
  };
};

type ReleaseBindingJSON = {
  metadata: { name: string };
  spec?: {
    environment?: string;
    componentTypeEnvironmentConfigs?: Record<string, unknown>;
    workloadOverrides?: {
      container?: WorkloadJSON['spec']['container'];
    };
  };
};

function getWorkload(): WorkloadJSON {
  return kGetJSON<WorkloadJSON>('workload', `${COMPONENT_NAME}-workload`, NS);
}

function getWorkloadEnvByKey(envKey: string) {
  return getWorkload().spec?.container?.env?.find(e => e.key === envKey);
}

function getWorkloadFileByKey(fileKey: string) {
  return getWorkload().spec?.container?.files?.find(f => f.key === fileKey);
}

function getDevelopmentReleaseBinding(): ReleaseBindingJSON {
  const list = JSON.parse(
    kubectl(
      [
        'get',
        'releasebinding',
        '-n',
        NS,
        '-l',
        `openchoreo.dev/component=${COMPONENT_NAME}`,
        '-o',
        'json',
      ],
      { check: false },
    ).stdout,
  );
  const binding = list.items?.find(
    (item: ReleaseBindingJSON) => item.spec?.environment === 'development',
  );
  if (!binding) throw new Error('No development ReleaseBinding found');
  return binding;
}

function getOverrideEnvByKey(envKey: string) {
  const rb = getDevelopmentReleaseBinding();
  return rb.spec?.workloadOverrides?.container?.env?.find(
    (e: { key: string }) => e.key === envKey,
  );
}

function getOverrideFileByKey(fileKey: string) {
  const rb = getDevelopmentReleaseBinding();
  return rb.spec?.workloadOverrides?.container?.files?.find(
    (f: { key: string }) => f.key === fileKey,
  );
}

// ── Seed fixtures ───────────────────────────────────────────────────────

const secretRefYAML = `
apiVersion: openchoreo.dev/v1alpha1
kind: SecretReference
metadata:
  name: ${SECRET_REF_NAME}
  namespace: ${NS}
spec:
  template:
    type: Opaque
  data:
    - secretKey: token
      remoteRef:
        key: npm-token
    - secretKey: config-data
      remoteRef:
        key: github-pat
`;

// ABAC: narrow role + deny binding for production releasebinding actions.
// This mirrors the pattern in abac-env-restriction.spec.ts — the deny must
// use its own scoped role so only releasebinding actions are denied, not
// all developer actions.
const abacSeedYAML = `
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRole
metadata:
  name: ${ABAC_BINDING_NAME}-rb-role
spec:
  description: releasebinding-only role for config-edit ABAC test
  actions:
    - releasebinding:create
    - releasebinding:update
---
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRoleBinding
metadata:
  name: ${ABAC_BINDING_NAME}
spec:
  entitlement:
    claim: groups
    value: abac-developers
  effect: allow
  roleMappings:
    - roleRef:
        kind: ClusterAuthzRole
        name: developer
      scope:
        namespace: ${NS}
      conditions:
        - actions:
            - releasebinding:create
            - releasebinding:update
          expression: resource.environment in ["${NS}/development", "${NS}/staging"]
---
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRoleBinding
metadata:
  name: ${ABAC_BINDING_NAME}-deny
spec:
  entitlement:
    claim: groups
    value: abac-developers
  effect: deny
  roleMappings:
    - roleRef:
        kind: ClusterAuthzRole
        name: ${ABAC_BINDING_NAME}-rb-role
      scope:
        namespace: ${NS}
      conditions:
        - actions:
            - releasebinding:create
            - releasebinding:update
          expression: resource.environment == "${NS}/production"
`;

// ── Test suite ──────────────────────────────────────────────────────────

test.describe('component config edits through Backstage UI', () => {
  test.describe.configure({ mode: 'serial' });
  test.beforeAll(async ({ mintAuthState }) => {
    await mintAuthState('pe');
    await mintAuthState('abac');
    kApplyYAML(secretRefYAML);
    kApplyYAML(abacSeedYAML);
  });

  test.afterAll(async () => {
    kDelete('component', COMPONENT_NAME, NS);
    kDelete('project', PROJECT_NAME, NS);
    kDelete('secretreference', SECRET_REF_NAME, NS);
    kDelete('clusterauthzrolebinding', ABAC_BINDING_NAME, '');
    kDelete('clusterauthzrolebinding', `${ABAC_BINDING_NAME}-deny`, '');
    kDelete('clusterauthzrole', `${ABAC_BINDING_NAME}-rb-role`, '');
  });

  test.use({ storageState: storageStateFor('pe') });

  // ─── 1. Setup ─────────────────────────────────────────────────────────
  test('creates project, component, and initial deployment', async ({
    page,
  }) => {
    test.setTimeout(360_000);
    const project = new ProjectPO(page);
    const component = new ComponentPO(page);
    const release = new ReleasePO(page);

    await page.goto('/');

    await project.create({
      name: PROJECT_NAME,
      namespace: NS,
      displayName: PROJECT_NAME,
      description: 'UI config edit e2e',
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

    await component.create({
      name: COMPONENT_NAME,
      project: PROJECT_NAME,
      template: 'Web Application',
      image: PRE_BUILT_IMAGE,
      endpointPort: ENDPOINT_PORT,
      description: 'greeter config edit e2e',
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

    await component.deployTo(COMPONENT_NAME, 'development', {
      args: ['--port', String(ENDPOINT_PORT)],
    });
    await release.expectActive(COMPONENT_NAME, 'development', 120_000);

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
  });

  // ─── 2. Validation: empty env var name ────────────────────────────────
  test('validates that empty env var name disables apply', async ({
    page,
  }) => {
    test.setTimeout(120_000);
    const wc = new WorkloadConfigPO(page);
    await wc.open(COMPONENT_NAME);

    await page
      .getByRole('button', { name: 'Add Environment Variable', exact: true })
      .click();
    await page
      .getByRole('button', { name: 'Apply changes' })
      .waitFor({ state: 'visible', timeout: 10_000 });
    // Fill only the Value field, leave Name empty.
    await page.getByLabel('Value', { exact: true }).last().fill('some-value');

    await wc.expectApplyDisabled();
    await wc.cancelEditing();
  });

  // ─── 3. Add + edit component-level plain and secret env vars ──────────
  test('adds and edits component-level plain and secret env vars', async ({
    page,
  }) => {
    test.setTimeout(180_000);
    const wc = new WorkloadConfigPO(page);
    await wc.open(COMPONENT_NAME);

    await wc.addPlainEnv({ name: 'GREETING_PREFIX', value: 'hello' });
    await wc.expectEnvVisible('GREETING_PREFIX');

    await wc.addSecretEnv({
      name: 'GREETER_TOKEN',
      secretRef: { name: SECRET_REF_NAME, key: 'token' },
    });
    await wc.expectEnvVisible('GREETER_TOKEN');

    await wc.editPlainEnv('GREETING_PREFIX', {
      name: 'GREETING_PREFIX',
      value: 'hola',
    });

    await wc.saveAndCreateRelease();

    await expect
      .poll(() => getWorkloadEnvByKey('GREETING_PREFIX')?.value, {
        timeout: 30_000,
      })
      .toBe('hola');

    const secretEnv = getWorkloadEnvByKey('GREETER_TOKEN');
    expect(secretEnv?.valueFrom?.secretKeyRef?.name).toBe(SECRET_REF_NAME);
    expect(secretEnv?.valueFrom?.secretKeyRef?.key).toBe('token');
  });

  // ─── 4. Add + edit component-level plain and secret file mounts ───────
  test('adds and edits component-level plain and secret file mounts', async ({
    page,
  }) => {
    test.setTimeout(180_000);
    const wc = new WorkloadConfigPO(page);
    await wc.open(COMPONENT_NAME);

    await wc.addPlainFile({
      fileName: 'app.properties',
      mountPath: '/etc/greeter/app.properties',
      content: 'message=hello',
    });
    await wc.expectFileVisible('app.properties');

    await wc.addSecretFile({
      fileName: 'token.txt',
      mountPath: '/etc/greeter/token.txt',
      secretRef: { name: SECRET_REF_NAME, key: 'config-data' },
    });
    await wc.expectFileVisible('token.txt');

    await wc.editPlainFile('app.properties', {
      fileName: 'app.properties',
      mountPath: '',
      content: 'message=hola',
    });

    await wc.saveAndCreateRelease();

    await expect
      .poll(() => getWorkloadFileByKey('app.properties')?.value, {
        timeout: 30_000,
      })
      .toBe('message=hola');

    const secretFile = getWorkloadFileByKey('token.txt');
    expect(secretFile?.mountPath).toBe('/etc/greeter/token.txt');
    expect(secretFile?.valueFrom?.secretKeyRef?.name).toBe(SECRET_REF_NAME);
    expect(secretFile?.valueFrom?.secretKeyRef?.key).toBe('config-data');
  });

  // ─── 5. Deploy the updated release so overrides tests have current state
  // All 4 entries (plain + secret) survive — the fake ClusterSecretStore
  // resolves the SecretReference remote keys (npm-token, github-pat).
  test('deploys updated release so override tests see current workload', async ({
    page,
  }) => {
    test.setTimeout(240_000);
    const component = new ComponentPO(page);
    const release = new ReleasePO(page);

    await component.deployLatestRelease(COMPONENT_NAME, 'development');
    await release.expectActive(COMPONENT_NAME, 'development', 120_000);
  });

  // ─── 6. Per-environment env var overrides ─────────────────────────────
  test('adds per-environment env var overrides', async ({ page }) => {
    test.setTimeout(180_000);
    const overrides = new OverridesPO(page);
    await overrides.open(COMPONENT_NAME, 'development');
    await overrides.openWorkloadTab();

    await overrides.addPlainEnv({
      name: 'ENV_ONLY_GREETING',
      value: 'development',
    });

    await overrides.saveOverrides();

    await expect
      .poll(() => getOverrideEnvByKey('ENV_ONLY_GREETING')?.value, {
        timeout: 30_000,
      })
      .toBe('development');
  });

  // ─── 7. Per-environment file mount overrides ──────────────────────────
  test('adds per-environment file mount overrides', async ({ page }) => {
    test.setTimeout(180_000);
    const overrides = new OverridesPO(page);
    await overrides.open(COMPONENT_NAME, 'development');
    await overrides.openWorkloadTab();

    await overrides.addPlainFile({
      fileName: 'env-only.properties',
      mountPath: '/etc/greeter/env-only.properties',
      content: 'env=development',
    });

    await overrides.saveOverrides();

    await expect
      .poll(() => getOverrideFileByKey('env-only.properties')?.value, {
        timeout: 30_000,
      })
      .toBe('env=development');
  });

  // ─── 8. Override validation: empty name / file name ────────────────────
  test('validates per-environment editors reject empty name and file name', async ({
    page,
  }) => {
    test.setTimeout(120_000);
    const overrides = new OverridesPO(page);
    await overrides.open(COMPONENT_NAME, 'development');
    await overrides.openWorkloadTab();

    // Env var: empty Name with non-empty Value should disable Apply.
    await page
      .getByRole('button', { name: 'Add Environment Variable', exact: true })
      .click();
    await page
      .getByRole('button', { name: 'Apply changes' })
      .waitFor({ state: 'visible', timeout: 10_000 });
    await page.getByLabel('Value', { exact: true }).last().fill('orphan-val');

    await overrides.expectApplyDisabled();
    await overrides.cancelEditing();

    // File mount: empty File Name with non-empty Mount Path should disable Apply.
    await page
      .getByRole('button', { name: 'Add File Mount', exact: true })
      .click();
    await page
      .getByRole('button', { name: 'Apply changes' })
      .waitFor({ state: 'visible', timeout: 10_000 });
    await page
      .getByLabel('Mount Path', { exact: true })
      .last()
      .fill('/etc/greeter/orphan');

    await overrides.expectApplyDisabled();
    await overrides.cancelEditing();
  });

  // ─── 9. Override inherited component-level entries ────────────────────
  // Inherited plain entries (GREETING_PREFIX, app.properties) survived
  // test 5 and were deployed in test 6. The Override button copies them
  // into override state with a locked (read-only) Name/FileName field.
  test('overrides inherited entries with read-only name fields', async ({
    page,
  }) => {
    test.setTimeout(180_000);
    const overrides = new OverridesPO(page);
    await overrides.open(COMPONENT_NAME, 'development');
    await overrides.openWorkloadTab();

    // Override inherited env var — Name must be disabled.
    const envCard = page
      .getByText('GREETING_PREFIX', { exact: true })
      .locator('xpath=ancestor::div[.//button]')
      .filter({
        has: page.getByRole('button', {
          name: /^(edit|override|remove environment variable)$/i,
        }),
      })
      .last();
    await envCard
      .getByRole('button', { name: 'Override', exact: true })
      .click();
    const envNameField = page.getByLabel('Name', { exact: true }).last();
    await expect(envNameField).toBeDisabled();

    const envValueField = page.getByLabel('Value', { exact: true }).last();
    await envValueField.clear();
    await envValueField.fill('overridden-hello');
    await overrides.clickApply();

    // Override inherited file mount — File Name must be disabled.
    await overrides.cancelAnyOpenEditor();
    const fileCard = page
      .getByText('app.properties', { exact: true })
      .locator('xpath=ancestor::div[.//button]')
      .filter({
        has: page.getByRole('button', {
          name: /^(edit|override|remove file mount)$/i,
        }),
      })
      .last();
    await fileCard
      .getByRole('button', { name: 'Override', exact: true })
      .click();
    const fileNameField = page
      .getByLabel('File Name', { exact: true })
      .last();
    await expect(fileNameField).toBeDisabled();

    const expandBtn = page.getByRole('button', { name: /expand content/i });
    if (await expandBtn.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await expandBtn.click();
    }
    const content = page.getByLabel(/^(Edit )?Content$/).last();
    await content.waitFor({ state: 'visible', timeout: 5_000 });
    await content.scrollIntoViewIfNeeded();
    await content.clear();
    await content.fill('message=overridden');
    await overrides.clickApply();

    await overrides.saveOverrides();

    await expect
      .poll(() => getOverrideEnvByKey('GREETING_PREFIX')?.value, {
        timeout: 30_000,
      })
      .toBe('overridden-hello');

    await expect
      .poll(() => getOverrideFileByKey('app.properties')?.value, {
        timeout: 30_000,
      })
      .toBe('message=overridden');
  });

  // ─── 10. Edit + delete per-environment overrides ──────────────────────
  // Covers both "add new" overrides (from tests 7+8) and inherited
  // overrides (from test 10). Edits the "add new" entries, then deletes
  // all four overrides.
  test('edits and then deletes per-environment overrides', async ({
    page,
  }) => {
    test.setTimeout(180_000);
    const overrides = new OverridesPO(page);
    await overrides.open(COMPONENT_NAME, 'development');
    await overrides.openWorkloadTab();

    // Edit "add new" overrides created in tests 7+8.
    await overrides.editOverrideEnv(
      'ENV_ONLY_GREETING',
      'development-updated',
    );
    await overrides.editOverrideFile(
      'env-only.properties',
      'env=development-updated',
    );

    await overrides.saveOverrides();

    await expect
      .poll(() => getOverrideEnvByKey('ENV_ONLY_GREETING')?.value, {
        timeout: 30_000,
      })
      .toBe('development-updated');

    await expect
      .poll(() => getOverrideFileByKey('env-only.properties')?.value, {
        timeout: 30_000,
      })
      .toBe('env=development-updated');

    // Re-open and delete all overrides (inherited + new).
    await overrides.open(COMPONENT_NAME, 'development');
    await overrides.openWorkloadTab();

    await overrides.deleteOverrideEnv('GREETING_PREFIX');
    await overrides.deleteOverrideEnv('ENV_ONLY_GREETING');
    await overrides.deleteOverrideFile('app.properties');
    await overrides.deleteOverrideFile('env-only.properties');

    await overrides.saveOverrides();

    await expect
      .poll(() => getOverrideEnvByKey('GREETING_PREFIX'), {
        timeout: 30_000,
      })
      .toBeUndefined();
    await expect
      .poll(() => getOverrideEnvByKey('ENV_ONLY_GREETING'), {
        timeout: 30_000,
      })
      .toBeUndefined();
    await expect
      .poll(() => getOverrideFileByKey('app.properties'), {
        timeout: 30_000,
      })
      .toBeUndefined();
    await expect
      .poll(() => getOverrideFileByKey('env-only.properties'), {
        timeout: 30_000,
      })
      .toBeUndefined();
  });

  // ─── 11. Per-environment component overrides (environmentConfigs) ─────
  test('edits per-environment component overrides', async ({ page }) => {
    test.setTimeout(180_000);
    const overrides = new OverridesPO(page);
    await overrides.open(COMPONENT_NAME, 'development');
    await overrides.openComponentTab();

    const replicasField = page.getByRole('spinbutton', {
      name: /replicas/i,
    });
    await expect(replicasField).toBeVisible({ timeout: 10_000 });
    await replicasField.clear();
    await replicasField.fill('3');

    await overrides.saveOverrides();

    await expect
      .poll(
        () => {
          const rb = getDevelopmentReleaseBinding();
          return rb.spec?.componentTypeEnvironmentConfigs?.replicas;
        },
        { timeout: 30_000 },
      )
      .toBe(3);
  });

  // ─── 12. ABAC: production override denied ─────────────────────────────
  // The ABAC deny binding scoped to production releasebinding actions
  // prevents the abac-developers group from mutating production overrides.
  // The UI gates this client-side via permission hooks (useEnvScopedPermission).
  test('denies configure-overrides to ABAC-restricted user on production', async ({
    browser,
    baseURL,
  }) => {
    test.setTimeout(120_000);
    const abacState = storageStateFor('abac');
    const { existsSync } = await import('node:fs');
    if (!existsSync(abacState)) {
      test.skip(true, 'abac storage state not minted');
      return;
    }

    const context = await browser.newContext({
      baseURL,
      storageState: abacState,
    });
    const { cryptoUUIDPolyfill } = await import('../../fixtures/auth');
    await context.addInitScript(cryptoUUIDPolyfill);
    const page = await context.newPage();

    try {
      // Snapshot production ReleaseBinding state before the test to prove
      // no mutation occurs. Production may not have a binding at all.
      const beforeList = JSON.parse(
        kubectl(
          [
            'get',
            'releasebinding',
            '-n',
            NS,
            '-l',
            `openchoreo.dev/component=${COMPONENT_NAME}`,
            '-o',
            'json',
          ],
          { check: false },
        ).stdout,
      );
      const productionBefore = beforeList.items?.find(
        (item: ReleaseBindingJSON) => item.spec?.environment === 'production',
      );

      // Navigate to production overrides page as the ABAC user.
      await page.goto(
        `/catalog/default/component/${COMPONENT_NAME}/environments/overrides/production`,
      );

      // Wait for the overrides page to finish loading — the component
      // entity header proves Backstage resolved the entity and rendered
      // the route, so absence of controls is a real denial signal, not
      // a loading/redirect artifact.
      await page
        .getByText(COMPONENT_NAME)
        .first()
        .waitFor({ state: 'visible', timeout: 30_000 });

      // Assert the page signals denial. The UI renders one of:
      //   (a) explicit permission error text,
      //   (b) a disabled/absent "Save Overrides" button, or
      //   (c) all mutating affordances (Add, Override) are disabled/absent.
      // We check all three and require at least one to be true.
      const permissionText = page.getByText(
        /permission|forbidden|not authorized|not allowed/i,
      );
      const addEnvBtn = page.getByRole('button', {
        name: 'Add Environment Variable',
        exact: true,
      });
      const saveBtn = page.getByRole('button', {
        name: /save overrides/i,
      });

      await expect(async () => {
        const hasPermissionText = await permissionText
          .isVisible({ timeout: 2_000 })
          .catch(() => false);
        const addEnvVisible = (await addEnvBtn.count()) > 0 &&
          (await addEnvBtn.isEnabled().catch(() => false));
        const saveVisible = (await saveBtn.count()) > 0 &&
          (await saveBtn.isEnabled().catch(() => false));
        expect(hasPermissionText || (!addEnvVisible && !saveVisible)).toBe(true);
      }).toPass({ timeout: 30_000 });

      // Verify no production ReleaseBinding was created or mutated.
      const afterList = JSON.parse(
        kubectl(
          [
            'get',
            'releasebinding',
            '-n',
            NS,
            '-l',
            `openchoreo.dev/component=${COMPONENT_NAME}`,
            '-o',
            'json',
          ],
          { check: false },
        ).stdout,
      );
      const productionAfter = afterList.items?.find(
        (item: ReleaseBindingJSON) => item.spec?.environment === 'production',
      );
      expect(JSON.stringify(productionAfter)).toBe(
        JSON.stringify(productionBefore),
      );
    } finally {
      await context.close();
    }
  });

  // ─── 13. Delete component-level entries ────────────────────────────────
  test('deletes all component-level env var and file mount entries', async ({
    page,
  }) => {
    test.setTimeout(180_000);
    const wc = new WorkloadConfigPO(page);
    await wc.open(COMPONENT_NAME);

    await wc.deleteEnv('GREETING_PREFIX');
    await wc.deleteEnv('GREETER_TOKEN');
    await wc.deleteFile('app.properties');
    await wc.deleteFile('token.txt');

    await wc.saveAndCreateRelease();

    await expect
      .poll(() => getWorkloadEnvByKey('GREETING_PREFIX'), { timeout: 30_000 })
      .toBeUndefined();
    await expect
      .poll(() => getWorkloadEnvByKey('GREETER_TOKEN'), { timeout: 30_000 })
      .toBeUndefined();
    await expect
      .poll(() => getWorkloadFileByKey('app.properties'), { timeout: 30_000 })
      .toBeUndefined();
    await expect
      .poll(() => getWorkloadFileByKey('token.txt'), { timeout: 30_000 })
      .toBeUndefined();
  });

  // ─── 14. Cleanup ──────────────────────────────────────────────────────
  test('cleans up component and project via UI', async ({ page }) => {
    test.setTimeout(180_000);
    const component = new ComponentPO(page);
    const del = new DeletePO(page);
    const catalog = new CatalogTablePO(page);

    await component.openByName(COMPONENT_NAME);
    await del.openOverflowAndDelete('Component');
    await del.confirm();
    await expect
      .poll(() => kNotFound('component', COMPONENT_NAME, NS), {
        timeout: 60_000,
      })
      .toBe(true);

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
