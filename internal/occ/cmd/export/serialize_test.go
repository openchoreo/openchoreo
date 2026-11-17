// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const (
	testExportNamespace = "acme"
	testExportName      = "sample"
	testExportVersion   = "openchoreo.dev/v1alpha1"
)

func manifestResource(kind string) Resource {
	namespace := testExportNamespace
	if kind == testNamespaceKind || strings.HasPrefix(kind, "Cluster") {
		namespace = ""
	}
	metadata := map[string]any{"name": testExportName}
	if namespace != "" {
		metadata["namespace"] = namespace
	}
	version := testExportVersion
	if kind == testNamespaceKind {
		version = "v1"
	}
	return Resource{
		Kind: kind, Name: testExportName, Namespace: namespace,
		Object: map[string]any{
			"apiVersion": version, "kind": kind, "metadata": metadata,
			"spec": map[string]any{"owner": map[string]any{
				"projectName": "store", "componentName": "backend", "resourceName": "database",
			}},
		},
	}
}

// Expected paths are independent of the production layout table.
func TestSerialize_RequiredLayout(t *testing.T) {
	for _, tc := range []struct{ kind, path string }{
		{"ClusterProjectType", "platform-shared/project-types/sample.yaml"},
		{"ClusterComponentType", "platform-shared/component-types/sample.yaml"},
		{"ClusterResourceType", "platform-shared/resource-types/sample.yaml"},
		{"ClusterTrait", "platform-shared/traits/sample.yaml"},
		{"ClusterWorkflow", "platform-shared/workflows/sample.yaml"},
		{"ClusterDataPlane", "platform-shared/infra/data-planes/sample.yaml"},
		{"ClusterWorkflowPlane", "platform-shared/infra/workflow-planes/sample.yaml"},
		{"ClusterObservabilityPlane", "platform-shared/infra/observability-planes/sample.yaml"},
		{"ClusterAuthzRole", "platform-shared/authz/roles/sample.yaml"},
		{"ClusterAuthzRoleBinding", "platform-shared/authz/role-bindings/sample.yaml"},
		{testNamespaceKind, "namespaces/sample/namespace.yaml"},
		{"DeploymentPipeline", "namespaces/acme/platform/infra/deployment-pipelines/sample.yaml"},
		{"Environment", "namespaces/acme/platform/infra/environments/sample.yaml"},
		{"DataPlane", "namespaces/acme/platform/infra/data-planes/sample.yaml"},
		{"WorkflowPlane", "namespaces/acme/platform/infra/workflow-planes/sample.yaml"},
		{"ObservabilityPlane", "namespaces/acme/platform/infra/observability-planes/sample.yaml"},
		{"ProjectType", "namespaces/acme/platform/project-types/sample.yaml"},
		{"ComponentType", "namespaces/acme/platform/component-types/sample.yaml"},
		{"ResourceType", "namespaces/acme/platform/resource-types/sample.yaml"},
		{"Trait", "namespaces/acme/platform/traits/sample.yaml"},
		{"Workflow", "namespaces/acme/platform/workflows/sample.yaml"},
		{"AuthzRole", "namespaces/acme/platform/authz/roles/sample.yaml"},
		{"AuthzRoleBinding", "namespaces/acme/platform/authz/role-bindings/sample.yaml"},
		{"SecretReference", "namespaces/acme/platform/secret-references/sample.yaml"},
		{"Project", "namespaces/acme/projects/sample/project.yaml"},
		{"ProjectRelease", "namespaces/acme/projects/store/releases/sample.yaml"},
		{"ProjectReleaseBinding", "namespaces/acme/projects/store/release-bindings/sample.yaml"},
		{testComponentKind, "namespaces/acme/projects/store/components/sample/component.yaml"},
		{testWorkloadKind, "namespaces/acme/projects/store/components/backend/workloads/sample.yaml"},
		{"ComponentRelease", "namespaces/acme/projects/store/components/backend/releases/sample.yaml"},
		{"ReleaseBinding", "namespaces/acme/projects/store/components/backend/release-bindings/sample.yaml"},
		{"Resource", "namespaces/acme/projects/store/resources/sample/resource.yaml"},
		{"ResourceRelease", "namespaces/acme/projects/store/resources/database/releases/sample.yaml"},
		{"ResourceReleaseBinding", "namespaces/acme/projects/store/resources/database/release-bindings/sample.yaml"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			resource := manifestResource(tc.kind)
			before, err := json.Marshal(resource.Object)
			require.NoError(t, err)
			files, err := Serialize(context.Background(), []Resource{resource})
			require.NoError(t, err)
			require.Len(t, files, 1)
			assert.Equal(t, tc.path, files[0].Path)
			encoded, err := yaml.YAMLToJSON(files[0].Data)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(encoded))
			after, err := json.Marshal(resource.Object)
			require.NoError(t, err)
			assert.Equal(t, string(before), string(after), "serialization must not mutate its input")
		})
	}
}

