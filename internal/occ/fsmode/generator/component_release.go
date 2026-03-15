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

const (
	traitKindTrait        = "Trait"
	traitKindClusterTrait = "ClusterTrait"
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
	traitsList, profileTraits, err := g.buildTraitsData(traitRefs)
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
	release := g.buildRelease(releaseName, opts.Namespace, comp, ct, wl, traitsList, profileTraits)

	return release, nil
}

// buildTraitsData fetches traits and builds both the traits list and profile traits
func (g *ReleaseGenerator) buildTraitsData(traitRefs []typed2.TraitRef) (
	[]interface{}, // traitsList: ComponentReleaseTrait entries with kind, name, spec
	[]interface{}, // profileTraits: trait references for componentProfile
	error,
) {
	if len(traitRefs) == 0 {
		return nil, nil, nil
	}

	// Collect unique traits by kind+name and fetch their specs
	type traitKey struct{ kind, name string }
	seen := make(map[traitKey]bool)
	traitsList := make([]interface{}, 0, len(traitRefs))
	for _, ref := range traitRefs {
		kind := ref.Kind
		if kind == "" {
			kind = traitKindTrait
		}

		key := traitKey{kind: kind, name: ref.Name}
		if seen[key] {
			continue
		}
		seen[key] = true

		var traitSpec map[string]interface{}
		switch kind {
		case traitKindTrait:
			t, err := g.index.GetTypedTrait(ref.Name)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to look up trait %s/%s: %w", kind, ref.Name, err)
			}
			traitSpec = t.GetSpec()
		case traitKindClusterTrait:
			return nil, nil, fmt.Errorf("ClusterTrait %q lookup is not yet supported in fs-mode index", ref.Name)
		default:
			return nil, nil, fmt.Errorf("unsupported trait kind %q for trait %q", kind, ref.Name)
		}
		traitsList = append(traitsList, map[string]interface{}{
			"kind": kind,
			"name": ref.Name,
			"spec": traitSpec,
		})
	}

	// Build profile traits (references with instance params)
	profileTraits := make([]interface{}, 0, len(traitRefs))
	for _, ref := range traitRefs {
		kind := ref.Kind
		if kind == "" {
			kind = traitKindTrait
		}
		traitRef := map[string]interface{}{
			"kind":         kind,
			"name":         ref.Name,
			"instanceName": ref.InstanceName,
		}
		if ref.Parameters != nil {
			traitRef["parameters"] = ref.Parameters
		}
		profileTraits = append(profileTraits, traitRef)
	}

	return traitsList, profileTraits, nil
}

// buildWorkloadData constructs the workload section for the ComponentRelease spec,
// including container, endpoints, and connections from the Workload resource.
func (g *ReleaseGenerator) buildWorkloadData(wl *typed2.Workload) map[string]interface{} {
	workloadMap := map[string]interface{}{
		"container": wl.GetContainer(),
	}
	if endpoints := wl.GetEndpoints(); len(endpoints) > 0 {
		workloadMap["endpoints"] = endpoints
	}
	if connections := wl.GetConnections(); len(connections) > 0 {
		workloadMap["dependencies"] = map[string]interface{}{
			"endpoints": connections,
		}
	}
	return workloadMap
}

// buildRelease constructs the ComponentRelease unstructured object
func (g *ReleaseGenerator) buildRelease(
	releaseName, namespace string,
	comp *typed2.Component,
	ct *typed2.ComponentType,
	wl *typed2.Workload,
	traitsList []interface{},
	profileTraits []interface{},
) *unstructured.Unstructured {
	// Build componentType.spec with workloadType, schema, and resources
	componentTypeSpec := map[string]interface{}{
		"workloadType": ct.WorkloadType(),
		"resources":    ct.GetResources(),
	}
	if schema := ct.GetSchema(); len(schema) > 0 {
		for k, v := range schema {
			componentTypeSpec[k] = v
		}
	}

	// Determine the kind from the component's componentType reference
	ctKind := string(comp.Spec.ComponentType.Kind)
	if ctKind == "" {
		ctKind = "ComponentType"
	}

	spec := map[string]interface{}{
		"owner": map[string]interface{}{
			"componentName": comp.Name,
			"projectName":   comp.ProjectName(),
		},
		"componentType": map[string]interface{}{
			"kind": ctKind,
			"name": comp.Spec.ComponentType.Name,
			"spec": componentTypeSpec,
		},
		"workload": g.buildWorkloadData(wl),
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

	// Add traits list if present
	if len(traitsList) > 0 {
		spec["traits"] = traitsList
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
