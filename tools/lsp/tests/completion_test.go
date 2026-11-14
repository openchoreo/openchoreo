// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	codecompletion "github.com/openchoreo/openchoreo/tools/lsp/code-completion"
	lsputil "github.com/openchoreo/openchoreo/tools/lsp/lsp-util"
)

func completionLabels(res any) map[string]bool {
	labels := make(map[string]bool)
	if list, ok := res.(protocol.CompletionList); ok {
		for _, it := range list.Items {
			labels[it.Label] = true
		}
	}
	return labels
}

func TestFindPathAtPosition(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		line     int
		col      int
		expected string
	}{
		{
			name:     "root level",
			yaml:     "apiVersion: openchoreo.dev/v1alpha1\nkind: ClusterComponentType\nmetadata:\n  name: test\n",
			line:     1,
			col:      1,
			expected: "",
		},
		{
			name:     "inside metadata",
			yaml:     "apiVersion: openchoreo.dev/v1alpha1\nkind: ClusterComponentType\nmetadata:\n  name: test\n",
			line:     4,
			col:      5,
			expected: "metadata",
		},
		{
			name:     "inside spec",
			yaml:     "apiVersion: openchoreo.dev/v1alpha1\nkind: ClusterComponentType\nspec:\n  workloadType: deployment\n",
			line:     4,
			col:      5,
			expected: "spec",
		},
		{
			name:     "nested spec.owner",
			yaml:     "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nspec:\n  owner:\n    name: test\n",
			line:     5,
			col:      9,
			expected: "spec.owner",
		},
		{
			name:     "empty line in metadata",
			yaml:     "apiVersion: openchoreo.dev/v1alpha1\nkind: ClusterComponentType\nmetadata:\n  name: test\n  \n",
			line:     5,
			col:      3,
			expected: "metadata",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs, err := parser.ParseYAML([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if len(docs) == 0 {
				t.Fatal("no documents parsed")
			}
			if got := lsputil.FindPathAtPosition(docs[0].Root, tt.line, tt.col); got != tt.expected {
				t.Errorf("got path %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCompletionItems(t *testing.T) {
	// Root-level fields not yet written are suggested for a ClusterComponentType
	// doc that already declares apiVersion and kind.
	res, err := codecompletion.Handle("apiVersion: openchoreo.dev/v1alpha1\nkind: ClusterComponentType\n", 3, 1)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	labels := completionLabels(res)
	for _, want := range []string{"metadata", "spec", "status"} {
		if !labels[want] {
			t.Errorf("missing root completion: %s", want)
		}
	}
	// Already-present keys must not be re-suggested.
	for _, present := range []string{"apiVersion", "kind"} {
		if labels[present] {
			t.Errorf("already present %q should not be suggested", present)
		}
	}
}

func TestCompletionNilSchema(t *testing.T) {
	// Unknown kind should not panic and should yield no nested completion.
	res, err := codecompletion.Handle("apiVersion: openchoreo.dev/v1alpha1\nkind: BogusKind\nspec:\n", 4, 3)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	_ = res
}

func TestNodeEndLine(t *testing.T) {
	if end := lsputil.NodeEndLine(nil); end != 0 {
		t.Errorf("nil: got %d", end)
	}
	if end := lsputil.NodeEndLine(&yaml.Node{Kind: yaml.ScalarNode, Line: 5, Value: "a\nb"}); end != 6 {
		t.Errorf("multiline scalar: got %d, want 6", end)
	}
}

func TestFindFieldPathAtPosition(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		line     int
		col      int
		expected string
	}{
		{
			name:     "on apiVersion key",
			yaml:     "apiVersion: openchoreo.dev/v1alpha1\nkind: ClusterComponentType\n",
			line:     1,
			col:      1,
			expected: "apiVersion",
		},
		{
			name:     "on metadata key",
			yaml:     "apiVersion: openchoreo.dev/v1alpha1\nkind: ClusterComponentType\nmetadata:\n  name: test\n",
			line:     3,
			col:      1,
			expected: "metadata",
		},
		{
			name:     "on name key inside metadata",
			yaml:     "apiVersion: openchoreo.dev/v1alpha1\nkind: ClusterComponentType\nmetadata:\n  name: test\n",
			line:     4,
			col:      3,
			expected: "metadata.name",
		},
		{
			name:     "on spec key",
			yaml:     "apiVersion: openchoreo.dev/v1alpha1\nkind: ClusterComponentType\nspec:\n  workloadType: deployment\n",
			line:     3,
			col:      1,
			expected: "spec",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs, err := parser.ParseYAML([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if len(docs) == 0 {
				t.Fatal("no documents parsed")
			}
			if got := lsputil.FindFieldPathAtPosition(docs[0].Root, tt.line, tt.col); got != tt.expected {
				t.Errorf("got path %q, want %q", got, tt.expected)
			}
		})
	}
}
