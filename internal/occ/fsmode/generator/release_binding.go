// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/pipeline"
)

// BindingGenerator generates ReleaseBinding resources
type BindingGenerator struct {
	index *fsmode.Index
}

// NewBindingGenerator creates a new binding generator
func NewBindingGenerator(index *fsmode.Index) *BindingGenerator {
	return &BindingGenerator{index: index}
}

// BindingOptions defines options for generating a single binding
type BindingOptions struct {
	ProjectName      string
	ComponentName    string
	ComponentRelease string // If empty, auto-select based on environment position
	TargetEnv        string
	PipelineInfo     *pipeline.PipelineInfo
	Namespace        string
}

// BulkBindingOptions defines options for bulk binding generation
type BulkBindingOptions struct {
	All          bool
	ProjectName  string
	TargetEnv    string
	PipelineInfo *pipeline.PipelineInfo
	Namespace    string
}

// BulkBindingResult contains the results of bulk binding generation
type BulkBindingResult struct {
	Bindings []BindingInfo
	Errors   []BindingError
}

// BindingInfo contains information about a generated binding
type BindingInfo struct {
	BindingName      string
	ProjectName      string
	ComponentName    string
	ReleaseName      string
	Environment      string
	Binding          *unstructured.Unstructured
	IsUpdate         bool   // true if updating existing binding, false if creating new
	ExistingFilePath string // original file path when IsUpdate is true
}

// BindingError contains error information for a failed binding
type BindingError struct {
	ProjectName   string
	ComponentName string
	Error         error
}

// GenerateBinding generates a single ReleaseBinding
func (g *BindingGenerator) GenerateBinding(opts BindingOptions) (*unstructured.Unstructured, error) {
	// 1. Validate options
	if opts.ProjectName == "" {
		return nil, fmt.Errorf("project name is required")
	}
	if opts.ComponentName == "" {
		return nil, fmt.Errorf("component name is required")
	}
	if opts.TargetEnv == "" {
		return nil, fmt.Errorf("target environment is required")
	}
	if opts.PipelineInfo == nil {
		return nil, fmt.Errorf("pipeline info is required")
	}
	if opts.Namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	// 2. Validate target environment exists in pipeline
	if err := opts.PipelineInfo.ValidateEnvironment(opts.TargetEnv); err != nil {
		return nil, err
	}

	// 3. Select component release
	releaseName, err := g.selectComponentRelease(opts)
	if err != nil {
		return nil, err
	}

	// 4. Check if binding already exists
	existingBinding, exists := g.index.GetReleaseBindingForEnv(opts.ProjectName, opts.ComponentName, opts.TargetEnv)

	if exists {
		// UPDATE mode: Read the original file from disk to preserve all fields,
		// then only update releaseName.
		updated, err := readBindingFromFile(existingBinding.FilePath)
		if err != nil {
			return nil, err
		}
		if err := unstructured.SetNestedField(updated.Object, releaseName, "spec", "releaseName"); err != nil {
			return nil, fmt.Errorf("failed to update releaseName in existing binding: %w", err)
		}
		return updated, nil
	}

	// CREATE mode: Generate minimal new binding
	return g.buildMinimalBinding(opts.ProjectName, opts.ComponentName, releaseName, opts.TargetEnv, opts.Namespace), nil
}

// GenerateBindingWithInfo generates a single ReleaseBinding and returns rich info
// including whether this is an update and the existing file path.
func (g *BindingGenerator) GenerateBindingWithInfo(opts BindingOptions) (*BindingInfo, error) {
	binding, err := g.GenerateBinding(opts)
	if err != nil {
		return nil, fmt.Errorf("generate binding: %w", err)
	}

	bindingName := binding.GetName()
	releaseName := getNestedString(binding.Object, "spec", "releaseName")

	info := &BindingInfo{
		BindingName:   bindingName,
		ProjectName:   opts.ProjectName,
		ComponentName: opts.ComponentName,
		ReleaseName:   releaseName,
		Environment:   opts.TargetEnv,
		Binding:       binding,
	}

	// Check if this is an update and capture existing file path
	entry, exists := g.index.GetReleaseBindingForEnv(opts.ProjectName, opts.ComponentName, opts.TargetEnv)
	if exists {
		info.IsUpdate = true
		info.ExistingFilePath = entry.FilePath
	}

	return info, nil
}

