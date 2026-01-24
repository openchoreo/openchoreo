// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"fmt"
	"os"
	"path/filepath"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	synth "github.com/openchoreo/openchoreo/internal/occ/resources/workload"
)

// WorkloadWriteParams contains parameters for writing a workload in file-system mode
type WorkloadWriteParams struct {
	Namespace     string
	RepoPath      string
	ProjectName   string
	ComponentName string
	OutputPath    string                       // If specified, always use this path
	WorkloadCR    *openchoreov1alpha1.Workload // The generated workload CR
	DryRun        bool
}

// WorkloadWriter handles writing workloads in file-system mode
type WorkloadWriter struct {
	index *fsmode.Index
}

// NewWorkloadWriter creates a new WorkloadWriter
func NewWorkloadWriter(idx *fsmode.Index) *WorkloadWriter {
	return &WorkloadWriter{index: idx}
}

// WriteWorkload writes a workload to the appropriate location
func (w *WorkloadWriter) WriteWorkload(params WorkloadWriteParams) (string, error) {
	// Convert workload to YAML
	yamlContent, err := synth.ConvertWorkloadCRToYAML(params.WorkloadCR)
	if err != nil {
		return "", fmt.Errorf("failed to convert workload to YAML: %w", err)
	}

	// Determine output path
	outputPath := w.resolveOutputPath(params)

	// Handle dry-run mode
	if params.DryRun {
		fmt.Println(string(yamlContent))
		return outputPath, nil
	}

	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Write file
	if err := os.WriteFile(outputPath, yamlContent, 0600); err != nil {
		return "", fmt.Errorf("failed to write workload to %s: %w", outputPath, err)
	}

	return outputPath, nil
}

// resolveOutputPath determines where to write the workload
func (w *WorkloadWriter) resolveOutputPath(params WorkloadWriteParams) string {
	// Priority 1: Explicit output path from CLI flag
	if params.OutputPath != "" {
		if filepath.IsAbs(params.OutputPath) {
			return params.OutputPath
		}
		return filepath.Join(params.RepoPath, params.OutputPath)
	}

	// Priority 2: Check if workload already exists for this component
	existingWorkload, ok := w.index.GetWorkloadForComponent(params.ProjectName, params.ComponentName)
	if ok && existingWorkload != nil {
		// Replace existing workload file
		return existingWorkload.FilePath
	}

	workloadFileName := "workload.yaml"

	// Priority 3: Find component location and write alongside it
	component, ok := w.index.GetComponent(params.Namespace, params.ComponentName)
	if ok && component != nil {
		componentDir := filepath.Dir(component.FilePath)
		return filepath.Join(componentDir, workloadFileName)
	}

	// Priority 4: Default path structure
	return filepath.Join(
		params.RepoPath,
		"projects",
		params.ProjectName,
		"components",
		params.ComponentName,
		workloadFileName,
	)
}
