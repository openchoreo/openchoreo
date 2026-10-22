// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestRun_ExportsNormalizedComponent(t *testing.T) {
	root := t.TempDir()
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	labels := map[string]string{
		"team":                                  "payments",
		"openchoreo.dev/project-uid":            "runtime-id",
		"kustomize.toolkit.fluxcd.io/name":      "openchoreo-projects",
		"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
	}
	component := gen.Component{
		Metadata: gen.ObjectMeta{Name: "api", Uid: stringPointer("uid"), CreationTimestamp: &created, Labels: &labels},
		Spec: &gen.ComponentSpec{
			Owner: struct {
				ProjectName string `json:"projectName"`
			}{ProjectName: "shop"},
		},
		Status: &gen.ComponentStatus{},
	}
	mockClient := mocks.NewMockInterface(t)
	expectNoClusterResources(mockClient)
	expectNoNamespacePlatformResources(mockClient)
	mockClient.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{
		Items: []gen.Namespace{{Metadata: gen.ObjectMeta{Name: "acme"}}},
	}, nil)
	mockClient.EXPECT().ListProjects(mock.Anything, "acme", mock.Anything).Return(&gen.ProjectList{
		Items: []gen.Project{{Metadata: gen.ObjectMeta{Name: "shop"}}},
	}, nil)
	mockClient.EXPECT().ListComponents(mock.Anything, "acme", "shop", mock.Anything).Return(&gen.ComponentList{
		Items: []gen.Component{component},
	}, nil)
	mockClient.EXPECT().ListWorkloads(mock.Anything, "acme", mock.Anything).Return(&gen.WorkloadList{
		Items: []gen.Workload{{
			Metadata: gen.ObjectMeta{Name: "api-workload"},
			Spec: &gen.WorkloadSpec{Owner: &struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{ComponentName: "api", ProjectName: "shop"}},
		}},
	}, nil)
	mockClient.EXPECT().ListReleaseBindings(mock.Anything, "acme", mock.Anything).Return(&gen.ReleaseBindingList{}, nil)

	report, err := runEngine(context.Background(), mockClient, Options{OutputDir: root})
	require.NoError(t, err)
	assert.Equal(t, 4, report.Created)
	assert.Equal(t, 4, report.Exported)
	assert.Equal(t, map[string]int{"Namespace": 1, "Project": 1, "Component": 1, "Workload": 1}, report.ExportedByKind)
	assert.NotEmpty(t, report.Unsupported)

	namespaceObject := readManifest(t, root, "namespaces/acme/namespace.yaml")
	assert.Equal(t, "v1", namespaceObject["apiVersion"])
	assert.Equal(t, "Namespace", namespaceObject["kind"])
	assert.NotContains(t, namespaceObject, "status")
	assert.NotContains(t, namespaceObject["metadata"].(map[string]any), "namespace")

	projectObject := readManifest(t, root, "namespaces/acme/projects/shop/project.yaml")
	assert.Equal(t, openchoreoAPIVersion, projectObject["apiVersion"])
	assert.Equal(t, "Project", projectObject["kind"])
	assert.NotContains(t, projectObject, "status")
	assert.Equal(t, "acme", projectObject["metadata"].(map[string]any)["namespace"])

	path := filepath.Join(root, "namespaces", "acme", "projects", "shop", "components", "api", "component.yaml")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var object map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &object))
	assert.Equal(t, "openchoreo.dev/v1alpha1", object["apiVersion"])
	assert.Equal(t, "Component", object["kind"])
	assert.NotContains(t, object, "status")
	metadata := object["metadata"].(map[string]any)
	assert.Equal(t, "acme", metadata["namespace"])
	assert.NotContains(t, metadata, "uid")
	assert.Equal(t, map[string]any{"team": "payments"}, metadata["labels"])
	workloadObject := readManifest(t, root, "namespaces/acme/projects/shop/components/api/workload.yaml")
	assert.Equal(t, "Workload", workloadObject["kind"])
	assert.Equal(t, openchoreoAPIVersion, workloadObject["apiVersion"])
	assert.NotContains(t, workloadObject, "status")
	assert.Equal(t, "acme", workloadObject["metadata"].(map[string]any)["namespace"])
}

