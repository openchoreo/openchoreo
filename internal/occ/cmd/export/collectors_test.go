// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestCollectClusterResources_UsesExportPaths(t *testing.T) {
	c := mocks.NewMockInterface(t)
	c.EXPECT().ListClusterProjectTypes(mock.Anything, mock.Anything).Return(&gen.ClusterProjectTypeList{
		Items: []gen.ClusterProjectType{{Metadata: gen.ObjectMeta{Name: "standard"}}},
	}, nil)
	c.EXPECT().ListClusterResourceTypes(mock.Anything, mock.Anything).Return(&gen.ClusterResourceTypeList{
		Items: []gen.ClusterResourceType{{Metadata: gen.ObjectMeta{Name: "postgres"}}},
	}, nil)
	c.EXPECT().ListClusterComponentTypes(mock.Anything, mock.Anything).Return(&gen.ClusterComponentTypeList{
		Items: []gen.ClusterComponentType{{Metadata: gen.ObjectMeta{Name: "service"}}},
	}, nil)
	c.EXPECT().ListClusterTraits(mock.Anything, mock.Anything).Return(&gen.ClusterTraitList{}, nil)
	c.EXPECT().ListClusterWorkflows(mock.Anything, mock.Anything).Return(&gen.ClusterWorkflowList{}, nil)
	c.EXPECT().ListClusterDataPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterDataPlaneList{}, nil)
	c.EXPECT().ListClusterWorkflowPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterWorkflowPlaneList{}, nil)
	c.EXPECT().ListClusterObservabilityPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterObservabilityPlaneList{}, nil)
	c.EXPECT().ListClusterRoles(mock.Anything, mock.Anything).Return(&gen.ClusterAuthzRoleList{}, nil)
	c.EXPECT().ListClusterRoleBindings(mock.Anything, mock.Anything).Return(&gen.ClusterAuthzRoleBindingList{}, nil)

	resources, err := collectClusterResources(context.Background(), c)
	require.NoError(t, err)
	require.Len(t, resources, 3)
	paths := map[string]string{}
	for _, item := range resources {
		paths[item.kind] = item.path
		require.NotContains(t, item.object["metadata"], "namespace")
	}
	require.Equal(t, "platform-shared/component-types/service.yaml", paths[KindClusterComponentType])
	require.Equal(t, "platform-shared/project-types/standard.yaml", paths[KindClusterProjectType])
	require.Equal(t, "platform-shared/resource-types/postgres.yaml", paths[KindClusterResourceType])
}

func TestCollectNamespacePlatformResources_UsesExportPaths(t *testing.T) {
	c := mocks.NewMockInterface(t)
	c.EXPECT().ListEnvironments(mock.Anything, "acme", mock.Anything).Return(&gen.EnvironmentList{
		Items: []gen.Environment{{Metadata: gen.ObjectMeta{Name: "development"}}},
	}, nil)
	c.EXPECT().ListDeploymentPipelines(mock.Anything, "acme", mock.Anything).Return(&gen.DeploymentPipelineList{}, nil)
	c.EXPECT().ListDataPlanes(mock.Anything, "acme", mock.Anything).Return(&gen.DataPlaneList{}, nil)
	c.EXPECT().ListWorkflowPlanes(mock.Anything, "acme", mock.Anything).Return(&gen.WorkflowPlaneList{}, nil)
	c.EXPECT().ListObservabilityPlanes(mock.Anything, "acme", mock.Anything).Return(&gen.ObservabilityPlaneList{}, nil)
	c.EXPECT().ListProjectTypes(mock.Anything, "acme", mock.Anything).Return(&gen.ProjectTypeList{
		Items: []gen.ProjectType{{Metadata: gen.ObjectMeta{Name: "custom"}}},
	}, nil)
	c.EXPECT().ListResourceTypes(mock.Anything, "acme", mock.Anything).Return(&gen.ResourceTypeList{
		Items: []gen.ResourceType{{Metadata: gen.ObjectMeta{Name: "postgres"}}},
	}, nil)
	c.EXPECT().ListComponentTypes(mock.Anything, "acme", mock.Anything).Return(&gen.ComponentTypeList{}, nil)
	c.EXPECT().ListTraits(mock.Anything, "acme", mock.Anything).Return(&gen.TraitList{}, nil)
	componentWorkflowLabels := map[string]string{labels.LabelKeyWorkflowType: labels.LabelValueWorkflowTypeComponent}
	c.EXPECT().ListWorkflows(mock.Anything, "acme", mock.Anything).Return(&gen.WorkflowList{
		Items: []gen.Workflow{
			{Metadata: gen.ObjectMeta{Name: "docker-build", Labels: &componentWorkflowLabels}},
			{Metadata: gen.ObjectMeta{Name: "release"}},
		},
	}, nil)
	c.EXPECT().ListNamespaceRoles(mock.Anything, "acme", mock.Anything).Return(&gen.AuthzRoleList{}, nil)
	c.EXPECT().ListNamespaceRoleBindings(mock.Anything, "acme", mock.Anything).Return(&gen.AuthzRoleBindingList{}, nil)
	c.EXPECT().ListSecretReferences(mock.Anything, "acme", mock.Anything).Return(&gen.SecretReferenceList{}, nil)

	resources, err := collectNamespacePlatformResources(context.Background(), c, "acme")
	require.NoError(t, err)
	require.Len(t, resources, 5)
	paths := map[string]bool{}
	for _, resource := range resources {
		paths[resource.path] = true
	}
	require.True(t, paths["namespaces/acme/platform/infra/environments/development.yaml"])
	require.True(t, paths["namespaces/acme/platform/component-workflows/docker-build.yaml"])
	require.True(t, paths["namespaces/acme/platform/workflows/release.yaml"])
	require.True(t, paths["namespaces/acme/platform/project-types/custom.yaml"])
	require.True(t, paths["namespaces/acme/platform/resource-types/postgres.yaml"])
	for _, resource := range resources {
		if resource.kind == KindEnvironment {
			metadata := resource.object["metadata"].(map[string]any)
			require.Equal(t, "acme", metadata["namespace"])
		}
	}
}

