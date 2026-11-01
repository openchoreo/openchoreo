package postrendervalidator

import (
	"context"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"strings"
	"testing"
)

func TestWorkspaceValidatesIndependentComponents(t *testing.T) {
	var sources []SourceDocument
	for _, name := range []string{"first", "second"} {
		text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: " + name + "\nspec:\n  componentType:\n    kind: ComponentType\n    name: missing\n"
		docs, err := parser.ParseYAML([]byte(text))
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, SourceDocument{Path: name + ".yaml", Document: docs[0]})
	}
	got := ValidateWorkspace(context.Background(), sources)
	if len(got) != 2 {
		t.Fatalf("both independent failures must be reported: %+v", got)
	}
	for _, f := range got {
		if !strings.Contains(f.Message, "not found") {
			t.Fatalf("unexpected: %+v", f)
		}
	}
}

func TestWorkspaceValidatesEveryReleaseBinding(t *testing.T) {
	text := `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata: {name: web, namespace: default}
spec:
  workloadType: deployment
  resources:
    - id: config
      template:
        apiVersion: v1
        kind: ConfigMap
        metadata: {name: '${metadata.name}'}
        data: {purpose: test}
---
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata: {name: app, namespace: default}
spec:
  owner: {projectName: demo}
  componentType: {kind: ComponentType, name: deployment/web}
---
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata: {name: workload, namespace: default}
spec:
  owner: {projectName: demo, componentName: app}
  container: {image: 'busybox:1.36'}
---
apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseBinding
metadata: {name: first, namespace: default}
spec:
  owner: {projectName: demo, componentName: app}
  environment: development
---
apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseBinding
metadata: {name: second, namespace: default}
spec:
  owner: {projectName: demo, componentName: app}
  environment: missing
---
apiVersion: openchoreo.dev/v1alpha1
kind: Environment
metadata: {name: development, namespace: default}
spec: {}
`
	docs, err := parser.ParseYAML([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	var sources []SourceDocument
	for _, d := range docs {
		sources = append(sources, SourceDocument{Path: "project.yaml", Document: d})
	}
	got := ValidateWorkspace(context.Background(), sources)
	if len(got) != 1 || !strings.Contains(got[0].Message, "missing") || strings.Contains(got[0].Message, "multiple ReleaseBindings") {
		t.Fatalf("each binding must be processed independently: %+v", got)
	}
}
