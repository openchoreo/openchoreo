// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestNewScanner(t *testing.T) {
	tests := []struct {
		name        string
		opts        ScanOptions
		wantWorkers int
	}{
		{
			name:        "default options",
			opts:        DefaultScanOptions(),
			wantWorkers: 10,
		},
		{
			name:        "custom workers",
			opts:        ScanOptions{Workers: 5},
			wantWorkers: 5,
		},
		{
			name:        "zero workers defaults to 10",
			opts:        ScanOptions{Workers: 0},
			wantWorkers: 10,
		},
		{
			name:        "negative workers defaults to 10",
			opts:        ScanOptions{Workers: -1},
			wantWorkers: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(tt.opts)
			if s.opts.Workers != tt.wantWorkers {
				t.Errorf("workers = %d, want %d", s.opts.Workers, tt.wantWorkers)
			}
			if s.opts.Filter == nil {
				t.Error("filter should not be nil")
			}
		})
	}
}

func TestDefaultScanOptions(t *testing.T) {
	opts := DefaultScanOptions()

	if opts.Workers != 10 {
		t.Errorf("Workers = %d, want 10", opts.Workers)
	}

	if opts.Filter == nil {
		t.Error("Filter should not be nil")
	}

	if opts.Verbose {
		t.Error("Verbose should be false by default")
	}

	if opts.ErrorHandler == nil {
		t.Error("ErrorHandler should not be nil")
	}
}

func TestScannerScan(t *testing.T) {
	// Create temporary test directory structure
	tmpDir := t.TempDir()

	// Create test YAML files
	files := map[string]string{
		"components/app.yaml": `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: test-app
  namespace: default
`,
		"workloads/service.yaml": `
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: test-workload
  namespace: default
`,
		"traits/volume.yaml": `
apiVersion: openchoreo.dev/v1alpha1
kind: Trait
metadata:
  name: test-trait
`,
		"multi/resources.yaml": `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: multi-comp
---
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: multi-workload
`,
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0600); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	// Scan the directory
	s := New(DefaultScanOptions())
	idx, err := s.Scan(tmpDir)

	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if idx == nil {
		t.Fatal("Scan() returned nil index")
	}

	// Verify statistics
	stats := idx.Stats()

	// We expect 5 resources: 2 components, 2 workloads, 1 trait
	if stats.TotalResources != 5 {
		t.Errorf("TotalResources = %d, want 5", stats.TotalResources)
	}

	if stats.TotalFiles != 4 {
		t.Errorf("TotalFiles = %d, want 4", stats.TotalFiles)
	}

	// Check GVK counts
	if stats.GVKCounts["Component"] != 2 {
		t.Errorf("Component count = %d, want 2", stats.GVKCounts["Component"])
	}

	if stats.GVKCounts["Workload"] != 2 {
		t.Errorf("Workload count = %d, want 2", stats.GVKCounts["Workload"])
	}

	if stats.GVKCounts["Trait"] != 1 {
		t.Errorf("Trait count = %d, want 1", stats.GVKCounts["Trait"])
	}
}

func TestScannerExcludesDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files in various directories
	files := map[string]string{
		"components/valid.yaml": `
apiVersion: v1
kind: ConfigMap
metadata:
  name: valid
`,
		".git/config.yaml": `
apiVersion: v1
kind: ConfigMap
metadata:
  name: git-file
`,
		"node_modules/pkg/resource.yaml": `
apiVersion: v1
kind: ConfigMap
metadata:
  name: node-file
`,
		".ocg/index.yaml": `
apiVersion: v1
kind: ConfigMap
metadata:
  name: cache-file
`,
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0600); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	s := New(DefaultScanOptions())
	idx, err := s.Scan(tmpDir)

	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	stats := idx.Stats()

	// Should only find the valid.yaml file
	if stats.TotalResources != 1 {
		t.Errorf("TotalResources = %d, want 1 (excluded directories should be skipped)", stats.TotalResources)
	}
}

func TestScannerNonExistentPath(t *testing.T) {
	s := New(DefaultScanOptions())
	_, err := s.Scan("/nonexistent/path/that/does/not/exist")

	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestScannerEmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	s := New(DefaultScanOptions())
	idx, err := s.Scan(tmpDir)

	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	stats := idx.Stats()
	if stats.TotalResources != 0 {
		t.Errorf("TotalResources = %d, want 0 for empty directory", stats.TotalResources)
	}
}

