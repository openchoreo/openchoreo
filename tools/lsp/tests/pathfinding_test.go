// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"testing"

	"gopkg.in/yaml.v3"

	lsputil "github.com/openchoreo/openchoreo/tools/lsp/lsp-util"
)

func parseDocument(t *testing.T, doc string) *yaml.Node {
	t.Helper()
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(doc), &root); err != nil {
		t.Fatal(err)
	}
	return &root
}

func TestFindFieldPathAtPositionSpecKey(t *testing.T) {
	doc := `apiVersion: openchoreo.dev/v1alpha1
kind: DeploymentPipeline
metadata:
  name: standard
  namespace: default
spec:
  promotionPaths:
    - sourceEnvironmentRef:
        name: development
`
	node := parseDocument(t, doc)

	tests := []struct {
		name string
		line int
		col  int
		want string
	}{
		{"on metadata key", 3, 1, "metadata"},
		{"inside metadata body", 4, 5, "metadata.name"},
		{"on spec key", 6, 1, "spec"},
		{"on spec key trailing whitespace", 6, 3, "spec"},
		{"inside spec body", 7, 5, "spec.promotionPaths"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lsputil.FindFieldPathAtPosition(node, tt.line, tt.col)
			if got != tt.want {
				t.Errorf("FindFieldPathAtPosition(%d,%d) = %q, want %q", tt.line, tt.col, got, tt.want)
			}
		})
	}
}

func TestFindFieldPathAtPositionBlankBetweenSiblings(t *testing.T) {
	doc := `apiVersion: openchoreo.dev/v1alpha1
kind: DeploymentPipeline
metadata:
  name: standard
  namespace: default

spec:
  promotionPaths:
`
	node := parseDocument(t, doc)

	tests := []struct {
		name string
		line int
		col  int
		want string
	}{
		{"blank line before spec still belongs to metadata", 6, 1, "metadata"},
		{"on spec key", 7, 1, "spec"},
		{"inside spec body", 8, 5, "spec.promotionPaths"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lsputil.FindFieldPathAtPosition(node, tt.line, tt.col)
			if got != tt.want {
				t.Errorf("FindFieldPathAtPosition(%d,%d) = %q, want %q", tt.line, tt.col, got, tt.want)
			}
		})
	}
}
