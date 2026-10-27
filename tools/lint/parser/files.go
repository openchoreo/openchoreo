// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"os"
	"path/filepath"
	"strings"
)

// CollectYAMLFiles returns the .yaml/.yml files under folder that declare an
// OpenChoreo apiVersion. When recursive is true it scans every subfolder;
// otherwise it only looks at the files directly inside folder. Unreadable and
// non-OpenChoreo YAML files are ignored.
func CollectYAMLFiles(folder string, recursive bool) ([]string, error) {
	var files []string
	if recursive {
		err := filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".yaml" || ext == ".yml" {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return filterOpenChoreoFiles(files), nil
	}

	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	for _, d := range entries {
		if d.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == ".yaml" || ext == ".yml" {
			files = append(files, filepath.Join(folder, d.Name()))
		}
	}
	return filterOpenChoreoFiles(files), nil
}

// filterOpenChoreoFiles keeps only the YAML files whose content declares an
// OpenChoreo apiVersion. Unreadable files are also dropped.
func filterOpenChoreoFiles(files []string) []string {
	kept := files[:0]
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if HasOpenChoreoAPIVersion(data) {
			kept = append(kept, f)
		}
	}
	return kept
}
