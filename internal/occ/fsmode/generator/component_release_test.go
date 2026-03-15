// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"strings"
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

// addTrait adds a Trait resource entry to the index.
func addTrait(t *testing.T, idx *index.Index, name string, spec map[string]any, filePath string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "Trait",
				"metadata": map[string]any{
					"name":      name,
					"namespace": "default",
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

// addComponentWithTraits adds a Component with trait references to the index.
func addComponentWithTraits(t *testing.T, idx *index.Index, namespace, name, project, componentTypeName string, traits []map[string]any, filePath string) {
	t.Helper()
	spec := map[string]any{
		"owner": map[string]any{
			"projectName": project,
		},
		"componentType": map[string]any{
			"name": componentTypeName,
			"kind": "ComponentType",
		},
	}
	if len(traits) > 0 {
		spec["traits"] = traits
	}
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "Component",
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

func TestGenerateRelease_ManifestShape(t *testing.T) {
	const (
		namespace     = "default"
		projectName   = "myproj"
		componentName = "my-svc"
		releaseName   = "my-svc-release-1"
	)

	idx := index.New("/repo")

	// Component with two trait refs: one explicit "Trait" kind, one with empty kind (should normalize to "Trait")
	addComponentWithTraits(t, idx, namespace, componentName, projectName, "deployment/service",
		[]map[string]any{
			{"kind": "Trait", "name": "ingress", "instanceName": "ingress-1"},
			{"name": "logging", "instanceName": "logging-1"},
		},
		"/repo/projects/myproj/components/my-svc/component.yaml")

	addComponentType(t, idx, "service", "deployment",
		"/repo/platform/component-types/service.yaml")

	addWorkload(t, idx, namespace, "my-svc-workload", projectName, componentName,
		map[string]any{
			"container": map[string]any{"image": "reg/my-svc:v1"},
		},
		"/repo/projects/myproj/components/my-svc/workload.yaml")

	addTrait(t, idx, "ingress",
		map[string]any{"creates": []any{map[string]any{"apiVersion": "networking.k8s.io/v1", "kind": "Ingress"}}},
		"/repo/platform/traits/ingress.yaml")

	addTrait(t, idx, "logging",
		map[string]any{"creates": []any{map[string]any{"apiVersion": "v1", "kind": "ConfigMap"}}},
		"/repo/platform/traits/logging.yaml")

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

	// --- Verify top-level metadata ---
	if got := release.GetKind(); got != "ComponentRelease" {
		t.Errorf("kind = %q, want ComponentRelease", got)
	}
	if got := release.GetName(); got != releaseName {
		t.Errorf("metadata.name = %q, want %q", got, releaseName)
	}
	if got := release.GetNamespace(); got != namespace {
		t.Errorf("metadata.namespace = %q, want %q", got, namespace)
	}

	// --- Verify spec.componentType ---
	ctKind, _, _ := unstructured.NestedString(release.Object, "spec", "componentType", "kind")
	ctName, _, _ := unstructured.NestedString(release.Object, "spec", "componentType", "name")
	ctWorkloadType, _, _ := unstructured.NestedString(release.Object, "spec", "componentType", "spec", "workloadType")
	if ctKind != "ComponentType" {
		t.Errorf("spec.componentType.kind = %q, want ComponentType", ctKind)
	}
	if ctName != "deployment/service" {
		t.Errorf("spec.componentType.name = %q, want deployment/service", ctName)
	}
	if ctWorkloadType != "deployment" {
		t.Errorf("spec.componentType.spec.workloadType = %q, want deployment", ctWorkloadType)
	}

	// --- Verify spec.traits[] ---
	traitsSlice, ok, _ := unstructured.NestedSlice(release.Object, "spec", "traits")
	if !ok {
		t.Fatal("expected spec.traits to exist")
	}
	if len(traitsSlice) != 2 {
		t.Fatalf("expected 2 traits in spec.traits, got %d", len(traitsSlice))
	}
	for i, expected := range []struct{ kind, name string }{
		{"Trait", "ingress"},
		{"Trait", "logging"},
	} {
		traitMap, ok := traitsSlice[i].(map[string]interface{})
		if !ok {
			t.Fatalf("spec.traits[%d] is not a map", i)
		}
		if traitMap["kind"] != expected.kind {
			t.Errorf("spec.traits[%d].kind = %v, want %q", i, traitMap["kind"], expected.kind)
		}
		if traitMap["name"] != expected.name {
			t.Errorf("spec.traits[%d].name = %v, want %q", i, traitMap["name"], expected.name)
		}
		if traitMap["spec"] == nil {
			t.Errorf("spec.traits[%d].spec should not be nil", i)
		}
	}

	// --- Verify spec.componentProfile.traits[] ---
	profileTraits, ok, _ := unstructured.NestedSlice(release.Object, "spec", "componentProfile", "traits")
	if !ok {
		t.Fatal("expected spec.componentProfile.traits to exist")
	}
	if len(profileTraits) != 2 {
		t.Fatalf("expected 2 profile traits, got %d", len(profileTraits))
	}
	for i, expected := range []struct{ kind, name, instanceName string }{
		{"Trait", "ingress", "ingress-1"},
		{"Trait", "logging", "logging-1"},
	} {
		pt, ok := profileTraits[i].(map[string]interface{})
		if !ok {
			t.Fatalf("spec.componentProfile.traits[%d] is not a map", i)
		}
		if pt["kind"] != expected.kind {
			t.Errorf("spec.componentProfile.traits[%d].kind = %v, want %q", i, pt["kind"], expected.kind)
		}
		if pt["name"] != expected.name {
			t.Errorf("spec.componentProfile.traits[%d].name = %v, want %q", i, pt["name"], expected.name)
		}
		if pt["instanceName"] != expected.instanceName {
			t.Errorf("spec.componentProfile.traits[%d].instanceName = %v, want %q", i, pt["instanceName"], expected.instanceName)
		}
	}

	// --- Verify spec.owner ---
	ownerComp, _, _ := unstructured.NestedString(release.Object, "spec", "owner", "componentName")
	ownerProj, _, _ := unstructured.NestedString(release.Object, "spec", "owner", "projectName")
	if ownerComp != componentName {
		t.Errorf("spec.owner.componentName = %q, want %q", ownerComp, componentName)
	}
	if ownerProj != projectName {
		t.Errorf("spec.owner.projectName = %q, want %q", ownerProj, projectName)
	}
}

func TestGenerateRelease_ClusterTraitRefErrors(t *testing.T) {
	const (
		namespace     = "staging"
		projectName   = "myproj"
		componentName = "my-svc"
	)

	idx := index.New("/repo")

	addComponentWithTraits(t, idx, namespace, componentName, projectName, "deployment/service",
		[]map[string]any{
			{"kind": "ClusterTrait", "name": "global-ingress", "instanceName": "gi-1"},
		},
		"/repo/projects/myproj/components/my-svc/component.yaml")

	addComponentType(t, idx, "service", "deployment",
		"/repo/platform/component-types/service.yaml")

	addWorkload(t, idx, namespace, "my-svc-workload", projectName, componentName,
		map[string]any{
			"container": map[string]any{"image": "reg/my-svc:v1"},
		},
		"/repo/projects/myproj/components/my-svc/workload.yaml")

	ocIndex := fsmode.WrapIndex(idx)
	gen := NewReleaseGenerator(ocIndex)

	_, err := gen.GenerateRelease(ReleaseOptions{
		ComponentName: componentName,
		ProjectName:   projectName,
		Namespace:     namespace,
		ReleaseName:   "test-release",
	})
	if err == nil {
		t.Fatal("expected error for ClusterTrait reference, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "ClusterTrait") || !strings.Contains(got, "global-ingress") {
		t.Errorf("error should mention ClusterTrait and trait name, got: %s", got)
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
			"dependencies": map[string]any{
				"endpoints": []any{
					map[string]any{
						"component":  "postgres",
						"name":       "tcp",
						"visibility": "project",
						"envBindings": map[string]any{
							"address": "DATABASE_URL",
						},
					},
					map[string]any{
						"component":  "nats",
						"name":       "tcp",
						"visibility": "project",
						"envBindings": map[string]any{
							"address": "NATS_URL",
						},
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

	// Verify dependencies.endpoints are present
	workload, _, _ := unstructured.NestedMap(release.Object, "spec", "workload")
	dependencies, ok := workload["dependencies"]
	if !ok || dependencies == nil {
		t.Fatal("expected spec.workload.dependencies to exist — this was the bug: connections were dropped during ComponentRelease generation")
	}

	depsMap, ok := dependencies.(map[string]interface{})
	if !ok {
		t.Fatalf("expected dependencies to be a map, got %T", dependencies)
	}

	connSlice, ok := depsMap["endpoints"].([]interface{})
	if !ok {
		t.Fatalf("expected dependencies.endpoints to be a slice, got %T", depsMap["endpoints"])
	}

	if len(connSlice) != 2 {
		t.Fatalf("expected 2 endpoint connections, got %d", len(connSlice))
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

	addComponentType(t, idx, "worker", "statefulset",
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
