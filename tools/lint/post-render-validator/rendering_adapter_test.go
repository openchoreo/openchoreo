// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

func TestRenderInput_ConvertsBundle(t *testing.T) {
	parsed, err := parser.ParseYAML([]byte(ctWeb))
	if err != nil {
		t.Fatal(err)
	}
	p2parsed, _ := parser.ParseYAML([]byte(compDemo))
	p3parsed, _ := parser.ParseYAML([]byte(wlDemo))
	docs := append(append(parsed, p2parsed...), p3parsed...)
	b, err := BundleFromDocuments(docs)
	if err != nil {
		t.Fatal(err)
	}
	ri, err := b.RenderInput()
	if err != nil {
		t.Fatalf("RenderInput: %v", err)
	}
	if ri.ComponentType == nil || ri.Component == nil || ri.Workload == nil {
		t.Fatal("missing required fields")
	}
	if ri.Metadata.PodSelectors == nil || len(ri.Metadata.PodSelectors) == 0 {
		t.Fatal("metadata not synthesized")
	}
}

func TestRenderBundle_Valid(t *testing.T) {
	parsed, _ := parser.ParseYAML([]byte(ctWeb))
	p2parsed, _ := parser.ParseYAML([]byte(compDemo))
	p3parsed, _ := parser.ParseYAML([]byte(wlDemo))
	docs := append(append(parsed, p2parsed...), p3parsed...)
	b, err := BundleFromDocuments(docs)
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := RenderBundle(context.Background(), b)
	if err != nil {
		t.Fatalf("RenderBundle: %v", err)
	}
	if out == nil || len(out.Resources) == 0 {
		t.Fatal("expected rendered resources")
	}
	found := false
	for _, r := range out.Resources {
		if r.Resource["kind"] == "Deployment" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected Deployment resource")
	}
}

func TestRenderBundle_MissingRequired(t *testing.T) {
	parsed, _ := parser.ParseYAML([]byte(ctWeb))
	b, _ := BundleFromDocuments(parsed)
	_, _, err := RenderBundle(context.Background(), b)
	if err == nil {
		t.Fatal("expected error for missing required inputs")
	}
}

func TestBundleFromDocuments_FromTestdata(t *testing.T) {
	files := []string{"valid-componenttype.yaml", "valid-component.yaml", "valid-workload.yaml", "valid-trait.yaml"}
	docs := []*parser.DocumentNode{}
	for _, f := range files {
		path := filepath.Join("testdata", f)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		parsed, err := parser.ParseYAML(data)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		docs = append(docs, parsed...)
	}
	b, err := BundleFromDocuments(docs)
	if err != nil {
		t.Fatalf("BundleFromDocuments: %v", err)
	}
	if b.ComponentType == nil || b.Component == nil || b.Workload == nil {
		t.Fatal("missing required from testdata")
	}
	if len(b.Traits) == 0 {
		t.Fatal("expected trait")
	}
	out, _, err := RenderBundle(context.Background(), b)
	if err != nil {
		t.Fatalf("RenderBundle: %v", err)
	}
	if len(out.Resources) == 0 {
		t.Fatal("expected resources")
	}
}

const ctWeb = `
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: web-service
  namespace: default
spec:
  workloadType: deployment
  parameters:
    openAPIV3Schema:
      replicas: "integer | default=1"
      port: "integer | default=8080"
  resources:
    - id: deployment
      template:
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: ${metadata.name}
          namespace: ${metadata.namespace}
          labels: ${metadata.labels}
        spec:
          replicas: ${parameters.replicas}
          selector:
            matchLabels: ${metadata.podSelectors}
          template:
            metadata:
              labels: ${metadata.podSelectors}
            spec:
              containers:
                - name: app
                  image: ${workload.container.image}
                  ports:
                    - containerPort: ${parameters.port}
                      protocol: TCP
`

const compDemo = `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: demo
  namespace: default
spec:
  componentType:
    name: web-service
    kind: ComponentType
  parameters:
    replicas: 1
    port: 8080
`

const wlDemo = `
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: demo
  namespace: default
spec:
  owner:
    projectName: default
    componentName: demo
  container:
    image: nginx:1.25
`

const traitLog = `
apiVersion: openchoreo.dev/v1alpha1
kind: Trait
metadata:
  name: logging
  namespace: default
spec:
  creates: []
`
