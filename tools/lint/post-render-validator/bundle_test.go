// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import (
	"testing"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

func TestBundleFromDocuments_BasicKinds(t *testing.T) {
	parsedCT, err := parser.ParseYAML([]byte(ctWeb))
	if err != nil {
		t.Fatal(err)
	}
	parsedComp, _ := parser.ParseYAML([]byte(compDemo))
	parsedWL, _ := parser.ParseYAML([]byte(wlDemo))
	parsedT, _ := parser.ParseYAML([]byte(traitLog))
	docs := append(append(append(parsedCT, parsedComp...), parsedWL...), parsedT...)
	b, err := BundleFromDocuments(docs)
	if err != nil {
		t.Fatalf("BundleFromDocuments: %v", err)
	}
	if b.ComponentType == nil || b.ComponentType.Name != "web-service" {
		t.Fatalf("ComponentType not loaded")
	}
	if b.Component == nil || b.Component.Name != "demo" {
		t.Fatalf("Component not loaded")
	}
	if b.Workload == nil || b.Workload.Name != "demo" {
		t.Fatalf("Workload not loaded")
	}
	if len(b.Traits) != 1 || b.Traits[0].Name != "logging" {
		t.Fatalf("Trait not loaded")
	}
}

func TestBundleFromDocuments_ClusterVariants(t *testing.T) {
	docs, err := parser.ParseYAML([]byte(clusterCT))
	if err != nil {
		t.Fatal(err)
	}
	docs2, err := parser.ParseYAML([]byte(clusterTrait))
	if err != nil {
		t.Fatal(err)
	}
	docs = append(docs, docs2...)
	b, err := BundleFromDocuments(docs)
	if err != nil {
		t.Fatalf("BundleFromDocuments: %v", err)
	}
	if b.ComponentType == nil || b.ComponentType.Name != "web" {
		t.Fatalf("ClusterComponentType not converted")
	}
	if len(b.Traits) != 1 || b.Traits[0].Name != "ct1" {
		t.Fatalf("ClusterTrait not converted")
	}
}

func TestBundleFromDocuments_UnknownAndNil(t *testing.T) {
	docs := make([]*parser.DocumentNode, 1, 2)
	// Also try unknown kind
	unk, _ := parser.ParseYAML([]byte("apiVersion: v1\nkind: Service\nmetadata:\n  name: x\n"))
	docs = append(docs, unk...)
	b, err := BundleFromDocuments(docs)
	if err != nil {
		t.Fatalf("BundleFromDocuments: %v", err)
	}
	if b == nil {
		t.Fatal("bundle nil")
	}
	if b.ComponentType != nil || b.Component != nil {
		t.Fatal("should be empty")
	}
}

const clusterCT = `
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterComponentType
metadata:
  name: web
spec:
  workloadType: deployment
  resources:
    - id: r
      template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: x
`

const clusterTrait = `
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterTrait
metadata:
  name: ct1
spec:
  creates: []
`
