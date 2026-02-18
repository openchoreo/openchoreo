// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package labelresolver

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/openchoreo/openchoreo/internal/events-collector/labelcache"
)

func TestKindToResource(t *testing.T) {
	tests := []struct {
		kind     string
		expected string
	}{
		{"Pod", "pods"},
		{"Deployment", "deployments"},
		{"Service", "services"},
		{"ReplicaSet", "replicasets"},
		{"ConfigMap", "configmaps"},
		{"Secret", "secrets"},
		{"Namespace", "namespaces"},
		// Irregular plurals
		{"Ingress", "ingresses"},
		{"NetworkPolicy", "networkpolicies"},
		{"PodDisruptionBudget", "poddisruptionbudgets"},
		// Mixed case
		{"pod", "pods"},
		{"DEPLOYMENT", "deployments"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := kindToResource(tt.kind)
			if got != tt.expected {
				t.Errorf("kindToResource(%q) = %q, want %q", tt.kind, got, tt.expected)
			}
		})
	}
}

func TestFilterOpenChoreoLabels(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]string
		expect map[string]string
	}{
		{
			name:   "nil input",
			input:  nil,
			expect: nil,
		},
		{
			name:   "empty input",
			input:  map[string]string{},
			expect: nil,
		},
		{
			name: "no matching labels",
			input: map[string]string{
				"app":     "my-app",
				"version": "v1",
			},
			expect: nil,
		},
		{
			name: "only openchoreo labels",
			input: map[string]string{
				"openchoreo.dev/component":   "my-component",
				"openchoreo.dev/project":     "my-project",
				"openchoreo.dev/environment": "dev",
			},
			expect: map[string]string{
				"openchoreo.dev/component":   "my-component",
				"openchoreo.dev/project":     "my-project",
				"openchoreo.dev/environment": "dev",
			},
		},
		{
			name: "mixed labels",
			input: map[string]string{
				"app":                      "my-app",
				"openchoreo.dev/component": "my-component",
				"version":                  "v1",
				"openchoreo.dev/project":   "my-project",
			},
			expect: map[string]string{
				"openchoreo.dev/component": "my-component",
				"openchoreo.dev/project":   "my-project",
			},
		},
		{
			name: "similar prefix but not matching",
			input: map[string]string{
				"openchoreo.io/component": "my-component",
				"openchoreodev/project":   "my-project",
			},
			expect: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterOpenChoreoLabels(tt.input)
			if diff := cmp.Diff(tt.expect, got); diff != "" {
				t.Errorf("filterOpenChoreoLabels() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestObjectRefToGVR(t *testing.T) {
	tests := []struct {
		name    string
		obj     corev1.ObjectReference
		wantGVR schema.GroupVersionResource
		wantErr bool
	}{
		{
			name: "core v1 Pod",
			obj: corev1.ObjectReference{
				APIVersion: "v1",
				Kind:       "Pod",
			},
			wantGVR: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			},
			wantErr: false,
		},
		{
			name: "apps/v1 Deployment",
			obj: corev1.ObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			wantGVR: schema.GroupVersionResource{
				Group:    "apps",
				Version:  "v1",
				Resource: "deployments",
			},
			wantErr: false,
		},
		{
			name: "networking.k8s.io/v1 Ingress",
			obj: corev1.ObjectReference{
				APIVersion: "networking.k8s.io/v1",
				Kind:       "Ingress",
			},
			wantGVR: schema.GroupVersionResource{
				Group:    "networking.k8s.io",
				Version:  "v1",
				Resource: "ingresses",
			},
			wantErr: false,
		},
		{
			name: "invalid API version",
			obj: corev1.ObjectReference{
				APIVersion: "invalid/version/format",
				Kind:       "Pod",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := objectRefToGVR(tt.obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("objectRefToGVR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantGVR {
				t.Errorf("objectRefToGVR() = %v, want %v", got, tt.wantGVR)
			}
		})
	}
}

func TestResolver_Resolve_CacheHit(t *testing.T) {
	cache := labelcache.New(1*time.Hour, testLogger())
	fakeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	resolver := New(fakeClient, cache, testLogger())

	// Pre-populate cache
	key := labelcache.Key("default", "Pod", "my-pod")
	expectedLabels := map[string]string{"openchoreo.dev/component": "cached-component"}
	cache.Set(key, expectedLabels)

	obj := corev1.ObjectReference{
		APIVersion: "v1",
		Kind:       "Pod",
		Name:       "my-pod",
		Namespace:  "default",
	}

	got, err := resolver.Resolve(context.Background(), obj)
	if err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}

	if diff := cmp.Diff(expectedLabels, got); diff != "" {
		t.Errorf("Resolve() mismatch (-want +got):\n%s", diff)
	}
}

func TestResolver_Resolve_CacheMiss(t *testing.T) {
	cache := labelcache.New(1*time.Hour, testLogger())

	// Create a Pod with labels
	pod := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "my-pod",
				"namespace": "default",
				"labels": map[string]interface{}{
					"app":                      "my-app",
					"openchoreo.dev/component": "my-component",
					"openchoreo.dev/project":   "my-project",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme, pod)

	resolver := New(fakeClient, cache, testLogger())

	obj := corev1.ObjectReference{
		APIVersion: "v1",
		Kind:       "Pod",
		Name:       "my-pod",
		Namespace:  "default",
	}

	got, err := resolver.Resolve(context.Background(), obj)
	if err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}

	// Should only return openchoreo.dev labels
	expected := map[string]string{
		"openchoreo.dev/component": "my-component",
		"openchoreo.dev/project":   "my-project",
	}
	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Resolve() mismatch (-want +got):\n%s", diff)
	}

	// Verify result was cached
	key := labelcache.Key("default", "Pod", "my-pod")
	cached, found := cache.Get(key)
	if !found {
		t.Error("Result should have been cached")
	}
	if diff := cmp.Diff(expected, cached); diff != "" {
		t.Errorf("Cached result mismatch (-want +got):\n%s", diff)
	}
}