func TestCollectResourceReleases_RejectsInvalidBinding(t *testing.T) {
	for _, failure := range []string{"wrong project", "wrong resource", "missing spec", "unsafe reference", "missing release"} {
		t.Run(failure, func(t *testing.T) {
			c := mocks.NewMockInterface(t)
			c.EXPECT().ListResourceReleases(mock.Anything, "acme", mock.Anything).Return(&gen.ResourceReleaseList{}, nil).Maybe()
			spec := &gen.ResourceReleaseBindingSpec{}
			spec.Owner.ProjectName = "shop"
			spec.Owner.ResourceName = "database"
			switch failure {
			case "wrong project":
				spec.Owner.ProjectName = "other-project"
			case "wrong resource":
				spec.Owner.ResourceName = "other-resource"
			case "missing spec":
				spec = nil
			case "unsafe reference":
				spec.ResourceRelease = stringPointer("../escape")
			case "missing release":
				spec.ResourceRelease = stringPointer("missing")
			}
			c.EXPECT().ListResourceReleaseBindings(mock.Anything, "acme", mock.Anything).Return(&gen.ResourceReleaseBindingList{
				Items: []gen.ResourceReleaseBinding{{Metadata: gen.ObjectMeta{Name: "database-dev"}, Spec: spec}},
			}, nil)
			_, err := collectResourceReleases(t.Context(), c, "acme", "shop", "database", "resources/database", &releaseFilter{})
			require.Error(t, err)
		})
	}
}

func TestCollectResourceReleases_RejectsInvalidOwner(t *testing.T) {
	for _, field := range []string{"project", "resource"} {
		t.Run(field, func(t *testing.T) {
			c := mocks.NewMockInterface(t)
			c.EXPECT().ListResourceReleaseBindings(mock.Anything, "acme", mock.Anything).Return(&gen.ResourceReleaseBindingList{}, nil)
			spec := &gen.ResourceReleaseSpec{}
			spec.Owner.ProjectName = "shop"
			spec.Owner.ResourceName = "database"
			if field == "project" {
				spec.Owner.ProjectName = "other-project"
			} else {
				spec.Owner.ResourceName = "other-resource"
			}
			c.EXPECT().ListResourceReleases(mock.Anything, "acme", mock.Anything).Return(&gen.ResourceReleaseList{
				Items: []gen.ResourceRelease{{Metadata: gen.ObjectMeta{Name: "database-release"}, Spec: spec}},
			}, nil)
			_, err := collectResourceReleases(t.Context(), c, "acme", "shop", "database", "resources/database", &releaseFilter{})
			require.ErrorContains(t, err, "invalid owner")
		})
	}
}
