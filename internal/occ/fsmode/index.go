// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package fsmode

import (
	"fmt"
	"sync"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode/typed"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// OwnerRef represents OpenChoreo-specific owner reference information
type OwnerRef struct {
	ProjectName   string
	ComponentName string
}

// ExtractOwnerRef extracts owner reference information from a resource entry
func ExtractOwnerRef(entry *index.ResourceEntry) *OwnerRef {
	if entry == nil || entry.Resource == nil {
		return nil
	}

	// For Component resources, componentName is in metadata.name
	// For ComponentRelease and ReleaseBinding, both are in spec.owner
	kind := entry.Resource.GroupVersionKind().Kind

	ownerMap := entry.GetNestedMap("spec", "owner")
	if ownerMap == nil {
		return nil
	}

	projectName, _ := ownerMap["projectName"].(string)
	componentName, _ := ownerMap["componentName"].(string)

	// For Component resources, get componentName from metadata.name
	if kind == "Component" {
		componentName = entry.Name()
	}

	if projectName == "" && componentName == "" {
		return nil
	}

	return &OwnerRef{
		ProjectName:   projectName,
		ComponentName: componentName,
	}
}

// Index wraps the generic index with OpenChoreo-specific functionality
type Index struct {
	*index.Index
	mu sync.RWMutex

	// OpenChoreo-specific indexes
	componentsByProject  map[string][]*index.ResourceEntry // projectName -> components
	workloadsByComponent map[string]*index.ResourceEntry   // "project/component" -> workload
	componentTypes       map[string]*index.ResourceEntry   // typeName -> componentType
	traits               map[string]*index.ResourceEntry   // traitName -> trait
	releasesByComponent  map[string][]*index.ResourceEntry // "project/component" -> releases
	releaseBindingsByEnv map[string][]*index.ResourceEntry // "project/component/env" -> bindings
	deploymentPipelines  map[string]*index.ResourceEntry   // pipelineName -> pipeline
}

// WrapIndex wraps an existing generic index with OpenChoreo-specific functionality
func WrapIndex(idx *index.Index) *Index {
	ocIndex := &Index{
		Index:                idx,
		componentsByProject:  make(map[string][]*index.ResourceEntry),
		workloadsByComponent: make(map[string]*index.ResourceEntry),
		componentTypes:       make(map[string]*index.ResourceEntry),
		traits:               make(map[string]*index.ResourceEntry),
		releasesByComponent:  make(map[string][]*index.ResourceEntry),
		releaseBindingsByEnv: make(map[string][]*index.ResourceEntry),
		deploymentPipelines:  make(map[string]*index.ResourceEntry),
	}

	// Build OpenChoreo-specific indexes from existing resources
	ocIndex.rebuildSpecializedIndexes()

	return ocIndex
}

// addToSpecializedIndexesUnsafe adds entries without locking (caller must hold lock)
func (idx *Index) addToSpecializedIndexesUnsafe(entry *index.ResourceEntry) {
	gvk := entry.Resource.GroupVersionKind()

	switch gvk {
	case ComponentGVK:
		// Index by project
		projectName := entry.GetNestedString("spec", "owner", "projectName")
		if projectName != "" {
			idx.componentsByProject[projectName] = append(idx.componentsByProject[projectName], entry)
		}

	case WorkloadGVK:
		// Index by component
		owner := ExtractOwnerRef(entry)
		if owner != nil && owner.ProjectName != "" && owner.ComponentName != "" {
			key := fmt.Sprintf("%s/%s", owner.ProjectName, owner.ComponentName)
			idx.workloadsByComponent[key] = entry
		}

	case ComponentTypeGVK:
		// Index by type name
		name := entry.Name()
		if name != "" {
			idx.componentTypes[name] = entry
		}

	case TraitGVK:
		// Index by trait name
		name := entry.Name()
		if name != "" {
			idx.traits[name] = entry
		}

	case ComponentReleaseGVK:
		// Index by component
		owner := ExtractOwnerRef(entry)
		if owner != nil && owner.ProjectName != "" && owner.ComponentName != "" {
			key := fmt.Sprintf("%s/%s", owner.ProjectName, owner.ComponentName)
			idx.releasesByComponent[key] = append(idx.releasesByComponent[key], entry)
		}

	case ReleaseBindingGVK:
		// Index by component and environment
		owner := ExtractOwnerRef(entry)
		envName := entry.GetNestedString("spec", "environment")
		if owner != nil && owner.ProjectName != "" && owner.ComponentName != "" && envName != "" {
			key := fmt.Sprintf("%s/%s/%s", owner.ProjectName, owner.ComponentName, envName)
			idx.releaseBindingsByEnv[key] = append(idx.releaseBindingsByEnv[key], entry)
		}

	case DeploymentPipelineGVK:
		// Index by pipeline name
		name := entry.Name()
		if name != "" {
			idx.deploymentPipelines[name] = entry
		}
	}
}

// rebuildSpecializedIndexes rebuilds OpenChoreo-specific indexes from generic index
func (idx *Index) rebuildSpecializedIndexes() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Clear existing indexes
	idx.componentsByProject = make(map[string][]*index.ResourceEntry)
	idx.workloadsByComponent = make(map[string]*index.ResourceEntry)
	idx.componentTypes = make(map[string]*index.ResourceEntry)
	idx.traits = make(map[string]*index.ResourceEntry)
	idx.releasesByComponent = make(map[string][]*index.ResourceEntry)
	idx.releaseBindingsByEnv = make(map[string][]*index.ResourceEntry)
	idx.deploymentPipelines = make(map[string]*index.ResourceEntry)

	// Rebuild from all resources (using unsafe version since we hold the lock)
	for _, entry := range idx.Index.ListAll() {
		idx.addToSpecializedIndexesUnsafe(entry)
	}
}

