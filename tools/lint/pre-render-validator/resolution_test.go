// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemavalidator

import (
	"context"
	"fmt"
	"testing"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

func parseResources(t *testing.T, text string) []Resource {
	t.Helper()
	docs, err := parser.ParseYAML([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	result := make([]Resource, 0, len(docs))
	for _, d := range docs {
		result = append(result, Resource{Path: "input.yaml", Document: d})
	}
	return result
}

func TestComponentAndTraitResolution(t *testing.T) {
	for _, cluster := range []bool{false, true} {
		for _, trait := range []bool{false, true} {
			for _, scenario := range []string{"valid", "invalid", testValueOtherNamespace, "duplicate"} {
				t.Run(fmt.Sprint(cluster, trait, scenario), func(t *testing.T) {
					kind := valueComponentType
					ref := testValueComponentTypeValue
					if trait {
						kind = valueTrait
						ref = "traits"
					}
					if cluster {
						kind = "Cluster" + kind
					}
					ns := "a"
					if scenario == testValueOtherNamespace {
						ns = "b"
					}
					definition := fmt.Sprintf("apiVersion: openchoreo.dev/v1alpha1\nkind: %s\nmetadata:\n  name: def\n  namespace: %s\nspec:\n  parameters:\n    openAPIV3Schema:\n      type: object\n      properties:\n        count:\n          type: integer\n", kind, ns)
					value := "2"
					if scenario != "valid" {
						value = testValueWrong
					}
					binding := fmt.Sprintf("  %s:\n    kind: %s\n    name: def\n  parameters:\n    count: %s\n", ref, kind, value)
					if trait {
						binding = fmt.Sprintf("  traits:\n    - kind: %s\n      name: def\n      instanceName: test\n      parameters:\n        count: %s\n", kind, value)
					}
					input := definition + "---\napiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: app\n  namespace: a\nspec:\n" + binding
					if scenario == "duplicate" {
						input += "---\n" + definition
					}
					findings := ValidateDocuments(context.Background(), parseResources(t, input))
					mismatch := false
					for _, f := range findings {
						if f.Code == testValueSchemaParametersMismatch {
							mismatch = true
						}
					}
					want := scenario == "invalid" || (scenario == testValueOtherNamespace && cluster)
					if mismatch != want {
						t.Fatalf("mismatch=%v want=%v findings=%v", mismatch, want, findings)
					}
				})
			}
		}
	}
}

func TestPrefixedComponentTypeSchemaEdit(t *testing.T) {
	input := `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: web-application
  namespace: default
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        replicas:
          type: string
---
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: web-app
  namespace: default
spec:
  componentType:
    kind: ComponentType
    name: deployment/web-application
  parameters:
    replicas: 1
`
	findings := ValidateDocuments(context.Background(), parseResources(t, input))
	for _, f := range findings {
		if f.Code == testValueSchemaParametersMismatch {
			return
		}
	}
	t.Fatalf("missing parameter mismatch: %v", findings)
}
