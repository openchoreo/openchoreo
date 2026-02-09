// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"fmt"
	"os"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/output"
	"github.com/openchoreo/openchoreo/internal/occ/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	configContext "github.com/openchoreo/openchoreo/pkg/cli/cmd/config"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
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

	// Load config to check current mode
	cfg, err := config.LoadStoredConfig()
	if err != nil {
		// If no config, default to API server mode (existing behavior)
		return i.createWorkloadAPIServerMode(params)
	}

	// Get current context
	var ctx *configContext.Context
	if cfg.CurrentContext != "" {
		for _, c := range cfg.Contexts {
			if c.Name == cfg.CurrentContext {
				ctxCopy := c
				ctx = &ctxCopy
				break
			}
		}
	}

	// Route to appropriate implementation based on mode
	if ctx != nil && ctx.Mode == configContext.ModeFileSystem {
		return i.createWorkloadFileSystemMode(ctx, params)
	}

	// Default: API server mode (existing implementation)
	return i.createWorkloadAPIServerMode(params)
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
func (i *WorkloadImpl) createWorkloadFileSystemMode(ctx *configContext.Context, params api.CreateWorkloadParams) error {
	// Determine repo path
	repoPath := ctx.RootDirectoryPath
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

	// Generate workload CR using existing logic
	workloadRes, err := kinds.NewWorkloadResource(i.config, params.NamespaceName)
	if err != nil {
		return fmt.Errorf("failed to create Workload resource: %w", err)
	}

	workloadCR, err := workloadRes.GenerateWorkloadCR(params)
	if err != nil {
		return fmt.Errorf("failed to generate workload: %w", err)
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
