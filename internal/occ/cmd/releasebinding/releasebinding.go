// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	printoutput "github.com/openchoreo/openchoreo/internal/occ/cmd/list/output"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	occonfig "github.com/openchoreo/openchoreo/internal/occ/fsmode/config"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/generator"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/output"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/pipeline"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	configContext "github.com/openchoreo/openchoreo/pkg/cli/cmd/config"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
	"github.com/openchoreo/openchoreo/pkg/fsindex/cache"
)

const (
	releaseConfigFileName = "release-config.yaml"
	actionCreated         = "Created"
	actionUpdated         = "Updated"
)

// ReleaseBindingImpl implements ReleaseBindingAPI
type ReleaseBindingImpl struct{}

// NewReleaseBindingImpl creates a new ReleaseBindingImpl
func NewReleaseBindingImpl() *ReleaseBindingImpl {
	return &ReleaseBindingImpl{}
}

// ListReleaseBindings lists all release bindings for a component
func (l *ReleaseBindingImpl) ListReleaseBindings(params api.ListReleaseBindingsParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceReleaseBinding, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListReleaseBindings(ctx, params.Namespace, params.Project, params.Component)
	if err != nil {
		return fmt.Errorf("failed to list release bindings: %w", err)
	}

	return printoutput.PrintReleaseBindings(result)
}

// GenerateReleaseBinding implements the release-binding generate command
func (r *ReleaseBindingImpl) GenerateReleaseBinding(params api.GenerateReleaseBindingParams) error {
	// 1. Determine mode from params (default to api-server)
	mode := params.Mode
	if mode == "" {
		mode = flags.ModeAPIServer
	}

	if mode != flags.ModeFileSystem {
		return fmt.Errorf("releasebinding generate only supports file-system mode; use --mode file-system (got %q)", mode)
	}

	// 2. Load context for other defaults (namespace, etc.)
	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.CurrentContext == "" {
		return fmt.Errorf("no current context set")
	}

	// Find current context
	var ctx *configContext.Context
	for _, c := range cfg.Contexts {
		if c.Name == cfg.CurrentContext {
			ctxCopy := c
			ctx = &ctxCopy
			break
		}
	}

	if ctx == nil {
		return fmt.Errorf("current context %q not found in config", cfg.CurrentContext)
	}

	repoPath := params.RootDir
	if repoPath == "" {
		var err error
		repoPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// 2. Load or build index
	fmt.Println("Loading index...")
	persistentIndex, err := cache.LoadOrBuild(repoPath)
	if err != nil {
		return fmt.Errorf("failed to build index: %w", err)
	}

	ocIndex := fsmode.WrapIndex(persistentIndex.Index)

	// 3. Get namespace from context
	namespace := ctx.Namespace
	if namespace == "" {
		return fmt.Errorf("namespace is required in context")
	}

	// 4. Load release config (optional - when absent, output dirs are inferred from index)
	releaseConfig, err := r.loadReleaseConfig(repoPath, false)
	if err != nil {
		return err
	}

	// 5. Derive pipeline from project if not specified
	if err := deriveUsePipeline(ocIndex, namespace, &params); err != nil {
		return err
	}

	// Get and validate deployment pipeline
	pipelineEntry, ok := ocIndex.GetDeploymentPipeline(params.UsePipeline)
	if !ok {
		return fmt.Errorf("deployment pipeline %q not found", params.UsePipeline)
	}

	pipelineInfo, err := pipeline.ParsePipeline(pipelineEntry.Resource)
	if err != nil {
		return fmt.Errorf("failed to parse deployment pipeline: %w", err)
	}

	// 6. Derive target environment from pipeline root if not specified
	if params.TargetEnv == "" {
		if params.All {
			return fmt.Errorf("--target-env is required when using --all")
		}
		params.TargetEnv = pipelineInfo.RootEnvironment
		fmt.Printf("No --target-env specified, using root environment: %s\n", params.TargetEnv)
	}

	// Validate target environment exists in pipeline
	if err := pipelineInfo.ValidateEnvironment(params.TargetEnv); err != nil {
		return fmt.Errorf("invalid target environment: %w", err)
	}

	// 7. Create generator
	gen := generator.NewBindingGenerator(ocIndex)
	baseDir := repoPath

	// 8. Build output directory resolver for when no release-config.yaml exists
	resolver := buildBindingOutputDirResolver(ocIndex, namespace)

	// 9. Generate bindings based on scope
	if params.All {
		return r.generateAll(gen, namespace, params.TargetEnv, pipelineInfo, baseDir, params.OutputPath, params.DryRun, releaseConfig, resolver)
	}

	if params.ComponentName != "" {
		// Single component mode
		return r.generateForComponent(gen, params, namespace, pipelineInfo, baseDir, releaseConfig, resolver)
	}

	// Project-only scope
	if params.ProjectName != "" {
		return r.generateForProject(gen, params.ProjectName, namespace, params.TargetEnv, pipelineInfo, baseDir, params.OutputPath, params.DryRun, releaseConfig, resolver)
	}

	return nil
}

// loadReleaseConfig loads the release-config.yaml file
func (r *ReleaseBindingImpl) loadReleaseConfig(repoPath string, requireForBulk bool) (*occonfig.ReleaseConfig, error) {
	configPath := filepath.Join(repoPath, releaseConfigFileName)

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if requireForBulk {
			return nil, fmt.Errorf("release-config.yaml not found in %s (required for --all or --project operations)", repoPath)
		}
		return nil, nil
	}

	// Load the config
	releaseConfig, err := occonfig.LoadReleaseConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load release-config.yaml: %w", err)
	}

	return releaseConfig, nil
}

