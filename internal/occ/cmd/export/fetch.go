// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/releaseline"
)

// resourcePage decodes raw list responses while retaining generated item types
// and sharing pagination across all resource kinds.
type resourcePage[T any] struct {
	Items      []T            `json:"items"`
	Pagination gen.Pagination `json:"pagination"`
}

type resourceFetcher func(context.Context, string) ([]Resource, error)

// Fetch collects configuration and selected releases using the authenticated
// API client. The default window is three calendar months, plus every release
// referenced by an exported binding. AllReleases bypasses the date cutoff.
// Calls and pages are sequential. On failure it returns no partial collection.
// The API controls namespace discovery, authorization, and field visibility;
// this operation does not provide an atomic snapshot across resource kinds.
func Fetch(ctx context.Context, c client.Interface, allReleases bool) (*FetchResult, error) {
	return fetchResources(ctx, c, allReleases, releaseCutoff(time.Now()))
}

func fetchResources(ctx context.Context, c client.Interface, allReleases bool, cutoff time.Time) (*FetchResult, error) {
	if c == nil {
		return nil, fmt.Errorf("API client is required")
	}

	provider, ok := c.(generatedClientProvider)
	if !ok {
		return nil, fmt.Errorf("API client does not expose the generated client required to preserve export numbers")
	}
	api := provider.GetClient()
	if api == nil {
		return nil, fmt.Errorf("generated API client is missing")
	}

	resources, err := fetchCollections(ctx, "", clusterFetchers(api))
	if err != nil {
		return nil, err
	}

	namespaces, err := fetchNamespaces(ctx, api)
	if err != nil {
		return nil, err
	}

	fetchers := namespacedFetchers(api)
	preservedPaths := make(map[string]struct{})
	var warnings []string
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
		lines, err := fetchReleaseLines(ctx, api, name, items)
		if err != nil {
			return nil, err
		}
		resources = append(resources, lines...)
		selection := releaseSelection{
			all: allReleases, cutoff: cutoff, references: bindingReleaseReferences(items), preservedPaths: preservedPaths,
		}
		releases, err := fetchCollections(ctx, name, releaseFetchers(api, &selection))
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, selection.referenceWarnings()...)
		resources = append(resources, releases...)
	}
	result := &FetchResult{Resources: resources, PreservedPaths: preservedPaths, Warnings: warnings}
	if !allReleases {
		result.ReleaseCutoff = &cutoff
	}
	return result, nil
}

// The API exposes release lines under each component and includes a synthetic
// main entry. Only persisted ReleaseLine configuration belongs in the export;
// it is always fetched in full, independently of the release-history window.
func fetchReleaseLines(ctx context.Context, c *gen.ClientWithResponses, namespace string, resources []Resource) ([]Resource, error) {
	var lines []Resource
	for _, component := range resources {
		if component.Kind != "Component" {
			continue
		}
		fetch := newFetcher("ReleaseLine",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ReleaseLine], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				response, err := decodePage[gen.ReleaseLine](c.ListComponentReleaseLines(ctx, namespace, component.Name,
					&gen.ListComponentReleaseLinesParams{Limit: pageLimit, Cursor: pageCursor}))
				if err != nil || response == nil {
					return response, err
				}
				items := make([]gen.ReleaseLine, 0, len(response.Items))
				for _, line := range response.Items {
					if line.Metadata.Name != releaseline.MainLine {
						items = append(items, line)
					}
				}
				return &resourcePage[gen.ReleaseLine]{Items: items, Pagination: response.Pagination}, nil
			},
			func(item gen.ReleaseLine) string { return item.Metadata.Name },
		)
		items, err := fetch(ctx, namespace)
		if err != nil {
			return nil, fmt.Errorf("component %q: %w", component.Name, err)
		}
		lines = append(lines, items...)
	}
	return lines, nil
}

// Clamp the day to the target month's last day (May 31 -> February 28/29).
// One UTC cutoff is captured for the entire export, including every namespace.
func releaseCutoff(now time.Time) time.Time {
	now = now.UTC()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -3, 0)
	day := min(now.Day(), month.AddDate(0, 1, -1).Day())
	return time.Date(month.Year(), month.Month(), day, now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), time.UTC)
}

