// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package celvalidator

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/openchoreo/openchoreo/tools/lint/cel-validator/internal/contextdocs"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

func expressionDoc(t *testing.T, expr string) *parser.DocumentNode {
	t.Helper()
	source := `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata: {name: demo}
spec:
  workloadType: deployment
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        retries:
          type: integer
          description: Number of retry attempts.
  resources:
    - id: demo
      template:
        value: '${` + strings.ReplaceAll(expr, "'", "''") + `}'
`
	docs, err := parser.ParseYAML([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	return docs[0]
}
func TestContextFieldSuggestions(t *testing.T) {
	for _, tc := range []struct{ expr, want string }{
		{"metadata.podSelectores", "podSelectors"},
		{"configurations.toConfigEvsByContainer()", "toConfigEnvsByContainer"},
		{"parameters.retrise", "retries"},
		{"metadtaa.name", "metadata"},
		{"metadata.podSelectors", ""},
		{"metadata.labels.podSelectores", ""},
		{"'metadata.podSelectores'", ""},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			doc := expressionDoc(t, tc.expr)
			pair, err := ExtractSchemas(doc.Spec)
			if err != nil {
				t.Fatal(err)
			}
			findings, err := ValidateDocument(doc, pair)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if len(findings) != 0 {
					t.Fatal(findings)
				}
				return
			}
			if len(findings) != 1 || !strings.Contains(findings[0].Message, "Did you mean \""+tc.want+"\"?") {
				t.Fatalf("missing contextual suggestion: %+v", findings)
			}
		})
	}
}
func TestCELSymbolInspection(t *testing.T) {
	for _, tc := range []struct{ expr, field, typ, description string }{
		{"metadata.podSelectors", "podSelectors", "map(string, string)", "platform-injected selectors"},
		{"workload.container.image", "image", "string", ""},
		{"parameters.retries", "retries", "int", "Number of retry attempts"},
		{"metadata.labels.anyKey", "anyKey", "string", ""},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			doc := expressionDoc(t, tc.expr)
			pair, _ := ExtractSchemas(doc.Spec)
			lines := strings.Split(string(doc.Source), "\n")
			line, column := 0, 0
			for i, text := range lines {
				if strings.Contains(text, "${") {
					line = i + 1
					column = strings.Index(text, tc.field) + 2
				}
			}
			info, err := InspectExpression(doc, pair, line, column)
			if err != nil || info == nil {
				t.Fatalf("missing CEL hover: %+v %v", info, err)
			}
			if info.Name != tc.expr || info.Type != tc.typ || !strings.Contains(info.Description, tc.description) {
				t.Fatalf("unexpected symbol info: %+v", info)
			}
		})
	}
}
func TestGeneratedContextDocumentation(t *testing.T) {
	data, err := contextdocs.Generate("../../../internal/pipeline")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(data), bytes.TrimSpace(contextDescriptionData)) {
		t.Fatal("regenerate CEL context documentation")
	}
}

func TestInspectionAndSuggestionUseForEachScope(t *testing.T) {
	source := `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata: {name: demo}
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        backends:
          type: array
          items:
            type: object
            properties:
              portNumber: {type: integer, description: Backend port.}
  resources:
    - id: backend
      forEach: '${parameters.backends}'
      var: backend
      template:
        port: '${backend.portNumber}'
`
	for _, bad := range []bool{false, true} {
		text := source
		if bad {
			text = strings.Replace(text, "backend.portNumber", "backend.portNumbre", 1)
		}
		docs, err := parser.ParseYAML([]byte(text))
		if err != nil {
			t.Fatal(err)
		}
		pair, _ := ExtractSchemas(docs[0].Spec)
		if bad {
			findings, err := ValidateDocument(docs[0], pair)
			if err != nil || len(findings) != 1 || !strings.Contains(findings[0].Message, `Did you mean "portNumber"?`) {
				t.Fatalf("wrong loop scope suggestion: %+v %v", findings, err)
			}
		} else {
			line := strings.Split(text, "\n")[19]
			info, err := InspectExpression(docs[0], pair, 20, strings.Index(line, "portNumber")+2)
			if err != nil || info == nil || info.Type != "int" {
				t.Fatalf("wrong loop scope hover: %+v %v", info, err)
			}
			parentLine := strings.Split(text, "\n")[16]
			listInfo, err := InspectExpression(docs[0], pair, 17, strings.Index(parentLine, "backends")+2)
			if err != nil || listInfo == nil || listInfo.ChildrenParent != "parameters.backends[]" || len(listInfo.Children) != 1 || listInfo.Children[0].Name != "parameters.backends[].portNumber" || listInfo.Children[0].Description != "Backend port." {
				t.Fatalf("wrong schema-defined list children: %+v %v", listInfo, err)
			}
		}
	}
}

