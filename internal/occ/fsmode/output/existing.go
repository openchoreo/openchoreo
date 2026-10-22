// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// ReleaseNameBase is the part of a generated release name before its date: the component name,
// followed by the release line for a line's release.
func ReleaseNameBase(componentName, releaseLine string) string {
	if releaseLine == "" {
		return componentName
	}
	return componentName + "-" + releaseLine
}

// releaseNameBaseOf returns the ReleaseNameBase of a release's owner.
func releaseNameBaseOf(release *unstructured.Unstructured) string {
	return ReleaseNameBase(getNestedString(release.Object, "spec", "owner", "componentName"),
		getNestedString(release.Object, "spec", "owner", "releaseLine"))
}

// FindLatestRelease finds the most recent release file named <base>-<YYYYMMDD>-<version>.yaml,
// where base is a ReleaseNameBase. A component's main line and each of its release lines have their own base.
// Returns the release object and the file path, or nil if no existing release found
func FindLatestRelease(outputDir, base string) (*unstructured.Unstructured, string, error) {
	// Check if directory exists
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		return nil, "", nil // No existing releases
	}

	// Read all files in the directory
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read output directory: %w", err)
	}

	// Find the release file with the latest date and version
	var latestFile, latestDate string
	latestVersion := -1
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		dateStr, version, ok := parseReleaseFileName(entry.Name(), base)
		if !ok {
			continue
		}
		if dateStr > latestDate || (dateStr == latestDate && version > latestVersion) {
			latestFile, latestDate, latestVersion = entry.Name(), dateStr, version
		}
	}

	if latestFile == "" {
		return nil, "", nil // No existing releases
	}

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

// parseReleaseFileName parses the date and version of a release file named <base>-<YYYYMMDD>-<version>.yaml.
func parseReleaseFileName(name, base string) (string, int, bool) {
	rest, ok := strings.CutPrefix(name, base+"-")
	if !ok {
		return "", 0, false
	}
	rest, ok = strings.CutSuffix(rest, ".yaml")
	if !ok {
		return "", 0, false
	}
	dateStr, versionStr, ok := strings.Cut(rest, "-")
	if !ok {
		return "", 0, false
	}
	if _, err := time.Parse("20060102", dateStr); err != nil {
		return "", 0, false
	}
	version, err := strconv.Atoi(versionStr)
	if err != nil {
		return "", 0, false
	}
	return dateStr, version, true
}

// GetNextVersionNumber finds the next version number for a ReleaseNameBase on a given date
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
	var maxVersion int

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
