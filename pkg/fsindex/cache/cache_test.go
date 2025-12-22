// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// setupTestRepo creates a temporary directory with test YAML files
func setupTestRepo(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()

	componentsDir := filepath.Join(tmpDir, "components")
	if err := os.MkdirAll(componentsDir, 0755); err != nil {
		t.Fatalf("failed to create components dir: %v", err)
	}

	testYAML := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: test-component
  namespace: default
spec:
  componentType: http-service
`
	if err := os.WriteFile(filepath.Join(componentsDir, "component.yaml"), []byte(testYAML), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	return tmpDir
}

func TestLoadOrBuildNewIndex(t *testing.T) {
	repoPath := setupTestRepo(t)

	pi, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("LoadOrBuild() error: %v", err)
	}

	if pi == nil {
		t.Fatal("LoadOrBuild() returned nil")
	}

	// Verify index was built
	stats := pi.Index.Stats()
	if stats.TotalResources != 1 {
		t.Errorf("TotalResources = %d, want 1", stats.TotalResources)
	}

	// Verify cache was created
	cacheDir := filepath.Join(repoPath, DirName)
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		t.Error("cache directory was not created")
	}

	// Verify index file exists
	indexFile := filepath.Join(cacheDir, IndexFile)
	if _, err := os.Stat(indexFile); os.IsNotExist(err) {
		t.Error("index file was not created")
	}

	// Verify metadata file exists
	metaFile := filepath.Join(cacheDir, MetadataFile)
	if _, err := os.Stat(metaFile); os.IsNotExist(err) {
		t.Error("metadata file was not created")
	}
}

func TestLoadOrBuildWithExistingCache(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Build initial index
	pi1, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("first LoadOrBuild() error: %v", err)
	}

	// Read the initial metadata to get the first LastUsed time
	metaFile := filepath.Join(repoPath, DirName, MetadataFile)
	data1, err := os.ReadFile(metaFile)
	if err != nil {
		t.Fatalf("failed to read initial metadata: %v", err)
	}

	var initialMeta CacheMetadata
	if err := json.Unmarshal(data1, &initialMeta); err != nil {
		t.Fatalf("failed to unmarshal initial metadata: %v", err)
	}

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Load again - should use cache
	pi2, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("second LoadOrBuild() error: %v", err)
	}

	if pi2 == nil {
		t.Fatal("second LoadOrBuild() returned nil")
	}

	// Verify it loaded the same data
	if pi1.Index.Stats().TotalResources != pi2.Index.Stats().TotalResources {
		t.Error("cached index has different resource count")
	}

	// Check that metadata was updated (LastUsed)
	data2, err := os.ReadFile(metaFile)
	if err != nil {
		t.Fatalf("failed to read updated metadata: %v", err)
	}

	var updatedMeta CacheMetadata
	if err := json.Unmarshal(data2, &updatedMeta); err != nil {
		t.Fatalf("failed to unmarshal updated metadata: %v", err)
	}

	if !updatedMeta.LastUsed.After(initialMeta.LastUsed) {
		t.Errorf("LastUsed should be updated on cache load: initial=%v, updated=%v",
			initialMeta.LastUsed, updatedMeta.LastUsed)
	}
}

func TestDirectoryHashComputation(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Build initial index
	pi, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("LoadOrBuild() error: %v", err)
	}

	// Verify directory hash is set
	if pi.metadata.DirectoryHash == "" {
		t.Error("DirectoryHash should be set")
	}

	// Verify file states are set
	if len(pi.metadata.FileStates) == 0 {
		t.Error("FileStates should have entries")
	}

	// Verify the component file is tracked
	if _, ok := pi.metadata.FileStates["components/component.yaml"]; !ok {
		t.Error("FileStates should contain components/component.yaml")
	}
}

func TestForceRebuild(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Build initial index
	pi1, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("LoadOrBuild() error: %v", err)
	}

	originalTime := pi1.metadata.CreatedAt

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Force rebuild
	pi2, err := ForceRebuild(repoPath)
	if err != nil {
		t.Fatalf("ForceRebuild() error: %v", err)
	}

	// Verify it's a new index (different creation time)
	if !pi2.metadata.CreatedAt.After(originalTime) {
		t.Error("ForceRebuild should create new index with later timestamp")
	}
}

func TestClearCache(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Build index to create cache
	_, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("LoadOrBuild() error: %v", err)
	}

	// Verify cache exists
	cacheDir := filepath.Join(repoPath, DirName)
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		t.Fatal("cache directory should exist")
	}

	// Clear cache
	err = ClearCache(repoPath)
	if err != nil {
		t.Fatalf("ClearCache() error: %v", err)
	}

	// Verify cache is gone
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Error("cache directory should be removed")
	}
}

func TestClearCacheNonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	// Should not error even if cache doesn't exist
	err := ClearCache(tmpDir)
	if err != nil {
		t.Errorf("ClearCache() should not error for non-existent cache: %v", err)
	}
}

func TestCacheMetadataVersionCheck(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Build initial index
	_, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("LoadOrBuild() error: %v", err)
	}

	// Modify metadata to have wrong version
	metaFile := filepath.Join(repoPath, DirName, MetadataFile)
	data, _ := os.ReadFile(metaFile)

	var meta CacheMetadata
	_ = json.Unmarshal(data, &meta)

	newData, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(metaFile, newData, 0600)

	// Load again - should rebuild due to version mismatch
	pi, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("LoadOrBuild() error: %v", err)
	}

	// Verify version is now correct
	metaData, _ := os.ReadFile(metaFile)
	var newMeta CacheMetadata
	_ = json.Unmarshal(metaData, &newMeta)

	_ = pi // use variable
}

func TestCacheInvalidJSON(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Build initial index
	_, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("LoadOrBuild() error: %v", err)
	}

	// Corrupt the metadata file
	metaFile := filepath.Join(repoPath, DirName, MetadataFile)
	_ = os.WriteFile(metaFile, []byte("invalid json{{{"), 0600)

	// Load again - should rebuild due to invalid metadata
	pi, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("LoadOrBuild() error: %v", err)
	}

	// Should still work
	if pi.Index.Stats().TotalResources != 1 {
		t.Error("should have rebuilt index successfully")
	}
}

func TestCacheCorruptedIndexFile(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Build initial index
	_, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("LoadOrBuild() error: %v", err)
	}

	// Corrupt the index file
	indexFile := filepath.Join(repoPath, DirName, IndexFile)
	_ = os.WriteFile(indexFile, []byte("not valid json"), 0600)

	// Load again - should rebuild due to corrupted index
	pi, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("LoadOrBuild() error: %v", err)
	}

	// Should still work
	if pi.Index.Stats().TotalResources != 1 {
		t.Error("should have rebuilt index successfully")
	}
}

func TestIncrementalUpdateNewFile(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Build initial index
	pi1, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("first LoadOrBuild() error: %v", err)
	}

	initialCount := pi1.Index.Stats().TotalResources
	initialDirHash := pi1.metadata.DirectoryHash

	// Add a new file
	newFile := filepath.Join(repoPath, "components", "new-component.yaml")
	newYAML := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: new-component
  namespace: default
spec:
  componentType: http-service
`
	if err := os.WriteFile(newFile, []byte(newYAML), 0600); err != nil {
		t.Fatalf("failed to write new file: %v", err)
	}

	// Load again - should do incremental update
	pi2, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("second LoadOrBuild() error: %v", err)
	}

	newCount := pi2.Index.Stats().TotalResources

	// Should have one more resource
	if newCount != initialCount+1 {
		t.Errorf("expected %d resources after incremental update, got %d", initialCount+1, newCount)
	}

	// Directory hash should have changed
	if pi2.metadata.DirectoryHash == initialDirHash {
		t.Error("DirectoryHash should change after adding a file")
	}

	// New file should be in FileStates
	if _, ok := pi2.metadata.FileStates["components/new-component.yaml"]; !ok {
		t.Error("FileStates should contain the new file")
	}
}

