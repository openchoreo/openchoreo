// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
)

// GenerateReleaseName generates a release name following the naming convention
// Format: <component-name>-<YYYYMMDD>-<version>
func GenerateReleaseName(componentName string, date time.Time, version string, index *fsmode.Index) (string, error) {
	if date.IsZero() {
		date = time.Now()
	}
	dateStr := date.Format("20060102")

	if version == "" {
		// Auto-detect version
		latestVersion := getLatestVersionForDate(componentName, dateStr, index)
		version = IncrementVersion(latestVersion)
	}

	return fmt.Sprintf("%s-%s-%s", componentName, dateStr, version), nil
}

// getLatestVersionForDate finds the latest version number for a component on a specific date.
// It returns "0" if no releases are found for that day.
func getLatestVersionForDate(componentName, dateStr string, index *fsmode.Index) string {
	// 1. Get all releases from the index
	releases := index.ListReleases()
	if len(releases) == 0 {
		return "0" // No releases found, start with version 0
	}

	// 2. Filter releases for the target component and date, then collect versions
	var versionsOnDate []int
	for _, releaseEntry := range releases {
		relCompName, relDateStr, relVersion, err := ParseReleaseName(releaseEntry.Name())
		if err != nil {
			// Skip entries that don't match the release name format
			continue
		}

		if relCompName == componentName && relDateStr == dateStr {
			v, err := strconv.Atoi(relVersion)
			if err == nil {
				versionsOnDate = append(versionsOnDate, v)
			}
		}
	}

	if len(versionsOnDate) == 0 {
		return "0" // No releases for this specific component and day
	}

	// 3. Find the highest version number
	sort.Ints(versionsOnDate)
	latestVersion := versionsOnDate[len(versionsOnDate)-1]

	return strconv.Itoa(latestVersion)
}

// ParseReleaseName extracts component name, date, and version from a release name
// Returns component name, date string, version, and error if parsing fails
func ParseReleaseName(releaseName string) (componentName, dateStr, version string, err error) {
	parts := strings.Split(releaseName, "-")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("invalid release name format: %s", releaseName)
	}

	version = parts[len(parts)-1]
	dateStr = parts[len(parts)-2]
	componentName = strings.Join(parts[:len(parts)-2], "-")

	// Validate parts
	if _, err := time.Parse("20060102", dateStr); err != nil {
		return "", "", "", fmt.Errorf("invalid date part in release name: %s", releaseName)
	}
	if _, err := strconv.Atoi(version); err != nil {
		return "", "", "", fmt.Errorf("invalid version part in release name: %s", releaseName)
	}

	return componentName, dateStr, version, nil
}

// IncrementVersion parses a version string and returns the next version
func IncrementVersion(version string) string {
	num, err := strconv.Atoi(version)
	if err != nil {
		// If the version is not a simple integer, handle appropriately.
		// For this context, we assume versions are simple integers.
		// A more robust implementation might handle semantic versioning.
		return "1" // Default to 1 if the existing version is not a number
	}
	return strconv.Itoa(num + 1)
}
