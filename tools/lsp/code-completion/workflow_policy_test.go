// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codecompletion

import (
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/tools/lint/indexer"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

func workflowIndex(t *testing.T, allowed string) dynamicTestIndex {
	t.Helper()
	docs, err := parser.ParseYAML([]byte("apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentType\nmetadata: {name: web-application, namespace: default}\nspec:\n  workloadType: deployment\n  allowedWorkflows: " + allowed + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return dynamicTestIndex{[]indexer.SnapshotDocument{{Document: docs[0]}}}
}

const workflowComponent = "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: app\n  namespace: default\nspec:\n  componentType:\n    kind: ComponentType\n    name: deployment/web-application\n"

func TestWorkflowPreviewUsesAllowedReference(t *testing.T) {
	idx := workflowIndex(t, "[{kind: Workflow, name: docker-gitops-release}]")
	text := workflowComponent + "  "
	preview := RequiredPreviewWithIndex(text, 9, 2, "  ", "", idx)
	if !strings.Contains(preview, "kind: Workflow") || !strings.Contains(preview, "name: docker-gitops-release") || strings.Contains(preview, "ClusterWorkflow") {
		t.Fatal(preview)
	}
	// Explicitly empty allowlists must not manufacture a workflow block.
	idx = workflowIndex(t, "[]")
	if preview := RequiredPreviewWithIndex(text, 9, 2, "  ", "", idx); strings.Contains(preview, "workflow:") {
		t.Fatal(preview)
	}
}
func TestWorkflowDropdownPairs(t *testing.T) {
	idx := workflowIndex(t, "[{kind: Workflow, name: local-build}, {kind: ClusterWorkflow, name: shared-build}]")
	for _, tc := range []struct{ tail, want string }{
		{"  workflow:\n    kind: Workflow\n    name: ", "local-build"},
		{"  workflow:\n    name: shared-build\n    kind: ", "ClusterWorkflow"},
	} {
		text := workflowComponent + tc.tail
		lines := strings.Split(text, "\n")
		items, ok := workflowPolicyItems(text, len(lines), len(lines[len(lines)-1])+1, "", idx)
		if !ok || len(items) != 1 || items[0].Label != tc.want {
			t.Fatalf("%v %+v", ok, items)
		}
	}
	// Never use a definition from another namespace.
	idx.docs[0].Document.Metadata.Namespace = "other"
	if _, ok, _ := allowedWorkflowChoices(workflowComponent, 9, "", idx); ok {
		t.Fatal("namespace leak")
	}
}
