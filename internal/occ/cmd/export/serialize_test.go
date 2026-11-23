// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
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
		{testReleaseLineKind, "namespaces/acme/projects/store/components/backend/release-lines/sample.yaml"},
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

func TestPortablePathPart(t *testing.T) {
	reserved := make([]string, 0, 30)
	reserved = append(reserved, "con", "PRN", "aux", "NuL", "CONIN$", "conout$")
	for _, prefix := range []string{"com", "LPT"} {
		for _, digit := range "123456789¹²³" {
			reserved = append(reserved, prefix+string(digit))
		}
	}
	for _, name := range reserved {
		for _, suffix := range []string{"", ".txt"} {
			assert.Equal(t, "~"+name+suffix, portablePathPart(name+suffix))
		}
	}
	for _, name := range []string{"normal", "null", "console", "com", "com0", "com10", "lpt", "lpt0", "lpt10"} {
		assert.Equal(t, name, portablePathPart(name))
	}
	assert.Equal(t, "~~nul", portablePathPart("~nul"))
	assert.Equal(t, "~~~~nul", portablePathPart("~~nul"))
	assert.Equal(t, "name~~suffix", portablePathPart("name~suffix"))
}

func TestSerialize_PortableNames(t *testing.T) {
	for _, tc := range []struct{ kind, name, namespace, path string }{
		{testComponentKind, "nul", "acme", "namespaces/acme/projects/~con/components/~nul/component.yaml"},
		{testComponentKind, "~nul", "acme", "namespaces/acme/projects/~con/components/~~nul/component.yaml"},
		{testWorkloadKind, "LPT9", "nul", "namespaces/~nul/projects/~con/components/~aux/workloads/~LPT9.yaml"},
		{testNamespaceKind, "aux", "", "namespaces/~aux/namespace.yaml"},
		{"Project", "prn", "acme", "namespaces/acme/projects/~prn/project.yaml"},
		{"Resource", "con", "acme", "namespaces/acme/projects/~con/resources/~con/resource.yaml"},
		{"ResourceRelease", "nul.txt", "acme", "namespaces/acme/projects/~con/resources/~prn/releases/~nul.txt.yaml"},
		{"ProjectRelease", "lpt1", "acme", "namespaces/acme/projects/~con/releases/~lpt1.yaml"},
		{"ClusterTrait", "aux", "", "platform-shared/traits/~aux.yaml"},
	} {
		t.Run(tc.kind+"/"+tc.name, func(t *testing.T) {
			resource := manifestResource(tc.kind)
			resource.Name, resource.Namespace = tc.name, tc.namespace
			object := resource.Object.(map[string]any)
			metadata := object["metadata"].(map[string]any)
			metadata["name"] = tc.name
			if tc.namespace != "" {
				metadata["namespace"] = tc.namespace
			}
			object["spec"].(map[string]any)["owner"] = map[string]any{
				"projectName": "con", "componentName": "aux", "resourceName": "prn",
			}
			before, err := json.Marshal(object)
			require.NoError(t, err)
			files := serializedResources(t, resource)
			require.Len(t, files, 1)
			assert.Equal(t, tc.path, files[0].Path)
			encoded, err := yaml.YAMLToJSONStrict(files[0].Data)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(encoded), "manifest names must remain unchanged")
			assert.True(t, isManagedManifest(tc.path, files[0].Data))
		})
	}
}

