// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test, expect, storageStateFor, ROLES } from '../../fixtures/auth';
import { kApplyYAML, kDelete, kubectl } from '../../fixtures/kube';
import { SidebarPO } from '../../po/sidebar';
import { ComponentPO } from '../../po/component';
import { ReleasePO } from '../../po/release';

const ts = Date.now().toString(36);
const PROJECT_NAME = `ui-abac-${ts}`;
const COMPONENT_NAME = `abac-comp-${ts}`;
const BINDING_NAME = `abac-env-${ts}`;
// Pinned digest, same as full-lifecycle: serves HTTP on 9090 when started
// with `--port 9090` (ghcr.io/openchoreo/sample-greeter:latest does not
// exist).
const PRE_BUILT_IMAGE =
  'ghcr.io/openchoreo/samples/greeter-service@sha256:5c67732c99ac3505dbab14c7ec92c33be57904420d62812694c64b56c5f92d40';

// PE-seeded scaffolding: project + roles/bindings exercising the
// resource.environment ABAC predicate for the abac-developers group:
//
//   - allow: developer role, releasebinding actions conditioned on
//     environment in [development, staging]
//   - deny: explicit deny on environment == production, bound through a
//     NARROW role carrying only the releasebinding actions
//
// staging is in the allow set because the default DeploymentPipeline's
// promotion paths are development → staging → production — there is no
// direct dev → production path in the UI, so reaching the production deny
// requires the staging promotion to be permitted.
//
// The deny needs its own narrow role because conditions only constrain the
// actions they name — every OTHER action granted by the bound role gets the
// binding's effect unconditionally (ConditionMatcher in
// internal/authz/casbin/helpers.go: "if the condition entries don't target
// the considered action then the RBAC decision stands as-is"). A deny
// binding over the full developer role would therefore deny component:create
// etc. outright, and deny overrides allow.
//
// CEL `resource.environment` values are NAMESPACE-PREFIXED ("default/development",
// not "development") — see the AuthzContext schema ("Namespace-prefixed target
// deployment Environment name (e.g. acme/dev)") and formatDualScopedName in
// the Backstage permission policy module. Bare names never match.
//
// The allow binding scopes to the namespace (not the project) so the
// developer role's view actions keep working across the scaffolder pickers;
// the environment conditions are the thing under test.
const seedYAML = `
apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: ${PROJECT_NAME}
  namespace: default
spec:
  deploymentPipelineRef:
    name: default
---
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRole
metadata:
  name: ${BINDING_NAME}-rb-role
spec:
  description: releasebinding-only role so the deny below stays scoped
  actions:
    - releasebinding:create
    - releasebinding:update
---
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRoleBinding
metadata:
  name: ${BINDING_NAME}
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
        namespace: default
      conditions:
        - actions:
            - releasebinding:create
            - releasebinding:update
          expression: resource.environment in ["default/development", "default/staging"]
---
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRoleBinding
metadata:
  name: ${BINDING_NAME}-deny
spec:
  entitlement:
    claim: groups
    value: abac-developers
  effect: deny
  roleMappings:
    - roleRef:
        kind: ClusterAuthzRole
        name: ${BINDING_NAME}-rb-role
      scope:
        namespace: default
      conditions:
        - actions:
            - releasebinding:create
            - releasebinding:update
          expression: resource.environment == "default/production"
`;

// Skip the entire suite when the ABAC identity isn't seeded. Fresh installs
// provision it via the Thunder bootstrap overlay (52-abac-user.sh in
// test/e2e/k3d/values-thunder.yaml); clusters installed before that overlay
// need test/ui/scripts/seed-idp-users.sh (re-runs the Thunder setup Job —
// the admin API is auth-gated once the server is up, even from loopback).
// The other Phase 1 specs run regardless.
import { existsSync as _existsSync } from 'node:fs';
const ABAC_STATE = storageStateFor('abac');
test.describe.configure({ mode: 'serial' });
test.skip(
  !_existsSync(ABAC_STATE),
  'abac storage state not minted (abac user not seeded in this Thunder install — see test/ui/scripts/seed-idp-users.sh)',
);