// GetComponent retrieves a component by namespace and name
func (idx *Index) GetComponent(namespace, name string) (*index.ResourceEntry, bool) {
	return idx.Index.Get(ComponentGVK, namespace, name)
}

// GetComponentType retrieves a component type by name
func (idx *Index) GetComponentType(name string) (*index.ResourceEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entry, ok := idx.componentTypes[name]
	return entry, ok
}

// GetTrait retrieves a trait by name
func (idx *Index) GetTrait(name string) (*index.ResourceEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entry, ok := idx.traits[name]
	return entry, ok
}

// GetWorkloadForComponent retrieves the workload for a specific component
func (idx *Index) GetWorkloadForComponent(projectName, componentName string) (*index.ResourceEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	key := fmt.Sprintf("%s/%s", projectName, componentName)
	entry, ok := idx.workloadsByComponent[key]
	return entry, ok
}

// ListComponents returns all components
func (idx *Index) ListComponents() []*index.ResourceEntry {
	return idx.Index.List(ComponentGVK)
}

// ListComponentsForProject returns all components for a specific project
func (idx *Index) ListComponentsForProject(projectName string) []*index.ResourceEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.componentsByProject[projectName]
}

// ListReleases returns all component releases
func (idx *Index) ListReleases() []*index.ResourceEntry {
	return idx.Index.List(ComponentReleaseGVK)
}

// GetTypedComponent retrieves a component by namespace and name and returns a typed wrapper
func (idx *Index) GetTypedComponent(namespace, name string) (*typed.Component, error) {
	entry, ok := idx.GetComponent(namespace, name)
	if !ok {
		return nil, fmt.Errorf("component %q not found in namespace %q", name, namespace)
	}
	return typed.NewComponent(entry)
}

// GetTypedComponentType retrieves a component type by name and returns a typed wrapper
func (idx *Index) GetTypedComponentType(name string) (*typed.ComponentType, error) {
	entry, ok := idx.GetComponentType(name)
	if !ok {
		return nil, fmt.Errorf("component type %q not found", name)
	}
	return typed.NewComponentType(entry)
}

// GetTypedTrait retrieves a trait by name and returns a typed wrapper
func (idx *Index) GetTypedTrait(name string) (*typed.Trait, error) {
	entry, ok := idx.GetTrait(name)
	if !ok {
		return nil, fmt.Errorf("trait %q not found", name)
	}
	return typed.NewTrait(entry)
}

// GetTypedWorkloadForComponent retrieves the workload for a specific component and returns a typed wrapper
func (idx *Index) GetTypedWorkloadForComponent(projectName, componentName string) (*typed.Workload, error) {
	entry, ok := idx.GetWorkloadForComponent(projectName, componentName)
	if !ok {
		return nil, fmt.Errorf("workload for component %q (project: %q) not found", componentName, projectName)
	}
	return typed.NewWorkload(entry)
}

// ListReleasesForComponent returns all component releases for a specific component
func (idx *Index) ListReleasesForComponent(projectName, componentName string) []*index.ResourceEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	key := fmt.Sprintf("%s/%s", projectName, componentName)
	return idx.releasesByComponent[key]
}

// GetLatestReleaseForComponent returns the latest component release for a specific component
// Releases are sorted by name (which includes date and version), so the last one is the latest
func (idx *Index) GetLatestReleaseForComponent(projectName, componentName string) (*index.ResourceEntry, error) {
	releases := idx.ListReleasesForComponent(projectName, componentName)
	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found for component %s/%s", projectName, componentName)
	}

	// Since release names follow the format: <component>-<YYYYMMDD>-<version>
	// they are lexicographically sortable, and the latest release is the one with the highest name
	latestRelease := releases[0]
	for _, release := range releases[1:] {
		if release.Name() > latestRelease.Name() {
			latestRelease = release
		}
	}

	return latestRelease, nil
}

// GetDeploymentPipeline retrieves a deployment pipeline by name
func (idx *Index) GetDeploymentPipeline(name string) (*index.ResourceEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entry, ok := idx.deploymentPipelines[name]
	return entry, ok
}

// ListReleaseBindings returns all release bindings
func (idx *Index) ListReleaseBindings() []*index.ResourceEntry {
	return idx.Index.List(ReleaseBindingGVK)
}

// GetReleaseBindingForEnv retrieves a release binding for a specific component and environment
// Returns the first binding found for the given component and environment
func (idx *Index) GetReleaseBindingForEnv(projectName, componentName, envName string) (*index.ResourceEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	key := fmt.Sprintf("%s/%s/%s", projectName, componentName, envName)
	bindings := idx.releaseBindingsByEnv[key]
	if len(bindings) == 0 {
		return nil, false
	}

	// Return the first binding (there should typically be only one per environment)
	return bindings[0], true
}