func TestWriteFiles_SynchronizesPortableNames(t *testing.T) {
	const componentName = "nul"
	component := manifestResource(testComponentKind)
	component.Name = componentName
	component.Object.(map[string]any)["metadata"].(map[string]any)["name"] = componentName
	collision := manifestResource(testComponentKind)
	collision.Name = "~nul"
	collision.Object.(map[string]any)["metadata"].(map[string]any)["name"] = "~nul"
	workload := manifestResource(testWorkloadKind)
	workload.Name = "aux"
	workload.Object.(map[string]any)["metadata"].(map[string]any)["name"] = "aux"
	workload.Object.(map[string]any)["spec"].(map[string]any)["owner"].(map[string]any)["componentName"] = componentName
	directory := t.TempDir()
	files := serializedResources(t, component, collision, workload)
	counts, err := writeFiles(context.Background(), directory, files, writeOptions{})
	require.NoError(t, err)
	assert.Equal(t, FileCounts{Created: 3}, counts)
	base := filepath.Join(directory, "namespaces", "acme", "projects", "store", "components")
	assert.FileExists(t, filepath.Join(base, "~nul", "component.yaml"))
	assert.FileExists(t, filepath.Join(base, "~~nul", "component.yaml"))
	assert.FileExists(t, filepath.Join(base, "~nul", "workloads", "~aux.yaml"))
	counts, err = writeFiles(context.Background(), directory, files, writeOptions{})
	require.NoError(t, err)
	assert.Equal(t, FileCounts{Unchanged: 3}, counts)
	component.Object.(map[string]any)["spec"].(map[string]any)["parameters"] = map[string]any{"replicas": 2}
	files = serializedResources(t, component, collision)
	counts, err = writeFiles(context.Background(), directory, files, writeOptions{})
	require.NoError(t, err)
	assert.Equal(t, FileCounts{Updated: 1, Unchanged: 1, Deleted: 1}, counts)
	assert.NoDirExists(t, filepath.Join(base, "~nul", "workloads"))
	counts, err = writeFiles(context.Background(), directory, serializedResources(t, collision), writeOptions{})
	require.NoError(t, err)
	assert.Equal(t, FileCounts{Unchanged: 1, Deleted: 1}, counts)
	assert.NoDirExists(t, filepath.Join(base, "~nul"))
	assert.FileExists(t, filepath.Join(base, "~~nul", "component.yaml"))
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
	resources, err := fetchAllTestResources(context.Background(), f.client)
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

func TestSerialize_PreservesNumberLiterals(t *testing.T) {
	resource := manifestResource(testComponentKind)
	resource.Object.(map[string]any)["spec"].(map[string]any)["parameters"] = map[string]any{
		"decimal":  json.Number("1.234567890123456789"),
		"large":    json.Number("9007199254740993"),
		"trailing": json.Number("2.5000"),
		"nested":   []any{map[string]any{"exponent": json.Number("1.2300e-04")}},
	}
	canonical, err := Canonicalize(context.Background(), []Resource{resource})
	require.NoError(t, err)
	files, err := Serialize(context.Background(), canonical)
	require.NoError(t, err)
	require.Len(t, files, 1)
	text := string(files[0].Data)
	assert.Contains(t, text, "decimal: 1.234567890123456789\n")
	assert.Contains(t, text, "large: 9007199254740993\n")
	assert.Contains(t, text, "trailing: 2.5000\n")
	assert.Contains(t, text, "exponent: 1.2300e-04\n")
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
			_, err := writeFiles(context.Background(), directory, []SerializedFile{{Path: filePath}}, writeOptions{})
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
	_, err := writeFiles(context.Background(), directory, files, writeOptions{})
	require.Error(t, err)
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
		_, err := writeFiles(context.Background(), directory, []SerializedFile{{Path: "manifest.yaml"}}, writeOptions{})
		require.ErrorContains(t, err, "already exists")
	}
}

func TestWriteFiles_PreservesUnmanagedDestination(t *testing.T) {
	directory := t.TempDir()
	name := filepath.Join(directory, "manifest.yaml")
	require.NoError(t, os.WriteFile(name, []byte("existing\n"), 0o600))
	_, err := writeFiles(context.Background(), directory,
		[]SerializedFile{{Path: "manifest.yaml", Data: []byte("replacement\n")}}, writeOptions{})
	require.ErrorContains(t, err, "not a managed manifest")
	data, err := os.ReadFile(name)
	require.NoError(t, err)
	assert.Equal(t, "existing\n", string(data))
}

func TestWriteFiles_CancellationDoesNotCreateOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	directory := filepath.Join(t.TempDir(), "export")
	_, err := writeFiles(ctx, directory, []SerializedFile{{Path: "manifest.yaml"}}, writeOptions{})
	require.ErrorIs(t, err, context.Canceled)
	assert.NoDirExists(t, directory)
}

func serializedResources(t *testing.T, resources ...Resource) []SerializedFile {
	t.Helper()
	files, err := Serialize(context.Background(), resources)
	require.NoError(t, err)
	return files
}

func seedFiles(t *testing.T, directory string, files []SerializedFile) {
	t.Helper()
	for _, file := range files {
		name := filepath.Join(directory, filepath.FromSlash(file.Path))
		require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o700))
		require.NoError(t, os.WriteFile(name, file.Data, 0o600))
	}
}

type treeEntry struct {
	Mode fs.FileMode
	Data string
}

