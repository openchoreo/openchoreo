// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// addComponent adds a Component resource entry to the index.
func addComponent(t *testing.T, idx *index.Index, namespace, name, project, componentTypeName string, filePath string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "Component",
				"metadata": map[string]any{
					"name":      name,
					"namespace": namespace,
				},
				"spec": map[string]any{
					"owner": map[string]any{
						"projectName": project,
					},
					"componentType": map[string]any{
						"name": componentTypeName,
						"kind": "ComponentType",
					},
				},
			},
		},
		FilePath: filePath,
	}
	if err := idx.Add(entry); err != nil {
		t.Fatal(err)
	}
}

// addComponentType adds a ComponentType resource entry to the index.
func addComponentType(t *testing.T, idx *index.Index, name, workloadType string, filePath string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "ComponentType",
				"metadata": map[string]any{
					"name":      name,
					"namespace": "default",
				},
				"spec": map[string]any{
					"workloadType": workloadType,
					"resources":    []any{},
					"schema":       map[string]any{},
				},
			},
		},
		FilePath: filePath,
	}
	if err := idx.Add(entry); err != nil {
		t.Fatal(err)
	}
}

// addWorkload adds a Workload resource entry to the index.
func addWorkload(t *testing.T, idx *index.Index, namespace, name, project, component string, workloadObj map[string]any, filePath string) {
	t.Helper()
	spec := map[string]any{
		"owner": map[string]any{
			"projectName":   project,
			"componentName": component,
		},
	}
	for k, v := range workloadObj {
		spec[k] = v
	}
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "Workload",
				"metadata": map[string]any{
					"name":      name,
					"namespace": namespace,
				},
				"spec": spec,
			},
		},
		FilePath: filePath,
	}
	if err := idx.Add(entry); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateRelease_WorkloadEndpointsIncluded(t *testing.T) {
	const (
		namespace     = "default"
		projectName   = "doclet"
		componentName = "document-svc"
		releaseName   = "document-svc-abc123"
	)

	idx := index.New("/repo")

	addComponent(t, idx, namespace, componentName, projectName, "deployment/service",
		"/repo/projects/doclet/components/document-svc/component.yaml")

	addComponentType(t, idx, "service", "deployment",
		"/repo/platform/component-types/service.yaml")

	addWorkload(t, idx, namespace, "document-svc-workload", projectName, componentName,
		map[string]any{
			"container": map[string]any{
				"image": "my-registry/document-svc:v1",
				"env": []any{
					map[string]any{"key": "PORT", "value": "8080"},
				},
			},
			"endpoints": map[string]any{
				"http": map[string]any{
					"type": "REST",
					"port": int64(8080),
				},
			},
		},
		"/repo/projects/doclet/components/document-svc/workload.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	release, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: componentName,
		ProjectName:   projectName,
		Namespace:     namespace,
		ReleaseName:   releaseName,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the release has the workload section
	workload, ok, err := unstructured.NestedMap(release.Object, "spec", "workload")
	if err != nil || !ok {
		t.Fatalf("expected spec.workload to exist, ok=%v, err=%v", ok, err)
	}

	// Verify container is present
	container, ok := workload["container"]
	if !ok || container == nil {
		t.Fatal("expected spec.workload.container to exist")
	}

	// Verify endpoints are present
	endpoints, ok := workload["endpoints"]
	if !ok || endpoints == nil {
		t.Fatal("expected spec.workload.endpoints to exist — this was the bug: endpoints were dropped during ComponentRelease generation")
	}

	endpointsMap, ok := endpoints.(map[string]interface{})
	if !ok {
		t.Fatalf("expected endpoints to be a map, got %T", endpoints)
	}

	httpEndpoint, ok := endpointsMap["http"]
	if !ok {
		t.Fatal("expected 'http' endpoint in endpoints map")
	}

	httpMap, ok := httpEndpoint.(map[string]interface{})
	if !ok {
		t.Fatalf("expected http endpoint to be a map, got %T", httpEndpoint)
	}

	if httpMap["type"] != "REST" {
		t.Errorf("endpoint type = %v, want REST", httpMap["type"])
	}
	if httpMap["port"] != int64(8080) {
		t.Errorf("endpoint port = %v, want 8080", httpMap["port"])
	}
}

func TestGenerateRelease_WorkloadConnectionsIncluded(t *testing.T) {
	const (
		namespace     = "default"
		projectName   = "doclet"
		componentName = "document-svc"
		releaseName   = "document-svc-abc123"
	)

	idx := index.New("/repo")

	addComponent(t, idx, namespace, componentName, projectName, "deployment/service",
		"/repo/projects/doclet/components/document-svc/component.yaml")

	addComponentType(t, idx, "service", "deployment",
		"/repo/platform/component-types/service.yaml")

	addWorkload(t, idx, namespace, "document-svc-workload", projectName, componentName,
		map[string]any{
			"container": map[string]any{
				"image": "my-registry/document-svc:v1",
			},
			"endpoints": map[string]any{
				"http": map[string]any{
					"type": "REST",
					"port": int64(8080),
				},
			},
			"connections": []any{
				map[string]any{
					"component":  "postgres",
					"endpoint":   "tcp",
					"visibility": "project",
					"envBindings": map[string]any{
						"address": "DATABASE_URL",
					},
				},
				map[string]any{
					"component":  "nats",
					"endpoint":   "tcp",
					"visibility": "project",
					"envBindings": map[string]any{
						"address": "NATS_URL",
					},
				},
			},
		},
		"/repo/projects/doclet/components/document-svc/workload.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	release, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: componentName,
		ProjectName:   projectName,
		Namespace:     namespace,
		ReleaseName:   releaseName,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify connections are present
	workload, _, _ := unstructured.NestedMap(release.Object, "spec", "workload")
	connections, ok := workload["connections"]
	if !ok || connections == nil {
		t.Fatal("expected spec.workload.connections to exist — this was the bug: connections were dropped during ComponentRelease generation")
	}

	connSlice, ok := connections.([]interface{})
	if !ok {
		t.Fatalf("expected connections to be a slice, got %T", connections)
	}

	if len(connSlice) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(connSlice))
	}

	first := connSlice[0].(map[string]interface{})
	if first["component"] != "postgres" {
		t.Errorf("first connection component = %v, want postgres", first["component"])
	}

	second := connSlice[1].(map[string]interface{})
	if second["component"] != "nats" {
		t.Errorf("second connection component = %v, want nats", second["component"])
	}
}

