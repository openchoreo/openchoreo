// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package celvalidator

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/targetschema"
)

func schemaRuleDoc(t *testing.T, expression, kind, target string) *parser.DocumentNode {
	t.Helper()
	source := "apiVersion: openchoreo.dev/v1alpha1\nkind: " + kind + "\nmetadata: {name: test}\nspec:\n  workloadType: deployment\n  postRenderValidations:\n    - target: " + target + "\n      rule: '${" + expression + "}'\n      message: test\n"
	docs, err := parser.ParseYAML([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	return docs[0]
}
func TestSchemaAwareRuleDiagnostics(t *testing.T) {
	for _, kind := range []string{"ComponentType", "ClusterComponentType", "Trait", "ClusterTrait"} {
		for _, tc := range []struct{ expr, code, typo string }{
			{"resource.spec.replicas <= 2", "", ""},
			{"resource.specc.replicas <= 2", CodeCELTypeError, "specc"},
			{"resource.spec.repclicas <= 2", CodeCELTypeError, "repclicas"},
			{"resocurce.spec.replicas <= 2", CodeCELUnknownName, "resocurce"},
			{"resource.metadata.namme == resource.metadata.name", CodeCELTypeError, "namme"},
			{"resource.spec.template.spec.containers[0].image != resource.metadata.name", "", ""},
			{"resource.spec.template.spec.containers.all(c, c.image != resource.metadata.name)", "", ""},
			{"has(resource.spec.replicas)", "", ""},
			{"resource.spec.?replicas.orValue(0) >= 0", "", ""},
			{"resource.metadata.labels.all(k, resource.metadata.labels[k] != resource.metadata.name)", "", ""},
		} {
			t.Run(kind+"/"+tc.expr, func(t *testing.T) {
				doc := schemaRuleDoc(t, tc.expr, kind, "{group: apps, version: v1, kind: Deployment}")
				findings, err := ValidateDocument(doc, SchemaPair{})
				if err != nil {
					t.Fatal(err)
				}
				if tc.code == "" {
					if len(findings) > 0 {
						t.Fatalf("valid expression rejected: %+v", findings)
					}
					return
				}
				if len(findings) != 1 || findings[0].Code != tc.code {
					t.Fatalf("unexpected findings: %+v", findings)
				}
				lines := strings.Split(string(doc.Source), "\n")
				line := 8
				column := strings.Index(lines[line-1], tc.typo) + 1
				if findings[0].Range.Start.Line != line || findings[0].Range.Start.Column != column {
					t.Fatalf("wrong token location expected %d:%d: %+v", line, column, findings[0])
				}
				if tc.typo == "repclicas" && !strings.Contains(findings[0].Message, `Did you mean "replicas"?`) {
					t.Fatalf("missing confident field suggestion: %+v", findings[0])
				}
			})
		}
	}
}

const customCRD = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {name: widgets.example.com}
spec:
  group: example.com
  names: {kind: Widget, plural: widgets}
  scope: Cluster
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                count: {type: integer}
                values: {type: array, items: {type: integer}}
                labels: {type: object, additionalProperties: {type: string}}
                free: {type: object, x-kubernetes-preserve-unknown-fields: true}
                open: {type: object, additionalProperties: true}
                closed: {type: object, additionalProperties: false}
                mixed: {type: object, properties: {known: {type: integer}}, additionalProperties: {type: string}}
                intOrString: {x-kubernetes-int-or-string: true, anyOf: [{type: integer}, {type: string}]}
`

func TestCustomSchemaFlexibleFieldsAndFallback(t *testing.T) {
	root := t.TempDir()
	sources := map[string][]byte{filepath.Join(root, "widget.yaml"): []byte(customCRD)}
	resolver := targetschema.NewScope(root, sources)
	for _, tc := range []struct {
		expr  string
		valid bool
	}{
		{"resource.spec.count <= 2", true},
		{"resource.spec.countt <= 2", false},
		{"resource.spec.count == resource.metadata.name", false},
		{"resource.metadata.labels.arbitrary == resource.metadata.name", true},
		{"resource.metadata.namme == resource.metadata.name", false},
		{"resource.spec.labels.arbitrary == resource.metadata.name", true},
		{"resource.spec.labels.all(k, resource.spec.labels[k] == resource.metadata.name)", true},
		{"resource.spec.free.anything.deep == 2", true},
		{"resource.spec.open.anything.deep == 2", true},
		{"resource.spec.closed.anything == 2", false},
		{"resource.spec.mixed.known == 1 && resource.spec.mixed.arbitrary != 1", true},
		{"resource.spec.values.all(v, v > 0)", true},
		{"resource.spec.intOrString == 2", true},
		{"resource.spec.intOrString == resource.metadata.name", true},
		{"resource.specc.count == 1", false},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			doc := schemaRuleDoc(t, tc.expr, "ComponentType", "{group: example.com, version: v1, kind: Widget}")
			doc.ValidationContext = targetschema.WithResolver(nil, resolver)
			findings, err := ValidateDocument(doc, SchemaPair{})
			if err != nil {
				t.Fatal(err)
			}
			if (len(findings) == 0) != tc.valid {
				t.Fatalf("valid=%v findings=%+v", tc.valid, findings)
			}
		})
	}
	// Conflicting definitions make strict checking unsafe, so retain dyn.
	sources[filepath.Join(root, "conflict.yaml")] = []byte(strings.Replace(customCRD, "count: {type: integer}", "count: {type: string}", 1))
	doc := schemaRuleDoc(t, "resource.spec.unknown.deep == 2", "ComponentType", "{group: example.com, version: v1, kind: Widget}")
	doc.ValidationContext = targetschema.WithResolver(nil, targetschema.NewScope(root, sources))
	if findings, err := ValidateDocument(doc, SchemaPair{}); err != nil || len(findings) > 0 {
		t.Fatal(findings, err)
	}
	for _, target := range []string{"{group: apps, version: v99, kind: Deployment}", "{group: apps, kind: Deployment}", "{group: apps, version: v1, kind: '*'}", "{group: null, version: v1, kind: Service}"} {
		doc := schemaRuleDoc(t, "resource.any.unknown == 1", "ComponentType", target)
		if findings, err := ValidateDocument(doc, SchemaPair{}); err != nil || len(findings) > 0 {
			t.Fatalf("unsafe target should retain dyn: %s %+v %v", target, findings, err)
		}
	}
}

func TestPostRenderWhereUsesTargetSchema(t *testing.T) {
	doc := schemaRuleDoc(t, "resource.spec.replicas <= 2", "ComponentType", "{group: apps, version: v1, kind: Deployment, where: '${resource.spec.repclicas > 0}'}")
	// Nested quotes in a flow mapping are valid YAML and yield one where issue.
	findings, err := ValidateDocument(doc, SchemaPair{})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Code != CodeCELTypeError {
		t.Fatal(findings)
	}
}
func TestMultilineTargetLocations(t *testing.T) {
	for _, style := range []string{"|", ">"} {
		text := fmt.Sprintf("apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentType\nmetadata: {name: test}\nspec:\n  workloadType: deployment\n  postRenderValidations:\n    - target: {group: apps, version: v1, kind: Deployment}\n      rule: %s\n        ${resource.spec.repclicas\n          <= 2}\n      message: test\n", style)
		docs, err := parser.ParseYAML([]byte(text))
		if err != nil {
			t.Fatal(err)
		}
		findings, err := ValidateDocument(docs[0], SchemaPair{})
		if err != nil {
			t.Fatal(err)
		}
		expected := strings.Index(strings.Split(text, "\n")[8], "repclicas") + 1
		if len(findings) != 1 || findings[0].Code != CodeCELTypeError || findings[0].Range.Start.Line != 9 || findings[0].Range.Start.Column != expected {
			t.Fatalf("%s wrong multiline range: %+v expected 9:%d", style, findings, expected)
		}
	}
}

func TestMultipleRulesDocumentsAndEscapedScalarLocations(t *testing.T) {
	first := schemaRuleDoc(t, "resource.spec.replicas <= 2", "ComponentType", "{group: apps, version: v1, kind: Deployment}")
	source := string(first.Source) + "    - target: {group: apps, version: v1, kind: Deployment}\n      rule: '${resource.spec.repclicas <= 2 && ''abc'' != resource.metadata.name}'\n      message: typo\n"
	source += "---\n" + strings.Replace(string(first.Source), "resource.spec.replicas", "resource.specc.replicas", 1)
	docs, err := parser.ParseYAML([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatal("expected two documents")
	}
	for i, doc := range docs {
		findings, err := ValidateDocument(doc, SchemaPair{})
		if err != nil || len(findings) != 1 || findings[0].Code != CodeCELTypeError {
			t.Fatalf("document %d: %+v %v", i, findings, err)
		}
		token := "repclicas"
		if i == 1 {
			token = "specc"
		}
		position := findings[0].Range.Start
		line := strings.Split(source, "\n")[position.Line-1]
		if position.Column != strings.Index(line, token)+1 {
			t.Fatalf("incorrect source token in document %d: %+v", i, position)
		}
	}
}