func (r *ReleaseBindingImpl) generateAll(gen *generator.BindingGenerator, namespace, targetEnv string, pipelineInfo *pipeline.PipelineInfo, baseDir, customOutputPath string, dryRun bool, releaseConfig *occonfig.ReleaseConfig, resolver output.OutputDirResolverFunc) error {
	result, err := gen.GenerateBulkBindings(generator.BulkBindingOptions{
		All:          true,
		TargetEnv:    targetEnv,
		PipelineInfo: pipelineInfo,
		Namespace:    namespace,
	})
	if err != nil {
		return err
	}

	return r.writeResults(result, baseDir, customOutputPath, dryRun, releaseConfig, resolver)
}

func (r *ReleaseBindingImpl) generateForProject(gen *generator.BindingGenerator, project, namespace, targetEnv string, pipelineInfo *pipeline.PipelineInfo, baseDir, customOutputPath string, dryRun bool, releaseConfig *occonfig.ReleaseConfig, resolver output.OutputDirResolverFunc) error {
	result, err := gen.GenerateBulkBindings(generator.BulkBindingOptions{
		ProjectName:  project,
		TargetEnv:    targetEnv,
		PipelineInfo: pipelineInfo,
		Namespace:    namespace,
	})
	if err != nil {
		return err
	}

	return r.writeResults(result, baseDir, customOutputPath, dryRun, releaseConfig, resolver)
}

func (r *ReleaseBindingImpl) generateForComponent(gen *generator.BindingGenerator, params api.GenerateReleaseBindingParams, namespace string, pipelineInfo *pipeline.PipelineInfo, baseDir string, releaseConfig *occonfig.ReleaseConfig, resolver output.OutputDirResolverFunc) error {
	bindingInfo, err := gen.GenerateBindingWithInfo(generator.BindingOptions{
		ProjectName:      params.ProjectName,
		ComponentName:    params.ComponentName,
		ComponentRelease: params.ComponentRelease,
		TargetEnv:        params.TargetEnv,
		PipelineInfo:     pipelineInfo,
		Namespace:        namespace,
	})
	if err != nil {
		return err
	}

	if params.DryRun {
		return r.printYAML(bindingInfo.Binding)
	}

	// Write to file
	writer := output.NewWriter(baseDir)

	// Determine output directory
	var componentOutputDir string

	if bindingInfo.IsUpdate && bindingInfo.ExistingFilePath != "" {
		// UPDATE: write back to the original file location
		componentOutputDir = filepath.Dir(bindingInfo.ExistingFilePath)
	} else {
		// CREATE: --output-path → release-config → resolver → default
		if params.OutputPath != "" {
			componentOutputDir = params.OutputPath
		} else if releaseConfig != nil {
			componentOutputDir = releaseConfig.GetBindingOutputDir(params.ProjectName, params.ComponentName)
		}
		if componentOutputDir == "" && resolver != nil {
			componentOutputDir = resolver(params.ProjectName, params.ComponentName)
		}
	}

	writeOpts := output.WriteOptions{
		DryRun:    false,
		OutputDir: componentOutputDir,
	}

	path, _, err := writer.WriteBinding(bindingInfo.Binding, writeOpts)
	if err != nil {
		return fmt.Errorf("failed to write binding: %w", err)
	}

	action := actionCreated
	if bindingInfo.IsUpdate {
		action = actionUpdated
	}
	fmt.Printf("%s: %s\n", action, path)
	return nil
}

