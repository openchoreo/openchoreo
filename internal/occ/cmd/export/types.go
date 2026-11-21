// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import "time"

// Resource associates an API object with its export identity. Namespace is empty
// for cluster-scoped objects, including Namespace objects. Fetch supplies a
// generated API value in Object; Filter replaces it with an independent JSON
// document containing the approved fields for export.
type Resource struct {
	Kind      string
	Namespace string
	Name      string
	Object    any
}

// FetchResult keeps selected API objects separate from paths deliberately left
// out by the release window. Only existing files at PreservedPaths are retained;
// no manifest is created or updated for those paths.
type FetchResult struct {
	Resources      []Resource
	PreservedPaths map[string]struct{}
	// ReleaseCutoff is the exact inclusive cutoff used for selection, or nil
	// when exporting the complete release history.
	ReleaseCutoff *time.Time
}

// SerializedFile is one manifest and its path relative to the export directory.
type SerializedFile struct {
	Path string
	Data []byte
}

// FileCounts describes successfully committed manifest changes. Historical
// files outside the selection window are excluded from these counts.
type FileCounts struct {
	Created   int
	Updated   int
	Unchanged int
	Deleted   int
}

// ExportResult supplies the committed export and its summary to the command.
type ExportResult struct {
	OutputDir     string
	Files         []SerializedFile
	Counts        FileCounts
	ReleaseCutoff *time.Time
}
