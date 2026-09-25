// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package fsmode

import (
	"fmt"
	"strings"
	"sync"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode/typed"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// nsKey builds a lookup key by joining namespace-qualified parts with "/".
// fsmode is GitOps-only and requires every namespace-scoped resource to set
// metadata.namespace explicitly, so callers always pass a concrete namespace as
// the leading part. A resource that omits its namespace keys under a leading
// "/" and simply never matches a real lookup, which is the intended outcome for
// such invalid input.
func nsKey(parts ...string) string {
	return strings.Join(parts, "/")
}

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

	// OpenChoreo-specific indexes.
	// Namespace-scoped indexes are keyed with a leading namespace segment so that
	// resources sharing a name across namespaces do not collide.
	componentsByProject   map[string][]*index.ResourceEntry // "namespace/project" -> components
	workloadsByComponent  map[string]*index.ResourceEntry   // "namespace/project/component" -> workload
	componentTypes        map[string]*index.ResourceEntry   // "namespace/typeName" -> componentType
	traits                map[string]*index.ResourceEntry   // "namespace/traitName" -> trait
	clusterComponentTypes map[string]*index.ResourceEntry   // typeName -> clusterComponentType (cluster-scoped)
	clusterTraits         map[string]*index.ResourceEntry   // traitName -> clusterTrait (cluster-scoped)
	releasesByComponent   map[string][]*index.ResourceEntry // "namespace/project/component" -> releases
	releaseBindingsByEnv  map[string][]*index.ResourceEntry // "namespace/project/component/env" -> bindings
	deploymentPipelines   map[string]*index.ResourceEntry   // "namespace/pipelineName" -> pipeline
}

