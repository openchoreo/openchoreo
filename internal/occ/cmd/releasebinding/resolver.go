// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"path/filepath"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/output"
)

// buildBindingOutputDirResolver creates an OutputDirResolverFunc that resolves output directories
// for release bindings using the filesystem index. This is used when no release-config.yaml
// is present, allowing --all and --project operations to infer output directories.
//
// Resolution priority:
//  1. If existing bindings exist for the component in the index, use the same directory.
//  2. Otherwise, use a "release-bindings/" directory alongside the component file.
func buildBindingOutputDirResolver(ocIndex *fsmode.Index, namespace string) output.OutputDirResolverFunc {
	return func(projectName, componentName string) string {
		// Priority 1: Use directory of existing bindings
		allBindings := ocIndex.ListReleaseBindings()
		for _, entry := range allBindings {
			owner := fsmode.ExtractOwnerRef(entry)
			if owner != nil && owner.ProjectName == projectName && owner.ComponentName == componentName {
				return filepath.Dir(entry.FilePath)
			}
		}

		// Priority 2: Use "release-bindings/" next to the component file
		compEntry, ok := ocIndex.GetComponent(namespace, componentName)
		if !ok {
			return "" // fall through to hardcoded default
		}

		return filepath.Join(filepath.Dir(compEntry.FilePath), "release-bindings")
	}
}