func TestRun_RepeatIsByteIdenticalAndPreservesOtherFiles(t *testing.T) {
	root := t.TempDir()
	unrelated := filepath.Join(root, "README.md")
	require.NoError(t, os.WriteFile(unrelated, []byte("owned by user\n"), 0o600))
	first := mockComponentClient(t, "", "")
	firstReport, err := runEngine(context.Background(), first, Options{OutputDir: root})
	require.NoError(t, err)
	componentPath := filepath.Join(root, "namespaces", "acme", "projects", "shop", "components", "api", "component.yaml")
	want, err := os.ReadFile(componentPath)
	require.NoError(t, err)

	second := mockComponentClient(t, "", "")
	secondReport, err := runEngine(context.Background(), second, Options{OutputDir: root})
	require.NoError(t, err)
	got, err := os.ReadFile(componentPath)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, 3, firstReport.Created)
	assert.Equal(t, 3, secondReport.Unchanged)
	assert.FileExists(t, unrelated)
}

func TestRun_UpdatesExistingComponentFile(t *testing.T) {
	root := t.TempDir()
	first, err := runEngine(context.Background(), mockComponentClient(t, "tier", "one"), Options{OutputDir: root})
	require.NoError(t, err)
	assert.Equal(t, 3, first.Created)

	second, err := runEngine(context.Background(), mockComponentClient(t, "tier", "two"), Options{OutputDir: root})
	require.NoError(t, err)
	assert.Equal(t, 1, second.Updated)

	componentPath := filepath.Join(root, "namespaces", "acme", "projects", "shop", "components", "api", "component.yaml")
	data, err := os.ReadFile(componentPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "two")
}

func TestRun_ExportsReleaseBindingsAndReferencedReleases(t *testing.T) {
	root := t.TempDir()
	mockClient := mocks.NewMockInterface(t)
	expectNoClusterResources(mockClient)
	expectNoNamespacePlatformResources(mockClient)
	mockClient.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{
		Items: []gen.Namespace{{Metadata: gen.ObjectMeta{Name: "acme"}}},
	}, nil)
	mockClient.EXPECT().ListProjects(mock.Anything, "acme", mock.Anything).Return(&gen.ProjectList{
		Items: []gen.Project{{Metadata: gen.ObjectMeta{Name: "shop"}}},
	}, nil)
	mockClient.EXPECT().ListComponents(mock.Anything, "acme", "shop", mock.Anything).Return(&gen.ComponentList{
		Items: []gen.Component{{Metadata: gen.ObjectMeta{Name: "api"}, Spec: componentSpec("shop")}},
	}, nil)
	mockClient.EXPECT().ListWorkloads(mock.Anything, "acme", mock.Anything).Return(&gen.WorkloadList{}, nil)
	mockClient.EXPECT().ListReleaseBindings(mock.Anything, "acme", mock.Anything).Return(&gen.ReleaseBindingList{
		Items: []gen.ReleaseBinding{{
			Metadata: gen.ObjectMeta{Name: "api-dev"},
			Spec: &gen.ReleaseBindingSpec{
				Environment: "dev",
				Owner: struct {
					ComponentName string `json:"componentName"`
					ProjectName   string `json:"projectName"`
				}{ComponentName: "api", ProjectName: "shop"},
				ReleaseName: stringPointer("api-release-1"),
			},
		}},
	}, nil)
	mockClient.EXPECT().GetComponentRelease(mock.Anything, "acme", "api-release-1").Return(&gen.ComponentRelease{
		Metadata: gen.ObjectMeta{Name: "api-release-1"},
		Spec: &gen.ComponentReleaseSpec{Owner: struct {
			ComponentName string `json:"componentName"`
			ProjectName   string `json:"projectName"`
		}{ComponentName: "api", ProjectName: "shop"}},
	}, nil)

	report, err := runEngine(context.Background(), mockClient, Options{OutputDir: root})
	require.NoError(t, err)
	assert.Equal(t, 5, report.Exported)
	assert.Equal(t, "ReleaseBinding", readManifest(t, root, "namespaces/acme/projects/shop/components/api/release-bindings/api-dev.yaml")["kind"])
	assert.Equal(t, "ComponentRelease", readManifest(t, root, "namespaces/acme/projects/shop/components/api/releases/api-release-1.yaml")["kind"])
}

