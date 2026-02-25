// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/output"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/typed"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
	"github.com/openchoreo/openchoreo/pkg/fsindex/cache"
)

// Workload implements workload operations
type Workload struct {
	config constants.CRDConfig
}

// New creates a new Workload with the default config
func New() *Workload {
	return &Workload{
		config: constants.WorkloadV1Config,
	}
}

// Create creates a workload from a descriptor or basic parameters
func (w *Workload) Create(params CreateParams) error {
	apiParams := toAPIParams(params)

	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceWorkload, apiParams); err != nil {
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
		return w.createFileSystemMode(params, apiParams)
	case flags.ModeAPIServer:
		return w.createAPIServerMode(apiParams)
	default:
		return fmt.Errorf("unsupported mode %q: must be %q or %q", mode, flags.ModeAPIServer, flags.ModeFileSystem)
	}
}

// createAPIServerMode handles the existing API server mode logic
func (w *Workload) createAPIServerMode(params api.CreateWorkloadParams) error {
	workloadRes, err := kinds.NewWorkloadResource(w.config, params.NamespaceName)
	if err != nil {
		return fmt.Errorf("failed to create Workload resource: %w", err)
	}

	if err := workloadRes.CreateWorkload(params); err != nil {
		return fmt.Errorf("failed to create workload from descriptor '%s': %w", params.FilePath, err)
	}

	return nil
}

// createFileSystemMode handles file-system mode for GitOps repos
func (w *Workload) createFileSystemMode(params CreateParams, apiParams api.CreateWorkloadParams) error {
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
		workloadRes, err := kinds.NewWorkloadResource(w.config, params.NamespaceName)
		if err != nil {
			return fmt.Errorf("failed to create Workload resource: %w", err)
		}

		workloadCR, err = workloadRes.GenerateWorkloadCR(apiParams)
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

// List lists all workloads in a namespace
func (w *Workload) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceWorkload, params); err != nil {
		return err
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListWorkloads(ctx, params.Namespace)
	if err != nil {
		return err
	}

	return printWorkloadList(result)
}

// Get retrieves a single workload and outputs it as YAML
func (w *Workload) Get(params GetParams) error {
	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceWorkload, params); err != nil {
		return err
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.GetWorkload(ctx, params.Namespace, params.WorkloadName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal workload to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single workload
func (w *Workload) Delete(params DeleteParams) error {
	if err := validation.ValidateParams(validation.CmdDelete, validation.ResourceWorkload, params); err != nil {
		return err
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if err := c.DeleteWorkload(ctx, params.Namespace, params.WorkloadName); err != nil {
		return err
	}

	fmt.Printf("Workload '%s' deleted\n", params.WorkloadName)
	return nil
}

// toAPIParams converts CreateParams to api.CreateWorkloadParams for downstream callers
func toAPIParams(p CreateParams) api.CreateWorkloadParams {
	return api.CreateWorkloadParams{
		FilePath:      p.FilePath,
		NamespaceName: p.NamespaceName,
		ProjectName:   p.ProjectName,
		ComponentName: p.ComponentName,
		ImageURL:      p.ImageURL,
		OutputPath:    p.OutputPath,
		DryRun:        p.DryRun,
		Mode:          p.Mode,
		RootDir:       p.RootDir,
	}
}

func printWorkloadList(list *gen.WorkloadList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No workloads found")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "NAME\tAGE")

	for _, wl := range list.Items {
		age := ""
		if wl.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*wl.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(tw, "%s\t%s\n",
			wl.Metadata.Name,
			age)
	}

	return tw.Flush()
}