func snapshotTree(t *testing.T, directory string) map[string]treeEntry {
	t.Helper()
	entries := make(map[string]treeEntry)
	err := fs.WalkDir(os.DirFS(directory), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		item := treeEntry{Mode: info.Mode()}
		if !entry.IsDir() {
			data, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(name)))
			if err != nil {
				return err
			}
			item.Data = string(data)
		}
		entries[name] = item
		return nil
	})
	require.NoError(t, err)
	return entries
}

func TestWriteFiles_SynchronizesOnlyLayoutManagedManifests(t *testing.T) {
	directory := t.TempDir()
	component := manifestResource(testComponentKind)
	project := manifestResource("Project")
	old := serializedResources(t, component, project, manifestResource(testWorkloadKind))
	seedFiles(t, directory, old)
	unrelated := []SerializedFile{
		{Path: ".github/workflows/ci.yaml", Data: []byte("name: CI\n")},
		{Path: "namespaces/acme/kustomization.yaml", Data: []byte("kind: Kustomization\n")},
		{Path: "namespaces/acme/manual/component.yaml", Data: old[0].Data},
		{Path: "namespaces/acme/platform/notes.yaml", Data: []byte("not: [valid yaml\n")},
	}
	seedFiles(t, directory, unrelated)
	projectPath := filepath.Join(directory, filepath.FromSlash("namespaces/acme/projects/sample/project.yaml"))
	stamp := time.Unix(1700000000, 0)
	require.NoError(t, os.Chtimes(projectPath, stamp, stamp))
	before, err := os.Stat(projectPath)
	require.NoError(t, err)
	component.Object.(map[string]any)["spec"].(map[string]any)["parameters"] = map[string]any{"replicas": 3}
	current := serializedResources(t, component, project, manifestResource("Environment"))
	counts, err := writeFiles(context.Background(), directory, current, writeOptions{})
	require.NoError(t, err)
	assert.Equal(t, FileCounts{Created: 1, Updated: 1, Unchanged: 1, Deleted: 1}, counts)
	for _, file := range append(current, unrelated...) {
		data, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(file.Path)))
		require.NoError(t, err)
		assert.Equal(t, file.Data, data, file.Path)
	}
	assert.NoDirExists(t, filepath.Join(directory, filepath.FromSlash("namespaces/acme/projects/store/components/backend")))
	after, err := os.Stat(projectPath)
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after), "identical files must not be replaced")
	assert.Equal(t, before.ModTime(), after.ModTime())
	assert.Equal(t, before.Mode(), after.Mode())
	for name := range snapshotTree(t, directory) {
		assert.NotContains(t, name, ".occ-export-", "transaction data must be cleaned up")
	}
}

func TestManagedManifest_RequiresExactLayoutIdentityAndSingleDocument(t *testing.T) {
	file := serializedResources(t, manifestResource(testComponentKind))[0]
	for _, tc := range []struct {
		name string
		path string
		data []byte
	}{
		{"wrong path", "component.yaml", file.Data},
		{"wrong namespace", strings.Replace(file.Path, "/acme/", "/other/", 1), file.Data},
		{"wrong project", strings.Replace(file.Path, "/store/", "/other/", 1), file.Data},
		{"wrong version", file.Path, bytes.ReplaceAll(file.Data, []byte(testExportVersion), []byte("example.org/v1"))},
		{"multiple documents", file.Path, append(append([]byte{}, file.Data...), []byte("---\nkind: ConfigMap\n")...)},
		{"duplicate keys", file.Path, append(append([]byte{}, file.Data...), []byte("kind: Component\n")...)},
		{"invalid YAML", file.Path, []byte("kind: [\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.False(t, isManagedManifest(tc.path, tc.data))
		})
	}
	for kind := range resourceLayouts {
		file := serializedResources(t, manifestResource(kind))[0]
		assert.True(t, isManagedManifest(file.Path, file.Data), kind)
	}
}

