// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package schemavalidator

import (
	"context"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
	"testing"
)

const runtimeFixture = `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: type
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        count:
          type: integer
          default: 2
  preRenderValidations:
    - rule: ${parameters.count > 3}
      message: count must exceed three
---
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: app
spec:
  componentType:
    name: deployment/type
`

func TestRuntimeConcreteRules(t *testing.T) {
	for _, tc := range []struct {
		name, expr string
		want       int
	}{
		{"false", "parameters.count > 3", 1}, {"true", "parameters.count > 1", 0},
		{"unavailable", "metadata.name == 'app'", 0}, {"syntax", "parameters.count >", 0},
		{"unknown", "unknown.value > 1", 0}, {"type", "parameters.count + 'text'", 0},
		{"comprehension", "[1,2].all(n, n > parameters.count)", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := strings.Replace(runtimeFixture, "parameters.count > 3", tc.expr, 1)
			got := ValidateDocuments(context.Background(), parseResources(t, text))
			if len(got) != tc.want {
				t.Fatalf("want=%d got=%+v", tc.want, got)
			}
			if tc.want > 0 && got[0].Code != "pre-render-validation-failed" {
				t.Fatalf("unexpected: %+v", got)
			}
		})
	}
}
func TestRuntimeSnapshotUsesFrozenParameters(t *testing.T) {
	// Construct via node views to retain exact schema/rule YAML without string indentation tricks.
	resources := parseResources(t, runtimeFixture)
	release := parseResources(t, "apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentRelease\nmetadata:\n  name: frozen\nspec: {}\n")[0]
	definition := resources[0].Document.Spec
	release.Document.Spec.Content = append(release.Document.Spec.Content,
		nodeKey("componentType"), nodeMap("spec", definition))
	got := ValidateDocuments(context.Background(), []Resource{release})
	if len(got) != 1 || got[0].Code != "pre-render-validation-failed" {
		t.Fatalf("snapshot runtime: %+v", got)
	}
}

func nodeKey(k string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Value: k} }
func nodeMap(k string, v *yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{nodeKey(k), v}}
}

func TestProjectParameterRule(t *testing.T) {
	text := strings.NewReplacer("kind: ComponentType", "kind: ProjectType", "kind: Component\n", "kind: Project\n", "componentType:", "type:", "deployment/type", "type", "preRenderValidations:", "validations:").Replace(runtimeFixture)
	got := ValidateDocuments(context.Background(), parseResources(t, text))
	if len(got) != 1 || got[0].Code != "pre-render-validation-failed" {
		t.Fatalf("project rules: %+v", got)
	}
}
func TestPinnedEnvironmentRule(t *testing.T) {
	text := `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentRelease
metadata:
  name: frozen
spec:
  componentType:
    spec:
      environmentConfigs:
        openAPIV3Schema:
          type: object
          properties:
            count:
              type: integer
              default: 2
      preRenderValidations:
        - rule: ${environmentConfigs.count > 3}
          message: count too small
---
apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseBinding
metadata:
  name: binding
spec:
  releaseName: frozen
`
	got := ValidateDocuments(context.Background(), parseResources(t, text))
	if len(got) != 1 || got[0].Code != "pre-render-validation-failed" || got[0].Line < 23 {
		t.Fatalf("pinned environment context: %+v", got)
	}
	got = ValidateDocuments(context.Background(), parseResources(t, strings.Replace(text, "count > 3", "count > 1", 1)))
	if len(got) != 0 {
		t.Fatalf("defaulted environment rule should pass: %+v", got)
	}
}

func TestRuntimeTraitInstances(t *testing.T) {
	for _, kind := range []string{"Trait", "ClusterTrait"} {
		text := strings.Replace(runtimeFixture, "count > 3", "count > 1", 1)
		text += "  traits:\n    - kind: " + kind + "\n      name: scaling\n      instanceName: scale\n---\napiVersion: openchoreo.dev/v1alpha1\nkind: " + kind + "\nmetadata:\n  name: scaling\nspec:\n  parameters:\n    openAPIV3Schema:\n      type: object\n      properties:\n        count:\n          type: integer\n          default: 2\n  preRenderValidations:\n    - rule: ${parameters.count > 3}\n      message: too small\n"
		got := ValidateDocuments(context.Background(), parseResources(t, text))
		if len(got) != 1 || got[0].Code != "pre-render-validation-failed" {
			t.Fatalf("%s rules: %+v", kind, got)
		}
	}
}
func TestRuntimeFrozenTraitRule(t *testing.T) {
	data, err := os.ReadFile("testdata/releases.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(data), "replicas: wrong", "replicas: 1", 1)
	text += "        preRenderValidations:\n          - rule: ${parameters.replicas > 3}\n            message: too small\n"
	got := ValidateDocuments(context.Background(), parseResources(t, text))
	count := 0
	for _, f := range got {
		if f.Code == "pre-render-validation-failed" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("component and frozen Trait rules must both fail: %+v", got)
	}
}

func TestProjectStaticCELContext(t *testing.T) {
	for _, kind := range []string{"ProjectType", "ClusterProjectType"} {
		for _, tc := range []struct{ expr, code string }{
			{"parameters.count > 1", ""}, {"metadata.projectName != ''", ""}, {"parameters.count >", "cel-parse-error"},
			{"parameters.missing > 1", "cel-type-error"}, {"parameters.count + 'text'", "cel-unknown-function"},
		} {
			text := strings.Split(runtimeFixture, "---")[0]
			text = strings.Replace(text, "kind: ComponentType", "kind: "+kind, 1)
			text = strings.Replace(text, "preRenderValidations:", "validations:", 1)
			text = strings.Replace(text, "parameters.count > 3", tc.expr, 1)
			got := ValidateDocuments(context.Background(), parseResources(t, text))
			if tc.code == "" {
				if len(got) > 0 {
					t.Fatalf("valid project CEL: %+v", got)
				}
				continue
			}
			if len(got) != 1 || got[0].Code != tc.code {
				t.Fatalf("%s %s: %+v", kind, tc.expr, got)
			}
		}
	}
}

func TestFailedRuleValueLocations(t *testing.T) {
	for _, tc := range []struct{ name, expr, values, token string }{
		{"single", "parameters.count > 3", "    count: 2", "count: 2"},
		{"nested", "parameters.tuning.count > 3", "    tuning:\n      count: 2", "count: 2"},
		{"multiple", "parameters.count > 3 && parameters.other > 3", "    count: 2\n    other: 2", "parameters:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata: {name: type}
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        count: {type: integer}
        other: {type: integer}
        tuning:
          type: object
          properties:
            count: {type: integer}
  preRenderValidations:
    - rule: '${` + tc.expr + `}'
      message: too small
---
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata: {name: app}
spec:
  componentType: {kind: ComponentType, name: deployment/type}
  parameters:
` + tc.values + "\n"
			resources := parseResources(t, source)
			got := ValidateDocuments(context.Background(), resources)
			if len(got) != 1 {
				t.Fatalf("%+v", got)
			}
			want := 0
			for i, line := range strings.Split(source, "\n") {
				if strings.Contains(line, tc.token) {
					want = i + 1
				}
			}
			if tc.name == "multiple" {
				want = nodeAt(resources[1].Document.Spec, "parameters").Line
				if got[0].Column != nodeAt(resources[1].Document.Spec, "parameters").Column {
					t.Fatal(got[0])
				}
			}
			if got[0].Line != want {
				t.Fatalf("want line %d, got %+v", want, got[0])
			}
		})
	}
}