type releaseIdentity struct {
	kind, namespace, name string
}

type releaseSelection struct {
	all            bool
	cutoff         time.Time
	references     map[releaseIdentity]bool
	preservedPaths map[string]struct{}
}

func bindingReleaseReferences(resources []Resource) map[releaseIdentity]bool {
	references := make(map[releaseIdentity]bool)
	for _, resource := range resources {
		var kind string
		var name *string
		switch binding := resource.Object.(type) {
		case gen.ReleaseBinding:
			if binding.Spec != nil && binding.Metadata.DeletionTimestamp == nil {
				kind, name = "ComponentRelease", binding.Spec.ReleaseName
			}
		case gen.ProjectReleaseBinding:
			if binding.Spec != nil && binding.Metadata.DeletionTimestamp == nil {
				kind, name = "ProjectRelease", binding.Spec.ProjectRelease
			}
		case gen.ResourceReleaseBinding:
			if binding.Spec != nil && binding.Metadata.DeletionTimestamp == nil {
				kind, name = "ResourceRelease", binding.Spec.ResourceRelease
			}
		case gen.EndpointReleaseBinding:
			if binding.Spec != nil && binding.Metadata.DeletionTimestamp == nil {
				kind, name = endpointReleaseKind, binding.Spec.EndpointRelease
			}
		}
		if name != nil && *name != "" {
			references[releaseIdentity{kind: kind, namespace: resource.Namespace, name: *name}] = false
		}
	}
	return references
}

func (s *releaseSelection) include(kind, namespace string, metadata gen.ObjectMeta, owner resourceOwner) (bool, error) {
	if metadata.Name == "" {
		return false, fmt.Errorf("response item has no metadata.name")
	}
	// Objects being deleted must neither satisfy references nor protect old files.
	if metadata.DeletionTimestamp != nil {
		return false, nil
	}
	identity := releaseIdentity{kind: kind, namespace: namespace, name: metadata.Name}
	_, referenced := s.references[identity]
	if referenced {
		s.references[identity] = true
	}
	if s.all || referenced || metadata.CreationTimestamp == nil || !metadata.CreationTimestamp.Before(s.cutoff) {
		return true, nil
	}
	name, err := resourcePath(Resource{Kind: kind, Namespace: namespace, Name: metadata.Name}, owner)
	if err != nil {
		return false, fmt.Errorf("locate excluded release %q: %w", metadata.Name, err)
	}
	s.preservedPaths[name] = struct{}{}
	return false, nil
}

func (s *releaseSelection) referenceWarnings() []string {
	var warnings []string
	for identity, found := range s.references {
		if !found {
			warnings = append(warnings, fmt.Sprintf("binding-referenced release is missing or marked for deletion: %s %q in namespace %q", identity.kind, identity.name, identity.namespace))
		}
	}
	sort.Strings(warnings)
	return warnings
}

// Filter each page before FetchAll accumulates it. A fresh slice prevents old
// release objects from remaining reachable through the page's backing array.
func newReleaseFetcher[T any](
	kind string,
	list func(context.Context, string, int, string) (*resourcePage[T], error),
	identity func(T) (gen.ObjectMeta, resourceOwner),
	selection *releaseSelection,
) resourceFetcher {
	return newFetcher(kind, func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[T], error) {
		page, err := list(ctx, namespace, limit, cursor)
		if err != nil || page == nil {
			return page, err
		}
		items := make([]T, 0, len(page.Items))
		for _, item := range page.Items {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			metadata, owner := identity(item)
			include, err := selection.include(kind, namespace, metadata, owner)
			if err != nil {
				return nil, err
			}
			if include {
				items = append(items, item)
			}
		}
		return &resourcePage[T]{Items: items, Pagination: page.Pagination}, nil
	}, func(item T) string {
		metadata, _ := identity(item)
		return metadata.Name
	})
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
func fetchNamespaces(ctx context.Context, c *gen.ClientWithResponses) ([]gen.Namespace, error) {
	return fetchAll(ctx, "Namespace", "", func(limit int, cursor string) (*resourcePage[gen.Namespace], error) {
		pageLimit, pageCursor := pageParams(limit, cursor)
		return decodePage[gen.Namespace](c.ListNamespaces(ctx, &gen.ListNamespacesParams{Limit: pageLimit, Cursor: pageCursor}))
	})
}