func TestRun_StripsPlaintextSecretByDefault(t *testing.T) {
	mockClient := mockComponentClient(t, "dbPassword", "should-not-be-written")
	root := t.TempDir()
	report, err := runEngine(context.Background(), mockClient, Options{OutputDir: root})
	require.NoError(t, err)
	require.Len(t, report.SecretDecisions, 1)
	assert.Equal(t, "stripped 1 value(s)", report.SecretDecisions[0].Action)
	assert.NotContains(t, report.SecretDecisions[0].Reasons[0], "should-not-be-written")
	component := readManifest(t, root, "namespaces/acme/projects/shop/components/api/component.yaml")
	assert.NotContains(t, component["spec"].(map[string]any)["parameters"].(map[string]any), "dbPassword")
}

func TestRun_KeepsPlaintextWhenDecisionAllows(t *testing.T) {
	const secretValue = "should-be-written-only-after-confirmation"
	mockClient := mockComponentClient(t, "dbPassword", secretValue)
	root := t.TempDir()
	report, err := runEngine(context.Background(), mockClient, Options{
		OutputDir: root,
		KeepPlaintext: func(finding Withheld) bool {
			assert.Equal(t, "Component", finding.Kind)
			assert.NotContains(t, finding.Reasons[0], secretValue)
			return true
		},
	})
	require.NoError(t, err)
	require.Len(t, report.SecretDecisions, 1)
	assert.Equal(t, "kept", report.SecretDecisions[0].Action)
	assert.NotContains(t, report.SecretDecisions[0].Reasons[0], secretValue)
	componentPath := filepath.Join(root, "namespaces", "acme", "projects", "shop", "components", "api", "component.yaml")
	data, err := os.ReadFile(componentPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), secretValue)
}

func TestRun_FetchFailureDoesNotWriteFiles(t *testing.T) {
	mockClient := mocks.NewMockInterface(t)
	mockClient.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(nil, errors.New("API unavailable"))
	root := filepath.Join(t.TempDir(), "output")
	_, err := runEngine(context.Background(), mockClient, Options{OutputDir: root})
	require.ErrorContains(t, err, "nothing was written")
	_, statErr := os.Stat(root)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestRun_RejectsMissingOutputDirectory(t *testing.T) {
	_, err := runEngine(context.Background(), mocks.NewMockInterface(t), Options{})
	assert.ErrorContains(t, err, "output directory is required")
}

func TestRun_DoesNotGenerateKustomization(t *testing.T) {
	root := t.TempDir()
	_, err := runEngine(context.Background(), mockComponentClient(t, "", ""), Options{OutputDir: root})
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(root, "kustomization.yaml"))
}

func TestRun_PreservesExistingRootKustomization(t *testing.T) {
	root := t.TempDir()
	kustomizationPath := filepath.Join(root, "kustomization.yaml")
	original := []byte("resources:\n  - custom.yaml\n")
	require.NoError(t, os.WriteFile(kustomizationPath, original, 0o600))

	_, err := runEngine(context.Background(), mockComponentClient(t, "", ""), Options{OutputDir: root})
	require.NoError(t, err)
	got, err := os.ReadFile(kustomizationPath)
	require.NoError(t, err)
	assert.Equal(t, original, got)
}

