// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package schemavalidator

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseSnapshots(t *testing.T) {
	for _, kind := range []string{"ComponentRelease", "ResourceRelease", "ProjectRelease"} {
		for _, bad := range []bool{false, true} {
			t.Run(fmt.Sprint(kind, bad), func(t *testing.T) {
				field, values := "componentType", "  componentProfile:\n    parameters:\n      count: wrong\n"
				if kind == "ResourceRelease" {
					field, values = "resourceType", "  parameters:\n    count: wrong\n"
				}
				if kind == "ProjectRelease" {
					field, values = "projectType", "  parameters:\n    count: wrong\n"
				}
				if !bad {
					values = strings.ReplaceAll(values, "wrong", "3")
				}
				text := fmt.Sprintf("apiVersion: openchoreo.dev/v1alpha1\nkind: %s\nmetadata:\n  name: release\nspec:\n  %s:\n    spec:\n      parameters:\n        openAPIV3Schema:\n          type: object\n          properties:\n            count:\n              type: integer\n%s", kind, field, values)
				got := ValidateDocuments(context.Background(), parseResources(t, text))
				if (len(got) > 0) != bad {
					t.Fatalf("bad=%v got=%+v", bad, got)
				}
				if bad && got[0].Line < 14 {
					t.Fatalf("snapshot location incorrect: %+v", got)
				}
			})
		}
	}
}
func TestEmbeddedTraitSnapshots(t *testing.T) {
	for _, kind := range []string{"Trait", "ClusterTrait"} {
		text := fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: ComponentRelease
metadata:
  name: release
spec:
  traits:
    - kind: %s
      name: scaling
      spec:
        parameters:
          openAPIV3Schema:
            type: object
            properties:
              count:
                type: integer
  componentProfile:
    traits:
      - kind: %s
        name: scaling
        instanceName: scale
        parameters:
          count: wrong
`, kind, kind)
		got := ValidateDocuments(context.Background(), parseResources(t, text))
		if len(got) != 1 || got[0].Code != "schema-parameters-mismatch" || got[0].Line != 22 {
			t.Fatalf("%s: %+v", kind, got)
		}
		text = strings.Replace(text, "type: integer", "types: integer", 1)
		got = ValidateDocuments(context.Background(), parseResources(t, text))
		found := false
		for _, f := range got {
			if f.Code == "schema-definition-invalid-fields" && f.Line == 15 {
				found = true
			}
		}
		if !found {
			t.Fatalf("embedded schema error missing: %+v", got)
		}
	}
}
func TestBindingEnvironmentSchemasAndDefaults(t *testing.T) {
	for _, binding := range []string{"ReleaseBinding", "ResourceReleaseBinding", "ProjectReleaseBinding"} {
		pin, release, definition, values := "releaseName", "ComponentRelease", "componentType", "componentTypeEnvironmentConfigs"
		if binding == "ResourceReleaseBinding" {
			pin, release, definition, values = "resourceRelease", "ResourceRelease", "resourceType", "resourceTypeEnvironmentConfigs"
		}
		if binding == "ProjectReleaseBinding" {
			pin, release, definition, values = "projectRelease", "ProjectRelease", "projectType", "environmentConfigs"
		}
		for _, value := range []string{"wrong", "3", ""} {
			text := fmt.Sprintf("apiVersion: openchoreo.dev/v1alpha1\nkind: %s\nmetadata:\n  name: frozen\n  namespace: test\nspec:\n  %s:\n    spec:\n      environmentConfigs:\n        openAPIV3Schema:\n          type: object\n          required: [count]\n          properties:\n            count:\n              type: integer\n              default: 2\n---\napiVersion: openchoreo.dev/v1alpha1\nkind: %s\nmetadata:\n  name: binding\n  namespace: test\nspec:\n  %s: frozen\n", release, definition, binding, pin)
			if value != "" {
				text += fmt.Sprintf("  %s:\n    count: %s\n", values, value)
			}
			got := ValidateDocuments(context.Background(), parseResources(t, text))
			if (len(got) > 0) != (value == "wrong") {
				t.Fatalf("%s value=%q findings=%+v", binding, value, got)
			}
			if value == "wrong" && got[0].Code != "schema-environment-configs-mismatch" {
				t.Fatalf("wrong code: %+v", got)
			}
		}
	}
}

func TestMultiDocumentReleaseAcceptance(t *testing.T) {
	got := ValidateFiles(context.Background(), []string{"testdata/releases.yaml"})
	counts := map[string]int{}
	for _, f := range got {
		counts[f.Code]++
		if f.Line < 1 || f.Column < 1 {
			t.Fatalf("invalid location: %+v", f)
		}
	}
	if counts["schema-parameters-mismatch"] != 2 || counts["schema-environment-configs-mismatch"] != 1 || counts["pre-render-validation-failed"] != 1 {
		t.Fatalf("unexpected coverage: %+v findings=%+v", counts, got)
	}
}
func TestMalformedFileDoesNotHideOtherFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.yaml")
	if err := os.WriteFile(path, []byte("apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nspec: [broken\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := ValidateFiles(context.Background(), []string{path, "testdata/releases.yaml"})
	if len(got) != 4 {
		t.Fatalf("malformed neighbor changed valid-file findings: %+v", got)
	}
}
func TestEmbeddedBindingsUseComponentParameters(t *testing.T) {
	text := runtimeFixture
	resources := parseResources(t, text)
	embedded := parseResources(t, `apiVersion: openchoreo.dev/v1alpha1
kind: Trait
metadata:
  name: scaling
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        count:
          type: string
`)[0]
	resources[0].Document.Spec.Content = append(resources[0].Document.Spec.Content, nodeKey("traits"), &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{nodeMap("name", nodeKey("scaling"))}})
	binding := nodeAt(resources[0].Document.Spec, "traits").Content[0]
	binding.Content = append(binding.Content, nodeKey("parameters"), nodeMap("count", nodeKey("${parameters.count}")))
	embedded.Path = "trait.yaml"
	resources = append(resources, embedded)
	got := ValidateDocuments(context.Background(), resources)
	found := false
	for _, f := range got {
		if f.Code == "schema-parameters-mismatch" {
			found = true
		}
	}
	if !found {
		t.Fatalf("binding mismatch missing: %+v", got)
	}
}
