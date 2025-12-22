// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// FindLatestRelease finds the most recent release file for a given component
// Returns the release object and the file path, or nil if no existing release found
func FindLatestRelease(outputDir, componentName string) (*unstructured.Unstructured, string, error) {
	// Check if directory exists
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		return nil, "", nil // No existing releases
	}

	// Read all files in the directory
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read output directory: %w", err)
	}

	// Find all release files for this component
	var releaseFiles []string
	prefix := componentName + "-"
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".yaml") {
			releaseFiles = append(releaseFiles, name)
		}
	}

	if len(releaseFiles) == 0 {
		return nil, "", nil // No existing releases
	}

	// Sort by name (descending) to get the latest
	// Release names follow pattern: component-YYYYMMDD-version.yaml
	sort.Sort(sort.Reverse(sort.StringSlice(releaseFiles)))
	latestFile := releaseFiles[0]
	latestPath := filepath.Join(outputDir, latestFile)

	// Read and parse the latest release
	data, err := os.ReadFile(latestPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read latest release file %s: %w", latestPath, err)
	}

	var release unstructured.Unstructured
	if err := yaml.Unmarshal(data, &release.Object); err != nil {
		return nil, "", fmt.Errorf("failed to parse latest release file %s: %w", latestPath, err)
	}

	return &release, latestPath, nil
}

// GetNextVersionNumber finds the next version number for a component on a given date
// Returns "1" if no releases exist for that date, otherwise returns the incremented version
func GetNextVersionNumber(outputDir, componentName, dateStr string) (string, error) {
	// Check if directory exists
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		return "1", nil // No existing releases
	}

	// Read all files in the directory
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return "", fmt.Errorf("failed to read output directory: %w", err)
	}

	// Find all release files for this component and date
	prefix := fmt.Sprintf("%s-%s-", componentName, dateStr)
	var maxVersion int = 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Check if file matches the pattern: componentName-dateStr-version.yaml
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".yaml") {
			// Extract version number
			versionPart := strings.TrimPrefix(name, prefix)
			versionPart = strings.TrimSuffix(versionPart, ".yaml")

			// Parse version as integer
			if version, err := strconv.Atoi(versionPart); err == nil {
				if version > maxVersion {
					maxVersion = version
				}
			}
		}
	}

	// Return next version
	return strconv.Itoa(maxVersion + 1), nil
}