// selectComponentRelease determines which ComponentRelease to use based on:
// 1. If explicit release name provided -> use it (after validation)
// 2. If target env is root environment -> use latest release for component
// 3. If target env is non-root -> find binding in previous environment and use its release
func (g *BindingGenerator) selectComponentRelease(opts BindingOptions) (string, error) {
	// If explicit release provided, validate it exists
	if opts.ComponentRelease != "" {
		// Validate release exists in index
		releases := g.index.ListReleases()
		found := false
		for _, release := range releases {
			if release.Name() == opts.ComponentRelease {
				// Verify it belongs to the specified component
				owner := fsmode.ExtractOwnerRef(release)
				if owner != nil && owner.ProjectName == opts.ProjectName && owner.ComponentName == opts.ComponentName {
					found = true
					break
				}
			}
		}
		if !found {
			return "", fmt.Errorf("component release %q not found or does not belong to component %s/%s",
				opts.ComponentRelease, opts.ProjectName, opts.ComponentName)
		}
		return opts.ComponentRelease, nil
	}

	// Check if target env is root environment
	if opts.PipelineInfo.IsRootEnvironment(opts.TargetEnv) {
		// Use latest release for this component
		release, err := g.index.GetLatestReleaseForComponent(opts.ProjectName, opts.ComponentName)
		if err != nil {
			return "", fmt.Errorf("failed to find latest release for component %s/%s: %w",
				opts.ProjectName, opts.ComponentName, err)
		}
		return release.Name(), nil
	}

	// Non-root environment: get release from previous environment's binding
	prevEnv, err := opts.PipelineInfo.GetPreviousEnvironment(opts.TargetEnv)
	if err != nil {
		return "", err
	}

	// Find binding in previous environment
	prevBinding, ok := g.index.GetReleaseBindingForEnv(opts.ProjectName, opts.ComponentName, prevEnv)
	if !ok {
		return "", fmt.Errorf("no ReleaseBinding found in previous environment %q for component %s/%s; "+
			"create binding in %q first or specify --component-release explicitly",
			prevEnv, opts.ProjectName, opts.ComponentName, prevEnv)
	}

	// Extract release name from previous binding
	releaseName := prevBinding.GetNestedString("spec", "releaseName")
	if releaseName == "" {
		return "", fmt.Errorf("ReleaseBinding in environment %q has no releaseName set", prevEnv)
	}

	return releaseName, nil
}

// buildMinimalBinding creates a new ReleaseBinding with only essential fields
func (g *BindingGenerator) buildMinimalBinding(
	projectName, componentName, releaseName, envName, namespace string,
) *unstructured.Unstructured {
	bindingName := fmt.Sprintf("%s-%s", componentName, envName)

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "openchoreo.dev/v1alpha1",
			"kind":       "ReleaseBinding",
			"metadata": map[string]interface{}{
				"name":      bindingName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"owner": map[string]interface{}{
					"projectName":   projectName,
					"componentName": componentName,
				},
				"environment": envName,
				"releaseName": releaseName,
			},
		},
	}
}

// GenerateBulkBindings generates bindings for multiple components
func (g *BindingGenerator) GenerateBulkBindings(opts BulkBindingOptions) (*BulkBindingResult, error) {
	result := &BulkBindingResult{
		Bindings: []BindingInfo{},
		Errors:   []BindingError{},
	}

	// Validate options
	if opts.TargetEnv == "" {
		return nil, fmt.Errorf("target environment is required")
	}
	if opts.PipelineInfo == nil {
		return nil, fmt.Errorf("pipeline info is required")
	}
	if opts.Namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	// Validate target environment
	if err := opts.PipelineInfo.ValidateEnvironment(opts.TargetEnv); err != nil {
		return nil, err
	}

	// Determine which components to process
	var components []*fsmode.OwnerRef

	if opts.All {
		// Process all components in the repository
		allComponents := g.index.ListComponents()
		for _, comp := range allComponents {
			owner := fsmode.ExtractOwnerRef(comp)
			if owner != nil {
				components = append(components, owner)
			}
		}
	} else if opts.ProjectName != "" {
		// Process all components in the specified project
		projectComponents := g.index.ListComponentsForProject(opts.ProjectName)
		for _, comp := range projectComponents {
			owner := fsmode.ExtractOwnerRef(comp)
			if owner != nil {
				components = append(components, owner)
			}
		}
	} else {
		return nil, fmt.Errorf("either All or ProjectName must be specified")
	}

	// Generate bindings for each component
	for _, owner := range components {
		bindingOpts := BindingOptions{
			ProjectName:      owner.ProjectName,
			ComponentName:    owner.ComponentName,
			ComponentRelease: "", // Auto-select based on environment
			TargetEnv:        opts.TargetEnv,
			PipelineInfo:     opts.PipelineInfo,
			Namespace:        opts.Namespace,
		}

		binding, err := g.GenerateBinding(bindingOpts)
		if err != nil {
			result.Errors = append(result.Errors, BindingError{
				ProjectName:   owner.ProjectName,
				ComponentName: owner.ComponentName,
				Error:         err,
			})
			continue
		}

		// Check if this is an update or create
		entry, exists := g.index.GetReleaseBindingForEnv(owner.ProjectName, owner.ComponentName, opts.TargetEnv)

		bindingName := binding.GetName()
		releaseName := getNestedString(binding.Object, "spec", "releaseName")

		info := BindingInfo{
			BindingName:   bindingName,
			ProjectName:   owner.ProjectName,
			ComponentName: owner.ComponentName,
			ReleaseName:   releaseName,
			Environment:   opts.TargetEnv,
			Binding:       binding,
			IsUpdate:      exists,
		}
		if exists {
			info.ExistingFilePath = entry.FilePath
		}

		result.Bindings = append(result.Bindings, info)
	}

	return result, nil
}

// readBindingFromFile reads a ReleaseBinding YAML file from disk and returns it
// as an unstructured object, preserving all fields from the original file.
func readBindingFromFile(filePath string) (*unstructured.Unstructured, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read existing binding file %s: %w", filePath, err)
	}

	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(data, &obj.Object); err != nil {
		return nil, fmt.Errorf("failed to parse existing binding file %s: %w", filePath, err)
	}

	return obj, nil
}

// getNestedString is a helper to safely extract nested string values
func getNestedString(obj map[string]interface{}, fields ...string) string {
	val, found, err := unstructured.NestedString(obj, fields...)
	if err != nil || !found {
		return ""
	}
	return val
}
