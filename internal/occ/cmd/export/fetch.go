// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// resourcePage has the same underlying structure as the generated list types.
// Adapters use checked Go pointer conversions to share pagination without
// reflection or a JSON round trip.
type resourcePage[T any] struct {
	Items      []T            `json:"items"`
	Pagination gen.Pagination `json:"pagination"`
}

type resourceFetcher func(context.Context, string) ([]Resource, error)

// Fetch collects the requested inventory using existing OCC list methods.
// Calls and pages are sequential. On failure it returns no partial collection.
// The API controls namespace discovery, authorization, and field visibility;
// this operation does not provide an atomic snapshot across resource kinds.
func Fetch(ctx context.Context, c client.Interface) ([]Resource, error) {
	if c == nil {
		return nil, fmt.Errorf("API client is required")
	}

	resources, err := fetchCollections(ctx, "", clusterFetchers(c))
	if err != nil {
		return nil, err
	}

	namespaces, err := fetchNamespaces(ctx, c)
	if err != nil {
		return nil, err
	}

	fetchers := namespacedFetchers(c)
	for _, namespace := range namespaces {
		name := namespace.Metadata.Name
		if name == "" {
			return nil, fmt.Errorf("fetch Namespace: response item has no metadata.name")
		}
		resources = append(resources, Resource{Kind: "Namespace", Name: name, Object: namespace})
		items, err := fetchCollections(ctx, name, fetchers)
		if err != nil {
			return nil, err
		}
		resources = append(resources, items...)
	}
	return resources, nil
}

// fetchCollections runs every collection fetcher in the given scope.
func fetchCollections(ctx context.Context, namespace string, fetchers []resourceFetcher) ([]Resource, error) {
	var resources []Resource
	for _, fetch := range fetchers {
		items, err := fetch(ctx, namespace)
		if err != nil {
			return nil, err
		}
		resources = append(resources, items...)
	}
	return resources, nil
}

// fetchNamespaces returns all pages of namespace objects as well as their names
// for the subsequent namespaced requests.
func fetchNamespaces(ctx context.Context, c client.Interface) ([]gen.Namespace, error) {
	return fetchAll(ctx, "Namespace", "", func(limit int, cursor string) (*resourcePage[gen.Namespace], error) {
		pageLimit, pageCursor := pageParams(limit, cursor)
		response, err := c.ListNamespaces(ctx, &gen.ListNamespacesParams{Limit: pageLimit, Cursor: pageCursor})
		return (*resourcePage[gen.Namespace])(response), err
	})
}

// fetchAll keeps the shared pagination behavior while propagating cancellation
// and adding the resource kind and scope to failures.
func fetchAll[T any](
	ctx context.Context,
	kind, namespace string,
	fetchPage func(int, string) (*resourcePage[T], error),
) ([]T, error) {
	items, err := pagination.FetchAll(func(limit int, cursor string) ([]T, string, error) {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		page, err := fetchPage(limit, cursor)
		if err != nil {
			return nil, "", err
		}
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		if page == nil {
			return nil, "", fmt.Errorf("API returned a nil list response")
		}
		next := ""
		if page.Pagination.NextCursor != nil {
			next = *page.Pagination.NextCursor
		}
		return page.Items, next, nil
	})
	if err != nil {
		if namespace != "" {
			return nil, fmt.Errorf("fetch %s in namespace %q: %w", kind, namespace, err)
		}
		return nil, fmt.Errorf("fetch %s: %w", kind, err)
	}
	return items, nil
}

// newFetcher adapts a typed collection into the flat pipeline collection without
// changing the API objects. Scope is supplied by namespace discovery.
func newFetcher[T any](
	kind string,
	list func(context.Context, string, int, string) (*resourcePage[T], error),
	nameOf func(T) string,
) resourceFetcher {
	return func(ctx context.Context, namespace string) ([]Resource, error) {
		items, err := fetchAll(ctx, kind, namespace, func(limit int, cursor string) (*resourcePage[T], error) {
			return list(ctx, namespace, limit, cursor)
		})
		if err != nil {
			return nil, err
		}
		resources := make([]Resource, 0, len(items))
		for _, item := range items {
			name := nameOf(item)
			if name == "" {
				return nil, fmt.Errorf("fetch %s in namespace %q: response item has no metadata.name", kind, namespace)
			}
			resources = append(resources, Resource{Kind: kind, Namespace: namespace, Name: name, Object: item})
		}
		return resources, nil
	}
}