test.describe('abac-ui: env-restricted bindings, promote deny, cache eviction on relogin', () => {
  test.beforeAll(async () => {
    kApplyYAML(seedYAML);
  });

  test.afterAll(async () => {
    // Components live in the project's namespace (default), not a
    // namespace named after the project; ClusterAuthzRoleBindings are
    // cluster-scoped (namespace "" omits -n).
    kDelete('component', COMPONENT_NAME, 'default');
    kDelete('project', PROJECT_NAME, 'default');
    kDelete('clusterauthzrolebinding', BINDING_NAME, '');
    kDelete('clusterauthzrolebinding', `${BINDING_NAME}-deny`, '');
    kDelete('clusterauthzrole', `${BINDING_NAME}-rb-role`, '');
  });

  test.use({ storageState: ABAC_STATE });

  test('deploy to development succeeds, promote to production denied', async ({
    page,
  }) => {
    // Deploy + a promotion + an Active wait per hop, with room for a full
    // catalog-provider sync cycle inside the entity-route wait.
    test.setTimeout(900_000);
    const component = new ComponentPO(page);
    const release = new ReleasePO(page);

    await page.goto('/');
    await component.create({
      name: COMPONENT_NAME,
      project: PROJECT_NAME,
      template: 'Web Application',
      image: PRE_BUILT_IMAGE,
      endpointPort: 9090,
    });

    // Development: allowed by the binding's condition.
    await component.deployTo(COMPONENT_NAME, 'development', {
      args: ['--port', '9090'],
    });
    await release.expectActive(COMPONENT_NAME, 'development', 120_000);

    // Staging: in the allow set — the promotion path to production runs
    // dev → staging → production, so this hop must succeed first. Assert
    // the staging ReleaseBinding lands via kubectl (the Deploy graph is a
    // single article, so a UI text match can't be scoped per environment).
    await component.promoteToNext('staging');
    await expect
      .poll(
        () =>
          kubectl(
            [
              'get',
              'releasebinding',
              '-n',
              'default',
              '-l',
              `openchoreo.dev/component=${COMPONENT_NAME}`,
              '-o',
              'jsonpath={range .items[*]}{.spec.environment}{"\\n"}{end}',
            ],
            { check: false },
          )
            .stdout.split('\n')
            .filter(e => e.trim() === 'staging').length,
        { timeout: 120_000 },
      )
      .toBe(1);

    // Production: explicit deny. The UI gates Promote client-side per
    // target environment, so the deny renders staging's Promote button
    // disabled with a permission tooltip (no click → server-deny path).
    await component.expectPromoteDenied('production');

    // Assert via .spec.environment rather than an environment label — the
    // label set on the ReleaseBinding CR itself is not part of the API
    // contract, but spec.environment is.
    await expect
      .poll(
        () =>
          kubectl(
            [
              'get',
              'releasebinding',
              '-n',
              'default',
              '-l',
              `openchoreo.dev/component=${COMPONENT_NAME}`,
              '-o',
              'jsonpath={range .items[*]}{.spec.environment}{"\\n"}{end}',
            ],
            { check: false },
          )
            .stdout.split('\n')
            .filter(e => e.trim() === 'production').length,
        { timeout: 30_000 },
      )
      .toBe(0);
  });

  test('relogin clears Casbin evaluation cache (regression for backstage-plugins#549)', async ({
    page,
  }) => {
    test.setTimeout(300_000);
    const sidebar = new SidebarPO(page);
    const component = new ComponentPO(page);
    const release = new ReleasePO(page);

    // Sign out, then sign back in. Even though we already have a stored
    // session, sign-out → sign-in is what reproduces the cache-eviction bug.
    await page.goto('/');
    await sidebar.signOut();
    await page
      .getByRole('button', { name: 'Sign In', exact: true })
      .first()
      .waitFor({ state: 'visible', timeout: 30_000 });

    // Drive the same redirect-based consent flow used in fixtures/auth.ts.
    // We intentionally do NOT reuse the persisted state — the whole point of
    // this test is to walk through the full re-auth path so the Casbin
    // evaluation cache gets cleared.
    await page
      .getByRole('button', { name: 'Sign In', exact: true })
      .first()
      .click();
    await page
      .getByPlaceholder('Enter your username')
      .fill(ROLES.abac.username);
    await page
      .getByPlaceholder('Enter your password')
      .fill(ROLES.abac.password);
    await page.getByRole('button', { name: 'Sign In', exact: true }).click();
    await page
      .getByRole('link', { name: 'Home' })
      .first()
      .waitFor({ state: 'visible', timeout: 60_000 });

    // Same permission shape must apply post-relogin: staging's Promote
    // (whose target is production) must still render denied.
    await release.openDeployTab(COMPONENT_NAME);
    await component.expectPromoteDenied('production');
  });
});
