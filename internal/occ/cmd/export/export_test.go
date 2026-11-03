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
	lastApplied := map[string]string{
		"kubectl.kubernetes.io/last-applied-configuration": `{"spec":{"parameters":{"dbPassword":"old-password"}}}`,
	}
	annotations := map[string]string{
		"kubectl.kubernetes.io/last-applied-configuration": lastApplied["kubectl.kubernetes.io/last-applied-configuration"],
		"openchoreo.dev/description":                       "Payments API",
	}
	component := gen.Component{
		Metadata: gen.ObjectMeta{Name: "api", Uid: stringPointer("uid"), CreationTimestamp: &created, Labels: &labels, Annotations: &annotations},
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
	expectNoProjectResources(mockClient)
	mockClient.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{
		Items: []gen.Namespace{{Metadata: gen.ObjectMeta{Name: "acme", Annotations: &lastApplied}}},
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
	mockClient.EXPECT().ListComponentReleases(mock.Anything, "acme", mock.Anything).Return(&gen.ComponentReleaseList{}, nil)
	mockClient.EXPECT().ListReleaseBindings(mock.Anything, "acme", mock.Anything).Return(&gen.ReleaseBindingList{}, nil)

	report, err := runEngine(context.Background(), mockClient, Options{OutputDir: root})
	require.NoError(t, err)
	assert.Equal(t, 4, report.Created)
	assert.Equal(t, 4, report.Exported)
	assert.Equal(t, map[string]int{"Namespace": 1, "Project": 1, "Component": 1, "Workload": 1}, report.ExportedByKind)

	namespaceObject := readManifest(t, root, "namespaces/acme/namespace.yaml")
	assert.Equal(t, "v1", namespaceObject["apiVersion"])
	assert.Equal(t, "Namespace", namespaceObject["kind"])
	assert.NotContains(t, namespaceObject, "status")
	assert.NotContains(t, namespaceObject["metadata"].(map[string]any), "namespace")
	assert.NotContains(t, namespaceObject["metadata"].(map[string]any), "annotations")

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
	assert.Equal(t, map[string]any{"openchoreo.dev/description": "Payments API"}, metadata["annotations"])
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

func TestRun_ExportsReleaseBindingsAndAllComponentReleases(t *testing.T) {
	root := t.TempDir()
	mockClient := mocks.NewMockInterface(t)
	expectNoClusterResources(mockClient)
	expectNoNamespacePlatformResources(mockClient)
	expectNoProjectResources(mockClient)
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
	spec := &gen.ComponentReleaseSpec{}
	spec.Owner.ProjectName = "shop"
	spec.Owner.ComponentName = "api"
	mockClient.EXPECT().ListComponentReleases(mock.Anything, "acme", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, p *gen.ListComponentReleasesParams) (*gen.ComponentReleaseList, error) {
			require.Equal(t, "api", *p.Component)
			require.NotNil(t, p.Limit)
			if p.Cursor == nil {
				return &gen.ComponentReleaseList{
					Items:      []gen.ComponentRelease{{Metadata: gen.ObjectMeta{Name: "api-release-1"}, Spec: spec}},
					Pagination: gen.Pagination{NextCursor: stringPointer("next-release")},
				}, nil
			}
			require.Equal(t, "next-release", *p.Cursor)
			return &gen.ComponentReleaseList{
				Items: []gen.ComponentRelease{{Metadata: gen.ObjectMeta{Name: "api-unbound"}, Spec: spec}},
			}, nil
		}).Twice()

	report, err := runEngine(context.Background(), mockClient, Options{OutputDir: root})
	require.NoError(t, err)
	assert.Equal(t, 6, report.Exported)
	assert.Equal(t, "ReleaseBinding", readManifest(t, root, "namespaces/acme/projects/shop/components/api/release-bindings/api-dev.yaml")["kind"])
	assert.Equal(t, "ComponentRelease", readManifest(t, root, "namespaces/acme/projects/shop/components/api/releases/api-release-1.yaml")["kind"])
	assert.Equal(t, "ComponentRelease", readManifest(t, root, "namespaces/acme/projects/shop/components/api/releases/api-unbound.yaml")["kind"])
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
		KeepPlaintext: func(finding Withheld) (bool, error) {
			assert.Equal(t, "Component", finding.Kind)
			assert.NotContains(t, finding.Reasons[0], secretValue)
			return true, nil
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
	expectNoProjectResources(mockClient)
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
	mockClient.EXPECT().ListComponentReleases(mock.Anything, "acme", mock.Anything).Return(&gen.ComponentReleaseList{}, nil)
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
	mc.EXPECT().ListClusterProjectTypes(empty, empty).Return(&gen.ClusterProjectTypeList{}, nil)
	mc.EXPECT().ListClusterResourceTypes(empty, empty).Return(&gen.ClusterResourceTypeList{}, nil)
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
	mc.EXPECT().ListProjectTypes(empty, empty, empty).Return(&gen.ProjectTypeList{}, nil)
	mc.EXPECT().ListResourceTypes(empty, empty, empty).Return(&gen.ResourceTypeList{}, nil)
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

func expectNoProjectResources(mc *mocks.MockInterface) {
	empty := mock.Anything
	mc.EXPECT().ListProjectReleases(empty, empty, empty).Return(&gen.ProjectReleaseList{}, nil)
	mc.EXPECT().ListProjectReleaseBindings(empty, empty, empty).Return(&gen.ProjectReleaseBindingList{}, nil)
	mc.EXPECT().ListResources(empty, empty, empty).Return(&gen.ResourceInstanceList{}, nil)
}

func TestRun_ExportsProjectAndResourceReleases(t *testing.T) {
	c := mocks.NewMockInterface(t)
	expectNoClusterResources(c)
	expectNoNamespacePlatformResources(c)
	c.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{
		Items: []gen.Namespace{{Metadata: gen.ObjectMeta{Name: "acme"}}},
	}, nil)
	c.EXPECT().ListProjects(mock.Anything, "acme", mock.Anything).Return(&gen.ProjectList{
		Items: []gen.Project{{Metadata: gen.ObjectMeta{Name: "shop"}}},
	}, nil)
	c.EXPECT().ListComponents(mock.Anything, "acme", "shop", mock.Anything).Return(&gen.ComponentList{}, nil)
	projectSpec := &gen.ProjectReleaseSpec{}
	projectSpec.Owner.ProjectName = "shop"
	c.EXPECT().ListProjectReleases(mock.Anything, "acme", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, p *gen.ListProjectReleasesParams) (*gen.ProjectReleaseList, error) {
			require.Equal(t, "shop", *p.Project)
			require.NotNil(t, p.Limit)
			if p.Cursor == nil {
				return &gen.ProjectReleaseList{
					Items:      []gen.ProjectRelease{{Metadata: gen.ObjectMeta{Name: "shop-release"}, Spec: projectSpec}},
					Pagination: gen.Pagination{NextCursor: stringPointer("project-page-2")},
				}, nil
			}
			require.Equal(t, "project-page-2", *p.Cursor)
			return &gen.ProjectReleaseList{
				Items: []gen.ProjectRelease{{Metadata: gen.ObjectMeta{Name: "shop-unbound"}, Spec: projectSpec}},
			}, nil
		}).Twice()
	projectBindingSpec := &gen.ProjectReleaseBindingSpec{Environment: "development", ProjectRelease: stringPointer("shop-release")}
	projectBindingSpec.Owner.ProjectName = "shop"
	c.EXPECT().ListProjectReleaseBindings(mock.Anything, "acme", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, p *gen.ListProjectReleaseBindingsParams) (*gen.ProjectReleaseBindingList, error) {
			require.Equal(t, "shop", *p.Project)
			require.NotNil(t, p.Limit)
			if p.Cursor == nil {
				return &gen.ProjectReleaseBindingList{
					Items:      []gen.ProjectReleaseBinding{{Metadata: gen.ObjectMeta{Name: "shop-development"}, Spec: projectBindingSpec}},
					Pagination: gen.Pagination{NextCursor: stringPointer("binding-page-2")},
				}, nil
			}
			require.Equal(t, "binding-page-2", *p.Cursor)
			pending := &gen.ProjectReleaseBindingSpec{Environment: "staging"}
			pending.Owner.ProjectName = "shop"
			return &gen.ProjectReleaseBindingList{
				Items: []gen.ProjectReleaseBinding{{Metadata: gen.ObjectMeta{Name: "shop-staging"}, Spec: pending}},
			}, nil
		}).Twice()
	parameters := map[string]any{"dbPassword": "plaintext-to-strip", "storage": "10Gi"}
	resourceSpec := &gen.ResourceInstanceSpec{Parameters: &parameters}
	resourceSpec.Owner.ProjectName = "shop"
	c.EXPECT().ListResources(mock.Anything, "acme", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, p *gen.ListResourcesParams) (*gen.ResourceInstanceList, error) {
			require.Equal(t, "shop", *p.Project)
			require.NotNil(t, p.Limit)
			if p.Cursor == nil {
				return &gen.ResourceInstanceList{Pagination: gen.Pagination{NextCursor: stringPointer("resource-page-2")}}, nil
			}
			require.Equal(t, "resource-page-2", *p.Cursor)
			return &gen.ResourceInstanceList{
				Items: []gen.ResourceInstance{{Metadata: gen.ObjectMeta{Name: "database"}, Spec: resourceSpec}},
			}, nil
		}).Twice()
	resourceReleaseSpec := &gen.ResourceReleaseSpec{}
	resourceReleaseSpec.Owner.ProjectName = "shop"
	resourceReleaseSpec.Owner.ResourceName = "database"
	c.EXPECT().ListResourceReleases(mock.Anything, "acme", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, p *gen.ListResourceReleasesParams) (*gen.ResourceReleaseList, error) {
			require.Equal(t, "database", *p.Resource)
			require.NotNil(t, p.Limit)
			if p.Cursor == nil {
				return &gen.ResourceReleaseList{
					Items:      []gen.ResourceRelease{{Metadata: gen.ObjectMeta{Name: "database-release"}, Spec: resourceReleaseSpec}},
					Pagination: gen.Pagination{NextCursor: stringPointer("release-page-2")},
				}, nil
			}
			require.Equal(t, "release-page-2", *p.Cursor)
			return &gen.ResourceReleaseList{
				Items: []gen.ResourceRelease{{Metadata: gen.ObjectMeta{Name: "database-unbound"}, Spec: resourceReleaseSpec}},
			}, nil
		}).Twice()
	resourceBindingSpec := &gen.ResourceReleaseBindingSpec{Environment: "development", ResourceRelease: stringPointer("database-release")}
	resourceBindingSpec.Owner.ProjectName = "shop"
	resourceBindingSpec.Owner.ResourceName = "database"
	c.EXPECT().ListResourceReleaseBindings(mock.Anything, "acme", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, p *gen.ListResourceReleaseBindingsParams) (*gen.ResourceReleaseBindingList, error) {
			require.Equal(t, "database", *p.Resource)
			require.NotNil(t, p.Limit)
			if p.Cursor == nil {
				return &gen.ResourceReleaseBindingList{
					Items:      []gen.ResourceReleaseBinding{{Metadata: gen.ObjectMeta{Name: "database-development"}, Spec: resourceBindingSpec}},
					Pagination: gen.Pagination{NextCursor: stringPointer("binding-page-2")},
				}, nil
			}
			require.Equal(t, "binding-page-2", *p.Cursor)
			return &gen.ResourceReleaseBindingList{
				Items: []gen.ResourceReleaseBinding{{Metadata: gen.ObjectMeta{Name: "database-staging"}, Spec: resourceBindingSpec}},
			}, nil
		}).Twice()

	root := t.TempDir()
	report, err := runEngine(t.Context(), c, Options{OutputDir: root})
	require.NoError(t, err)
	assert.Equal(t, 11, report.Exported)
	assert.Equal(t, map[string]int{
		"Namespace": 1, "Project": 1, KindProjectRelease: 2, KindProjectReleaseBinding: 2,
		KindResource: 1, KindResourceRelease: 2, KindResourceReleaseBinding: 2,
	}, report.ExportedByKind)
	for relative, kind := range map[string]string{
		"releases/shop-release.yaml":                                    KindProjectRelease,
		"releases/shop-unbound.yaml":                                    KindProjectRelease,
		"release-bindings/shop-development.yaml":                        KindProjectReleaseBinding,
		"release-bindings/shop-staging.yaml":                            KindProjectReleaseBinding,
		"resources/database/resource.yaml":                              KindResource,
		"resources/database/releases/database-release.yaml":             KindResourceRelease,
		"resources/database/releases/database-unbound.yaml":             KindResourceRelease,
		"resources/database/release-bindings/database-development.yaml": KindResourceReleaseBinding,
		"resources/database/release-bindings/database-staging.yaml":     KindResourceReleaseBinding,
	} {
		object := readManifest(t, root, "namespaces/acme/projects/shop/"+relative)
		assert.Equal(t, kind, object["kind"])
		assert.Equal(t, openchoreoAPIVersion, object["apiVersion"])
		assert.Equal(t, "acme", object["metadata"].(map[string]any)["namespace"])
		assert.NotContains(t, object, "status")
	}
	require.Len(t, report.SecretDecisions, 1)
	assert.Equal(t, KindResource, report.SecretDecisions[0].Kind)
	object := readManifest(t, root, "namespaces/acme/projects/shop/resources/database/resource.yaml")
	assert.Equal(t, map[string]any{"storage": "10Gi"}, object["spec"].(map[string]any)["parameters"])
}