// WrapIndex wraps an existing generic index with OpenChoreo-specific functionality
func WrapIndex(idx *index.Index) *Index {
	ocIndex := &Index{
		Index:                 idx,
		componentsByProject:   make(map[string][]*index.ResourceEntry),
		workloadsByComponent:  make(map[string]*index.ResourceEntry),
		componentTypes:        make(map[string]*index.ResourceEntry),
		traits:                make(map[string]*index.ResourceEntry),
		clusterComponentTypes: make(map[string]*index.ResourceEntry),
		clusterTraits:         make(map[string]*index.ResourceEntry),
		releasesByComponent:   make(map[string][]*index.ResourceEntry),
		releaseBindingsByEnv:  make(map[string][]*index.ResourceEntry),
		deploymentPipelines:   make(map[string]*index.ResourceEntry),
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
		// Index by namespace and project
		projectName := entry.GetNestedString("spec", "owner", "projectName")
		if projectName != "" {
			key := nsKey(entry.Namespace(), projectName)
			idx.componentsByProject[key] = append(idx.componentsByProject[key], entry)
		}

	case WorkloadGVK:
		// Index by namespace and component
		owner := ExtractOwnerRef(entry)
		if owner != nil && owner.ProjectName != "" && owner.ComponentName != "" {
			key := nsKey(entry.Namespace(), owner.ProjectName, owner.ComponentName)
			idx.workloadsByComponent[key] = entry
		}

	case ComponentTypeGVK:
		// Index by namespace and type name
		name := entry.Name()
		if name != "" {
			idx.componentTypes[nsKey(entry.Namespace(), name)] = entry
		}

	case TraitGVK:
		// Index by namespace and trait name
		name := entry.Name()
		if name != "" {
			idx.traits[nsKey(entry.Namespace(), name)] = entry
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
		// Index by namespace and component
		owner := ExtractOwnerRef(entry)
		if owner != nil && owner.ProjectName != "" && owner.ComponentName != "" {
			key := nsKey(entry.Namespace(), owner.ProjectName, owner.ComponentName)
			idx.releasesByComponent[key] = append(idx.releasesByComponent[key], entry)
		}

	case ReleaseBindingGVK:
		// Index by namespace, component, and environment
		owner := ExtractOwnerRef(entry)
		envName := entry.GetNestedString("spec", "environment")
		if owner != nil && owner.ProjectName != "" && owner.ComponentName != "" && envName != "" {
			key := nsKey(entry.Namespace(), owner.ProjectName, owner.ComponentName, envName)
			idx.releaseBindingsByEnv[key] = append(idx.releaseBindingsByEnv[key], entry)
		}

	case DeploymentPipelineGVK:
		// Index by namespace and pipeline name
		name := entry.Name()
		if name != "" {
			idx.deploymentPipelines[nsKey(entry.Namespace(), name)] = entry
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
	idx.clusterComponentTypes = make(map[string]*index.ResourceEntry)
	idx.clusterTraits = make(map[string]*index.ResourceEntry)
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

// GetComponentType retrieves a component type by namespace and name
func (idx *Index) GetComponentType(namespace, name string) (*index.ResourceEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entry, ok := idx.componentTypes[nsKey(namespace, name)]
	return entry, ok
}

// GetTrait retrieves a trait by namespace and name
func (idx *Index) GetTrait(namespace, name string) (*index.ResourceEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entry, ok := idx.traits[nsKey(namespace, name)]
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

// GetWorkloadForComponent retrieves the workload for a specific component
func (idx *Index) GetWorkloadForComponent(namespace, projectName, componentName string) (*index.ResourceEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	key := nsKey(namespace, projectName, componentName)
	entry, ok := idx.workloadsByComponent[key]
	return entry, ok
}

// ListComponents returns all components
func (idx *Index) ListComponents() []*index.ResourceEntry {
	return idx.Index.List(ComponentGVK)
}

// ListComponentsForProject returns all components for a specific project
func (idx *Index) ListComponentsForProject(namespace, projectName string) []*index.ResourceEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.componentsByProject[nsKey(namespace, projectName)]
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

// GetTypedComponentType retrieves a component type by namespace and name and returns a typed wrapper
func (idx *Index) GetTypedComponentType(namespace, name string) (*typed.ComponentType, error) {
	entry, ok := idx.GetComponentType(namespace, name)
	if !ok {
		return nil, fmt.Errorf("component type %q not found", name)
	}
	return typed.NewComponentType(entry)
}

// GetTypedTrait retrieves a trait by namespace and name and returns a typed wrapper
func (idx *Index) GetTypedTrait(namespace, name string) (*typed.Trait, error) {
	entry, ok := idx.GetTrait(namespace, name)
	if !ok {
		return nil, fmt.Errorf("trait %q not found", name)
	}
	return typed.NewTrait(entry)
}

// GetTypedWorkloadForComponent retrieves the workload for a specific component and returns a typed wrapper
func (idx *Index) GetTypedWorkloadForComponent(namespace, projectName, componentName string) (*typed.Workload, error) {
	entry, ok := idx.GetWorkloadForComponent(namespace, projectName, componentName)
	if !ok {
		return nil, fmt.Errorf("workload for component %q (project: %q) not found", componentName, projectName)
	}
	return typed.NewWorkload(entry)
}

// ListReleasesForComponent returns all component releases for a specific component
func (idx *Index) ListReleasesForComponent(namespace, projectName, componentName string) []*index.ResourceEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	key := nsKey(namespace, projectName, componentName)
	return idx.releasesByComponent[key]
}

// GetProject retrieves a project by namespace and name
func (idx *Index) GetProject(namespace, name string) (*index.ResourceEntry, bool) {
	return idx.Index.Get(ProjectGVK, namespace, name)
}

// GetDeploymentPipeline retrieves a deployment pipeline by namespace and name
func (idx *Index) GetDeploymentPipeline(namespace, name string) (*index.ResourceEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entry, ok := idx.deploymentPipelines[nsKey(namespace, name)]
	return entry, ok
}

// ListReleaseBindings returns all release bindings
func (idx *Index) ListReleaseBindings() []*index.ResourceEntry {
	return idx.Index.List(ReleaseBindingGVK)
}

// GetReleaseBindingForEnv retrieves a release binding for a specific component and environment
// Returns the first binding found for the given component and environment
func (idx *Index) GetReleaseBindingForEnv(namespace, projectName, componentName, envName string) (*index.ResourceEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	key := nsKey(namespace, projectName, componentName, envName)
	bindings := idx.releaseBindingsByEnv[key]
	if len(bindings) == 0 {
		return nil, false
	}

	// Return the first binding (there should typically be only one per environment)
	return bindings[0], true
}