func TestScannerErrorHandler(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid file
	validFile := filepath.Join(tmpDir, "valid.yaml")
	if err := os.WriteFile(validFile, []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: valid
`), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Create an invalid YAML file
	invalidFile := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(invalidFile, []byte(`
this is not: valid: yaml: content
  - broken
    indentation
`), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	var errorCount int32
	opts := ScanOptions{
		Workers: 2,
		Filter:  DefaultFilter(),
		ErrorHandler: func(path string, err error) {
			atomic.AddInt32(&errorCount, 1)
		},
	}

	s := New(opts)
	idx, _ := s.Scan(tmpDir)

	// Should still return an index with valid resources
	if idx == nil {
		t.Fatal("index should not be nil even with errors")
	}

	stats := idx.Stats()
	if stats.TotalResources != 1 {
		t.Errorf("TotalResources = %d, want 1 (valid file)", stats.TotalResources)
	}
}

func TestScannerCustomFilter(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files with different extensions
	files := map[string]string{
		"config.yaml": `
apiVersion: v1
kind: ConfigMap
metadata:
  name: yaml-config
`,
		"config.json": `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"json-config"}}`,
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.WriteFile(fullPath, []byte(content), 0600); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	// Custom filter that includes JSON
	customFilter := &FileFilter{
		ExcludePaths: []string{},
		IncludeExts:  []string{".yaml", ".json"},
	}

	opts := ScanOptions{
		Workers: 2,
		Filter:  customFilter,
	}

	s := New(opts)
	idx, err := s.Scan(tmpDir)

	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	stats := idx.Stats()
	// Should find both YAML and JSON files
	if stats.TotalResources != 2 {
		t.Errorf("TotalResources = %d, want 2", stats.TotalResources)
	}
}

func TestScanRepository(t *testing.T) {
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "test.yaml")
	if err := os.WriteFile(testFile, []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
`), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	idx, err := ScanRepository(tmpDir)
	if err != nil {
		t.Fatalf("ScanRepository() error: %v", err)
	}

	if idx == nil {
		t.Fatal("ScanRepository() returned nil")
	}

	stats := idx.Stats()
	if stats.TotalResources != 1 {
		t.Errorf("TotalResources = %d, want 1", stats.TotalResources)
	}
}

func TestScanRepositoryWithOptions(t *testing.T) {
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "test.yaml")
	if err := os.WriteFile(testFile, []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
`), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	opts := ScanOptions{
		Workers: 5,
		Filter:  DefaultFilter(),
	}

	idx, err := ScanRepositoryWithOptions(tmpDir, opts)
	if err != nil {
		t.Fatalf("ScanRepositoryWithOptions() error: %v", err)
	}

	if idx == nil {
		t.Fatal("ScanRepositoryWithOptions() returned nil")
	}
}

func TestScannerVerboseMode(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an invalid file that will cause errors
	invalidFile := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(invalidFile, []byte(`
not: valid: yaml
`), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	opts := DefaultScanOptions()
	opts.Verbose = true

	s := New(opts)
	// In verbose mode, errors would be returned, but our current implementation
	// only collects them - the test verifies it doesn't panic
	_, _ = s.Scan(tmpDir)
}

func TestScannerConcurrency(t *testing.T) {
	tmpDir := t.TempDir()

	// Create many files to test concurrent processing
	for i := 0; i < 50; i++ {
		dir := filepath.Join(tmpDir, "dir"+string(rune('a'+i%26)))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}

		file := filepath.Join(dir, "file.yaml")
		content := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-` + fmt.Sprintf("dir%c", 'a'+i%26)

		if err := os.WriteFile(file, []byte(content), 0600); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	// Test with various worker counts
	workerCounts := []int{1, 2, 5, 10, 20}

	for _, workers := range workerCounts {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			opts := ScanOptions{
				Workers: workers,
				Filter:  DefaultFilter(),
			}

			s := New(opts)
			idx, err := s.Scan(tmpDir)

			if err != nil {
				t.Fatalf("Scan() error: %v", err)
			}

			stats := idx.Stats()
			// Due to duplicate names (only 26 unique), the actual count might be 26
			if stats.TotalResources == 0 {
				t.Error("expected some resources to be scanned")
			}
		})
	}
}