// Decode list responses locally to preserve numbers in every free-form map.
// This preserves digits received from the API, not digits already lost server-side.
func decodePage[T any](response *http.Response, requestErr error) (*resourcePage[T], error) {
	if requestErr != nil {
		return nil, requestErr
	}
	if response == nil || response.Body == nil {
		return nil, fmt.Errorf("API returned a nil list response or body")
	}
	defer func() { _ = response.Body.Close() }()
	decoder := json.NewDecoder(response.Body)
	decoder.UseNumber()
	if response.StatusCode != http.StatusOK {
		var problem gen.ErrorResponse
		if err := decoder.Decode(&problem); err == nil && problem.Error != "" {
			return nil, fmt.Errorf("list request failed (HTTP %d): %s", response.StatusCode, problem.Error)
		}
		return nil, fmt.Errorf("list request failed (HTTP %d)", response.StatusCode)
	}
	var page resourcePage[T]
	if err := decoder.Decode(&page); err != nil {
		return nil, fmt.Errorf("decode list: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("list response contains trailing data")
	}
	return &page, nil
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
func clusterFetchers(c *gen.ClientWithResponses) []resourceFetcher {
	return []resourceFetcher{
		newFetcher("ClusterProjectType",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterProjectType], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ClusterProjectType](c.ListClusterProjectTypes(ctx, &gen.ListClusterProjectTypesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ClusterProjectType) string { return item.Metadata.Name },
		),
		newFetcher("ClusterComponentType",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterComponentType], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ClusterComponentType](c.ListClusterComponentTypes(ctx, &gen.ListClusterComponentTypesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ClusterComponentType) string { return item.Metadata.Name },
		),
		newFetcher("ClusterResourceType",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterResourceType], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ClusterResourceType](c.ListClusterResourceTypes(ctx, &gen.ListClusterResourceTypesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ClusterResourceType) string { return item.Metadata.Name },
		),
		newFetcher("ClusterEndpointType",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterEndpointType], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ClusterEndpointType](c.ListClusterEndpointTypes(ctx, &gen.ListClusterEndpointTypesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ClusterEndpointType) string { return item.Metadata.Name },
		),
		newFetcher("ClusterTrait",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterTrait], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ClusterTrait](c.ListClusterTraits(ctx, &gen.ListClusterTraitsParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ClusterTrait) string { return item.Metadata.Name },
		),
		newFetcher("ClusterWorkflow",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterWorkflow], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ClusterWorkflow](c.ListClusterWorkflows(ctx, &gen.ListClusterWorkflowsParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ClusterWorkflow) string { return item.Metadata.Name },
		),
		newFetcher("ClusterDataPlane",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterDataPlane], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ClusterDataPlane](c.ListClusterDataPlanes(ctx, &gen.ListClusterDataPlanesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ClusterDataPlane) string { return item.Metadata.Name },
		),
		newFetcher("ClusterWorkflowPlane",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterWorkflowPlane], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ClusterWorkflowPlane](c.ListClusterWorkflowPlanes(ctx, &gen.ListClusterWorkflowPlanesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ClusterWorkflowPlane) string { return item.Metadata.Name },
		),
		newFetcher("ClusterObservabilityPlane",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterObservabilityPlane], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ClusterObservabilityPlane](c.ListClusterObservabilityPlanes(ctx, &gen.ListClusterObservabilityPlanesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ClusterObservabilityPlane) string { return item.Metadata.Name },
		),
		newFetcher("ClusterAuthzRole",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterAuthzRole], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ClusterAuthzRole](c.ListClusterRoles(ctx, &gen.ListClusterRolesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ClusterAuthzRole) string { return item.Metadata.Name },
		),
		newFetcher("ClusterAuthzRoleBinding",
			func(ctx context.Context, _ string, limit int, cursor string) (*resourcePage[gen.ClusterAuthzRoleBinding], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ClusterAuthzRoleBinding](c.ListClusterRoleBindings(ctx, &gen.ListClusterRoleBindingsParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ClusterAuthzRoleBinding) string { return item.Metadata.Name },
		),
	}
}

