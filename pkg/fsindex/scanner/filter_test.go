// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"testing"
)

func TestDefaultFilter(t *testing.T) {
	f := DefaultFilter()

	if f == nil {
		t.Fatal("DefaultFilter() returned nil")
	}

	// Check default exclude paths
	expectedExcludes := []string{".git", ".ocg", "node_modules", "vendor", ".terraform", "target", "build", "dist"}
	if len(f.ExcludePaths) != len(expectedExcludes) {
		t.Errorf("expected %d exclude paths, got %d", len(expectedExcludes), len(f.ExcludePaths))
	}

	// Check default extensions
	if len(f.IncludeExts) != 2 {
		t.Errorf("expected 2 include extensions, got %d", len(f.IncludeExts))
	}
}

func TestFileFilterShouldScan(t *testing.T) {
	f := DefaultFilter()

	tests := []struct {
		name string
		path string
		want bool
	}{
		// Should scan
		{"yaml file", "/repo/components/app.yaml", true},
		{"yml file", "/repo/workloads/service.yml", true},
		{"nested yaml", "/repo/deep/nested/path/resource.yaml", true},
		{"uppercase ext", "/repo/file.YAML", true},
		{"mixed case", "/repo/file.YaML", true},

		// Should not scan - wrong extension
		{"json file", "/repo/config.json", false},
		{"go file", "/repo/main.go", false},
		{"markdown", "/repo/README.md", false},
		{"no extension", "/repo/Makefile", false},

		// Should not scan - excluded directories
		{".git directory", "/repo/.git/config.yaml", false},
		{".ocg cache", "/repo/.ocg/index.yaml", false},
		{"node_modules", "/repo/node_modules/pkg/config.yaml", false},
		{"vendor", "/repo/vendor/k8s.io/file.yaml", false},
		{".terraform", "/repo/.terraform/state.yaml", false},
		{"target dir", "/repo/target/classes/app.yaml", false},
		{"build dir", "/repo/build/output.yaml", false},
		{"dist dir", "/repo/dist/bundle.yaml", false},

		// Edge cases
		{"git in name (not .git)", "/repo/git-config/app.yaml", true},
		{"vendor in name", "/repo/my-vendor-app/config.yaml", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.ShouldScan(tt.path)
			if got != tt.want {
				t.Errorf("ShouldScan(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestFileFilterShouldDescendIntoDir(t *testing.T) {
	f := DefaultFilter()

	tests := []struct {
		dirName string
		want    bool
	}{
		// Should descend
		{"src", true},
		{"components", true},
		{"manifests", true},
		{"apps", true},

		// Should not descend - excluded paths
		{".git", false},
		{".ocg", false},
		{"node_modules", false},
		{"vendor", false},
		{".terraform", false},
		{"target", false},
		{"build", false},
		{"dist", false},

		// Should not descend - hidden directories (start with .)
		{".hidden", false},
		{".cache", false},
		{".github", false},
		{".vscode", false},
	}

	for _, tt := range tests {
		t.Run(tt.dirName, func(t *testing.T) {
			got := f.ShouldDescendIntoDir(tt.dirName)
			if got != tt.want {
				t.Errorf("ShouldDescendIntoDir(%q) = %v, want %v", tt.dirName, got, tt.want)
			}
		})
	}
}

func TestCustomFileFilter(t *testing.T) {
	f := &FileFilter{
		ExcludePaths: []string{"custom-exclude"},
		IncludeExts:  []string{".json", ".yaml"},
	}

	// Should scan JSON with custom filter
	if !f.ShouldScan("/repo/config.json") {
		t.Error("custom filter should allow .json files")
	}

	// Should scan YAML
	if !f.ShouldScan("/repo/config.yaml") {
		t.Error("custom filter should allow .yaml files")
	}

	// Should not scan yml (not in custom include)
	if f.ShouldScan("/repo/config.yml") {
		t.Error("custom filter should not allow .yml files")
	}

	// Should exclude custom path
	if f.ShouldScan("/repo/custom-exclude/file.yaml") {
		t.Error("custom filter should exclude custom-exclude directory")
	}

	// Should not exclude default paths (not in custom exclude)
	if !f.ShouldScan("/repo/.git/hooks.yaml") {
		t.Error("custom filter should not exclude .git (not in custom excludes)")
	}
}

func TestIsYAMLFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/path/to/file.yaml", true},
		{"/path/to/file.yml", true},
		{"/path/to/file.YAML", true},
		{"/path/to/file.YML", true},
		{"/path/to/file.Yaml", true},
		{"/path/to/file.json", false},
		{"/path/to/file.go", false},
		{"/path/to/file", false},
		{"file.yaml", true},
		{".yaml", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := IsYAMLFile(tt.path)
			if got != tt.want {
				t.Errorf("IsYAMLFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestFileFilterEmptyExcludePaths(t *testing.T) {
	f := &FileFilter{
		ExcludePaths: []string{},
		IncludeExts:  []string{".yaml"},
	}

	// Should scan YAML files from non-hidden dirs
	if !f.ShouldScan("/repo/config.yaml") {
		t.Error("empty exclude should allow yaml files")
	}

	// Hidden directories are always skipped (regardless of exclude list)
	if f.ShouldDescendIntoDir(".git") {
		t.Error("should not descend into hidden directories")
	}

	if f.ShouldDescendIntoDir(".hidden") {
		t.Error("should not descend into hidden directories")
	}

	// Non-hidden dirs should be traversed with empty exclude
	if !f.ShouldDescendIntoDir("src") {
		t.Error("empty exclude should descend into non-hidden directories")
	}
}

func TestFileFilterEmptyIncludeExts(t *testing.T) {
	f := &FileFilter{
		ExcludePaths: []string{},
		IncludeExts:  []string{},
	}

	// Should not scan any files
	if f.ShouldScan("/repo/file.yaml") {
		t.Error("empty include exts should not scan any files")
	}

	if f.ShouldScan("/repo/file.json") {
		t.Error("empty include exts should not scan any files")
	}
}