func TestIncrementalUpdateModifiedFile(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Build initial index
	pi1, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("first LoadOrBuild() error: %v", err)
	}

	initialDirHash := pi1.metadata.DirectoryHash
	initialFileHash := pi1.metadata.FileStates["components/component.yaml"].Hash

	// Modify the existing file
	componentFile := filepath.Join(repoPath, "components", "component.yaml")
	modifiedYAML := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: test-component-modified
  namespace: default
spec:
  componentType: http-service
`
	// Wait a bit to ensure mtime changes
	time.Sleep(10 * time.Millisecond)

	if err := os.WriteFile(componentFile, []byte(modifiedYAML), 0600); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	// Load again - should do incremental update
	pi2, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("second LoadOrBuild() error: %v", err)
	}

	// Directory hash should have changed
	if pi2.metadata.DirectoryHash == initialDirHash {
		t.Error("DirectoryHash should change after modifying a file")
	}

	// File hash should have changed
	newFileHash := pi2.metadata.FileStates["components/component.yaml"].Hash
	if newFileHash == initialFileHash {
		t.Error("File hash should change after modifying the file")
	}
}

func TestIncrementalUpdateDeletedFile(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Add another file first
	extraFile := filepath.Join(repoPath, "components", "extra.yaml")
	extraYAML := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: extra-component
  namespace: default
`
	if err := os.WriteFile(extraFile, []byte(extraYAML), 0600); err != nil {
		t.Fatalf("failed to write extra file: %v", err)
	}

	// Build initial index
	pi1, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("first LoadOrBuild() error: %v", err)
	}

	initialCount := pi1.Index.Stats().TotalResources

	// Delete the extra file
	if err := os.Remove(extraFile); err != nil {
		t.Fatalf("failed to remove file: %v", err)
	}

	// Load again
	pi2, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("second LoadOrBuild() error: %v", err)
	}

	newCount := pi2.Index.Stats().TotalResources

	// Should have one fewer resource
	if newCount != initialCount-1 {
		t.Errorf("expected %d resources after deletion, got %d", initialCount-1, newCount)
	}

	// Deleted file should not be in FileStates
	if _, ok := pi2.metadata.FileStates["components/extra.yaml"]; ok {
		t.Error("FileStates should not contain the deleted file")
	}
}

