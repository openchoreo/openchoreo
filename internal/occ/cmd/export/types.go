// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import "time"

// Resource associates an API object or filtered document with its export identity.
// Namespace is empty for cluster-scoped objects, including Namespace objects.
type Resource struct {
	Kind      string
	Namespace string
	Name      string
	Object    any
}

// FetchResult contains selected resources, preserved history paths, and warnings.
type FetchResult struct {
	Resources []Resource
	// PreservedPaths are retained without being created or updated.
	PreservedPaths map[string]struct{}
	Warnings       []string
	// ReleaseCutoff is inclusive, or nil when exporting all history.
	ReleaseCutoff *time.Time
}

// SerializedFile is one manifest and its path relative to the export directory.
type SerializedFile struct {
	Path string
	Data []byte
}

// FileCounts records committed changes, excluding preserved historical files.
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
	Warnings      []string
}
