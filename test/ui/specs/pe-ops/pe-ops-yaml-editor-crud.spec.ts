// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

import { test, expect, storageStateFor } from '../../fixtures/auth';
import {
  kGetJSONScoped,
  kNotFoundScoped,
  kExists,
  kDelete,
} from '../../fixtures/kube';
import { cmSetContent, cmGetContent } from '../../fixtures/codemirror';
import { DeletePO } from '../../po/delete';
import { CreatePO } from '../../po/create';
import { CatalogTablePO } from '../../po/catalogTable';
import { ScaffolderWizardPO } from '../../po/scaffolderWizard';
import { DefinitionTabPO } from '../../po/definitionTab';

// PE CRUD for CRDs that use the two-step scaffolder flow:
//   Step 1: name + description (+ namespace for namespace-scoped)
//   Step 2: CodeMirror YAML editor (pre-populated from Step 1)
//
// Each row exercises: Create via scaffolder → Update via Definition tab → Delete
// via catalog overflow menu. kubectl cross-checks every mutation.

interface YamlEditorRow {
  kind: string;
  crdKind: string;
  catalogKind: string;
  templateTitle: string;
  nameFieldLabel: string;
  kindDisplayName: string;
  namespace: string;
  name: string;
  description: string;
  // Returns YAML that should fail server-side validation when saved.
  // When present, the test verifies that saving shows an error alert
  // and the invalid change does NOT persist to the cluster.
  invalidEdit?: (yaml: string) => string;
}

const ts = Date.now().toString(36);

// One namespace-scoped and one cluster-scoped representative per variation axis:
//   - with invalidEdit (ComponentType, ClusterComponentType)
//   - without invalidEdit (Trait, ClusterTrait)
// The remaining CRDs (Workflow, ResourceType, ClusterWorkflow, ClusterResourceType)
// follow the identical UI path and are covered implicitly.
const rows: YamlEditorRow[] = [
  {
    kind: 'componenttype',
    crdKind: 'ComponentType',
    catalogKind: 'componenttype',
    templateTitle: 'ComponentType',
    nameFieldLabel: 'ComponentType Name',
    kindDisplayName: 'Component Type',
    namespace: 'default',
    name: `yaml-ct-${ts}`,
    description: 'e2e yaml editor component type',
    invalidEdit: (yaml) => {
      if (/targetPlane:/.test(yaml))
        return yaml.replace(/targetPlane:\s*\S+/, 'targetPlane: workflowplane');
      return yaml.replace(/(resources:)/m, 'targetPlane: workflowplane\n    $1');
    },
  },
  {
    kind: 'trait',
    crdKind: 'Trait',
    catalogKind: 'traittype',
    templateTitle: 'Trait',
    nameFieldLabel: 'Trait Name',
    kindDisplayName: 'Trait Type',
    namespace: 'default',
    name: `yaml-trait-${ts}`,
    description: 'e2e yaml editor trait',
  },
  {
    kind: 'clustercomponenttype',
    crdKind: 'ClusterComponentType',
    catalogKind: 'clustercomponenttype',
    templateTitle: 'ClusterComponentType',
    nameFieldLabel: 'ClusterComponentType Name',
    kindDisplayName: 'Cluster Component Type',
    namespace: '',
    name: `yaml-cct-${ts}`,
    description: 'e2e yaml editor cluster component type',
    invalidEdit: (yaml) => {
      if (/targetPlane:/.test(yaml))
        return yaml.replace(/targetPlane:\s*\S+/, 'targetPlane: workflowplane');
      return yaml.replace(/(resources:)/m, 'targetPlane: workflowplane\n    $1');
    },
  },
  {
    kind: 'clustertrait',
    crdKind: 'ClusterTrait',
    catalogKind: 'clustertraittype',
    templateTitle: 'ClusterTrait',
    nameFieldLabel: 'ClusterTrait Name',
    kindDisplayName: 'Cluster Trait Type',
    namespace: '',
    name: `yaml-ctrait-${ts}`,
    description: 'e2e yaml editor cluster trait',
  },
];

test.describe.configure({ mode: 'serial' });