func TestResolver_Resolve_NotFound(t *testing.T) {
	cache := labelcache.New(1*time.Hour, testLogger())

	// Empty client - no objects
	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)

	resolver := New(fakeClient, cache, testLogger())

	obj := corev1.ObjectReference{
		APIVersion: "v1",
		Kind:       "Pod",
		Name:       "nonexistent-pod",
		Namespace:  "default",
	}

	got, err := resolver.Resolve(context.Background(), obj)
	if err != nil {
		t.Fatalf("Resolve() returned error for not-found: %v (should return nil, nil)", err)
	}
	if got != nil {
		t.Errorf("Resolve() = %v, want nil for not-found", got)
	}

	// Verify not-found was cached
	key := labelcache.Key("default", "Pod", "nonexistent-pod")
	labels, found := cache.Get(key)
	if !found {
		t.Error("Not-found result should have been cached")
	}
	if labels != nil {
		t.Errorf("Cached not-found should return nil labels, got %v", labels)
	}
}

func TestResolver_Resolve_NoOpenChoreoLabels(t *testing.T) {
	cache := labelcache.New(1*time.Hour, testLogger())

	// Create a Pod with no openchoreo labels
	pod := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "my-pod",
				"namespace": "default",
				"labels": map[string]interface{}{
					"app":     "my-app",
					"version": "v1",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme, pod)

	resolver := New(fakeClient, cache, testLogger())

	obj := corev1.ObjectReference{
		APIVersion: "v1",
		Kind:       "Pod",
		Name:       "my-pod",
		Namespace:  "default",
	}

	got, err := resolver.Resolve(context.Background(), obj)
	if err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}
	if got != nil {
		t.Errorf("Resolve() = %v, want nil when no openchoreo labels", got)
	}
}

func TestResolver_Resolve_ClusterScoped(t *testing.T) {
	cache := labelcache.New(1*time.Hour, testLogger())

	// Create a cluster-scoped resource (Node)
	node := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]interface{}{
				"name": "my-node",
				"labels": map[string]interface{}{
					"openchoreo.dev/managed": "true",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme, node)

	resolver := New(fakeClient, cache, testLogger())

	obj := corev1.ObjectReference{
		APIVersion: "v1",
		Kind:       "Node",
		Name:       "my-node",
		Namespace:  "", // cluster-scoped
	}

	got, err := resolver.Resolve(context.Background(), obj)
	if err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}

	expected := map[string]string{
		"openchoreo.dev/managed": "true",
	}
	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Resolve() mismatch (-want +got):\n%s", diff)
	}
}

func TestResolver_New_NilLogger(t *testing.T) {
	// Test that New() handles nil logger gracefully by using default
	cache := labelcache.New(1*time.Hour, testLogger())
	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)

	resolver := New(fakeClient, cache, nil)

	if resolver == nil {
		t.Fatal("New() returned nil")
	}
	if resolver.logger == nil {
		t.Error("logger should be set to default, not nil")
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}
