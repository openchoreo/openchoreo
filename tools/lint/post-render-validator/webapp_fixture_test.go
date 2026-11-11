// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

// TestWebappFixture_AutoscalingConflict exercises the webapp fixture:
//
//   - the ComponentType renders a Deployment with one replica;
//   - the autoscaling trait patches replicas from 1 to 3;
//   - the storage trait declares "replicas <= 1", which 3 violates.
//
// It also pins the important contract that the rendered Deployment is still
// returned alongside the failure, so a caller can report it.
func TestWebappFixture_AutoscalingConflict(t *testing.T) {
	dir := filepath.Join("testdata", "webapp-conflict")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read checked-in fixture: %v", err)
	}

	var docs []*parser.DocumentNode
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		parsed, err := parser.ParseYAML(data)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		docs = append(docs, parsed...)
	}

	bundle, err := BundleFromDocuments(docs)
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}

	out, failures, err := RenderBundle(t.Context(), bundle)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if len(failures) != 1 {
		t.Fatalf("failures = %v, want exactly the storage replica failure", failures)
	}
	if !strings.Contains(failures[0].Message, "storage requires at most 1 replica") {
		t.Fatalf("failure message = %q, want the storage replica conflict", failures[0].Message)
	}
	if failures[0].Label != "Trait storage/data" {
		t.Fatalf("failure label = %q, want Trait storage/data", failures[0].Label)
	}

	if out == nil || len(out.Resources) == 0 {
		t.Fatal("rendered output is empty; resources must be returned alongside the failure")
	}

	deployment := findResource(out.Resources, "Deployment")
	if deployment == nil {
		t.Fatal("rendered Deployment not found")
	}
	spec, ok := deployment["spec"].(map[string]any)
	if !ok {
		t.Fatal("rendered Deployment has no spec")
	}
	replicas, ok := numericValue(spec["replicas"])
	if !ok {
		t.Fatalf("rendered Deployment replicas = %v, want 3", spec["replicas"])
	}
	if replicas != 3 {
		t.Fatalf("rendered Deployment replicas = %d, want 3 (autoscaling trait)", replicas)
	}
}

// findResource returns the first rendered resource with the given kind.
func findResource(resources []renderer.RenderedResource, kind string) map[string]any {
	for _, r := range resources {
		if k, _ := r.Resource["kind"].(string); k == kind {
			return r.Resource
		}
	}
	return nil
}