// namespacedFetchers declares the requested namespaced inventory.
func namespacedFetchers(c *gen.ClientWithResponses) []resourceFetcher {
	return []resourceFetcher{
		newFetcher("DeploymentPipeline",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.DeploymentPipeline], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.DeploymentPipeline](c.ListDeploymentPipelines(ctx, namespace, &gen.ListDeploymentPipelinesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.DeploymentPipeline) string { return item.Metadata.Name },
		),
		newFetcher("Environment",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.Environment], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.Environment](c.ListEnvironments(ctx, namespace, &gen.ListEnvironmentsParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.Environment) string { return item.Metadata.Name },
		),
		newFetcher("DataPlane",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.DataPlane], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.DataPlane](c.ListDataPlanes(ctx, namespace, &gen.ListDataPlanesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.DataPlane) string { return item.Metadata.Name },
		),
		newFetcher("WorkflowPlane",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.WorkflowPlane], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.WorkflowPlane](c.ListWorkflowPlanes(ctx, namespace, &gen.ListWorkflowPlanesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.WorkflowPlane) string { return item.Metadata.Name },
		),
		newFetcher("ObservabilityPlane",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ObservabilityPlane], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ObservabilityPlane](c.ListObservabilityPlanes(ctx, namespace, &gen.ListObservabilityPlanesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ObservabilityPlane) string { return item.Metadata.Name },
		),
		newFetcher("ProjectType",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ProjectType], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ProjectType](c.ListProjectTypes(ctx, namespace, &gen.ListProjectTypesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ProjectType) string { return item.Metadata.Name },
		),
		newFetcher("ComponentType",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ComponentType], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ComponentType](c.ListComponentTypes(ctx, namespace, &gen.ListComponentTypesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ComponentType) string { return item.Metadata.Name },
		),
		newFetcher("ResourceType",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ResourceType], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ResourceType](c.ListResourceTypes(ctx, namespace, &gen.ListResourceTypesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ResourceType) string { return item.Metadata.Name },
		),
		newFetcher("EndpointType",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.EndpointType], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.EndpointType](c.ListEndpointTypes(ctx, namespace, &gen.ListEndpointTypesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.EndpointType) string { return item.Metadata.Name },
		),
		newFetcher("Trait",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.Trait], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.Trait](c.ListTraits(ctx, namespace, &gen.ListTraitsParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.Trait) string { return item.Metadata.Name },
		),
		newFetcher("Workflow",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.Workflow], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.Workflow](c.ListWorkflows(ctx, namespace, &gen.ListWorkflowsParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.Workflow) string { return item.Metadata.Name },
		),
		newFetcher("AuthzRole",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.AuthzRole], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.AuthzRole](c.ListNamespaceRoles(ctx, namespace, &gen.ListNamespaceRolesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.AuthzRole) string { return item.Metadata.Name },
		),
		newFetcher("AuthzRoleBinding",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.AuthzRoleBinding], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.AuthzRoleBinding](c.ListNamespaceRoleBindings(ctx, namespace, &gen.ListNamespaceRoleBindingsParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.AuthzRoleBinding) string { return item.Metadata.Name },
		),
		newFetcher("ObservabilityAlertsNotificationChannel",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ObservabilityAlertsNotificationChannel], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ObservabilityAlertsNotificationChannel](c.ListObservabilityAlertsNotificationChannels(ctx, namespace, &gen.ListObservabilityAlertsNotificationChannelsParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ObservabilityAlertsNotificationChannel) string { return item.Metadata.Name },
		),
		newFetcher("SecretReference",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.SecretReference], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.SecretReference](c.ListSecretReferences(ctx, namespace, &gen.ListSecretReferencesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.SecretReference) string { return item.Metadata.Name },
		),
		newFetcher("Project",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.Project], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.Project](c.ListProjects(ctx, namespace, &gen.ListProjectsParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.Project) string { return item.Metadata.Name },
		),
		newFetcher("ProjectReleaseBinding",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ProjectReleaseBinding], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ProjectReleaseBinding](c.ListProjectReleaseBindings(ctx, namespace, &gen.ListProjectReleaseBindingsParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ProjectReleaseBinding) string { return item.Metadata.Name },
		),
		newFetcher("Component",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.Component], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.Component](c.ListComponents(ctx, namespace, &gen.ListComponentsParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.Component) string { return item.Metadata.Name },
		),
		newFetcher("Workload",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.Workload], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.Workload](c.ListWorkloads(ctx, namespace, &gen.ListWorkloadsParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.Workload) string { return item.Metadata.Name },
		),
		newFetcher("ReleaseBinding",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ReleaseBinding], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ReleaseBinding](c.ListReleaseBindings(ctx, namespace, &gen.ListReleaseBindingsParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ReleaseBinding) string { return item.Metadata.Name },
		),
		newFetcher(endpointKind,
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.Endpoint], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.Endpoint](c.ListEndpoints(ctx, namespace, &gen.ListEndpointsParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.Endpoint) string { return item.Metadata.Name },
		),
		newFetcher("EndpointReleaseBinding",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.EndpointReleaseBinding], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.EndpointReleaseBinding](c.ListEndpointReleaseBindings(ctx, namespace, &gen.ListEndpointReleaseBindingsParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.EndpointReleaseBinding) string { return item.Metadata.Name },
		),
		newFetcher("Resource",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ResourceInstance], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ResourceInstance](c.ListResources(ctx, namespace, &gen.ListResourcesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ResourceInstance) string { return item.Metadata.Name },
		),
		newFetcher("ResourceReleaseBinding",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ResourceReleaseBinding], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ResourceReleaseBinding](c.ListResourceReleaseBindings(ctx, namespace, &gen.ListResourceReleaseBindingsParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ResourceReleaseBinding) string { return item.Metadata.Name },
		),
	}
}

