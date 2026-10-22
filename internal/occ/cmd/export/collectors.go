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