func TestGenerateRelease_WorkloadWithoutEndpoints(t *testing.T) {
	const (
		namespace     = "default"
		projectName   = "doclet"
		componentName = "worker-svc"
		releaseName   = "worker-svc-abc123"
	)

	idx := index.New("/repo")

	addComponent(t, idx, namespace, componentName, projectName, "deployment/worker",
		"/repo/projects/doclet/components/worker-svc/component.yaml")

	addComponentType(t, idx, "worker", "deployment",
		"/repo/platform/component-types/worker.yaml")

	addWorkload(t, idx, namespace, "worker-svc-workload", projectName, componentName,
		map[string]any{
			"container": map[string]any{
				"image": "my-registry/worker-svc:v1",
			},
		},
		"/repo/projects/doclet/components/worker-svc/workload.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	release, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: componentName,
		ProjectName:   projectName,
		Namespace:     namespace,
		ReleaseName:   releaseName,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the release has the workload section with container only
	workload, ok, err := unstructured.NestedMap(release.Object, "spec", "workload")
	if err != nil || !ok {
		t.Fatalf("expected spec.workload to exist, ok=%v, err=%v", ok, err)
	}

	// Container should be present
	if _, ok := workload["container"]; !ok {
		t.Fatal("expected spec.workload.container to exist")
	}

	// Endpoints and connections should not be present when empty
	if _, ok := workload["endpoints"]; ok {
		t.Error("expected spec.workload.endpoints to be absent when workload has no endpoints")
	}
	if _, ok := workload["connections"]; ok {
		t.Error("expected spec.workload.connections to be absent when workload has no connections")
	}
}
