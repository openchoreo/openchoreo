// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"os"
	"regexp"
	"testing"

	"sigs.k8s.io/yaml"
)

// TestWorkloadSourceCRDValidations pins the constraints on spec.source in the
// generated Workload CRD. They are declared as kubebuilder markers on
// WorkloadSource and only take effect once `make manifests` has run, so a marker
// dropped in a refactor -- or a regeneration that silently omits one -- would
// leave the API accepting provenance it should reject, with nothing else failing.
//
// The commit pattern is the one that earns its keep: --source-branch sits next to
// --source-commit, so passing a branch or tag as the commit is an easy mistake,
// and without the pattern it would be stored as provenance pointing at nothing.
func TestWorkloadSourceCRDValidations(t *testing.T) {
	const crdPath = "../../config/crd/bases/openchoreo.dev_workloads.yaml"

	raw, err := os.ReadFile(crdPath)
	if err != nil {
		t.Fatalf("reading %s: %v", crdPath, err)
	}

	var crd struct {
		Spec struct {
			Versions []struct {
				Schema struct {
					OpenAPIV3Schema map[string]any `json:"openAPIV3Schema"`
				} `json:"schema"`
			} `json:"versions"`
		} `json:"spec"`
	}
	if err := yaml.Unmarshal(raw, &crd); err != nil {
		t.Fatalf("unmarshalling CRD: %v", err)
	}
	if len(crd.Spec.Versions) == 0 {
		t.Fatal("CRD has no versions")
	}

	// spec.source lives under the top-level schema's spec property.
	node := crd.Spec.Versions[0].Schema.OpenAPIV3Schema
	for _, key := range []string{"spec", "source"} {
		props, ok := node["properties"].(map[string]any)
		if !ok {
			t.Fatalf("no properties while descending to %q", key)
		}
		next, ok := props[key].(map[string]any)
		if !ok {
			t.Fatalf("CRD schema has no %q property; is spec.source still generated?", key)
		}
		node = next
	}
	fields, ok := node["properties"].(map[string]any)
	if !ok {
		t.Fatal("spec.source has no properties")
	}

	tests := []struct {
		field     string
		minLength float64
		maxLength float64
		pattern   string
		format    string
	}{
		{field: "commit", minLength: 7, maxLength: 40, pattern: `^[0-9a-fA-F]+$`},
		{field: "branch", minLength: 1, maxLength: 255},
		{field: "repository", minLength: 1, maxLength: 2048},
		{field: "authoredAt", format: "date-time"},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			f, ok := fields[tt.field].(map[string]any)
			if !ok {
				t.Fatalf("spec.source has no %q field", tt.field)
			}
			if tt.minLength != 0 && f["minLength"] != tt.minLength {
				t.Errorf("minLength = %v, want %v", f["minLength"], tt.minLength)
			}
			if tt.maxLength != 0 && f["maxLength"] != tt.maxLength {
				t.Errorf("maxLength = %v, want %v", f["maxLength"], tt.maxLength)
			}
			if tt.pattern != "" && f["pattern"] != tt.pattern {
				t.Errorf("pattern = %v, want %v", f["pattern"], tt.pattern)
			}
			if tt.format != "" && f["format"] != tt.format {
				t.Errorf("format = %v, want %v", f["format"], tt.format)
			}
		})
	}

	// The pattern is only useful if it actually rejects the mistake it exists for.
	t.Run("pattern rejects a branch or tag passed as the commit", func(t *testing.T) {
		commit := fields["commit"].(map[string]any)
		re := regexp.MustCompile(commit["pattern"].(string))
		for _, ok := range []string{
			"9f2c1ab4d5e6f70819a2b3c4d5e6f70819a2b3c4",
			"9f2c1ab",
			"9F2C1AB4D5E6F708",
		} {
			if !re.MatchString(ok) {
				t.Errorf("pattern rejected a valid SHA %q", ok)
			}
		}
		for _, bad := range []string{"main", "v1.2.3", "release-1.3", "HEAD", "feature/x"} {
			if re.MatchString(bad) {
				t.Errorf("pattern accepted %q, which is a ref name rather than a commit", bad)
			}
		}
	})
}
