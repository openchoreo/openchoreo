// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package index

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestGvkToString(t *testing.T) {
	tests := []struct {
		name string
		gvk  schema.GroupVersionKind
		want string
	}{
		{
			name: "standard GVK",
			gvk:  schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "Component"},
			want: "openchoreo.dev/v1alpha1/Component",
		},
		{
			name: "core group (empty group)",
			gvk:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
			want: "/v1/ConfigMap",
		},
		{
			name: "multi-segment group",
			gvk:  schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
			want: "rbac.authorization.k8s.io/v1/Role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gvkToString(tt.gvk)
			if got != tt.want {
				t.Errorf("gvkToString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStringToGVK(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    schema.GroupVersionKind
		wantErr bool
	}{
		{
			name:  "standard GVK",
			input: "openchoreo.dev/v1alpha1/Component",
			want:  schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "Component"},
		},
		{
			name:  "core group",
			input: "/v1/ConfigMap",
			want:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
		},
		{
			name:  "multi-segment group",
			input: "rbac.authorization.k8s.io/v1/Role",
			want:  schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stringToGVK(tt.input)
			if err != nil {
				t.Fatalf("stringToGVK() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("stringToGVK() = %v, want %v", got, tt.want)
			}
		})
	}

	// Malformed inputs return an error instead of panicking.
	malformedTests := []struct {
		name  string
		input string
	}{
		{"no slashes", "Component"},
		{"empty string", ""},
		{"trailing slash only", "apps/"},
	}
	for _, tt := range malformedTests {
		t.Run("malformed - "+tt.name, func(t *testing.T) {
			_, err := stringToGVK(tt.input)
			if err == nil {
				t.Errorf("stringToGVK(%q) expected error for malformed input, got nil", tt.input)
			}
		})
	}
}

func TestStringToGVKRoundTrip(t *testing.T) {
	gvks := []schema.GroupVersionKind{
		{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "Component"},
		{Group: "", Version: "v1", Kind: "ConfigMap"},
		{Group: "apps", Version: "v1", Kind: "Deployment"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"},
	}

	for _, gvk := range gvks {
		t.Run(gvk.Kind, func(t *testing.T) {
			s := gvkToString(gvk)
			restored, err := stringToGVK(s)
			if err != nil {
				t.Fatalf("round-trip failed: %v", err)
			}
			if restored != gvk {
				t.Errorf("round-trip mismatch: got %v, want %v", restored, gvk)
			}
		})
	}
}

func TestToSerializable(t *testing.T) {
	t.Run("empty index", func(t *testing.T) {
		idx := New("/test/repo")
		s := idx.ToSerializable()

		if len(s.Resources) != 0 {
			t.Errorf("expected 0 resources, got %d", len(s.Resources))
		}
		if s.RepoPath != "/test/repo" {
			t.Errorf("expected repoPath '/test/repo', got %q", s.RepoPath)
		}
	})

	t.Run("single resource", func(t *testing.T) {
		idx := New("/test/repo")
		idx.commitSHA = testCommitSHA
		entry := createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default", "comp1", "/test/comp.yaml")
		_ = idx.Add(entry)

		s := idx.ToSerializable()

		if len(s.Resources) != 1 {
			t.Fatalf("expected 1 resource, got %d", len(s.Resources))
		}
		if s.CommitSHA != testCommitSHA {
			t.Errorf("expected commitSHA '%s', got %q", testCommitSHA, s.CommitSHA)
		}

		r := s.Resources[0]
		if r.GVK != "openchoreo.dev/v1alpha1/Component" {
			t.Errorf("expected GVK 'openchoreo.dev/v1alpha1/Component', got %q", r.GVK)
		}
		if r.Namespace != "default" {
			t.Errorf("expected namespace 'default', got %q", r.Namespace)
		}
		if r.Name != "comp1" {
			t.Errorf("expected name 'comp1', got %q", r.Name)
		}
		if r.FilePath != "/test/comp.yaml" {
			t.Errorf("expected filePath '/test/comp.yaml', got %q", r.FilePath)
		}
	})

	t.Run("multiple GVKs", func(t *testing.T) {
		idx := New("/test/repo")
		_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default", "comp1", "/f1.yaml"))
		_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Trait", "", "trait1", "/f2.yaml"))
		_ = idx.Add(createTestEntry("apps", "v1", "Deployment", "default", "deploy1", "/f3.yaml"))

		s := idx.ToSerializable()

		if len(s.Resources) != 3 {
			t.Errorf("expected 3 resources, got %d", len(s.Resources))
		}
	})

	t.Run("cluster-scoped resource", func(t *testing.T) {
		idx := New("/test/repo")
		_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "ComponentType", "", "http-service", "/f.yaml"))

		s := idx.ToSerializable()

		if len(s.Resources) != 1 {
			t.Fatalf("expected 1 resource, got %d", len(s.Resources))
		}
		if s.Resources[0].Namespace != "" {
			t.Errorf("expected empty namespace for cluster-scoped resource, got %q", s.Resources[0].Namespace)
		}
	})
}

