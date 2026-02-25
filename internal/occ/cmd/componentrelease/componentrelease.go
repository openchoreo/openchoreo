// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	occonfig "github.com/openchoreo/openchoreo/internal/occ/fsmode/config"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/generator"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/output"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/fsindex/cache"
)

const releaseConfigFileName = "release-config.yaml"

// ComponentRelease implements component release operations
type ComponentRelease struct{}

// New creates a new ComponentRelease
func New() *ComponentRelease {
	return &ComponentRelease{}
}

// List lists all component releases for a component
func (cr *ComponentRelease) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceComponentRelease, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListComponentReleases(ctx, params.Namespace, params.Project, params.Component)
	if err != nil {
		return err
	}

	return printComponentReleases(result)
}

// Generate implements the component-release generate command
func (cr *ComponentRelease) Generate(params GenerateParams) error {
	// 1. Determine mode from params (default to api-server)
	mode := params.Mode
	if mode == "" {
		mode = flags.ModeAPIServer
	}

	if mode != flags.ModeFileSystem {
		return fmt.Errorf("componentrelease generate only supports file-system mode; use --mode file-system (got %q)", mode)
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
	var ctx *config.Context
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

	// Wrap generic index with OpenChoreo-specific functionality
	ocIndex := fsmode.WrapIndex(persistentIndex.Index)

	// 3. Load release config (optional - when absent, output dirs are inferred from index)
	releaseConfig, err := cr.loadReleaseConfig(repoPath, false)
	if err != nil {
		return err
	}

	// 4. Create generator
	gen := generator.NewReleaseGenerator(ocIndex)

	// 5. Determine base directory and custom output path
	// baseDir is where the writer will use for default path resolution
	// customOutputPath is only set when user explicitly provides --output-path
	baseDir := repoPath
	customOutputPath := params.OutputPath

	// 6. Get namespace from context (same as namespace)
	namespace := ctx.Namespace
	if namespace == "" {
		return fmt.Errorf("namespace is required in context")
	}

	// 7. Build output directory resolver for when no release-config.yaml exists
	resolver := buildOutputDirResolver(ocIndex, namespace)

	// 8. Generate releases based on scope
	if params.All {
		return cr.generateAll(gen, namespace, baseDir, customOutputPath, params.DryRun, releaseConfig, resolver)
	}

	// Check for specific component first (requires project to be specified)
	if params.ComponentName != "" {
		if params.ProjectName == "" {
			return fmt.Errorf("project name is required when specifying --component")
		}
		return cr.generateForComponent(gen, params.ComponentName, params.ProjectName, namespace, baseDir, customOutputPath, params.ReleaseName, params.DryRun, releaseConfig)
	}

	// Project-only scope (all components in project)
	if params.ProjectName != "" {
		return cr.generateForProject(gen, params.ProjectName, namespace, baseDir, customOutputPath, params.DryRun, releaseConfig, resolver)
	}

	return nil
}

// Get retrieves a single component release and outputs it as YAML
func (cr *ComponentRelease) Get(params GetParams) error {
	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceComponentRelease, params); err != nil {
		return err
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.GetComponentRelease(ctx, params.Namespace, params.ComponentReleaseName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal component release to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// loadReleaseConfig loads the release-config.yaml file
// If requireForBulk is true and the file doesn't exist, returns an error
func (cr *ComponentRelease) loadReleaseConfig(repoPath string, requireForBulk bool) (*occonfig.ReleaseConfig, error) {
	configPath := filepath.Join(repoPath, releaseConfigFileName)

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if requireForBulk {
			return nil, fmt.Errorf("release-config.yaml not found in %s (required for --all or --project operations)", repoPath)
		}
		// File doesn't exist but not required
		return nil, nil
	}

	// Load the config
	releaseConfig, err := occonfig.LoadReleaseConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load release-config.yaml: %w", err)
	}

	return releaseConfig, nil
}

func (cr *ComponentRelease) generateAll(gen *generator.ReleaseGenerator, namespace, baseDir, customOutputPath string, dryRun bool, releaseConfig *occonfig.ReleaseConfig, resolver output.OutputDirResolverFunc) error {
	result, err := gen.GenerateBulkReleases(generator.BulkReleaseOptions{
		All:       true,
		Namespace: namespace,
	})
	if err != nil {
		return err
	}

	return cr.writeResults(result, baseDir, customOutputPath, dryRun, releaseConfig, resolver)
}

func (cr *ComponentRelease) generateForProject(gen *generator.ReleaseGenerator, project, namespace, baseDir, customOutputPath string, dryRun bool, releaseConfig *occonfig.ReleaseConfig, resolver output.OutputDirResolverFunc) error {
	result, err := gen.GenerateBulkReleases(generator.BulkReleaseOptions{
		ProjectName: project,
		Namespace:   namespace,
	})
	if err != nil {
		return err
	}

	return cr.writeResults(result, baseDir, customOutputPath, dryRun, releaseConfig, resolver)
}

func (cr *ComponentRelease) generateForComponent(gen *generator.ReleaseGenerator, component, project, namespace, baseDir, customOutputPath, customReleaseName string, dryRun bool, releaseConfig *occonfig.ReleaseConfig) error {
	release, err := gen.GenerateRelease(generator.ReleaseOptions{
		ComponentName: component,
		ProjectName:   project,
		Namespace:     namespace,
		ReleaseName:   customReleaseName,
	})
	if err != nil {
		return err
	}

	if dryRun {
		return cr.printYAML(release)
	}

	// Write to file
	writer := output.NewWriter(baseDir)

	// Determine output directory using config if available
	var componentOutputDir string
	if releaseConfig != nil {
		componentOutputDir = releaseConfig.GetReleaseOutputDir(project, component)
	}
	// If user provided --output-path, use it; otherwise use config or default
	if componentOutputDir == "" && customOutputPath != "" {
		componentOutputDir = customOutputPath
	}

	writeOpts := output.WriteOptions{
		DryRun:          false,
		OutputDir:       componentOutputDir,
		SkipIfUnchanged: true,
	}

	path, skipped, err := writer.WriteRelease(release, writeOpts)
	if err != nil {
		return fmt.Errorf("failed to write release: %w", err)
	}

	if skipped {
		fmt.Printf("Skipped (unchanged): %s\n", release.GetName())
	} else {
		fmt.Printf("Generated: %s\n", path)
	}

	return nil
}

func (cr *ComponentRelease) writeResults(result *generator.BulkReleaseResult, baseDir, customOutputPath string, dryRun bool, releaseConfig *occonfig.ReleaseConfig, resolver output.OutputDirResolverFunc) error {
	// Print errors first
	for _, e := range result.Errors {
		fmt.Fprintf(os.Stderr, "Error generating release for %s/%s: %v\n", e.ProjectName, e.ComponentName, e.Error)
	}

	// Write or print releases
	if dryRun {
		// Dry-run mode: print all releases to stdout
		for _, info := range result.Releases {
			fmt.Printf("# Release: %s (project: %s, component: %s)\n", info.ReleaseName, info.ProjectName, info.ComponentName)
			if err := cr.printYAML(info.Release); err != nil {
				return err
			}
			fmt.Println("---")
		}
	} else {
		// Use bulk write with config for proper output directory resolution
		releases := make([]*unstructured.Unstructured, 0, len(result.Releases))
		for _, info := range result.Releases {
			releases = append(releases, info.Release)
		}

		writer := output.NewWriter(baseDir)
		writeResult, err := writer.WriteBulkReleases(releases, output.BulkWriteOptions{
			Config:          releaseConfig,
			OutputDir:       customOutputPath,
			Resolver:        resolver,
			DryRun:          false,
			SkipIfUnchanged: true,
		})
		if err != nil {
			return fmt.Errorf("failed to write releases: %w", err)
		}

		// Print results
		for _, path := range writeResult.OutputPaths {
			fmt.Printf("Generated: %s\n", path)
		}
		for _, skipped := range writeResult.Skipped {
			fmt.Printf("Skipped (unchanged): %s\n", skipped)
		}
		if len(writeResult.Errors) > 0 {
			fmt.Fprintf(os.Stderr, "\nWrite errors:\n")
			for _, err := range writeResult.Errors {
				fmt.Fprintf(os.Stderr, "  - %v\n", err)
			}
		}

		// Print summary with skipped count
		fmt.Printf("\nSummary: %d releases generated, %d unchanged (skipped), %d errors\n",
			len(writeResult.OutputPaths), len(writeResult.Skipped), len(result.Errors)+len(writeResult.Errors))
		return nil
	}

	fmt.Printf("\nSummary: %d releases generated, %d errors\n", len(result.Releases), len(result.Errors))
	return nil
}

func (cr *ComponentRelease) printYAML(resource interface{}) error {
	data, err := yaml.Marshal(resource)
	if err != nil {
		return fmt.Errorf("failed to marshal to YAML: %w", err)
	}
	fmt.Print(string(data))
	return nil
}

func printComponentReleases(list *gen.ComponentReleaseList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No component releases found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tCOMPONENT\tAGE")

	for _, release := range list.Items {
		componentName := ""
		if release.Spec != nil {
			componentName = release.Spec.Owner.ComponentName
		}
		age := ""
		if release.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*release.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			release.Metadata.Name,
			componentName,
			age)
	}

	return w.Flush()
}
