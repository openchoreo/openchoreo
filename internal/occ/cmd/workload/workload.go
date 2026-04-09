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
	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/output"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/typed"
	"github.com/openchoreo/openchoreo/internal/occ/resources"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/resources/kinds"
	synth "github.com/openchoreo/openchoreo/internal/occ/resources/workload"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/pkg/fsindex/cache"
)

// Workload implements workload operations.
type Workload struct {
	config resources.CRDConfig
	client client.Interface
}

// New creates a new Workload with the default config.
func New(c client.Interface) *Workload {
	return &Workload{
		config: resources.WorkloadV1Config,
		client: c,
	}
}

// Create creates a workload from a descriptor or basic parameters.
func (w *Workload) Create(params CreateParams) error {
	if err := cmdutil.RequireFields("create", "workload", map[string]string{
		"namespace": params.NamespaceName,
		"project":   params.ProjectName,
		"component": params.ComponentName,
		"image":     params.ImageURL,
	}); err != nil {
		return err
	}

	mode := params.Mode
	if mode == "" {
		mode = flags.ModeAPIServer
	}

	synthParams := toSynthParams(params)

	switch mode {
	case flags.ModeFileSystem:
		return w.createFileSystemMode(params, synthParams)
	case flags.ModeAPIServer:
		return w.createAPIServerMode(synthParams)
	default:
		return fmt.Errorf("unsupported mode %q: must be %q or %q", mode, flags.ModeAPIServer, flags.ModeFileSystem)
	}
}

func (w *Workload) createAPIServerMode(params synth.CreateWorkloadParams) error {
	workloadRes, err := kinds.NewWorkloadResource(w.config, params.NamespaceName)
	if err != nil {
		return fmt.Errorf("failed to create Workload resource: %w", err)
	}

	if err := workloadRes.CreateWorkload(params); err != nil {
		return fmt.Errorf("failed to create workload from descriptor '%s': %w", params.FilePath, err)
	}

	return nil
}

func (w *Workload) createFileSystemMode(params CreateParams, synthParams synth.CreateWorkloadParams) error {
	repoPath := params.RootDir
	if repoPath == "" {
		var err error
		repoPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	fmt.Println("Loading index...")
	persistentIndex, err := cache.LoadOrBuild(repoPath)
	if err != nil {
		return fmt.Errorf("failed to build filesystem index: %w", err)
	}

	idx := fsmode.WrapIndex(persistentIndex.Index)

	var workloadCR *openchoreov1alpha1.Workload

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

	if workloadCR == nil {
		workloadRes, err := kinds.NewWorkloadResource(w.config, params.NamespaceName)
		if err != nil {
			return fmt.Errorf("failed to create Workload resource: %w", err)
		}

		workloadCR, err = workloadRes.GenerateWorkloadCR(synthParams)
		if err != nil {
			return fmt.Errorf("failed to generate workload: %w", err)
		}
	}

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

// List lists all workloads in a namespace.
func (w *Workload) List(params ListParams) error {
	if err := cmdutil.RequireFields("list", "workload", map[string]string{
		"namespace": params.Namespace,
	}); err != nil {
		return err
	}

	ctx := context.Background()
	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.Workload, string, error) {
		p := &gen.ListWorkloadsParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := w.client.ListWorkloads(ctx, params.Namespace, p)
		if err != nil {
			return nil, "", err
		}
		next := ""
		if result.Pagination.NextCursor != nil {
			next = *result.Pagination.NextCursor
		}
		return result.Items, next, nil
	})
	if err != nil {
		return err
	}

	return printWorkloadList(items)
}

// Get retrieves a single workload and outputs it as YAML.
func (w *Workload) Get(params GetParams) error {
	if err := cmdutil.RequireFields("get", "workload", map[string]string{
		"namespace": params.Namespace,
	}); err != nil {
		return err
	}

	ctx := context.Background()
	result, err := w.client.GetWorkload(ctx, params.Namespace, params.WorkloadName)
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

// Delete deletes a single workload.
func (w *Workload) Delete(params DeleteParams) error {
	if err := cmdutil.RequireFields("delete", "workload", map[string]string{
		"namespace": params.Namespace,
		"name":      params.WorkloadName,
	}); err != nil {
		return err
	}

	ctx := context.Background()
	if err := w.client.DeleteWorkload(ctx, params.Namespace, params.WorkloadName); err != nil {
		return err
	}

	fmt.Printf("Workload '%s' deleted\n", params.WorkloadName)
	return nil
}

// toSynthParams converts CreateParams to synth.CreateWorkloadParams.
func toSynthParams(p CreateParams) synth.CreateWorkloadParams {
	return synth.CreateWorkloadParams{
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

func printWorkloadList(items []gen.Workload) error {
	if len(items) == 0 {
		fmt.Println("No workloads found")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "NAME\tAGE")

	for _, wl := range items {
		age := ""
		if wl.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*wl.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(tw, "%s\t%s\n", wl.Metadata.Name, age)
	}

	return tw.Flush()
}