// clusterFetchers declares the requested cluster-scoped inventory.
func clusterFetchers(c client.Interface) []resourceFetcher {
	return []resourceFetcher{
		newFetcher("ClusterProjectType",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterProjectType], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListClusterProjectTypes(ctx, &gen.ListClusterProjectTypesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ClusterProjectType])(response), err
			},
			func(item gen.ClusterProjectType) string { return item.Metadata.Name },
		),
		newFetcher("ClusterComponentType",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterComponentType], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListClusterComponentTypes(ctx, &gen.ListClusterComponentTypesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ClusterComponentType])(response), err
			},
			func(item gen.ClusterComponentType) string { return item.Metadata.Name },
		),
		newFetcher("ClusterResourceType",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterResourceType], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListClusterResourceTypes(ctx, &gen.ListClusterResourceTypesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ClusterResourceType])(response), err
			},
			func(item gen.ClusterResourceType) string { return item.Metadata.Name },
		),
		newFetcher("ClusterTrait",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterTrait], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListClusterTraits(ctx, &gen.ListClusterTraitsParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ClusterTrait])(response), err
			},
			func(item gen.ClusterTrait) string { return item.Metadata.Name },
		),
		newFetcher("ClusterWorkflow",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterWorkflow], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListClusterWorkflows(ctx, &gen.ListClusterWorkflowsParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ClusterWorkflow])(response), err
			},
			func(item gen.ClusterWorkflow) string { return item.Metadata.Name },
		),
		newFetcher("ClusterDataPlane",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterDataPlane], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListClusterDataPlanes(ctx, &gen.ListClusterDataPlanesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ClusterDataPlane])(response), err
			},
			func(item gen.ClusterDataPlane) string { return item.Metadata.Name },
		),
		newFetcher("ClusterWorkflowPlane",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterWorkflowPlane], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListClusterWorkflowPlanes(ctx, &gen.ListClusterWorkflowPlanesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ClusterWorkflowPlane])(response), err
			},
			func(item gen.ClusterWorkflowPlane) string { return item.Metadata.Name },
		),
		newFetcher("ClusterObservabilityPlane",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterObservabilityPlane], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListClusterObservabilityPlanes(ctx, &gen.ListClusterObservabilityPlanesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ClusterObservabilityPlane])(response), err
			},
			func(item gen.ClusterObservabilityPlane) string { return item.Metadata.Name },
		),
		newFetcher("ClusterAuthzRole",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterAuthzRole], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListClusterRoles(ctx, &gen.ListClusterRolesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ClusterAuthzRole])(response), err
			},
			func(item gen.ClusterAuthzRole) string { return item.Metadata.Name },
		),
		newFetcher("ClusterAuthzRoleBinding",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterAuthzRoleBinding], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListClusterRoleBindings(ctx, &gen.ListClusterRoleBindingsParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ClusterAuthzRoleBinding])(response), err
			},
			func(item gen.ClusterAuthzRoleBinding) string { return item.Metadata.Name },
		),
	}
}