test.describe('pe-ops: YAML editor CRUD for PE CRDs', () => {
  test.beforeAll(async ({ mintAuthState }) => {
    await mintAuthState('pe');
  });
  test.use({ storageState: storageStateFor('pe') });

  test.afterAll(async () => {
    for (const row of rows) kDelete(row.kind, row.name, row.namespace);
  });

  for (const row of rows) {
    test(`${row.kind}: create → update → delete`, async ({ page }) => {
      test.setTimeout(300_000);
      const catalog = new CatalogTablePO(page);
      const del = new DeletePO(page);

      // ── CREATE ──
      await page.goto('/');
      await new CreatePO(page).chooseTemplate(row.templateTitle);
      const wizard = new ScaffolderWizardPO(page);

      await wizard.fillField(row.nameFieldLabel, row.name);
      await wizard.fillField('Description', row.description);

      // Advance to YAML editor step
      await wizard.next();

      // Wait for the CodeMirror editor to render and verify it has content
      // from Step 1 (name + description seeded into the YAML template).
      const prePopulated = await cmGetContent(page);
      expect(prePopulated).toContain(row.name);

      // Edit the YAML to prove that create-time changes are wired to
      // scaffolder state — replace the description in the editor.
      const createDesc = `${row.description} via yaml`;
      const editedYaml = prePopulated.replace(row.description, createDesc);
      await cmSetContent(page, editedYaml);

      // Submit the wizard (Review → Create)
      await wizard.submit();

      // Poll kubectl until the CR exists
      await expect
        .poll(
          () => kExists(row.kind, row.name, row.namespace),
          { timeout: 60_000 },
        )
        .toBe(true);

      // Verify the YAML-edited description round-tripped into the CRD
      const created = kGetJSONScoped<{
        kind: string;
        metadata: { annotations?: Record<string, string> };
      }>(row.kind, row.name, row.namespace);
      expect(created.kind).toBe(row.crdKind);
      expect(
        created.metadata.annotations?.['openchoreo.dev/description'],
      ).toBe(createDesc);

      // ── UPDATE via Definition Tab ──
      // Wait for catalog provider to ingest the entity, then open it
      await catalog.openEntity(row.catalogKind, row.name, 90_000);

      const defTab = new DefinitionTabPO(page);
      await defTab.openViaEditIcon();

      // Modify the description annotation in the YAML
      const updatedDesc = `${createDesc} updated`;
      const currentYaml = await cmGetContent(page);
      const newYaml = currentYaml.replace(createDesc, updatedDesc);
      await cmSetContent(page, newYaml);

      // Save and verify success
      await defTab.save();
      await defTab.expectSaveSuccess();

      // Poll kubectl until the change is reflected
      await expect
        .poll(
          () => {
            const obj = kGetJSONScoped<{
              metadata: { annotations?: Record<string, string> };
            }>(row.kind, row.name, row.namespace);
            return obj.metadata.annotations?.['openchoreo.dev/description'];
          },
          { timeout: 60_000 },
        )
        .toBe(updatedDesc);

      // ── INVALID EDIT — verify server rejects bad schema ──
      if (row.invalidEdit) {
        const validYaml = await cmGetContent(page);
        const badYaml = row.invalidEdit(validYaml);
        expect(badYaml, 'invalidEdit must mutate the YAML').not.toBe(validYaml);

        await cmSetContent(page, badYaml);
        await defTab.save();
        await defTab.expectSaveError();

        // Verify the invalid change did NOT persist to the cluster
        const afterReject = kGetJSONScoped<{
          spec: { targetPlane?: string };
        }>(row.kind, row.name, row.namespace);
        expect(afterReject.spec.targetPlane).not.toBe('workflowplane');

        // Discard the invalid change so the editor is clean for delete
        await defTab.discard();
      }

      // ── DELETE ──
      // Navigate back to the entity page via catalog
      await catalog.openEntity(row.catalogKind, row.name, 60_000);

      await del.openOverflowAndDelete(row.kindDisplayName);
      await del.confirm();

      // Poll kubectl until the CR is gone
      await expect
        .poll(
          () => kNotFoundScoped(row.kind, row.name, row.namespace),
          { timeout: 60_000 },
        )
        .toBe(true);
    });
  }
});
