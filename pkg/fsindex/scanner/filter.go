// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"path/filepath"
	"strings"
)

// FileFilter determines which files should be scanned
type FileFilter struct {
	ExcludePaths []string
	IncludeExts  []string
}

// DefaultFilter returns a filter with sensible defaults for GitOps repos
func DefaultFilter() *FileFilter {
	return &FileFilter{
		ExcludePaths: []string{
			".git",
			".ocg",
			"node_modules",
			"vendor",
			".terraform",
			"target",
			"build",
			"dist",
		},
		IncludeExts: []string{".yaml", ".yml"},
	}
}

// ShouldScan determines if a file should be scanned
func (f *FileFilter) ShouldScan(path string) bool {
	// Check if path contains any excluded directory
	for _, exclude := range f.ExcludePaths {
		if strings.Contains(path, "/"+exclude+"/") || strings.HasPrefix(path, exclude+"/") {
			return false
		}
	}

	// Check file extension
	ext := strings.ToLower(filepath.Ext(path))
	for _, allowedExt := range f.IncludeExts {
		if ext == allowedExt {
			return true
		}
	}

	return false
}

// ShouldDescendIntoDir determines if a directory should be traversed
func (f *FileFilter) ShouldDescendIntoDir(dirName string) bool {
	// Don't descend into hidden directories (starting with .)
	if strings.HasPrefix(dirName, ".") {
		return false
	}

	// Don't descend into excluded directories
	for _, exclude := range f.ExcludePaths {
		if dirName == exclude {
			return false
		}
	}
	return true
}

// IsYAMLFile checks if a file has a YAML extension
func IsYAMLFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}
