// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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

// setupTestRepoMultiFile creates a temp dir with multiple YAML files for testing
func setupTestRepoMultiFile(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()

	componentsDir := filepath.Join(tmpDir, "components")
	if err := os.MkdirAll(componentsDir, 0755); err != nil {
		t.Fatalf("failed to create components dir: %v", err)
	}

	files := map[string]string{
		"components/comp1.yaml": `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: comp1
  namespace: default
`,
		"components/comp2.yaml": `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: comp2
  namespace: default
`,
		"components/comp3.yaml": `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: comp3
  namespace: default
`,
	}

	for relPath, content := range files {
		fullPath := filepath.Join(tmpDir, relPath)
		if err := os.WriteFile(fullPath, []byte(content), 0600); err != nil {
			t.Fatalf("failed to write %s: %v", relPath, err)
		}
	}

	return tmpDir
}

// buildPersistentIndex builds an index and returns the PersistentIndex with populated metadata
func buildPersistentIndex(t *testing.T, repoPath string) *PersistentIndex {
	t.Helper()
	pi, err := LoadOrBuild(repoPath)
	if err != nil {
		t.Fatalf("LoadOrBuild() error: %v", err)
	}
	return pi
}

// --- Tests for getChangedFiles ---

func TestGetChangedFiles_NilMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	pi := &PersistentIndex{
		repoPath: tmpDir,
		metadata: nil,
	}

	changed, err := pi.getChangedFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed != nil {
		t.Errorf("expected nil, got %v", changed)
	}
}

func TestGetChangedFiles_NilFileStates(t *testing.T) {
	tmpDir := t.TempDir()
	pi := &PersistentIndex{
		repoPath: tmpDir,
		metadata: &CacheMetadata{
			FileStates: nil,
		},
	}

	changed, err := pi.getChangedFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed != nil {
		t.Errorf("expected nil, got %v", changed)
	}
}

func TestGetChangedFiles_NoChanges(t *testing.T) {
	repoPath := setupTestRepo(t)
	pi := buildPersistentIndex(t, repoPath)

	changed, err := pi.getChangedFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("expected no changes, got %v", changed)
	}
}

func TestGetChangedFiles_NewFile(t *testing.T) {
	repoPath := setupTestRepo(t)
	pi := buildPersistentIndex(t, repoPath)

	// Add a new file after building the index
	newYAML := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: new-component
  namespace: default
`
	if err := os.WriteFile(filepath.Join(repoPath, "components", "new.yaml"), []byte(newYAML), 0600); err != nil {
		t.Fatalf("failed to write new file: %v", err)
	}

	changed, err := pi.getChangedFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(changed) == 0 {
		t.Fatal("expected new file to be detected as changed")
	}

	if !slices.Contains(changed, "components/new.yaml") {
		t.Errorf("expected 'components/new.yaml' in changed files, got %v", changed)
	}
}

func TestGetChangedFiles_ModifiedFile(t *testing.T) {
	repoPath := setupTestRepo(t)
	pi := buildPersistentIndex(t, repoPath)

	// Modify the existing file
	time.Sleep(10 * time.Millisecond)
	modifiedYAML := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: modified-name
  namespace: default
spec:
  componentType: http-service
  extra: field
`
	componentFile := filepath.Join(repoPath, "components", "component.yaml")
	if err := os.WriteFile(componentFile, []byte(modifiedYAML), 0600); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	changed, err := pi.getChangedFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(changed) == 0 {
		t.Fatal("expected modified file to be detected")
	}

	if !slices.Contains(changed, "components/component.yaml") {
		t.Errorf("expected 'components/component.yaml' in changed files, got %v", changed)
	}
}

func TestGetChangedFiles_DeletedFile(t *testing.T) {
	repoPath := setupTestRepoMultiFile(t)
	pi := buildPersistentIndex(t, repoPath)

	// Delete a file
	if err := os.Remove(filepath.Join(repoPath, "components", "comp2.yaml")); err != nil {
		t.Fatalf("failed to delete file: %v", err)
	}

	changed, err := pi.getChangedFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(changed) == 0 {
		t.Fatal("expected deleted file to be detected")
	}

	if !slices.Contains(changed, "components/comp2.yaml") {
		t.Errorf("expected 'components/comp2.yaml' in changed files, got %v", changed)
	}
}

// --- Tests for incrementalUpdate ---