func TestRun_ProjectReleaseFailureDoesNotWrite(t *testing.T) {
	for _, failure := range []string{"API error", "later page error", "wrong owner", "missing spec", "unsafe name", "missing release"} {
		t.Run(failure, func(t *testing.T) {
			c := mocks.NewMockInterface(t)
			expectNoClusterResources(c)
			expectNoNamespacePlatformResources(c)
			c.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{
				Items: []gen.Namespace{{Metadata: gen.ObjectMeta{Name: "acme"}}},
			}, nil)
			c.EXPECT().ListProjects(mock.Anything, "acme", mock.Anything).Return(&gen.ProjectList{
				Items: []gen.Project{{Metadata: gen.ObjectMeta{Name: "shop"}}},
			}, nil)
			spec := &gen.ProjectReleaseSpec{}
			spec.Owner.ProjectName = "shop"
			list := &gen.ProjectReleaseList{Items: []gen.ProjectRelease{{Metadata: gen.ObjectMeta{Name: "shop-release"}, Spec: spec}}}
			var apiErr error
			switch failure {
			case "API error":
				apiErr = errors.New("API unavailable")
			case "wrong owner":
				spec.Owner.ProjectName = "other-project"
			case "missing spec":
				list.Items[0].Spec = nil
			case "unsafe name":
				list.Items[0].Metadata.Name = "../../escape"
			case "missing release":
				bindingSpec := &gen.ProjectReleaseBindingSpec{ProjectRelease: stringPointer("missing")}
				bindingSpec.Owner.ProjectName = "shop"
				c.EXPECT().ListProjectReleaseBindings(mock.Anything, "acme", mock.Anything).Return(&gen.ProjectReleaseBindingList{
					Items: []gen.ProjectReleaseBinding{{Metadata: gen.ObjectMeta{Name: "shop-dev"}, Spec: bindingSpec}},
				}, nil)
			}
			if failure == "later page error" {
				list.Pagination.NextCursor = stringPointer("page-2")
				c.EXPECT().ListProjectReleases(mock.Anything, "acme", mock.Anything).
					RunAndReturn(func(_ context.Context, _ string, p *gen.ListProjectReleasesParams) (*gen.ProjectReleaseList, error) {
						if p.Cursor != nil {
							require.Equal(t, "page-2", *p.Cursor)
							return nil, errors.New("API unavailable")
						}
						return list, nil
					}).Twice()
			} else {
				c.EXPECT().ListProjectReleases(mock.Anything, "acme", mock.Anything).Return(list, apiErr)
			}
			root := filepath.Join(t.TempDir(), "output")
			report, err := runEngine(t.Context(), c, Options{OutputDir: root})
			require.ErrorContains(t, err, "nothing was written")
			assert.Nil(t, report)
			_, statErr := os.Stat(root)
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}