func TestFileTransaction_RollsBackCreatesUpdatesDeletesAndDirectories(t *testing.T) {
	directory := t.TempDir()
	component := manifestResource(testComponentKind)
	seedFiles(t, directory, serializedResources(t, component, manifestResource(testWorkloadKind)))
	seedFiles(t, directory, []SerializedFile{{Path: "README.md", Data: []byte("unrelated\n")}})
	before := snapshotTree(t, directory)
	component.Object.(map[string]any)["spec"].(map[string]any)["parameters"] = map[string]any{"replicas": 5}
	files := serializedResources(t, component, manifestResource("Environment"))
	root, err := os.OpenRoot(directory)
	require.NoError(t, err)
	defer root.Close()
	changes, err := planFileChanges(context.Background(), root, files, nil)
	require.NoError(t, err)
	tx := &fileTransaction{root: root, changes: changes}
	require.NoError(t, tx.stage(context.Background()))
	defer func() { _ = root.RemoveAll(tx.scratch) }()
	require.NoError(t, tx.apply(context.Background()))
	assert.FileExists(t, filepath.Join(directory, filepath.FromSlash("namespaces/acme/platform/infra/environments/sample.yaml")))
	assert.NoDirExists(t, filepath.Join(directory, filepath.FromSlash("namespaces/acme/projects/store/components/backend")))
	wantErr := errors.New("export failed")
	err = tx.abort(wantErr, directory)
	require.ErrorIs(t, err, wantErr)
	require.ErrorContains(t, err, "previous files restored")
	require.NoError(t, root.RemoveAll(tx.scratch))
	assert.Equal(t, before, snapshotTree(t, directory))
}

func TestFileTransaction_RollsBackMidApplyFailure(t *testing.T) {
	directory := t.TempDir()
	resources := []Resource{manifestResource("Project"), manifestResource(testComponentKind)}
	seedFiles(t, directory, serializedResources(t, resources...))
	before := snapshotTree(t, directory)
	for _, resource := range resources {
		resource.Object.(map[string]any)["spec"].(map[string]any)["parameters"] = map[string]any{"updated": true}
	}
	root, err := os.OpenRoot(directory)
	require.NoError(t, err)
	defer root.Close()
	changes, err := planFileChanges(context.Background(), root, serializedResources(t, resources...), nil)
	require.NoError(t, err)
	require.Len(t, changes, 2)
	tx := &fileTransaction{root: root, changes: changes}
	require.NoError(t, tx.stage(context.Background()))
	require.NoError(t, root.Remove(tx.stagedName("new", 1)))
	err = tx.apply(context.Background())
	require.Error(t, err)
	assert.True(t, tx.changes[0].installed)
	require.ErrorContains(t, tx.abort(err, directory), "previous files restored")
	require.NoError(t, root.RemoveAll(tx.scratch))
	assert.Equal(t, before, snapshotTree(t, directory))
}

func TestWriteFiles_EmptyExportDeletesManagedManifests(t *testing.T) {
	directory := t.TempDir()
	seedFiles(t, directory, serializedResources(t, manifestResource("Project")))
	seedFiles(t, directory, []SerializedFile{{Path: "README.md", Data: []byte("keep\n")}})
	counts, err := writeFiles(context.Background(), directory, nil, writeOptions{})
	require.NoError(t, err)
	assert.Equal(t, FileCounts{Deleted: 1}, counts)
	assert.NoDirExists(t, filepath.Join(directory, "namespaces"))
	data, err := os.ReadFile(filepath.Join(directory, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "keep\n", string(data))
}

func TestRun_ReexportAllowsDifferentServerWithoutChangingConfig(t *testing.T) {
	f := newFetchFixture(t, false)
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	configPath := filepath.Join(home, ".openchoreo", "config")
	before, err := os.ReadFile(configPath)
	require.NoError(t, err)
	directory := t.TempDir()
	_, err = Run(context.Background(), f.client, Params{OutputDir: directory})
	require.NoError(t, err)
	after, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, before, after, "export must not write directory bindings into CLI config")
	first := snapshotTree(t, directory)
	_, err = Run(context.Background(), f.client, Params{OutputDir: directory})
	require.NoError(t, err)
	assert.Equal(t, first, snapshotTree(t, directory))
	cfg, err := config.LoadStoredConfig()
	require.NoError(t, err)
	clear(f.calls)
	cfg.ControlPlanes[0].URL = "https://other.example.test"
	require.NoError(t, config.SaveStoredConfig(cfg))
	f.host = "other.example.test"
	f.client, err = client.NewClient()
	require.NoError(t, err)
	before, err = os.ReadFile(configPath)
	require.NoError(t, err)
	_, err = Run(context.Background(), f.client, Params{OutputDir: directory})
	require.NoError(t, err)
	assert.NotEmpty(t, f.calls)
	assert.Equal(t, first, snapshotTree(t, directory))
	after, err = os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, before, after, "re-export must not write directory bindings into CLI config")
}
