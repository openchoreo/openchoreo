// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemavalidator

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestParameterRelationships(t *testing.T) {
	for instance, rel := range parameterRelationships {
		for _, kind := range []string{rel.defaultKind, rel.clusterKind} {
			for _, scenario := range []string{"valid", "wrong-type", "required", "additional", "nested", "array", "missing-definition", testValueOtherNamespace, "duplicate", "no-schema"} {
				t.Run(instance+"/"+kind+"/"+scenario, func(t *testing.T) {
					schema := "      type: object\n      properties:\n        count:\n          type: integer\n"
					values := "    count: 3\n"
					switch scenario {
					case "wrong-type":
						values = "    count: wrong\n"
					case "required":
						schema += "      required: [count]\n"
						values = "    {}\n"
					case "additional":
						schema += "      additionalProperties: false\n"
						values = "    other: 3\n"
					case "nested":
						schema = "      type: object\n      properties:\n        child:\n          type: object\n          properties:\n            count:\n              type: integer\n"
						values = "    child:\n      count: wrong\n"
					case "array":
						schema = "      type: object\n      properties:\n        items:\n          type: array\n          items:\n            type: integer\n"
						values = "    items: [wrong]\n"
					}
					ns := "a"
					if scenario == testValueOtherNamespace {
						ns = "b"
					}
					definition := fmt.Sprintf("apiVersion: openchoreo.dev/v1alpha1\nkind: %s\nmetadata:\n  name: def\n  namespace: %s\nspec:\n  parameters:\n    openAPIV3Schema:\n%s", kind, ns, schema)
					if scenario == "no-schema" {
						definition = strings.Split(definition, "spec:")[0]
					}
					if scenario == "missing-definition" {
						definition = ""
					}
					if scenario == "duplicate" {
						definition += "---\n" + definition
					}
					binding := fmt.Sprintf("  type:\n    kind: %s\n    name: def\n  parameters:\n%s", kind, values)
					if instance == "WorkflowRun" {
						binding = fmt.Sprintf("  workflow:\n    kind: %s\n    name: def\n    parameters:\n%s", kind, indentValues(values))
					}
					input := definition + "---\n" + fmt.Sprintf("apiVersion: openchoreo.dev/v1alpha1\nkind: %s\nmetadata:\n  name: instance\n  namespace: a\nspec:\n%s", instance, binding)
					findings := ValidateDocuments(context.Background(), parseResources(t, input))
					mismatch := false
					for _, f := range findings {
						if f.Code == testValueSchemaParametersMismatch {
							mismatch = true
						}
					}
					want := scenario == "wrong-type" || scenario == "required" || scenario == "additional" || scenario == "nested" || scenario == "array"
					if mismatch != want {
						t.Fatalf("want mismatch=%v got %v: %v", want, mismatch, findings)
					}
				})
			}
		}
	}
}
func indentValues(s string) string {
	return "  " + strings.TrimSuffix(strings.ReplaceAll(s, "\n", "\n  "), "  ")
}
func TestAdditionalDefinitionSchemas(t *testing.T) {
	for kind := range additionalDefinitions {
		for _, section := range []string{"parameters", valueEnvironmentConfigs} {
			// Workflow defines only parameters.
			if strings.Contains(kind, "Workflow") && section != "parameters" {
				continue
			}
			t.Run(kind+section, func(t *testing.T) {
				input := fmt.Sprintf("apiVersion: openchoreo.dev/v1alpha1\nkind: %s\nmetadata:\n  name: def\nspec:\n  %s:\n    openAPIV3Schema:\n      type: object\n      misspelled: true\n", kind, section)
				got := ValidateDocuments(context.Background(), parseResources(t, input))
				if len(got) == 0 {
					t.Fatal("unknown schema field accepted")
				}
				for _, f := range got {
					if !strings.HasPrefix(f.Code, "schema-definition-") {
						t.Fatalf("unexpected diagnostic: %v", f)
					}
				}
			})
		}
	}
}
