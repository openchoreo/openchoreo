// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"errors"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/output"
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
	existingBinding, exists := g.index.GetReleaseBindingForEnv(opts.Namespace, opts.ProjectName, opts.ComponentName, opts.TargetEnv)

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
	entry, exists := g.index.GetReleaseBindingForEnv(opts.Namespace, opts.ProjectName, opts.ComponentName, opts.TargetEnv)
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
		// Validate release exists in index, scoped to the active namespace
		releases := g.index.ListReleases()
		found := false
		for _, release := range releases {
			if release.Namespace() != opts.Namespace {
				continue
			}
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
		// Generate expected release spec from current component state
		releaseGen := NewReleaseGenerator(g.index)
		expectedRelease, err := releaseGen.GenerateRelease(ReleaseOptions{
			ComponentName: opts.ComponentName,
			ProjectName:   opts.ProjectName,
			Namespace:     opts.Namespace,
			ReleaseName:   "temp", // placeholder name, not used for spec comparison
		})
		if err != nil {
			return "", fmt.Errorf("failed to generate release spec for component %s/%s: %w",
				opts.ProjectName, opts.ComponentName, err)
		}

		// Find the existing release whose spec matches the current component state
		releases := g.index.ListReleasesForComponent(opts.Namespace, opts.ProjectName, opts.ComponentName)
		if len(releases) == 0 {
			return "", fmt.Errorf("no releases found for component %s/%s",
				opts.ProjectName, opts.ComponentName)
		}

		var compareErrs []error
		for _, release := range releases {
			match, err := output.CompareReleaseSpecs(expectedRelease, release.Resource)
			if err != nil {
				compareErrs = append(compareErrs, err)
				continue
			}
			if match {
				return release.Name(), nil
			}
		}

		if len(compareErrs) > 0 {
			return "", fmt.Errorf("comparing release specs for %s/%s: %w",
				opts.ProjectName, opts.ComponentName, errors.Join(compareErrs...))
		}

		// No release strict-matched the current component state. Before reporting a
		// generic miss, detect the common upgrade case: a release written by an occ
		// version that predates the full-spec ComponentRelease format. Such a release
		// can never strict-match once the component/ComponentType uses any of the
		// newer fields, so we compare each candidate against a legacy-shaped
		// projection of the expected spec and fail with an actionable error.
		legacyExpected := projectToLegacyReleaseShape(expectedRelease)
		for _, release := range releases {
			match, err := output.CompareReleaseSpecs(legacyExpected, release.Resource)
			if err != nil {
				continue
			}
			if match {
				return "", fmt.Errorf("release %q matches this component but was generated by an older occ "+
					"version and is missing newer spec fields; run \"occ componentrelease generate\" to cut an "+
					"updated release (the old release file can be deleted)", release.Name())
			}
		}

		return "", fmt.Errorf("no matching release found for current component state %s/%s",
			opts.ProjectName, opts.ComponentName)
	}

	// Non-root environment: get release from previous environment's binding
	prevEnv, err := opts.PipelineInfo.GetPreviousEnvironment(opts.TargetEnv)
	if err != nil {
		return "", err
	}

	// Find binding in previous environment
	prevBinding, ok := g.index.GetReleaseBindingForEnv(opts.Namespace, opts.ProjectName, opts.ComponentName, prevEnv)
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
		// Process all components in the active namespace
		allComponents := g.index.ListComponents()
		for _, comp := range allComponents {
			if comp.Namespace() != opts.Namespace {
				continue
			}
			owner := fsmode.ExtractOwnerRef(comp)
			if owner != nil {
				components = append(components, owner)
			}
		}
	} else if opts.ProjectName != "" {
		// Process all components in the specified project
		projectComponents := g.index.ListComponentsForProject(opts.Namespace, opts.ProjectName)
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
		entry, exists := g.index.GetReleaseBindingForEnv(opts.Namespace, owner.ProjectName, owner.ComponentName, opts.TargetEnv)

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

// projectToLegacyReleaseShape returns a deep copy of the expected release with the spec
// fields that pre-BuildSpec occ versions never emitted stripped out, so it can be compared
// against releases written by an older occ binary. The on-disk release is never mutated:
// only this projection of the freshly generated expected spec is reshaped.
//
// FROZEN LEGACY-COMPAT SHIM: this exists solely to turn an old-format on-disk release into
// a precise, actionable "regenerate" error during root-environment binding generation. It is
// slated for removal after the deprecation window; do not extend it or add version machinery.
func projectToLegacyReleaseShape(expected *unstructured.Unstructured) *unstructured.Unstructured {
	projected := expected.DeepCopy()

	// The old generator froze the ComponentType spec without these fields.
	for _, field := range []string{"allowedTraits", "allowedWorkflows", "validations", "preRenderValidations", "postRenderValidations"} {
		unstructured.RemoveNestedField(projected.Object, "spec", "componentType", "spec", field)
	}

	// The old generator emitted only component-level traits in spec.traits; embedded
	// (ComponentType) traits are exactly those absent from componentProfile.traits.
	stripEmbeddedTraits(projected.Object)

	// The old generator never emitted workload dependency resources.
	unstructured.RemoveNestedField(projected.Object, "spec", "workload", "dependencies", "resources")
	if deps, found, _ := unstructured.NestedMap(projected.Object, "spec", "workload", "dependencies"); found && len(deps) == 0 {
		unstructured.RemoveNestedField(projected.Object, "spec", "workload", "dependencies")
	}

	return projected
}

// stripEmbeddedTraits removes spec.traits entries that do not correspond to a
// component-level trait ref in spec.componentProfile.traits. componentProfile.traits lists
// exactly the component-level trait references (buildComponentProfile in
// internal/componentrelease/builder.go), and embedded traits never appear there, so any
// spec.traits entry missing from that set was contributed by the ComponentType.
func stripEmbeddedTraits(obj map[string]interface{}) {
	traits, found, _ := unstructured.NestedSlice(obj, "spec", "traits")
	if !found || len(traits) == 0 {
		return
	}

	componentTraitKeys := componentProfileTraitKeys(obj)

	kept := make([]interface{}, 0, len(traits))
	for _, t := range traits {
		tm, ok := t.(map[string]interface{})
		if !ok {
			kept = append(kept, t)
			continue
		}
		if _, isComponentLevel := componentTraitKeys[traitRefKey(tm)]; isComponentLevel {
			kept = append(kept, t)
		}
	}

	if len(kept) == 0 {
		unstructured.RemoveNestedField(obj, "spec", "traits")
		return
	}
	_ = unstructured.SetNestedSlice(obj, kept, "spec", "traits")
}

// componentProfileTraitKeys collects the kind/name keys of the component-level trait refs
// under spec.componentProfile.traits.
func componentProfileTraitKeys(obj map[string]interface{}) map[string]struct{} {
	keys := map[string]struct{}{}
	profileTraits, found, _ := unstructured.NestedSlice(obj, "spec", "componentProfile", "traits")
	if !found {
		return keys
	}
	for _, pt := range profileTraits {
		if ptm, ok := pt.(map[string]interface{}); ok {
			keys[traitRefKey(ptm)] = struct{}{}
		}
	}
	return keys
}

// traitRefKey builds a stable kind/name key for a trait entry, defaulting an absent kind to
// Trait to match the normalization BuildSpec applies to both spec.traits and componentProfile.
func traitRefKey(m map[string]interface{}) string {
	kind, _ := m["kind"].(string)
	if kind == "" {
		kind = string(v1alpha1.TraitRefKindTrait)
	}
	name, _ := m["name"].(string)
	return kind + "/" + name
}

// getNestedString is a helper to safely extract nested string values
func getNestedString(obj map[string]interface{}, fields ...string) string {
	val, found, err := unstructured.NestedString(obj, fields...)
	if err != nil || !found {
		return ""
	}
	return val
}
