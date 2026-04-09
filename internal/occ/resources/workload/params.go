// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package synth

// CreateWorkloadParams holds the parameters needed to create or generate a Workload CR.
type CreateWorkloadParams struct {
	FilePath      string
	NamespaceName string
	ProjectName   string
	ComponentName string
	ImageURL      string
	OutputPath    string
	DryRun        bool
	Mode          string // "api-server" or "file-system"
	RootDir       string
}
