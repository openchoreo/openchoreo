// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"fmt"
	"path"

	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

type resourcePage[T any] func(limit int, cursor string) ([]T, string, error)

func collectResources[T any](kind, namespace, project string, fetch resourcePage[T], nameOf func(T) string, pathOf func(string) string) ([]resource, error) {
	items, err := pagination.FetchAll[T](pagination.FetchPage[T](fetch))
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", kind, err)
	}

	resources := make([]resource, 0, len(items))
	for _, item := range items {
		name := nameOf(item)
		if err := validName(kind, name); err != nil {
			return nil, err
		}
		obj, err := normalizeObject(item, kind, namespace)
		if err != nil {
			return nil, fmt.Errorf("%s %q: %w", kind, name, err)
		}
		resources = append(resources, resource{
			kind: kind, name: name, namespace: namespace, project: project,
			path: pathOf(name), object: obj,
		})
	}
	return resources, nil
}

func collectClusterResources(ctx context.Context, c client.Interface) ([]resource, error) {
	var out []resource
	add := func(items []resource, err error) error {
		if err != nil {
			return err
		}
		out = append(out, items...)
		return nil
	}
	clusterPath := func(category string) func(string) string {
		return func(name string) string { return path.Join("platform-shared", category, name+".yaml") }
	}

	if err := add(collectResources(KindClusterProjectType, "", "", func(limit int, cursor string) ([]gen.ClusterProjectType, string, error) {
		p := &gen.ListClusterProjectTypesParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListClusterProjectTypes(ctx, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ClusterProjectType) string { return v.Metadata.Name }, clusterPath("project-types"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindClusterResourceType, "", "", func(limit int, cursor string) ([]gen.ClusterResourceType, string, error) {
		p := &gen.ListClusterResourceTypesParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListClusterResourceTypes(ctx, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ClusterResourceType) string { return v.Metadata.Name }, clusterPath("resource-types"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindClusterComponentType, "", "", func(limit int, cursor string) ([]gen.ClusterComponentType, string, error) {
		p := &gen.ListClusterComponentTypesParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListClusterComponentTypes(ctx, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ClusterComponentType) string { return v.Metadata.Name }, clusterPath("component-types"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindClusterTrait, "", "", func(limit int, cursor string) ([]gen.ClusterTrait, string, error) {
		p := &gen.ListClusterTraitsParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListClusterTraits(ctx, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ClusterTrait) string { return v.Metadata.Name }, clusterPath("traits"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindClusterWorkflow, "", "", func(limit int, cursor string) ([]gen.ClusterWorkflow, string, error) {
		p := &gen.ListClusterWorkflowsParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListClusterWorkflows(ctx, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ClusterWorkflow) string { return v.Metadata.Name }, clusterPath("workflows"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindClusterDataPlane, "", "", func(limit int, cursor string) ([]gen.ClusterDataPlane, string, error) {
		p := &gen.ListClusterDataPlanesParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListClusterDataPlanes(ctx, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ClusterDataPlane) string { return v.Metadata.Name }, clusterPath("infra/data-planes"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindClusterWorkflowPlane, "", "", func(limit int, cursor string) ([]gen.ClusterWorkflowPlane, string, error) {
		p := &gen.ListClusterWorkflowPlanesParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListClusterWorkflowPlanes(ctx, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ClusterWorkflowPlane) string { return v.Metadata.Name }, clusterPath("infra/workflow-planes"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindClusterObservabilityPlane, "", "", func(limit int, cursor string) ([]gen.ClusterObservabilityPlane, string, error) {
		p := &gen.ListClusterObservabilityPlanesParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListClusterObservabilityPlanes(ctx, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ClusterObservabilityPlane) string { return v.Metadata.Name }, clusterPath("infra/observability-planes"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindClusterAuthzRole, "", "", func(limit int, cursor string) ([]gen.ClusterAuthzRole, string, error) {
		p := &gen.ListClusterRolesParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListClusterRoles(ctx, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ClusterAuthzRole) string { return v.Metadata.Name }, clusterPath("authz/roles"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindClusterAuthzRoleBinding, "", "", func(limit int, cursor string) ([]gen.ClusterAuthzRoleBinding, string, error) {
		p := &gen.ListClusterRoleBindingsParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListClusterRoleBindings(ctx, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ClusterAuthzRoleBinding) string { return v.Metadata.Name }, clusterPath("authz/role-bindings"))); err != nil {
		return nil, err
	}
	return out, nil
}

func collectNamespacePlatformResources(ctx context.Context, c client.Interface, namespace string) ([]resource, error) {
	var out []resource
	add := func(items []resource, err error) error {
		if err != nil {
			return err
		}
		out = append(out, items...)
		return nil
	}
	platformPath := func(category string) func(string) string {
		return func(name string) string {
			return path.Join("namespaces", namespace, "platform", category, name+".yaml")
		}
	}
	infraPath := func(category string) func(string) string {
		return func(name string) string {
			return path.Join("namespaces", namespace, "platform", "infra", category, name+".yaml")
		}
	}

	if err := add(collectResources(KindEnvironment, namespace, "", func(limit int, cursor string) ([]gen.Environment, string, error) {
		p := &gen.ListEnvironmentsParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListEnvironments(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.Environment) string { return v.Metadata.Name }, infraPath("environments"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindDeploymentPipeline, namespace, "", func(limit int, cursor string) ([]gen.DeploymentPipeline, string, error) {
		p := &gen.ListDeploymentPipelinesParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListDeploymentPipelines(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.DeploymentPipeline) string { return v.Metadata.Name }, infraPath("deployment-pipelines"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindDataPlane, namespace, "", func(limit int, cursor string) ([]gen.DataPlane, string, error) {
		p := &gen.ListDataPlanesParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListDataPlanes(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.DataPlane) string { return v.Metadata.Name }, infraPath("data-planes"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindWorkflowPlane, namespace, "", func(limit int, cursor string) ([]gen.WorkflowPlane, string, error) {
		p := &gen.ListWorkflowPlanesParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListWorkflowPlanes(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.WorkflowPlane) string { return v.Metadata.Name }, infraPath("workflow-planes"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindObservabilityPlane, namespace, "", func(limit int, cursor string) ([]gen.ObservabilityPlane, string, error) {
		p := &gen.ListObservabilityPlanesParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListObservabilityPlanes(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ObservabilityPlane) string { return v.Metadata.Name }, infraPath("observability-planes"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindProjectType, namespace, "", func(limit int, cursor string) ([]gen.ProjectType, string, error) {
		p := &gen.ListProjectTypesParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListProjectTypes(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ProjectType) string { return v.Metadata.Name }, platformPath("project-types"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindResourceType, namespace, "", func(limit int, cursor string) ([]gen.ResourceType, string, error) {
		p := &gen.ListResourceTypesParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListResourceTypes(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ResourceType) string { return v.Metadata.Name }, platformPath("resource-types"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindComponentType, namespace, "", func(limit int, cursor string) ([]gen.ComponentType, string, error) {
		p := &gen.ListComponentTypesParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListComponentTypes(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ComponentType) string { return v.Metadata.Name }, platformPath("component-types"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindTrait, namespace, "", func(limit int, cursor string) ([]gen.Trait, string, error) {
		p := &gen.ListTraitsParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListTraits(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.Trait) string { return v.Metadata.Name }, platformPath("traits"))); err != nil {
		return nil, err
	}
	workflowResources, err := collectResources(KindWorkflow, namespace, "", func(limit int, cursor string) ([]gen.Workflow, string, error) {
		p := &gen.ListWorkflowsParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListWorkflows(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.Workflow) string { return v.Metadata.Name }, func(name string) string { return name })
	if err != nil {
		return nil, err
	}
	for i := range workflowResources {
		category := "workflows"
		if isComponentWorkflowObject(workflowResources[i].object) {
			category = "component-workflows"
		}
		workflowResources[i].path = platformPath(category)(workflowResources[i].name)
	}
	out = append(out, workflowResources...)
	if err := add(collectResources(KindAuthzRole, namespace, "", func(limit int, cursor string) ([]gen.AuthzRole, string, error) {
		p := &gen.ListNamespaceRolesParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListNamespaceRoles(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.AuthzRole) string { return v.Metadata.Name }, platformPath("authz/roles"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindAuthzRoleBinding, namespace, "", func(limit int, cursor string) ([]gen.AuthzRoleBinding, string, error) {
		p := &gen.ListNamespaceRoleBindingsParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListNamespaceRoleBindings(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.AuthzRoleBinding) string { return v.Metadata.Name }, platformPath("authz/role-bindings"))); err != nil {
		return nil, err
	}
	if err := add(collectResources(KindSecretReference, namespace, "", func(limit int, cursor string) ([]gen.SecretReference, string, error) {
		p := &gen.ListSecretReferencesParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListSecretReferences(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.SecretReference) string { return v.Metadata.Name }, platformPath("secret-references"))); err != nil {
		return nil, err
	}
	return out, nil
}

func isComponentWorkflowObject(object map[string]any) bool {
	metadata, ok := object["metadata"].(map[string]any)
	if !ok {
		return false
	}
	labelsMap, ok := metadata["labels"].(map[string]any)
	if !ok {
		return false
	}
	return labelsMap[labels.LabelKeyWorkflowType] == labels.LabelValueWorkflowTypeComponent
}

func clusterResourcePath(category, name string) string {
	return path.Join("platform-shared", category, name+".yaml")
}

func namespacedResourcePath(namespace, category, name string) string {
	return path.Join("namespaces", namespace, "platform", category, name+".yaml")
}

// collectOwnedResources verifies API filters before placing objects under their owner.
func collectOwnedResources[T any](kind, namespace, project string, owner map[string]string, fetch resourcePage[T], nameOf func(T) string, pathOf func(string) string) ([]resource, error) {
	items, err := collectResources(kind, namespace, project, fetch, nameOf, pathOf)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		spec, _ := item.object["spec"].(map[string]any)
		actual, _ := spec["owner"].(map[string]any)
		for key, value := range owner {
			if actual[key] != value {
				return nil, fmt.Errorf("%s %q in namespace %q has an invalid owner; expected %s %q", kind, item.name, namespace, key, value)
			}
		}
	}
	return items, nil
}

// validateReleaseReferences ensures every pinned release is included under the same owner.
func validateReleaseReferences(bindings, releases []resource, field string) error {
	names := make(map[string]bool, len(releases))
	for _, release := range releases {
		names[release.name] = true
	}
	for _, binding := range bindings {
		spec := binding.object["spec"].(map[string]any)
		name, _ := spec[field].(string)
		if name == "" {
			continue
		}
		if err := validName("referenced release", name); err != nil {
			return err
		}
		if !names[name] {
			return fmt.Errorf("%s %q in namespace %q references release %q that was not exported for its owner", binding.kind, binding.name, binding.namespace, name)
		}
	}
	return nil
}

func collectProjectResources(ctx context.Context, c client.Interface, namespace, project, projectDir string) ([]resource, error) {
	owner := map[string]string{"projectName": project}
	releases, err := collectOwnedResources(KindProjectRelease, namespace, project, owner, func(limit int, cursor string) ([]gen.ProjectRelease, string, error) {
		p := &gen.ListProjectReleasesParams{Limit: &limit, Project: &project}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListProjectReleases(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ProjectRelease) string { return v.Metadata.Name }, func(name string) string {
		return path.Join(projectDir, "releases", name+".yaml")
	})
	if err != nil {
		return nil, err
	}
	bindings, err := collectOwnedResources(KindProjectReleaseBinding, namespace, project, owner, func(limit int, cursor string) ([]gen.ProjectReleaseBinding, string, error) {
		p := &gen.ListProjectReleaseBindingsParams{Limit: &limit, Project: &project}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListProjectReleaseBindings(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ProjectReleaseBinding) string { return v.Metadata.Name }, func(name string) string {
		return path.Join(projectDir, "release-bindings", name+".yaml")
	})
	if err != nil {
		return nil, err
	}
	if err := validateReleaseReferences(bindings, releases, "projectRelease"); err != nil {
		return nil, err
	}
	instances, err := collectOwnedResources(KindResource, namespace, project, owner, func(limit int, cursor string) ([]gen.ResourceInstance, string, error) {
		p := &gen.ListResourcesParams{Limit: &limit, Project: &project}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListResources(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ResourceInstance) string { return v.Metadata.Name }, func(name string) string {
		return path.Join(projectDir, "resources", name, "resource.yaml")
	})
	if err != nil {
		return nil, err
	}
	out := append(releases, bindings...)
	out = append(out, instances...)
	for _, instance := range instances {
		resourceDir := path.Join(projectDir, "resources", instance.name)
		children, err := collectResourceReleases(ctx, c, namespace, project, instance.name, resourceDir)
		if err != nil {
			return nil, err
		}
		out = append(out, children...)
	}
	return out, nil
}

func collectResourceReleases(ctx context.Context, c client.Interface, namespace, project, resourceName, resourceDir string) ([]resource, error) {
	owner := map[string]string{"projectName": project, "resourceName": resourceName}
	releases, err := collectOwnedResources(KindResourceRelease, namespace, project, owner, func(limit int, cursor string) ([]gen.ResourceRelease, string, error) {
		p := &gen.ListResourceReleasesParams{Limit: &limit, Resource: &resourceName}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListResourceReleases(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ResourceRelease) string { return v.Metadata.Name }, func(name string) string {
		return path.Join(resourceDir, "releases", name+".yaml")
	})
	if err != nil {
		return nil, err
	}
	bindings, err := collectOwnedResources(KindResourceReleaseBinding, namespace, project, owner, func(limit int, cursor string) ([]gen.ResourceReleaseBinding, string, error) {
		p := &gen.ListResourceReleaseBindingsParams{Limit: &limit, Resource: &resourceName}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListResourceReleaseBindings(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ResourceReleaseBinding) string { return v.Metadata.Name }, func(name string) string {
		return path.Join(resourceDir, "release-bindings", name+".yaml")
	})
	if err != nil {
		return nil, err
	}
	if err := validateReleaseReferences(bindings, releases, "resourceRelease"); err != nil {
		return nil, err
	}
	return append(releases, bindings...), nil
}

func collectComponentReleases(ctx context.Context, c client.Interface, namespace, project, component, componentDir string) ([]resource, error) {
	owner := map[string]string{"projectName": project, "componentName": component}
	releases, err := collectOwnedResources(KindComponentRelease, namespace, project, owner, func(limit int, cursor string) ([]gen.ComponentRelease, string, error) {
		p := &gen.ListComponentReleasesParams{Limit: &limit, Component: &component}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListComponentReleases(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ComponentRelease) string { return v.Metadata.Name }, func(name string) string {
		return path.Join(componentDir, "releases", name+".yaml")
	})
	if err != nil {
		return nil, err
	}
	bindings, err := collectOwnedResources(KindReleaseBinding, namespace, project, owner, func(limit int, cursor string) ([]gen.ReleaseBinding, string, error) {
		p := &gen.ListReleaseBindingsParams{Limit: &limit, Component: &component}
		if cursor != "" {
			p.Cursor = &cursor
		}
		r, err := c.ListReleaseBindings(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		return r.Items, nextCursor(r.Pagination), nil
	}, func(v gen.ReleaseBinding) string { return v.Metadata.Name }, func(name string) string {
		return path.Join(componentDir, "release-bindings", name+".yaml")
	})
	if err != nil {
		return nil, err
	}
	if err := validateReleaseReferences(bindings, releases, "releaseName"); err != nil {
		return nil, err
	}
	return append(releases, bindings...), nil
}
