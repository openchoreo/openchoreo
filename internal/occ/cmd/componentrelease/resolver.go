// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"os"
	"path/filepath"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/output"
)

// buildOutputDirResolver creates an OutputDirResolverFunc that resolves output directories
// for component releases using the filesystem index. This is used when no release-config.yaml
// is present, allowing --all and --project operations to infer output directories.
//
// Resolution priority:
//  1. If existing releases exist for the component in the index, use the same directory.
//  2. If no existing releases, use a "releases/" directory alongside the component file.
//  3. If "releases/" already exists at that location, use "releases-<componentName>/" to avoid conflicts.
func buildOutputDirResolver(ocIndex *fsmode.Index, namespace string) output.OutputDirResolverFunc {
	return func(projectName, componentName string) string {
		// Priority 1: Use directory of existing releases
		releases := ocIndex.ListReleasesForComponent(projectName, componentName)
		if len(releases) > 0 {
			return filepath.Dir(releases[0].FilePath)
		}

		// Look up the component to find its file path
		compEntry, ok := ocIndex.GetComponent(namespace, componentName)
		if !ok {
			return "" // fall through to hardcoded default
		}

		componentDir := filepath.Dir(compEntry.FilePath)
		releasesDir := filepath.Join(componentDir, "releases")

		// Priority 2: Use "releases/" next to the component file (if it doesn't already exist)
		if _, err := os.Stat(releasesDir); os.IsNotExist(err) {
			return releasesDir
		}

		// Priority 3: Use "releases-<componentName>/" to avoid conflicts
		return filepath.Join(componentDir, "releases-"+componentName)
	}
}