func TestSerialize_RejectsInvalidResources(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Resource, map[string]any)
		want   string
	}{
		{"unknown kind", func(r *Resource, object map[string]any) { r.Kind = "FutureKind"; object["kind"] = r.Kind }, "unsupported resource kind"},
		{"missing object", func(r *Resource, _ map[string]any) { r.Object = nil }, "metadata does not match"},
		{"unencodable object", func(r *Resource, _ map[string]any) { r.Object = make(chan int) }, "encode API object"},
		{"mismatched name", func(_ *Resource, object map[string]any) { object["metadata"].(map[string]any)["name"] = "other" }, "metadata does not match"},
		{"mismatched namespace", func(_ *Resource, object map[string]any) { object["metadata"].(map[string]any)["namespace"] = "other" }, "metadata does not match"},
		{"mismatched kind", func(_ *Resource, object map[string]any) { object["kind"] = "Project" }, "matching kind"},
		{"missing version", func(_ *Resource, object map[string]any) { delete(object, "apiVersion") }, "apiVersion"},
		{"missing owner", func(_ *Resource, object map[string]any) { delete(object, "spec") }, "spec.owner.projectName"},
		{"unsafe owner", func(_ *Resource, object map[string]any) {
			object["spec"].(map[string]any)["owner"].(map[string]any)["projectName"] = "../../outside"
		}, "safe path component"},
		{"unsafe name", func(r *Resource, object map[string]any) {
			r.Name = "../outside"
			object["metadata"].(map[string]any)["name"] = r.Name
		}, "safe path component"},
		{"unsafe namespace", func(r *Resource, object map[string]any) {
			r.Namespace = `..\outside`
			object["metadata"].(map[string]any)["namespace"] = r.Namespace
		}, "safe path component"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resource := manifestResource(testComponentKind)
			tc.change(&resource, resource.Object.(map[string]any))
			files, err := Serialize(context.Background(), []Resource{manifestResource("Environment"), resource})
			require.ErrorContains(t, err, tc.want)
			assert.Nil(t, files, "a serialization error must discard prepared results")
		})
	}
}

func TestSerialize_DuplicatePaths(t *testing.T) {
	resource := manifestResource(testWorkloadKind)
	files, err := Serialize(context.Background(), []Resource{resource, resource})
	require.ErrorContains(t, err, "duplicate export path")
	assert.Nil(t, files)
}

func TestSerialize_MultipleWorkloadsAndStableOrder(t *testing.T) {
	first, second := manifestResource(testWorkloadKind), manifestResource(testWorkloadKind)
	second.Name = "another"
	second.Object.(map[string]any)["metadata"].(map[string]any)["name"] = second.Name
	files, err := Serialize(context.Background(), []Resource{first, second})
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.Equal(t, "namespaces/acme/projects/store/components/backend/workloads/another.yaml", files[0].Path)
	assert.Equal(t, "namespaces/acme/projects/store/components/backend/workloads/sample.yaml", files[1].Path)
	reversed, err := Serialize(context.Background(), []Resource{second, first})
	require.NoError(t, err)
	assert.Equal(t, files, reversed)
}

func TestSerialize_FetchedTypedObjects(t *testing.T) {
	f := newFetchFixture(t, false)
	resources, err := Fetch(context.Background(), f.client)
	require.NoError(t, err)
	files, err := Serialize(context.Background(), resources)
	require.NoError(t, err)
	require.Len(t, files, len(resources))
	var component Resource
	for _, resource := range resources {
		if resource.Kind == testComponentKind && resource.Namespace == testExportNamespace {
			component = resource
			break
		}
	}
	require.NotNil(t, component.Object)
	want, err := json.Marshal(component.Object)
	require.NoError(t, err)
	for _, file := range files {
		if file.Path == "namespaces/acme/projects/store/components/components-0/component.yaml" {
			got, err := yaml.YAMLToJSON(file.Data)
			require.NoError(t, err)
			assert.JSONEq(t, string(want), string(got), "metadata, spec, and status must be preserved")
			return
		}
	}
	t.Fatal("component manifest was not serialized")
}

