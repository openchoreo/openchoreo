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

func TestCollectClusterResources_UsesProposalPaths(t *testing.T) {
	c := mocks.NewMockInterface(t)
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
	require.Len(t, resources, 1)
	require.Equal(t, KindClusterComponentType, resources[0].kind)
	require.Equal(t, "platform-shared/component-types/service.yaml", resources[0].path)
}

func TestCollectNamespacePlatformResources_UsesProposalPaths(t *testing.T) {
	c := mocks.NewMockInterface(t)
	c.EXPECT().ListEnvironments(mock.Anything, "acme", mock.Anything).Return(&gen.EnvironmentList{
		Items: []gen.Environment{{Metadata: gen.ObjectMeta{Name: "development"}}},
	}, nil)
	c.EXPECT().ListDeploymentPipelines(mock.Anything, "acme", mock.Anything).Return(&gen.DeploymentPipelineList{}, nil)
	c.EXPECT().ListDataPlanes(mock.Anything, "acme", mock.Anything).Return(&gen.DataPlaneList{}, nil)
	c.EXPECT().ListWorkflowPlanes(mock.Anything, "acme", mock.Anything).Return(&gen.WorkflowPlaneList{}, nil)
	c.EXPECT().ListObservabilityPlanes(mock.Anything, "acme", mock.Anything).Return(&gen.ObservabilityPlaneList{}, nil)
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
	require.Len(t, resources, 3)
	paths := map[string]bool{}
	for _, resource := range resources {
		paths[resource.path] = true
	}
	require.True(t, paths["namespaces/acme/platform/infra/environments/development.yaml"])
	require.True(t, paths["namespaces/acme/platform/component-workflows/docker-build.yaml"])
	require.True(t, paths["namespaces/acme/platform/workflows/release.yaml"])
	for _, resource := range resources {
		if resource.kind == KindEnvironment {
			metadata := resource.object["metadata"].(map[string]any)
			require.Equal(t, "acme", metadata["namespace"])
		}
	}
}
