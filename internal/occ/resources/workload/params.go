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

	// SourceCommit, SourceBranch, and SourceRepository record the VCS commit this
	// image was built from. Optional: DF/CFR/MTTR compute without them; Delivery
	// Insights' Lead Time for Changes reports unavailable when absent.
	SourceCommit     string
	SourceBranch     string
	SourceRepository string
	// SourceAuthoredAt is the commit's author timestamp, RFC3339. This, not the
	// commit or build time, is what Lead Time for Changes measures from.
	SourceAuthoredAt string
}