func mockComponentClient(t *testing.T, envKey, envValue string) *mocks.MockInterface {
	t.Helper()
	component := gen.Component{
		Metadata: gen.ObjectMeta{Name: "api"},
		Spec: &gen.ComponentSpec{
			Owner: struct {
				ProjectName string `json:"projectName"`
			}{ProjectName: "shop"},
		},
	}
	if envKey != "" {
		parameters := map[string]interface{}{envKey: envValue}
		component.Spec.Parameters = &parameters
	}
	mockClient := mocks.NewMockInterface(t)
	expectNoClusterResources(mockClient)
	expectNoNamespacePlatformResources(mockClient)
	mockClient.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{
		Items: []gen.Namespace{{Metadata: gen.ObjectMeta{Name: "acme"}}},
	}, nil)
	mockClient.EXPECT().ListProjects(mock.Anything, "acme", mock.Anything).Return(&gen.ProjectList{
		Items: []gen.Project{{Metadata: gen.ObjectMeta{Name: "shop"}}},
	}, nil)
	mockClient.EXPECT().ListComponents(mock.Anything, "acme", "shop", mock.Anything).Return(&gen.ComponentList{
		Items: []gen.Component{component},
	}, nil)
	mockClient.EXPECT().ListWorkloads(mock.Anything, "acme", mock.Anything).Return(&gen.WorkloadList{}, nil)
	mockClient.EXPECT().ListReleaseBindings(mock.Anything, "acme", mock.Anything).Return(&gen.ReleaseBindingList{}, nil)
	return mockClient
}

func componentSpec(project string) *gen.ComponentSpec {
	return &gen.ComponentSpec{Owner: struct {
		ProjectName string `json:"projectName"`
	}{ProjectName: project}}
}

func expectNoClusterResources(mc *mocks.MockInterface) {
	empty := mock.Anything
	mc.EXPECT().ListClusterComponentTypes(empty, empty).Return(&gen.ClusterComponentTypeList{}, nil)
	mc.EXPECT().ListClusterTraits(empty, empty).Return(&gen.ClusterTraitList{}, nil)
	mc.EXPECT().ListClusterWorkflows(empty, empty).Return(&gen.ClusterWorkflowList{}, nil)
	mc.EXPECT().ListClusterDataPlanes(empty, empty).Return(&gen.ClusterDataPlaneList{}, nil)
	mc.EXPECT().ListClusterWorkflowPlanes(empty, empty).Return(&gen.ClusterWorkflowPlaneList{}, nil)
	mc.EXPECT().ListClusterObservabilityPlanes(empty, empty).Return(&gen.ClusterObservabilityPlaneList{}, nil)
	mc.EXPECT().ListClusterRoles(empty, empty).Return(&gen.ClusterAuthzRoleList{}, nil)
	mc.EXPECT().ListClusterRoleBindings(empty, empty).Return(&gen.ClusterAuthzRoleBindingList{}, nil)
}

func expectNoNamespacePlatformResources(mc *mocks.MockInterface) {
	empty := mock.Anything
	mc.EXPECT().ListEnvironments(empty, empty, empty).Return(&gen.EnvironmentList{}, nil)
	mc.EXPECT().ListDeploymentPipelines(empty, empty, empty).Return(&gen.DeploymentPipelineList{}, nil)
	mc.EXPECT().ListDataPlanes(empty, empty, empty).Return(&gen.DataPlaneList{}, nil)
	mc.EXPECT().ListWorkflowPlanes(empty, empty, empty).Return(&gen.WorkflowPlaneList{}, nil)
	mc.EXPECT().ListObservabilityPlanes(empty, empty, empty).Return(&gen.ObservabilityPlaneList{}, nil)
	mc.EXPECT().ListComponentTypes(empty, empty, empty).Return(&gen.ComponentTypeList{}, nil)
	mc.EXPECT().ListTraits(empty, empty, empty).Return(&gen.TraitList{}, nil)
	mc.EXPECT().ListWorkflows(empty, empty, empty).Return(&gen.WorkflowList{}, nil)
	mc.EXPECT().ListNamespaceRoles(empty, empty, empty).Return(&gen.AuthzRoleList{}, nil)
	mc.EXPECT().ListNamespaceRoleBindings(empty, empty, empty).Return(&gen.AuthzRoleBindingList{}, nil)
	mc.EXPECT().ListSecretReferences(empty, empty, empty).Return(&gen.SecretReferenceList{}, nil)
}

func stringPointer(value string) *string { return &value }

func readManifest(t *testing.T, root, relativePath string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relativePath)))
	require.NoError(t, err, relativePath)
	var object map[string]any
	require.NoError(t, yaml.Unmarshal(data, &object), relativePath)
	return object
}
