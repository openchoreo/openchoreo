// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"fmt"
	"os"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/output"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/typed"
	"github.com/openchoreo/openchoreo/internal/occ/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
	"github.com/openchoreo/openchoreo/pkg/fsindex/cache"
)

type WorkloadImpl struct {
	config constants.CRDConfig
}

func NewWorkloadImpl(config constants.CRDConfig) *WorkloadImpl {
	return &WorkloadImpl{
		config: config,
	}
}

func (i *WorkloadImpl) CreateWorkload(params api.CreateWorkloadParams) error {
	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceWorkload, params); err != nil {
		return err
	}

	// Determine mode from params (default to api-server)
	mode := params.Mode
	if mode == "" {
		mode = flags.ModeAPIServer
	}

	// Route to appropriate implementation based on mode
	switch mode {
	case flags.ModeFileSystem:
		return i.createWorkloadFileSystemMode(params)
	case flags.ModeAPIServer:
		return i.createWorkloadAPIServerMode(params)
	default:
		return fmt.Errorf("unsupported mode %q: must be %q or %q", mode, flags.ModeAPIServer, flags.ModeFileSystem)
	}
}

// createWorkloadAPIServerMode handles the existing API server mode logic
func (i *WorkloadImpl) createWorkloadAPIServerMode(params api.CreateWorkloadParams) error {
	workloadRes, err := kinds.NewWorkloadResource(i.config, params.NamespaceName)
	if err != nil {
		return fmt.Errorf("failed to create Workload resource: %w", err)
	}

	if err := workloadRes.CreateWorkload(params); err != nil {
		return fmt.Errorf("failed to create workload from descriptor '%s': %w", params.FilePath, err)
	}

	return nil
}

// createWorkloadFileSystemMode handles file-system mode for GitOps repos
func (i *WorkloadImpl) createWorkloadFileSystemMode(params api.CreateWorkloadParams) error {
	// Determine repo path
	repoPath := params.RootDir
	if repoPath == "" {
		var err error
		repoPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Load or build index
	fmt.Println("Loading index...")
	persistentIndex, err := cache.LoadOrBuild(repoPath)
	if err != nil {
		return fmt.Errorf("failed to build filesystem index: %w", err)
	}

	// Wrap generic index with OpenChoreo-specific functionality
	idx := fsmode.WrapIndex(persistentIndex.Index)

	var workloadCR *openchoreov1alpha1.Workload

	// If no descriptor provided, check if a workload already exists for this component.
	// If it does, update only the container image instead of replacing the entire workload.
	if params.FilePath == "" {
		if entry, ok := idx.GetWorkloadForComponent(params.ProjectName, params.ComponentName); ok {
			typedWorkload, err := typed.NewWorkload(entry)
			if err != nil {
				return fmt.Errorf("failed to read existing workload: %w", err)
			}
			existing := typedWorkload.Workload
			existing.Spec.Container.Image = params.ImageURL
			workloadCR = existing
		}
	}

	// If we don't have a workload CR yet (no existing workload or descriptor provided),
	// generate one from scratch.
	if workloadCR == nil {
		workloadRes, err := kinds.NewWorkloadResource(i.config, params.NamespaceName)
		if err != nil {
			return fmt.Errorf("failed to create Workload resource: %w", err)
		}

		workloadCR, err = workloadRes.GenerateWorkloadCR(params)
		if err != nil {
			return fmt.Errorf("failed to generate workload: %w", err)
		}
	}

	// Create writer and write workload
	writer := output.NewWorkloadWriter(idx)
	writtenPath, err := writer.WriteWorkload(output.WorkloadWriteParams{
		Namespace:     params.NamespaceName,
		RepoPath:      repoPath,
		ProjectName:   params.ProjectName,
		ComponentName: params.ComponentName,
		OutputPath:    params.OutputPath,
		WorkloadCR:    workloadCR,
		DryRun:        params.DryRun,
	})
	if err != nil {
		return err
	}

	if !params.DryRun {
		fmt.Printf("Workload written to: %s\n", writtenPath)
	}
	return nil
}