func TestSerialize_PreservesValueTypes(t *testing.T) {
	resource := manifestResource(testComponentKind)
	object := resource.Object.(map[string]any)
	object["spec"].(map[string]any)["parameters"] = map[string]any{
		"text": "yes", "numberText": "123", "nullText": "null", "enabled": false,
		"replicas": 3, "nested": []any{map[string]any{"token": "preserve-this-value"}},
	}
	object["status"] = map[string]any{"observedGeneration": 7}
	object["metadata"].(map[string]any)["uid"] = "preserve-uid"
	want, err := json.Marshal(object)
	require.NoError(t, err)
	files, err := Serialize(context.Background(), []Resource{resource})
	require.NoError(t, err)
	got, err := yaml.YAMLToJSON(files[0].Data)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(got))
}

func TestSerialize_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	files, err := Serialize(ctx, []Resource{manifestResource(testComponentKind)})
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, files)
}

func TestSerialize_NamespaceEnvelopePreservesAPIFields(t *testing.T) {
	uid := "namespace-uid"
	phase := gen.NamespaceStatusPhase("Active")
	resource := Resource{
		Kind: testNamespaceKind, Name: testExportNamespace,
		Object: gen.Namespace{
			Metadata: gen.ObjectMeta{Name: testExportNamespace, Uid: &uid},
			Status:   &gen.NamespaceStatus{Phase: &phase},
		},
	}
	before, err := json.Marshal(resource.Object)
	require.NoError(t, err)
	files, err := Serialize(context.Background(), []Resource{resource})
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "namespaces/acme/namespace.yaml", files[0].Path)
	got, err := yaml.YAMLToJSON(files[0].Data)
	require.NoError(t, err)
	assert.JSONEq(t, `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"acme","uid":"namespace-uid"},"status":{"phase":"Active"}}`, string(got))
	after, err := json.Marshal(resource.Object)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestWriteFiles_RejectsUnsafePathsBeforeCreatingOutput(t *testing.T) {
	for _, filePath := range []string{"../outside.yaml", "/absolute.yaml", `folder\outside.yaml`, "", "./sample.yaml", "folder/../sample.yaml"} {
		t.Run(filePath, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "export")
			err := writeFiles(context.Background(), directory, []SerializedFile{{Path: filePath}})
			require.ErrorContains(t, err, "invalid export path")
			assert.NoDirExists(t, directory)
		})
	}
}

func TestWriteFiles_RejectsSymlinkEscapeBeforeWriting(t *testing.T) {
	directory, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(directory, "platform-shared")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	files := []SerializedFile{
		{Path: "first.yaml", Data: []byte("first\n")},
		{Path: "platform-shared/escaped.yaml", Data: []byte("outside\n")},
	}
	require.Error(t, writeFiles(context.Background(), directory, files))
	assert.NoFileExists(t, filepath.Join(directory, "first.yaml"))
	assert.NoFileExists(t, filepath.Join(outside, "escaped.yaml"))
}

func TestWriteFiles_RejectsExistingDirectoryOrSymlink(t *testing.T) {
	for _, directoryConflict := range []bool{true, false} {
		directory := t.TempDir()
		conflict := filepath.Join(directory, "manifest.yaml")
		if directoryConflict {
			require.NoError(t, os.Mkdir(conflict, 0o700))
		} else if err := os.Symlink("missing.yaml", conflict); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		err := writeFiles(context.Background(), directory, []SerializedFile{{Path: "manifest.yaml"}})
		require.ErrorContains(t, err, "already exists")
	}
}

func TestWriteManifest_ExclusiveCreationPreservesExistingFile(t *testing.T) {
	directory := t.TempDir()
	name := filepath.Join(directory, "manifest.yaml")
	require.NoError(t, os.WriteFile(name, []byte("existing\n"), 0o600))
	root, err := os.OpenRoot(directory)
	require.NoError(t, err)
	defer root.Close()
	err = writeManifest(root, SerializedFile{Path: "manifest.yaml", Data: []byte("replacement\n")})
	require.ErrorIs(t, err, os.ErrExist)
	data, err := os.ReadFile(name)
	require.NoError(t, err)
	assert.Equal(t, "existing\n", string(data))
}

func TestWriteFiles_CancellationDoesNotCreateOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	directory := filepath.Join(t.TempDir(), "export")
	err := writeFiles(ctx, directory, []SerializedFile{{Path: "manifest.yaml"}})
	require.ErrorIs(t, err, context.Canceled)
	assert.NoDirExists(t, directory)
}
