// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apply

// Params defines parameters for applying configuration files.
type Params struct {
	FilePath string
	DryRun   bool
}

// GetFilePath returns the file path.
func (p Params) GetFilePath() string { return p.FilePath }

// GetDryRun returns whether dry-run mode is enabled.
func (p Params) GetDryRun() bool { return p.DryRun }