// namespacedFetchers declares the requested namespaced inventory.
func namespacedFetchers(c client.Interface) []resourceFetcher {
	return []resourceFetcher{
		newFetcher("DeploymentPipeline",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.DeploymentPipeline], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListDeploymentPipelines(ctx, namespace, &gen.ListDeploymentPipelinesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.DeploymentPipeline])(response), err
			},
			func(item gen.DeploymentPipeline) string { return item.Metadata.Name },
		),
		newFetcher("Environment",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.Environment], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListEnvironments(ctx, namespace, &gen.ListEnvironmentsParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.Environment])(response), err
			},
			func(item gen.Environment) string { return item.Metadata.Name },
		),
		newFetcher("DataPlane",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.DataPlane], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListDataPlanes(ctx, namespace, &gen.ListDataPlanesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.DataPlane])(response), err
			},
			func(item gen.DataPlane) string { return item.Metadata.Name },
		),
		newFetcher("WorkflowPlane",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.WorkflowPlane], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListWorkflowPlanes(ctx, namespace, &gen.ListWorkflowPlanesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.WorkflowPlane])(response), err
			},
			func(item gen.WorkflowPlane) string { return item.Metadata.Name },
		),
		newFetcher("ObservabilityPlane",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ObservabilityPlane], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListObservabilityPlanes(ctx, namespace, &gen.ListObservabilityPlanesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ObservabilityPlane])(response), err
			},
			func(item gen.ObservabilityPlane) string { return item.Metadata.Name },
		),
		newFetcher("ProjectType",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ProjectType], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListProjectTypes(ctx, namespace, &gen.ListProjectTypesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ProjectType])(response), err
			},
			func(item gen.ProjectType) string { return item.Metadata.Name },
		),
		newFetcher("ComponentType",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ComponentType], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListComponentTypes(ctx, namespace, &gen.ListComponentTypesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ComponentType])(response), err
			},
			func(item gen.ComponentType) string { return item.Metadata.Name },
		),
		newFetcher("ResourceType",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ResourceType], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListResourceTypes(ctx, namespace, &gen.ListResourceTypesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ResourceType])(response), err
			},
			func(item gen.ResourceType) string { return item.Metadata.Name },
		),
		newFetcher("Trait",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.Trait], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListTraits(ctx, namespace, &gen.ListTraitsParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.Trait])(response), err
			},
			func(item gen.Trait) string { return item.Metadata.Name },
		),
		newFetcher("Workflow",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.Workflow], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListWorkflows(ctx, namespace, &gen.ListWorkflowsParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.Workflow])(response), err
			},
			func(item gen.Workflow) string { return item.Metadata.Name },
		),
		newFetcher("AuthzRole",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.AuthzRole], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListNamespaceRoles(ctx, namespace, &gen.ListNamespaceRolesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.AuthzRole])(response), err
			},
			func(item gen.AuthzRole) string { return item.Metadata.Name },
		),
		newFetcher("AuthzRoleBinding",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.AuthzRoleBinding], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListNamespaceRoleBindings(ctx, namespace, &gen.ListNamespaceRoleBindingsParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.AuthzRoleBinding])(response), err
			},
			func(item gen.AuthzRoleBinding) string { return item.Metadata.Name },
		),
		newFetcher("SecretReference",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.SecretReference], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListSecretReferences(ctx, namespace, &gen.ListSecretReferencesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.SecretReference])(response), err
			},
			func(item gen.SecretReference) string { return item.Metadata.Name },
		),
		newFetcher("Project",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.Project], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListProjects(ctx, namespace, &gen.ListProjectsParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.Project])(response), err
			},
			func(item gen.Project) string { return item.Metadata.Name },
		),
		newFetcher("ProjectRelease",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ProjectRelease], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListProjectReleases(ctx, namespace, &gen.ListProjectReleasesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ProjectRelease])(response), err
			},
			func(item gen.ProjectRelease) string { return item.Metadata.Name },
		),
		newFetcher("ProjectReleaseBinding",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ProjectReleaseBinding], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListProjectReleaseBindings(ctx, namespace, &gen.ListProjectReleaseBindingsParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ProjectReleaseBinding])(response), err
			},
			func(item gen.ProjectReleaseBinding) string { return item.Metadata.Name },
		),
		newFetcher("Component",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.Component], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListComponents(ctx, namespace, "", &gen.ListComponentsParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.Component])(response), err
			},
			func(item gen.Component) string { return item.Metadata.Name },
		),
		newFetcher("Workload",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.Workload], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListWorkloads(ctx, namespace, &gen.ListWorkloadsParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.Workload])(response), err
			},
			func(item gen.Workload) string { return item.Metadata.Name },
		),
		newFetcher("ComponentRelease",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ComponentRelease], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListComponentReleases(ctx, namespace, &gen.ListComponentReleasesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ComponentRelease])(response), err
			},
			func(item gen.ComponentRelease) string { return item.Metadata.Name },
		),
		newFetcher("ReleaseBinding",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ReleaseBinding], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListReleaseBindings(ctx, namespace, &gen.ListReleaseBindingsParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ReleaseBinding])(response), err
			},
			func(item gen.ReleaseBinding) string { return item.Metadata.Name },
		),
		newFetcher("Resource",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ResourceInstance], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListResources(ctx, namespace, &gen.ListResourcesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ResourceInstance])(response), err
			},
			func(item gen.ResourceInstance) string { return item.Metadata.Name },
		),
		newFetcher("ResourceRelease",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ResourceRelease], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListResourceReleases(ctx, namespace, &gen.ListResourceReleasesParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ResourceRelease])(response), err
			},
			func(item gen.ResourceRelease) string { return item.Metadata.Name },
		),
		newFetcher("ResourceReleaseBinding",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ResourceReleaseBinding], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := c.ListResourceReleaseBindings(ctx, namespace, &gen.ListResourceReleaseBindingsParams{Limit: pageLimit, Cursor: pageCursor})
				return (*resourcePage[gen.ResourceReleaseBinding])(response), err
			},
			func(item gen.ResourceReleaseBinding) string { return item.Metadata.Name },
		),
	}
}
