// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	typed2 "github.com/openchoreo/openchoreo/internal/occ/fsmode/typed"
)

// ReleaseGenerator generates ComponentRelease resources
type ReleaseGenerator struct {
	index *fsmode.Index
}

// NewReleaseGenerator creates a new release generator
func NewReleaseGenerator(index *fsmode.Index) *ReleaseGenerator {
	return &ReleaseGenerator{index: index}
}

// ReleaseOptions configures release generation
type ReleaseOptions struct {
	ComponentName string
	ProjectName   string
	Namespace     string
	ReleaseName   string    // Optional: custom release name (if empty, auto-generated from component, date, version)
	Version       string    // Optional: auto-generated if empty
	Date          time.Time // Optional: uses current date if zero
}

// GenerateRelease generates a ComponentRelease for the specified component
func (g *ReleaseGenerator) GenerateRelease(opts ReleaseOptions) (*unstructured.Unstructured, error) {
	// 1. Fetch Component
	comp, err := g.index.GetTypedComponent(opts.Namespace, opts.ComponentName)
	if err != nil {
		return nil, err
	}

	// Validate project name matches if specified
	if opts.ProjectName != "" && comp.ProjectName() != opts.ProjectName {
		return nil, fmt.Errorf("component %q belongs to project %q, not %q",
			opts.ComponentName, comp.ProjectName(), opts.ProjectName)
	}

	// 2. Fetch ComponentType
	typeName := comp.ComponentTypeName()
	ct, err := g.index.GetTypedComponentType(typeName)
	if err != nil {
		return nil, fmt.Errorf("component type %q not found (referenced by component %q): %w",
			typeName, opts.ComponentName, err)
	}

	// 3. Fetch Workload
	wl, err := g.index.GetTypedWorkloadForComponent(comp.ProjectName(), comp.Name)
	if err != nil {
		return nil, err
	}

	// 4. Fetch Traits referenced by Component
	traitRefs := comp.GetTraitRefs()
	traitsMap, profileTraits, err := g.buildTraitsData(traitRefs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch traits: %w", err)
	}

	// 5. Determine release name
	var releaseName string
	if opts.ReleaseName != "" {
		// Use custom release name if provided
		releaseName = opts.ReleaseName
	} else {
		// Auto-generate release name
		var err error
		releaseName, err = GenerateReleaseName(comp.Name, opts.Date, opts.Version, g.index)
		if err != nil {
			return nil, fmt.Errorf("failed to generate release name: %w", err)
		}
	}

	// 6. Build ComponentRelease
	release := g.buildRelease(releaseName, opts.Namespace, comp, ct, wl, traitsMap, profileTraits)

	return release, nil
}

// buildTraitsData fetches traits and builds both the traits map and profile traits
func (g *ReleaseGenerator) buildTraitsData(traitRefs []typed2.TraitRef) (
	map[string]interface{}, // traitsMap: traitName -> full TraitSpec
	[]interface{}, // profileTraits: trait references for componentProfile
	error,
) {
	if len(traitRefs) == 0 {
		return nil, nil, nil
	}

	// Collect unique trait names
	traitNames := make(map[string]bool)
	for _, ref := range traitRefs {
		traitNames[ref.Name] = true
	}

	// Fetch trait resources
	traitsMap := make(map[string]interface{})
	for name := range traitNames {
		t, err := g.index.GetTypedTrait(name)
		if err != nil {
			return nil, nil, err
		}
		traitsMap[name] = t.GetSpec() // Embed full TraitSpec
	}

	// Build profile traits (references with instance params)
	profileTraits := make([]interface{}, 0, len(traitRefs))
	for _, ref := range traitRefs {
		traitRef := map[string]interface{}{
			"name":         ref.Name,
			"instanceName": ref.InstanceName,
		}
		if ref.Parameters != nil {
			traitRef["parameters"] = ref.Parameters
		}
		profileTraits = append(profileTraits, traitRef)
	}

	return traitsMap, profileTraits, nil
}

// buildRelease constructs the ComponentRelease unstructured object
func (g *ReleaseGenerator) buildRelease(
	releaseName, namespace string,
	comp *typed2.Component,
	ct *typed2.ComponentType,
	wl *typed2.Workload,
	traitsMap map[string]interface{},
	profileTraits []interface{},
) *unstructured.Unstructured {
	spec := map[string]interface{}{
		"owner": map[string]interface{}{
			"componentName": comp.Name,
			"projectName":   comp.ProjectName(),
		},
		"componentType": map[string]interface{}{
			"workloadType": ct.WorkloadType(),
			"schema":       ct.GetSchema(),
			"resources":    ct.GetResources(),
		},
		"workload": map[string]interface{}{
			"container": wl.GetContainer(),
		},
	}

	// Build componentProfile only if there's content to add
	componentProfile := make(map[string]interface{})
	if params := comp.GetParameters(); len(params) > 0 {
		componentProfile["parameters"] = params
	}
	if len(profileTraits) > 0 {
		componentProfile["traits"] = profileTraits
	}
	// Only add componentProfile to spec if it has content
	if len(componentProfile) > 0 {
		spec["componentProfile"] = componentProfile
	}

	// Add traits map if present
	if len(traitsMap) > 0 {
		spec["traits"] = traitsMap
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "openchoreo.dev/v1alpha1",
			"kind":       "ComponentRelease",
			"metadata": map[string]interface{}{
				"name":      releaseName,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
}
