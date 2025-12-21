// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package index

import (
	"fmt"
	"sync"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Helper function to create a test resource entry
func createTestEntry(group, version, kind, namespace, name, filePath string) *ResourceEntry {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   group,
		Version: version,
		Kind:    kind,
	})
	obj.SetNamespace(namespace)
	obj.SetName(name)

	return &ResourceEntry{
		Resource: obj,
		FilePath: filePath,
	}
}

func TestNewIndex(t *testing.T) {
	idx := New("/test/path")

	if idx == nil {
		t.Fatal("New() returned nil")
	}

	if idx.repoPath != "/test/path" {
		t.Errorf("expected repoPath '/test/path', got '%s'", idx.repoPath)
	}

	if idx.byGVK == nil {
		t.Error("byGVK map is nil")
	}

	if idx.byFilePath == nil {
		t.Error("byFilePath map is nil")
	}
}

func TestIndexAdd(t *testing.T) {
	idx := New("/test/path")

	entry := createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default", "test-component", "/test/file.yaml")

	err := idx.Add(entry)
	if err != nil {
		t.Fatalf("Add() returned error: %v", err)
	}

	// Verify resource was added to GVK index
	gvk := schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "Component"}
	retrieved, ok := idx.Get(gvk, "default", "test-component")
	if !ok {
		t.Error("resource not found in index after Add()")
	}

	if retrieved != entry {
		t.Error("retrieved entry does not match added entry")
	}

	// Verify resource was added to file path index
	fileEntries := idx.GetByFile("/test/file.yaml")
	if len(fileEntries) != 1 {
		t.Errorf("expected 1 entry for file, got %d", len(fileEntries))
	}
}

func TestIndexAddNil(t *testing.T) {
	idx := New("/test/path")

	// Test adding nil entry
	err := idx.Add(nil)
	if err == nil {
		t.Error("expected error when adding nil entry")
	}

	// Test adding entry with nil resource
	entry := &ResourceEntry{
		Resource: nil,
		FilePath: "/test/file.yaml",
	}
	err = idx.Add(entry)
	if err == nil {
		t.Error("expected error when adding entry with nil resource")
	}
}

func TestIndexGet(t *testing.T) {
	idx := New("/test/path")

	entry := createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default", "test-component", "/test/file.yaml")
	_ = idx.Add(entry)

	gvk := schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "Component"}

	tests := []struct {
		name      string
		namespace string
		resName   string
		wantFound bool
	}{
		{"existing resource", "default", "test-component", true},
		{"wrong namespace", "other", "test-component", false},
		{"wrong name", "default", "other-component", false},
		{"empty namespace (cluster-scoped)", "", "test-component", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, found := idx.Get(gvk, tt.namespace, tt.resName)
			if found != tt.wantFound {
				t.Errorf("Get() found = %v, want %v", found, tt.wantFound)
			}
		})
	}
}

func TestIndexGetClusterScoped(t *testing.T) {
	idx := New("/test/path")

	// Create a cluster-scoped resource (no namespace)
	entry := createTestEntry("openchoreo.dev", "v1alpha1", "ComponentType", "", "http-service", "/test/file.yaml")
	_ = idx.Add(entry)

	gvk := schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "ComponentType"}

	// Should find with empty namespace
	_, found := idx.Get(gvk, "", "http-service")
	if !found {
		t.Error("cluster-scoped resource not found")
	}
}

func TestIndexList(t *testing.T) {
	idx := New("/test/path")

	componentGVK := schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "Component"}
	traitGVK := schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "Trait"}

	// Add multiple components
	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default", "comp1", "/f1.yaml"))
	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default", "comp2", "/f2.yaml"))
	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Component", "other", "comp3", "/f3.yaml"))

	// Add a trait
	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Trait", "", "volume", "/f4.yaml"))

	components := idx.List(componentGVK)
	if len(components) != 3 {
		t.Errorf("expected 3 components, got %d", len(components))
	}

	traits := idx.List(traitGVK)
	if len(traits) != 1 {
		t.Errorf("expected 1 trait, got %d", len(traits))
	}

	// List non-existent GVK
	emptyGVK := schema.GroupVersionKind{Group: "test", Version: "v1", Kind: "Nothing"}
	empty := idx.List(emptyGVK)
	if len(empty) != 0 {
		t.Errorf("expected 0 entries for non-existent GVK, got %d", len(empty))
	}
}

func TestIndexListAll(t *testing.T) {
	idx := New("/test/path")

	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default", "comp1", "/f1.yaml"))
	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Trait", "", "trait1", "/f2.yaml"))
	_ = idx.Add(createTestEntry("apps", "v1", "Deployment", "default", "deploy1", "/f3.yaml"))

	all := idx.ListAll()
	if len(all) != 3 {
		t.Errorf("expected 3 total entries, got %d", len(all))
	}
}

