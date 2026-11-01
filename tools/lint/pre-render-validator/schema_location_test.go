package schemavalidator

import (
	"context"
	"fmt"
	"testing"
)

func TestDefinitionUnknownFieldLocations(t *testing.T) {
	for _, kind := range []string{"ComponentType", "ClusterComponentType", "Trait", "ClusterTrait"} {
		for _, section := range []string{"parameters", "environmentConfigs"} {
			t.Run(kind+section, func(t *testing.T) {
				text := fmt.Sprintf("apiVersion: openchoreo.dev/v1alpha1\nkind: %s\nmetadata:\n  name: example\nspec:\n  %s:\n    openAPIV3Schema:\n      type: object\n      properties:\n        replicas:\n          types: integer\n", kind, section)
				got := ValidateDocuments(context.Background(), parseResources(t, text))
				if len(got) == 0 {
					t.Fatal("missing error")
				}
				for _, f := range got {
					if f.Code == "schema-definition-invalid-fields" {
						if f.Line != 11 || f.Column != 11 {
							t.Fatalf("incorrect location: %+v", f)
						}
						return
					}
				}
				t.Fatalf("missing strict field error: %+v", got)
			})
		}
	}
}

func TestNestedValuePathLocations(t *testing.T) {
	resources := parseResources(t, "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nspec:\n  parameters:\n    items:\n      - count: wrong\n")
	node := findMappingValue(resources[0].Document.Spec, "parameters")
	for _, path := range []string{"items[0].count", "items.0.count"} {
		got := valuePathNode(node, path)
		if got == nil || got.Line != 6 || got.Value != "wrong" {
			t.Fatalf("%s: incorrect node: %+v", path, got)
		}
	}
	got := valuePathNode(node, "items[0].missing")
	if got == nil || got.Line != 6 {
		t.Fatalf("missing field location: %+v", got)
	}
}

func TestSchemaDefinitionAcceptance(t *testing.T) {
	for _, kind := range []string{"ComponentType", "ClusterComponentType", "Trait", "ClusterTrait"} {
		for _, section := range []string{"parameters", "environmentConfigs"} {
			for _, tc := range []struct{ name, body, code string }{
				{"valid", "      type: object\n", ""},
				{"malformed", "      not-a-schema\n", "schema-definition-parse-error"},
				{"unknown", "      type: object\n      types: object\n", "schema-definition-invalid-fields"},
				{"reference", "      type: object\n      properties:\n        child:\n          $ref: '#/$defs/missing'\n", "schema-definition-parse-error"},
			} {
				t.Run(kind+"/"+section+"/"+tc.name, func(t *testing.T) {
					text := fmt.Sprintf("apiVersion: openchoreo.dev/v1alpha1\nkind: %s\nmetadata:\n  name: definition\nspec:\n  %s:\n    openAPIV3Schema:\n%s", kind, section, tc.body)
					got := ValidateDocuments(context.Background(), parseResources(t, text))
					if tc.code == "" {
						if len(got) != 0 {
							t.Fatalf("valid definition: %+v", got)
						}
						return
					}
					for _, f := range got {
						if f.Code == tc.code {
							if f.Line < 7 {
								t.Fatalf("wrong schema location: %+v", f)
							}
							return
						}
					}
					t.Fatalf("missing %s: %+v", tc.code, got)
				})
			}
			text := fmt.Sprintf("apiVersion: openchoreo.dev/v1alpha1\nkind: %s\nmetadata:\n  name: definition\nspec: {}\n", kind)
			if got := ValidateDocuments(context.Background(), parseResources(t, text)); len(got) != 0 {
				t.Fatalf("missing optional schemas: %+v", got)
			}
		}
	}
}

func TestEveryNestedValueErrorHasItsOwnLocation(t *testing.T) {
	text := `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: definition
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        nested:
          type: object
          properties:
            count:
              type: integer
        items:
          type: array
          items:
            type: integer
---
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: app
spec:
  componentType:
    name: deployment/definition
  parameters:
    nested:
      count: wrong
    items:
      - wrong
`
	got := ValidateDocuments(context.Background(), parseResources(t, text))
	lines := map[int]bool{}
	for _, f := range got {
		if f.Code == "schema-parameters-mismatch" {
			lines[f.Line] = true
		}
	}
	if len(got) != 2 || !lines[29] || !lines[31] {
		t.Fatalf("nested source locations: %+v", got)
	}
}

func TestMissingParameterUsesExistingContainerLocation(t *testing.T) {
	text := `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: definition
spec:
  parameters:
    openAPIV3Schema:
      type: object
      required: [count]
      properties:
        count:
          type: integer
---
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: app
spec:
  componentType:
    name: deployment/definition
`
	got := ValidateDocuments(context.Background(), parseResources(t, text))
	if len(got) != 1 || got[0].Code != "schema-parameters-mismatch" || got[0].Line < 18 {
		t.Fatalf("missing parameter must point to the Component container: %+v", got)
	}
}
