// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
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
//  2. Otherwise, use a "releases/" directory alongside the component file.
func buildOutputDirResolver(ocIndex *fsmode.Index, namespace string) output.OutputDirResolverFunc {
	return func(projectName, componentName string) string {
		// Priority 1: Use directory of existing releases
		releases := ocIndex.ListReleasesForComponent(projectName, componentName)
		if len(releases) > 0 {
			return filepath.Dir(releases[0].FilePath)
		}

		// Priority 2: Use "releases/" next to the component file
		compEntry, ok := ocIndex.GetComponent(namespace, componentName)
		if !ok {
			return "" // fall through to hardcoded default
		}

		return filepath.Join(filepath.Dir(compEntry.FilePath), "releases")
	}
}