func TestConcurrentSymbolInspectionAndValidation(t *testing.T) {
	doc := expressionDoc(t, "metadata.podSelectores")
	pair, _ := ExtractSchemas(doc.Spec)
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 4; j++ {
				line := strings.Split(string(doc.Source), "\n")[15]
				info, err := InspectExpression(doc, pair, 16, strings.Index(line, "podSelectores")+2)
				if err != nil || info == nil || info.Suggestion != "podSelectors" {
					t.Errorf("concurrent symbol result: %+v %v", info, err)
					return
				}
				findings, err := ValidateDocument(doc, pair)
				if err != nil || len(findings) != 1 || !strings.Contains(findings[0].Message, "podSelectors") {
					t.Errorf("concurrent diagnostic: %+v %v", findings, err)
					return
				}
			}
		}()
	}
	workers.Wait()
}

func TestSymbolChildrenMatchCELDeclarations(t *testing.T) {
	for _, token := range []string{"metadata", "podSelectors"} {
		doc := expressionDoc(t, "metadata.podSelectors")
		pair, _ := ExtractSchemas(doc.Spec)
		line := strings.Split(string(doc.Source), "\n")[15]
		info, err := InspectExpression(doc, pair, 16, strings.Index(line, token)+2)
		if err != nil || info == nil || info.ChildrenParent != "metadata" {
			t.Fatalf("missing metadata children: %+v %v", info, err)
		}
		built, err := envFor(doc.Kind, pair)
		if err != nil {
			t.Fatal(err)
		}
		expected := structFieldNames(built.env, variableType(built.env, "metadata"))
		if len(info.Children) != len(expected) {
			t.Fatalf("did not include all declared children: %d vs %d", len(info.Children), len(expected))
		}
		for i, name := range expected {
			if info.Children[i].Name != "metadata."+name || info.Children[i].Type == "" {
				t.Fatalf("wrong sorted child: %+v", info.Children[i])
			}
		}
	}
	doc := expressionDoc(t, "metadata.labels.anyKey")
	pair, _ := ExtractSchemas(doc.Spec)
	line := strings.Split(string(doc.Source), "\n")[15]
	info, err := InspectExpression(doc, pair, 16, strings.Index(line, "anyKey")+2)
	if err != nil || info == nil || !info.OpenKeys || len(info.Children) != 0 {
		t.Fatalf("map keys must not be enumerated: %+v %v", info, err)
	}
}

func TestConfigurationFunctionDiscovery(t *testing.T) {
	doc := expressionDoc(t, "configurations.toConfigEnvsByContainer()")
	pair, err := ExtractSchemas(doc.Spec)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(doc.Source), "\n")
	for i, line := range lines {
		if at := strings.Index(line, "configurations."); at >= 0 {
			info, err := InspectExpression(doc, pair, i+1, at+1)
			if err != nil || info == nil {
				t.Fatalf("%v %v", info, err)
			}
			for _, fn := range info.Functions {
				if fn.Name == "configurations.toConfigEnvsByContainer()" {
					return
				}
			}
			t.Fatalf("missing function: %+v", info.Functions)
		}
	}
	t.Fatal("no expression")
}

func TestFunctionSuggestionsRespectReceiver(t *testing.T) {
	doc := expressionDoc(t, "metadata.toConfigEvsByContainer()")
	pair, _ := ExtractSchemas(doc.Spec)
	findings, err := ValidateDocument(doc, pair)
	if err != nil || len(findings) == 0 {
		t.Fatalf("%v %v", findings, err)
	}
	for _, finding := range findings {
		if strings.Contains(finding.Message, "Did you mean") {
			t.Fatal(finding)
		}
	}
}
