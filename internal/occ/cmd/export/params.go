// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

// Params defines parameters for exporting resources.
type Params struct {
	// OutputDir is the root directory where resource manifests are written.
	OutputDir string
	// AllReleases includes the entire release history.
	AllReleases bool
}

// Options configures resource collection, credential handling, and output.
type Options struct {
	OutputDir string
	// KeepPlaintext decides whether to retain a detected value. Nil removes values;
	// an error cancels the export before any files are written.
	KeepPlaintext func(Withheld) (bool, error)
	AllReleases   bool
}