func TestIndexGetByFile(t *testing.T) {
	idx := New("/test/path")

	// Add multiple resources from same file
	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default", "comp1", "/multi.yaml"))
	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Workload", "default", "workload1", "/multi.yaml"))

	// Add resource from different file
	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default", "comp2", "/other.yaml"))

	multiEntries := idx.GetByFile("/multi.yaml")
	if len(multiEntries) != 2 {
		t.Errorf("expected 2 entries from /multi.yaml, got %d", len(multiEntries))
	}

	otherEntries := idx.GetByFile("/other.yaml")
	if len(otherEntries) != 1 {
		t.Errorf("expected 1 entry from /other.yaml, got %d", len(otherEntries))
	}

	// Non-existent file
	noEntries := idx.GetByFile("/nonexistent.yaml")
	if len(noEntries) != 0 {
		t.Errorf("expected 0 entries from non-existent file, got %d", len(noEntries))
	}
}

func TestIndexRemoveEntriesForFile(t *testing.T) {
	idx := New("/test/path")

	componentGVK := schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "Component"}

	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default", "comp1", "/file1.yaml"))
	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default", "comp2", "/file1.yaml"))
	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default", "comp3", "/file2.yaml"))

	// Verify initial state
	if len(idx.List(componentGVK)) != 3 {
		t.Fatal("initial setup failed")
	}

	// Remove entries for file1
	idx.RemoveEntriesForFile("/file1.yaml")

	// Verify removal from GVK index
	remaining := idx.List(componentGVK)
	if len(remaining) != 1 {
		t.Errorf("expected 1 remaining component, got %d", len(remaining))
	}

	// Verify comp1 and comp2 are gone
	_, found := idx.Get(componentGVK, "default", "comp1")
	if found {
		t.Error("comp1 should have been removed")
	}

	_, found = idx.Get(componentGVK, "default", "comp2")
	if found {
		t.Error("comp2 should have been removed")
	}

	// Verify comp3 still exists
	_, found = idx.Get(componentGVK, "default", "comp3")
	if !found {
		t.Error("comp3 should still exist")
	}

	// Verify file path index is cleared
	entries := idx.GetByFile("/file1.yaml")
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for removed file, got %d", len(entries))
	}
}

func TestIndexRemoveEntriesForNonexistentFile(t *testing.T) {
	idx := New("/test/path")

	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default", "comp1", "/file1.yaml"))

	// Should not panic or error
	idx.RemoveEntriesForFile("/nonexistent.yaml")

	// Original entry should still exist
	componentGVK := schema.GroupVersionKind{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "Component"}
	if len(idx.List(componentGVK)) != 1 {
		t.Error("existing entries should not be affected")
	}
}

func TestIndexStats(t *testing.T) {
	idx := New("/test/path")

	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default", "comp1", "/f1.yaml"))
	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default", "comp2", "/f2.yaml"))
	_ = idx.Add(createTestEntry("openchoreo.dev", "v1alpha1", "Trait", "", "trait1", "/f3.yaml"))

	stats := idx.Stats()

	if stats.TotalResources != 3 {
		t.Errorf("expected TotalResources=3, got %d", stats.TotalResources)
	}

	if stats.TotalFiles != 3 {
		t.Errorf("expected TotalFiles=3, got %d", stats.TotalFiles)
	}

	if stats.GVKCounts["Component"] != 2 {
		t.Errorf("expected 2 Components, got %d", stats.GVKCounts["Component"])
	}

	if stats.GVKCounts["Trait"] != 1 {
		t.Errorf("expected 1 Trait, got %d", stats.GVKCounts["Trait"])
	}
}

func TestIndexCommitSHA(t *testing.T) {
	idx := New("/test/path")

	// Initially empty
	if idx.GetCommitSHA() != "" {
		t.Error("commit SHA should be empty initially")
	}

	// Set and get
	idx.SetCommitSHA("abc123")
	if idx.GetCommitSHA() != "abc123" {
		t.Errorf("expected commit SHA 'abc123', got '%s'", idx.GetCommitSHA())
	}
}

func TestIndexRepoPath(t *testing.T) {
	idx := New("/my/repo/path")

	if idx.GetRepoPath() != "/my/repo/path" {
		t.Errorf("expected repo path '/my/repo/path', got '%s'", idx.GetRepoPath())
	}
}

func TestIndexConcurrentAccess(t *testing.T) {
	idx := New("/test/path")
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			entry := createTestEntry("openchoreo.dev", "v1alpha1", "Component", "default",
				fmt.Sprintf("comp%d", n%10), "/file.yaml")
			_ = idx.Add(entry)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = idx.ListAll()
			_ = idx.Stats()
		}()
	}

	wg.Wait()

	// Verify no corruption
	stats := idx.Stats()
	if stats.TotalResources == 0 {
		t.Error("expected some resources after concurrent operations")
	}
}