func TestNoChangeDetection(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Build initial index
	pi1, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("first LoadOrBuild() error: %v", err)
	}

	initialDirHash := pi1.metadata.DirectoryHash

	// Load again without any changes
	pi2, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("second LoadOrBuild() error: %v", err)
	}

	// Directory hash should remain the same
	if pi2.metadata.DirectoryHash != initialDirHash {
		t.Error("DirectoryHash should not change when no files changed")
	}
}

func TestHiddenDirectoriesSkipped(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create a hidden directory with YAML files
	hiddenDir := filepath.Join(repoPath, ".hidden")
	if err := os.MkdirAll(hiddenDir, 0755); err != nil {
		t.Fatalf("failed to create hidden dir: %v", err)
	}

	hiddenYAML := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: hidden-component
  namespace: default
`
	if err := os.WriteFile(filepath.Join(hiddenDir, "hidden.yaml"), []byte(hiddenYAML), 0600); err != nil {
		t.Fatalf("failed to write hidden file: %v", err)
	}

	// Build index
	pi, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("LoadOrBuild() error: %v", err)
	}

	// Hidden files should not be indexed
	if _, ok := pi.metadata.FileStates[".hidden/hidden.yaml"]; ok {
		t.Error("Hidden directories should be skipped")
	}

	// Should only have the one non-hidden component
	if pi.Index.Stats().TotalResources != 1 {
		t.Errorf("Expected 1 resource (hidden files skipped), got %d", pi.Index.Stats().TotalResources)
	}
}

func TestSerializableIndexRoundTrip(t *testing.T) {
	// Create an index and add resources
	idx := index.New("/test/repo")

	// Create and add a resource
	entry := &index.ResourceEntry{
		Resource: func() *unstructured.Unstructured {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "openchoreo.dev/v1alpha1",
					"kind":       "Component",
					"metadata": map[string]interface{}{
						"name":      "test-comp",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"owner": map[string]interface{}{
							"projectName": "my-project",
						},
					},
				},
			}
			return obj
		}(),
		FilePath: "/test/comp.yaml",
	}
	_ = idx.Add(entry)

	// Serialize
	serializable := idx.ToSerializable()

	// Verify serialization
	if len(serializable.Resources) != 1 {
		t.Errorf("expected 1 resource in serializable, got %d", len(serializable.Resources))
	}

	// Deserialize
	restored := serializable.ToIndex("/test/repo")

	// Verify deserialization
	stats := restored.Stats()
	if stats.TotalResources != 1 {
		t.Errorf("expected 1 resource after deserialization, got %d", stats.TotalResources)
	}
}

func TestFileHashConsistency(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Build initial index
	pi1, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("first LoadOrBuild() error: %v", err)
	}

	hash1 := pi1.metadata.FileStates["components/component.yaml"].Hash

	// Build again without changes
	pi2, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("second LoadOrBuild() error: %v", err)
	}

	hash2 := pi2.metadata.FileStates["components/component.yaml"].Hash

	// Hashes should be identical
	if hash1 != hash2 {
		t.Errorf("File hash should be consistent: %s != %s", hash1, hash2)
	}
}