func TestIncrementalUpdate_DeletedFile(t *testing.T) {
	repoPath := setupTestRepoMultiFile(t)
	pi := buildPersistentIndex(t, repoPath)

	initialCount := pi.Index.Stats().TotalResources

	// Delete the file from disk
	if err := os.Remove(filepath.Join(repoPath, "components", "comp2.yaml")); err != nil {
		t.Fatalf("failed to delete file: %v", err)
	}

	// Call incrementalUpdate directly with the deleted file
	err := pi.incrementalUpdate([]string{"components/comp2.yaml"})
	if err != nil {
		t.Fatalf("incrementalUpdate() error: %v", err)
	}

	// Should have one fewer resource
	newCount := pi.Index.Stats().TotalResources
	if newCount != initialCount-1 {
		t.Errorf("expected %d resources, got %d", initialCount-1, newCount)
	}

	// FileState should be removed
	if _, ok := pi.metadata.FileStates["components/comp2.yaml"]; ok {
		t.Error("deleted file should be removed from FileStates")
	}
}

func TestIncrementalUpdate_ValidReparse(t *testing.T) {
	repoPath := setupTestRepo(t)
	pi := buildPersistentIndex(t, repoPath)
	originalHash := pi.metadata.FileStates["components/component.yaml"].Hash

	// Modify the file with a new resource name
	modifiedYAML := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: renamed-component
  namespace: default
spec:
  componentType: http-service
`
	componentFile := filepath.Join(repoPath, "components", "component.yaml")
	if err := os.WriteFile(componentFile, []byte(modifiedYAML), 0600); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	err := pi.incrementalUpdate([]string{"components/component.yaml"})
	if err != nil {
		t.Fatalf("incrementalUpdate() error: %v", err)
	}

	// Should still have 1 resource
	if pi.Index.Stats().TotalResources != 1 {
		t.Errorf("expected 1 resource, got %d", pi.Index.Stats().TotalResources)
	}

	// FileState should be updated with new hash
	state, ok := pi.metadata.FileStates["components/component.yaml"]
	if !ok {
		t.Fatal("FileState should still exist for modified file")
	}
	if state.Hash == originalHash {
		t.Error("FileState hash should change after reparsing the modified file")
	}
}

func TestIncrementalUpdate_InvalidYAML(t *testing.T) {
	repoPath := setupTestRepo(t)
	pi := buildPersistentIndex(t, repoPath)

	// Overwrite with invalid YAML
	componentFile := filepath.Join(repoPath, "components", "component.yaml")
	if err := os.WriteFile(componentFile, []byte("this: is: not: valid: yaml\n  broken"), 0600); err != nil {
		t.Fatalf("failed to write invalid YAML: %v", err)
	}

	err := pi.incrementalUpdate([]string{"components/component.yaml"})
	if err != nil {
		t.Fatalf("incrementalUpdate() should not error on invalid YAML: %v", err)
	}

	if pi.Index.Stats().TotalResources != 0 {
		t.Fatalf("expected stale indexed resources to be removed, got %d", pi.Index.Stats().TotalResources)
	}

	// FileState should still be updated (hash of the invalid file)
	state, ok := pi.metadata.FileStates["components/component.yaml"]
	if !ok {
		t.Fatal("FileState should exist even for invalid YAML")
	}
	if state.Hash == "" {
		t.Error("FileState hash should not be empty")
	}
}

func TestIncrementalUpdate_MultipleChanges(t *testing.T) {
	repoPath := setupTestRepoMultiFile(t)
	pi := buildPersistentIndex(t, repoPath)

	// 1. Add a new file
	newYAML := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: new-comp
  namespace: default
`
	if err := os.WriteFile(filepath.Join(repoPath, "components", "new.yaml"), []byte(newYAML), 0600); err != nil {
		t.Fatalf("failed to write new file: %v", err)
	}

	// 2. Modify comp1
	modifiedYAML := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: comp1-modified
  namespace: default
`
	if err := os.WriteFile(filepath.Join(repoPath, "components", "comp1.yaml"), []byte(modifiedYAML), 0600); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	// 3. Delete comp3
	if err := os.Remove(filepath.Join(repoPath, "components", "comp3.yaml")); err != nil {
		t.Fatalf("failed to delete file: %v", err)
	}

	changedFiles := []string{
		"components/new.yaml",
		"components/comp1.yaml",
		"components/comp3.yaml",
	}

	err := pi.incrementalUpdate(changedFiles)
	if err != nil {
		t.Fatalf("incrementalUpdate() error: %v", err)
	}

	// Should have 3 resources: comp2 (unchanged) + comp1-modified + new-comp
	if pi.Index.Stats().TotalResources != 3 {
		t.Errorf("expected 3 resources, got %d", pi.Index.Stats().TotalResources)
	}

	// comp3 should be removed from FileStates
	if _, ok := pi.metadata.FileStates["components/comp3.yaml"]; ok {
		t.Error("deleted file should not be in FileStates")
	}

	// new file should be in FileStates
	if _, ok := pi.metadata.FileStates["components/new.yaml"]; !ok {
		t.Error("new file should be in FileStates")
	}
}

// --- Tests for isYAMLFile ---

func TestIsYAMLFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"file.yaml", true},
		{"file.yml", true},
		{"file.YAML", true},
		{"file.YML", true},
		{"file.json", false},
		{"file.go", false},
		{"file", false},
		{"path/to/file.yaml", true},
		{"path/to/file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isYAMLFile(tt.path)
			if got != tt.want {
				t.Errorf("isYAMLFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// --- Tests for hashFileContent ---

func TestHashFileContent(t *testing.T) {
	t.Run("known content", func(t *testing.T) {
		tmpDir := t.TempDir()
		f := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(f, []byte("hello"), 0600); err != nil {
			t.Fatal(err)
		}

		hash, err := hashFileContent(f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// SHA256 of "hello"
		expected := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
		if hash != expected {
			t.Errorf("hash = %q, want %q", hash, expected)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := hashFileContent("/nonexistent/file.txt")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("consistency", func(t *testing.T) {
		tmpDir := t.TempDir()
		f := filepath.Join(tmpDir, "test.yaml")
		if err := os.WriteFile(f, []byte("content"), 0600); err != nil {
			t.Fatal(err)
		}

		hash1, err := hashFileContent(f)
		if err != nil {
			t.Fatalf("first hashFileContent() error: %v", err)
		}

		hash2, err := hashFileContent(f)
		if err != nil {
			t.Fatalf("second hashFileContent() error: %v", err)
		}

		if hash1 != hash2 {
			t.Errorf("hash not consistent: %q != %q", hash1, hash2)
		}
	})
}

// --- Tests for saveToDisk error paths ---

func TestSaveToDisk_MkdirAllError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a regular file where the cache directory should be
	blocker := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blocker, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}

	pi := &PersistentIndex{
		Index:    index.New(tmpDir),
		repoPath: tmpDir,
		cacheDir: filepath.Join(blocker, "cache"), // parent is a file, not a dir
		metadata: &CacheMetadata{
			FileStates: make(map[string]FileState),
		},
	}

	err := pi.saveToDisk()
	if err == nil {
		t.Fatal("expected error when cache dir parent is a file")
	}
}

func TestSaveToDisk_WriteError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root")
	}

	tmpDir := t.TempDir()

	cacheDir := filepath.Join(tmpDir, DirName)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Make cache dir read-only so WriteFile fails
	if err := os.Chmod(cacheDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(cacheDir, 0755) // restore for cleanup
	})

	pi := &PersistentIndex{
		Index:    index.New(tmpDir),
		repoPath: tmpDir,
		cacheDir: cacheDir,
		metadata: &CacheMetadata{
			FileStates: make(map[string]FileState),
		},
	}

	err := pi.saveToDisk()
	if err == nil {
		t.Fatal("expected error when cache dir is read-only")
	}
}

// --- Tests for computeCurrentDirectoryHash ---

func TestComputeCurrentDirectoryHash_NewFile(t *testing.T) {
	repoPath := setupTestRepo(t)
	pi := buildPersistentIndex(t, repoPath)

	originalHash, err := pi.computeCurrentDirectoryHash()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Add a new file (not in FileStates, triggers the else branch in computeCurrentDirectoryHash)
	newYAML := `apiVersion: v1
kind: ConfigMap
metadata:
  name: new-config
`
	if err := os.WriteFile(filepath.Join(repoPath, "new.yaml"), []byte(newYAML), 0600); err != nil {
		t.Fatalf("failed to write new file: %v", err)
	}

	newHash, err := pi.computeCurrentDirectoryHash()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if newHash == originalHash {
		t.Error("hash should change after adding a new file")
	}
}

func TestComputeCurrentDirectoryHash_ChangedFile(t *testing.T) {
	repoPath := setupTestRepo(t)
	pi := buildPersistentIndex(t, repoPath)

	originalHash, err := pi.computeCurrentDirectoryHash()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Modify the file to change its size (triggers size mismatch branch)
	time.Sleep(10 * time.Millisecond)
	modifiedContent := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: test-component
  namespace: default
spec:
  componentType: http-service
  description: this makes the file larger to change the size
`
	modifiedFilePath := filepath.Join(repoPath, "components", "component.yaml")
	if err := os.WriteFile(modifiedFilePath, []byte(modifiedContent), 0600); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	newHash, err := pi.computeCurrentDirectoryHash()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if newHash == originalHash {
		t.Error("hash should change after modifying a file")
	}
}