// releaseFetchers run after namespace configuration and bindings have been read.
func releaseFetchers(c *gen.ClientWithResponses, selection *releaseSelection) []resourceFetcher {
	return []resourceFetcher{
		newReleaseFetcher("ProjectRelease",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ProjectRelease], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ProjectRelease](c.ListProjectReleases(ctx, namespace, &gen.ListProjectReleasesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ProjectRelease) (gen.ObjectMeta, resourceOwner) {
				var owner resourceOwner
				if item.Spec != nil {
					owner.ProjectName = item.Spec.Owner.ProjectName
				}
				return item.Metadata, owner
			}, selection,
		),
		newReleaseFetcher("ComponentRelease",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ComponentRelease], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ComponentRelease](c.ListComponentReleases(ctx, namespace, &gen.ListComponentReleasesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ComponentRelease) (gen.ObjectMeta, resourceOwner) {
				var owner resourceOwner
				if item.Spec != nil {
					owner.ProjectName, owner.ComponentName = item.Spec.Owner.ProjectName, item.Spec.Owner.ComponentName
				}
				return item.Metadata, owner
			}, selection,
		),
		newReleaseFetcher(endpointReleaseKind,
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.EndpointRelease], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.EndpointRelease](c.ListEndpointReleases(ctx, namespace, &gen.ListEndpointReleasesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.EndpointRelease) (gen.ObjectMeta, resourceOwner) {
				var owner resourceOwner
				if item.Spec != nil {
					owner.ProjectName, owner.EndpointName = item.Spec.Owner.ProjectName, item.Spec.Owner.EndpointName
				}
				return item.Metadata, owner
			}, selection,
		),
		newReleaseFetcher("ResourceRelease",
			func(ctx context.Context, namespace string, limit int, cursor string) (*resourcePage[gen.ResourceRelease], error) {
				pageLimit, pageCursor := pageParams(limit, cursor)
				return decodePage[gen.ResourceRelease](c.ListResourceReleases(ctx, namespace, &gen.ListResourceReleasesParams{Limit: pageLimit, Cursor: pageCursor}))
			},
			func(item gen.ResourceRelease) (gen.ObjectMeta, resourceOwner) {
				var owner resourceOwner
				if item.Spec != nil {
					owner.ProjectName, owner.ResourceName = item.Spec.Owner.ProjectName, item.Spec.Owner.ResourceName
				}
				return item.Metadata, owner
			}, selection,
		),
	}
}