func TestToIndex(t *testing.T) {
	t.Run("empty serializable", func(t *testing.T) {
		si := &SerializableIndex{
			Resources: []SerializableResource{},
			RepoPath:  "/original",
			CommitSHA: "sha1",
		}

		idx := si.ToIndex("/new/repo")
		if idx.GetRepoPath() != "/new/repo" {
			t.Errorf("expected repoPath '/new/repo', got %q", idx.GetRepoPath())
		}
		if idx.GetCommitSHA() != "sha1" {
			t.Errorf("expected commitSHA 'sha1', got %q", idx.GetCommitSHA())
		}
		if idx.Stats().TotalResources != 0 {
			t.Errorf("expected 0 resources, got %d", idx.Stats().TotalResources)
		}
	})

	t.Run("single resource", func(t *testing.T) {
		si := &SerializableIndex{
			Resources: []SerializableResource{
				{
					GVK:       "openchoreo.dev/v1alpha1/Component",
					Namespace: "default",
					Name:      "comp1",
					FilePath:  "/test/comp.yaml",
					Object: map[string]any{
						"apiVersion": "openchoreo.dev/v1alpha1",
						"kind":       "Component",
						"metadata": map[string]any{
							"name":      "comp1",
							"namespace": "default",
						},
					},
				},
			},
		}

		idx := si.ToIndex("/repo")
		gvk := schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "Component"}
		entry, found := idx.Get(gvk, "default", "comp1")
		if !found {
			t.Fatal("expected to find resource after deserialization")
		}
		if entry.FilePath != "/test/comp.yaml" {
			t.Errorf("expected filePath '/test/comp.yaml', got %q", entry.FilePath)
		}
	})

	t.Run("invalid ParseGroupVersion is skipped", func(t *testing.T) {
		// Use a GVK string with slashes but where
		// ParseGroupVersion returns an error due to too many slashes.
		si := &SerializableIndex{
			Resources: []SerializableResource{
				{
					GVK:       "a/b/c/d/e/BadKind",
					Namespace: "default",
					Name:      "comp1",
					FilePath:  "/test/comp.yaml",
					Object:    map[string]any{},
				},
				{
					GVK:       "openchoreo.dev/v1alpha1/Component",
					Namespace: "default",
					Name:      "comp2",
					FilePath:  "/test/comp2.yaml",
					Object: map[string]any{
						"apiVersion": "openchoreo.dev/v1alpha1",
						"kind":       "Component",
						"metadata": map[string]any{
							"name":      "comp2",
							"namespace": "default",
						},
					},
				},
			},
		}

		idx := si.ToIndex("/repo")
		stats := idx.Stats()
		if stats.TotalResources != 1 {
			t.Errorf("expected 1 resource (invalid GVK skipped), got %d", stats.TotalResources)
		}
	})

	t.Run("multiple GVKs", func(t *testing.T) {
		si := &SerializableIndex{
			Resources: []SerializableResource{
				{
					GVK:       "openchoreo.dev/v1alpha1/Component",
					Namespace: "default",
					Name:      "comp1",
					FilePath:  "/f1.yaml",
					Object: map[string]any{
						"apiVersion": "openchoreo.dev/v1alpha1",
						"kind":       "Component",
						"metadata":   map[string]any{"name": "comp1", "namespace": "default"},
					},
				},
				{
					GVK:      "openchoreo.dev/v1alpha1/Trait",
					Name:     "trait1",
					FilePath: "/f2.yaml",
					Object: map[string]any{
						"apiVersion": "openchoreo.dev/v1alpha1",
						"kind":       "Trait",
						"metadata":   map[string]any{"name": "trait1"},
					},
				},
			},
		}

		idx := si.ToIndex("/repo")
		if idx.Stats().TotalResources != 2 {
			t.Errorf("expected 2 resources, got %d", idx.Stats().TotalResources)
		}
	})
}

func TestSerializableRoundTrip(t *testing.T) {
	idx := New("/test/repo")
	idx.SetCommitSHA("deadbeef")

	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default", "comp1", "/f1.yaml"))
	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Component", "ns2", "comp2", "/f2.yaml"))
	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "ComponentType", "", "http-service", "/f3.yaml"))
	_ = idx.Add(createTestEntry("apps", "v1", "Deployment", "default", "deploy1", "/f4.yaml"))

	// Serialize
	serializable := idx.ToSerializable()

	// Deserialize to a new repo path
	restored := serializable.ToIndex("/new/repo")

	// Verify repo path uses the new one
	if restored.GetRepoPath() != "/new/repo" {
		t.Errorf("expected repoPath '/new/repo', got %q", restored.GetRepoPath())
	}

	// Verify commitSHA is preserved
	if restored.GetCommitSHA() != "deadbeef" {
		t.Errorf("expected commitSHA 'deadbeef', got %q", restored.GetCommitSHA())
	}

	// Verify all resources are present
	stats := restored.Stats()
	if stats.TotalResources != 4 {
		t.Errorf("expected 4 resources, got %d", stats.TotalResources)
	}

	// Verify specific lookups
	compGVK := schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "Component"}
	if _, found := restored.Get(compGVK, "default", "comp1"); !found {
		t.Error("comp1 not found after round-trip")
	}
	if _, found := restored.Get(compGVK, "ns2", "comp2"); !found {
		t.Error("comp2 not found after round-trip")
	}

	ctGVK := schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "ComponentType"}
	if _, found := restored.Get(ctGVK, "", "http-service"); !found {
		t.Error("http-service not found after round-trip")
	}

	deployGVK := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	if _, found := restored.Get(deployGVK, "default", "deploy1"); !found {
		t.Error("deploy1 not found after round-trip")
	}
}
