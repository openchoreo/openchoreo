// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package fsmode

import (
	"fmt"
	"sort"
	"sync"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/typed"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// OwnerRef represents OpenChoreo-specific owner reference information
type OwnerRef struct {
	ProjectName   string
	ComponentName string
	// ReleaseLine is the owner's release line, empty for the component's default line.
	ReleaseLine string
}

// ExtractOwnerRef extracts owner reference information from a resource entry
func ExtractOwnerRef(entry *index.ResourceEntry) *OwnerRef {
	if entry == nil || entry.Resource == nil {
		return nil
	}

	// For Component resources, componentName is in metadata.name
	// For ComponentRelease and ReleaseBinding, both are in spec.owner
	// For ReleaseLine resources, the release line is metadata.name
	kind := entry.Resource.GroupVersionKind().Kind

	ownerMap := entry.GetNestedMap("spec", "owner")
	if ownerMap == nil {
		return nil
	}

	projectName, _ := ownerMap["projectName"].(string)
	componentName, _ := ownerMap["componentName"].(string)
	releaseLine, _ := ownerMap["releaseLine"].(string)

	switch kind {
	case ComponentGVK.Kind:
		componentName = entry.Name()
		releaseLine = ""
	case ReleaseLineGVK.Kind:
		releaseLine = entry.Name()
	}

	if projectName == "" && componentName == "" {
		return nil
	}

	return &OwnerRef{
		ProjectName:   projectName,
		ComponentName: componentName,
		ReleaseLine:   releaseLine,
	}
}

// componentKey keys an index by component.
func componentKey(projectName, componentName string) string {
	return projectName + "/" + componentName
}

// lineKey keys an index by a component's release line, empty for its default line.
func lineKey(projectName, componentName, releaseLine string) string {
	return projectName + "/" + componentName + "/" + releaseLine
}

// Index wraps the generic index with OpenChoreo-specific functionality
type Index struct {
	*index.Index
	mu sync.RWMutex

	// OpenChoreo-specific indexes
	componentsByProject     map[string][]*index.ResourceEntry // projectName -> components
	workloadsByLine         map[string]*index.ResourceEntry   // "project/component/line" -> workload
	componentTypes          map[string]*index.ResourceEntry   // typeName -> componentType
	traits                  map[string]*index.ResourceEntry   // traitName -> trait
	clusterComponentTypes   map[string]*index.ResourceEntry   // typeName -> clusterComponentType
	clusterTraits           map[string]*index.ResourceEntry   // traitName -> clusterTrait
	releaseLinesByComponent map[string][]*index.ResourceEntry // "project/component" -> release lines
	releasesByLine          map[string][]*index.ResourceEntry // "project/component/line" -> releases
	releaseBindingsByEnv    map[string][]*index.ResourceEntry // "project/component/line/env" -> bindings
	deploymentPipelines     map[string]*index.ResourceEntry   // pipelineName -> pipeline
}

// WrapIndex wraps an existing generic index with OpenChoreo-specific functionality
func WrapIndex(idx *index.Index) *Index {
	ocIndex := &Index{
		Index:                   idx,
		componentsByProject:     make(map[string][]*index.ResourceEntry),
		workloadsByLine:         make(map[string]*index.ResourceEntry),
		componentTypes:          make(map[string]*index.ResourceEntry),
		traits:                  make(map[string]*index.ResourceEntry),
		clusterComponentTypes:   make(map[string]*index.ResourceEntry),
		clusterTraits:           make(map[string]*index.ResourceEntry),
		releaseLinesByComponent: make(map[string][]*index.ResourceEntry),
		releasesByLine:          make(map[string][]*index.ResourceEntry),
		releaseBindingsByEnv:    make(map[string][]*index.ResourceEntry),
		deploymentPipelines:     make(map[string]*index.ResourceEntry),
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
		// Index by release line
		owner := ExtractOwnerRef(entry)
		if owner != nil && owner.ProjectName != "" && owner.ComponentName != "" {
			idx.workloadsByLine[lineKey(owner.ProjectName, owner.ComponentName, owner.ReleaseLine)] = entry
		}

	case ReleaseLineGVK:
		// Index by component
		owner := ExtractOwnerRef(entry)
		if owner != nil && owner.ProjectName != "" && owner.ComponentName != "" {
			key := componentKey(owner.ProjectName, owner.ComponentName)
			idx.releaseLinesByComponent[key] = append(idx.releaseLinesByComponent[key], entry)
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

	case ClusterComponentTypeGVK:
		// Index by type name
		name := entry.Name()
		if name != "" {
			idx.clusterComponentTypes[name] = entry
		}

	case ClusterTraitGVK:
		// Index by trait name
		name := entry.Name()
		if name != "" {
			idx.clusterTraits[name] = entry
		}

	case ComponentReleaseGVK:
		// Index by release line
		owner := ExtractOwnerRef(entry)
		if owner != nil && owner.ProjectName != "" && owner.ComponentName != "" {
			key := lineKey(owner.ProjectName, owner.ComponentName, owner.ReleaseLine)
			idx.releasesByLine[key] = append(idx.releasesByLine[key], entry)
		}

	case ReleaseBindingGVK:
		// Index by release line and environment
		owner := ExtractOwnerRef(entry)
		envName := entry.GetNestedString("spec", "environment")
		if owner != nil && owner.ProjectName != "" && owner.ComponentName != "" && envName != "" {
			key := lineKey(owner.ProjectName, owner.ComponentName, owner.ReleaseLine) + "/" + envName
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
	idx.workloadsByLine = make(map[string]*index.ResourceEntry)
	idx.componentTypes = make(map[string]*index.ResourceEntry)
	idx.traits = make(map[string]*index.ResourceEntry)
	idx.clusterComponentTypes = make(map[string]*index.ResourceEntry)
	idx.clusterTraits = make(map[string]*index.ResourceEntry)
	idx.releaseLinesByComponent = make(map[string][]*index.ResourceEntry)
	idx.releasesByLine = make(map[string][]*index.ResourceEntry)
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

// GetClusterComponentType retrieves a cluster component type by name
func (idx *Index) GetClusterComponentType(name string) (*index.ResourceEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entry, ok := idx.clusterComponentTypes[name]
	return entry, ok
}

// GetClusterTrait retrieves a cluster trait by name
func (idx *Index) GetClusterTrait(name string) (*index.ResourceEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entry, ok := idx.clusterTraits[name]
	return entry, ok
}

// GetTypedClusterComponentType retrieves a cluster component type by name and returns a typed wrapper
func (idx *Index) GetTypedClusterComponentType(name string) (*typed.ClusterComponentType, error) {
	entry, ok := idx.GetClusterComponentType(name)
	if !ok {
		return nil, fmt.Errorf("cluster component type %q not found", name)
	}
	return typed.NewClusterComponentType(entry)
}

// GetTypedClusterTrait retrieves a cluster trait by name and returns a typed wrapper
func (idx *Index) GetTypedClusterTrait(name string) (*typed.ClusterTrait, error) {
	entry, ok := idx.GetClusterTrait(name)
	if !ok {
		return nil, fmt.Errorf("cluster trait %q not found", name)
	}
	return typed.NewClusterTrait(entry)
}

// GetWorkloadForComponent retrieves the workload of a component's release line, or of its default
// line when releaseLine is empty
func (idx *Index) GetWorkloadForComponent(projectName, componentName, releaseLine string) (*index.ResourceEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entry, ok := idx.workloadsByLine[lineKey(projectName, componentName, releaseLine)]
	return entry, ok
}

// GetReleaseLine retrieves a release line of a component
func (idx *Index) GetReleaseLine(projectName, componentName, releaseLine string) (*index.ResourceEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	for _, entry := range idx.releaseLinesByComponent[componentKey(projectName, componentName)] {
		if entry.Name() == releaseLine {
			return entry, true
		}
	}
	return nil, false
}

// GetTypedReleaseLine retrieves a release line of a component and returns it typed
func (idx *Index) GetTypedReleaseLine(projectName, componentName, releaseLine string) (*v1alpha1.ReleaseLine, error) {
	entry, ok := idx.GetReleaseLine(projectName, componentName, releaseLine)
	if !ok {
		return nil, fmt.Errorf("release line %q of component %q (project: %q) not found", releaseLine, componentName, projectName)
	}
	return typed.FromEntry[v1alpha1.ReleaseLine](entry)
}

// ListReleaseLinesForComponent returns the names of a component's release lines, sorted
func (idx *Index) ListReleaseLinesForComponent(projectName, componentName string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entries := idx.releaseLinesByComponent[componentKey(projectName, componentName)]
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

// ComponentLines returns the lines of a component to release: its default line, shown as "", then
// each release line. The default line is left out when it has no workload and the component has
// release lines, since the component then deploys only through them.
func (idx *Index) ComponentLines(projectName, componentName string) []string {
	lines := idx.ListReleaseLinesForComponent(projectName, componentName)
	if _, ok := idx.GetWorkloadForComponent(projectName, componentName, ""); !ok && len(lines) > 0 {
		return lines
	}
	return append([]string{""}, lines...)
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

// GetTypedWorkloadForComponent retrieves the workload of a component's release line, or of its default
// line when releaseLine is empty, and returns a typed wrapper
func (idx *Index) GetTypedWorkloadForComponent(projectName, componentName, releaseLine string) (*typed.Workload, error) {
	entry, ok := idx.GetWorkloadForComponent(projectName, componentName, releaseLine)
	if !ok {
		if releaseLine != "" {
			return nil, fmt.Errorf("workload for release line %q of component %q (project: %q) not found", releaseLine, componentName, projectName)
		}
		return nil, fmt.Errorf("workload for component %q (project: %q) not found", componentName, projectName)
	}
	return typed.NewWorkload(entry)
}

// ListReleasesForComponent returns the component releases of a component's release line, or of its
// default line when releaseLine is empty
func (idx *Index) ListReleasesForComponent(projectName, componentName, releaseLine string) []*index.ResourceEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.releasesByLine[lineKey(projectName, componentName, releaseLine)]
}

// GetProject retrieves a project by namespace and name
func (idx *Index) GetProject(namespace, name string) (*index.ResourceEntry, bool) {
	return idx.Index.Get(ProjectGVK, namespace, name)
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

// GetReleaseBindingForEnv retrieves a release binding of a component's release line, or of its default
// line when releaseLine is empty, in an environment
// Returns the first binding found for the given line and environment
func (idx *Index) GetReleaseBindingForEnv(projectName, componentName, releaseLine, envName string) (*index.ResourceEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	bindings := idx.releaseBindingsByEnv[lineKey(projectName, componentName, releaseLine)+"/"+envName]
	if len(bindings) == 0 {
		return nil, false
	}

	// Return the first binding (there should typically be only one per environment)
	return bindings[0], true
}