func (r *ReleaseBindingImpl) writeResults(result *generator.BulkBindingResult, baseDir, customOutputPath string, dryRun bool, releaseConfig *occonfig.ReleaseConfig, resolver output.OutputDirResolverFunc) error {
	// Print errors first
	for _, e := range result.Errors {
		fmt.Fprintf(os.Stderr, "Error generating binding for %s/%s: %v\n", e.ProjectName, e.ComponentName, e.Error)
	}

	// Write or print bindings
	if dryRun {
		// Dry-run mode: print all bindings to stdout
		for _, info := range result.Bindings {
			action := actionCreated
			if info.IsUpdate {
				action = actionUpdated
			}
			fmt.Printf("# %s binding: %s (project: %s, component: %s, release: %s)\n",
				action, info.BindingName, info.ProjectName, info.ComponentName, info.ReleaseName)
			if err := r.printYAML(info.Binding); err != nil {
				return err
			}
			fmt.Println("---")
		}
	} else {
		// Use bulk write with config for proper output directory resolution
		bindings := make([]*unstructured.Unstructured, 0, len(result.Bindings))
		existingPaths := make(map[string]string)
		for _, info := range result.Bindings {
			bindings = append(bindings, info.Binding)
			if info.IsUpdate && info.ExistingFilePath != "" {
				existingPaths[info.BindingName] = info.ExistingFilePath
			}
		}

		writer := output.NewWriter(baseDir)
		writeResult, err := writer.WriteBulkBindings(bindings, output.BulkBindingWriteOptions{
			Config:        releaseConfig,
			OutputDir:     customOutputPath,
			Resolver:      resolver,
			ExistingPaths: existingPaths,
			DryRun:        false,
		})
		if err != nil {
			return fmt.Errorf("failed to write bindings: %w", err)
		}

		// Build lookup map from binding name to IsUpdate
		updateMap := make(map[string]bool, len(result.Bindings))
		for _, info := range result.Bindings {
			updateMap[info.BindingName] = info.IsUpdate
		}

		// Print results
		for _, path := range writeResult.OutputPaths {
			bindingName := strings.TrimSuffix(filepath.Base(path), ".yaml")
			action := actionCreated
			if updateMap[bindingName] {
				action = actionUpdated
			}
			fmt.Printf("%s: %s\n", action, path)
		}
		if len(writeResult.Errors) > 0 {
			fmt.Fprintf(os.Stderr, "\nWrite errors:\n")
			for _, err := range writeResult.Errors {
				fmt.Fprintf(os.Stderr, "  - %v\n", err)
			}
		}

		// Print summary
		fmt.Printf("\nSummary: %d bindings generated, %d errors\n",
			len(writeResult.OutputPaths), len(result.Errors)+len(writeResult.Errors))
		return nil
	}

	fmt.Printf("\nSummary: %d bindings generated, %d errors\n", len(result.Bindings), len(result.Errors))
	return nil
}

// deriveUsePipeline resolves params.UsePipeline from the project's deploymentPipelineRef
// when UsePipeline is not explicitly set.
func deriveUsePipeline(ocIndex *fsmode.Index, namespace string, params *api.GenerateReleaseBindingParams) error {
	if params.UsePipeline == "" && params.All {
		return fmt.Errorf("--use-pipeline is required when using --all")
	}

	if params.UsePipeline == "" && params.ProjectName != "" {
		projectEntry, ok := ocIndex.GetProject(namespace, params.ProjectName)
		if !ok {
			return fmt.Errorf("project %q not found in namespace %q", params.ProjectName, namespace)
		}
		pipelineRef := projectEntry.GetNestedString("spec", "deploymentPipelineRef")
		if pipelineRef == "" {
			return fmt.Errorf("project %q has no deploymentPipelineRef set", params.ProjectName)
		}
		params.UsePipeline = pipelineRef
		fmt.Printf("No --use-pipeline specified, using project's deploymentPipelineRef: %s\n", pipelineRef)
	}

	if params.UsePipeline == "" {
		return fmt.Errorf("--use-pipeline is required (could not derive from project)")
	}

	return nil
}

func (r *ReleaseBindingImpl) printYAML(resource interface{}) error {
	data, err := yaml.Marshal(resource)
	if err != nil {
		return fmt.Errorf("failed to marshal to YAML: %w", err)
	}
	fmt.Print(string(data))
	return nil
}
