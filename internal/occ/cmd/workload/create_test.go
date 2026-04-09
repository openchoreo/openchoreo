// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

// --- helpers ---

// setupRepoWithComponent creates a temp repo with a Component and ComponentType.
func setupRepoWithComponent(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()

	testutil.WriteYAML(t, repoDir, "projects/myproj/components/my-svc/component.yaml", `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-svc
  namespace: test-ns
spec:
  owner:
    projectName: myproj
  componentType:
    name: service
    kind: ComponentType
`)

	testutil.WriteYAML(t, repoDir, "platform/component-types/service.yaml", `
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: service
  namespace: test-ns
spec:
  workloadType: deployment
  resources: []
  schema: {}
`)

	return repoDir
}

const expectedBasicWorkloadYAML = `
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: my-svc-workload
  namespace: test-ns
spec:
  owner:
    projectName: myproj
    componentName: my-svc
  container:
    image: registry/my-svc:v1
`

// setupRepoWithWorkload creates a repo that already has a workload for the component.
func setupRepoWithWorkload(t *testing.T) string {
	t.Helper()
	repoDir := setupRepoWithComponent(t)

	testutil.WriteYAML(t, repoDir, "projects/myproj/components/my-svc/workload.yaml", `
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: my-svc-workload
  namespace: test-ns
spec:
  owner:
    projectName: myproj
    componentName: my-svc
  container:
    image: registry/my-svc:v1
`)

	return repoDir
}

// setupRepoWithTwoComponents extends the single-component repo with a second component.
func setupRepoWithTwoComponents(t *testing.T) string {
	t.Helper()
	repoDir := setupRepoWithWorkload(t)

	testutil.WriteYAML(t, repoDir, "projects/myproj/components/my-worker/component.yaml", `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-worker
  namespace: test-ns
spec:
  owner:
    projectName: myproj
  componentType:
    name: service
    kind: ComponentType
`)

	return repoDir
}

// --- Create: file-system mode: new workload (no existing workload) ---

func TestCreate_FileSystem_NewWorkload_DryRun(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoWithComponent(t)
	mc := mocks.NewMockInterface(t)
	w := New(mc)

	out := testutil.CaptureStdout(t, func() {
		err := w.Create(CreateParams{
			Mode:          flags.ModeFileSystem,
			RootDir:       repoDir,
			NamespaceName: "test-ns",
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			ImageURL:      "registry/my-svc:v1",
			DryRun:        true,
		})
		require.NoError(t, err)
	})

	testutil.AssertYAMLEquals(t, expectedBasicWorkloadYAML, testutil.ExtractYAML(out))
}

func TestCreate_FileSystem_NewWorkload_Write(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoWithComponent(t)
	mc := mocks.NewMockInterface(t)
	w := New(mc)

	out := testutil.CaptureStdout(t, func() {
		err := w.Create(CreateParams{
			Mode:          flags.ModeFileSystem,
			RootDir:       repoDir,
			NamespaceName: "test-ns",
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			ImageURL:      "registry/my-svc:v1",
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "Workload written to:")

	// Verify the file was actually written
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var writtenPath string
	for _, l := range lines {
		if strings.HasPrefix(l, "Workload written to: ") {
			writtenPath = strings.TrimPrefix(l, "Workload written to: ")
			break
		}
	}
	require.NotEmpty(t, writtenPath, "expected a 'Workload written to:' line")
	data, err := os.ReadFile(writtenPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "kind: Workload")
	assert.Contains(t, string(data), "registry/my-svc:v1")
}

// --- Create: file-system mode: existing workload (image update) ---

func TestCreate_FileSystem_ExistingWorkload_ImageUpdate_DryRun(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoWithWorkload(t)
	mc := mocks.NewMockInterface(t)
	w := New(mc)

	out := testutil.CaptureStdout(t, func() {
		err := w.Create(CreateParams{
			Mode:          flags.ModeFileSystem,
			RootDir:       repoDir,
			NamespaceName: "test-ns",
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			ImageURL:      "registry/my-svc:v2",
			DryRun:        true,
		})
		require.NoError(t, err)
	})
	// Dry-run should print YAML to stdout, not write to disk
	assert.Contains(t, out, "kind: Workload")
	assert.Contains(t, out, "registry/my-svc:v2")
	assert.NotContains(t, out, "Workload written to:")

	// Verify the existing workload file was NOT modified (still has v1)
	data, err := os.ReadFile(filepath.Join(repoDir, "projects", "myproj", "components", "my-svc", "workload.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "registry/my-svc:v1")
	assert.NotContains(t, string(data), "registry/my-svc:v2")
}

func TestCreate_FileSystem_ExistingWorkload_ImageUpdate_Write(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoWithWorkload(t)
	mc := mocks.NewMockInterface(t)
	w := New(mc)

	out := testutil.CaptureStdout(t, func() {
		err := w.Create(CreateParams{
			Mode:          flags.ModeFileSystem,
			RootDir:       repoDir,
			NamespaceName: "test-ns",
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			ImageURL:      "registry/my-svc:v2",
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "Workload written to:")

	// Read the workload file from the original location — the existing file should be overwritten
	data, err := os.ReadFile(filepath.Join(repoDir, "projects", "myproj", "components", "my-svc", "workload.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "registry/my-svc:v2")
	assert.Contains(t, string(data), "my-svc")
}

// --- Create: file-system mode: custom output path ---

func TestCreate_FileSystem_CustomOutputPath(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoWithComponent(t)
	outDir := filepath.Join(repoDir, "custom-output")
	require.NoError(t, os.MkdirAll(outDir, 0755))
	outFile := filepath.Join(outDir, "my-workload.yaml")

	mc := mocks.NewMockInterface(t)
	w := New(mc)

	out := testutil.CaptureStdout(t, func() {
		err := w.Create(CreateParams{
			Mode:          flags.ModeFileSystem,
			RootDir:       repoDir,
			NamespaceName: "test-ns",
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			ImageURL:      "registry/my-svc:v1",
			OutputPath:    outFile,
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "Workload written to:")
	assert.Contains(t, out, outDir)

	// Verify the file exists at the custom path
	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "kind: Workload")
}

// --- Create: file-system mode: new workload for second component (no existing workload) ---

func TestCreate_FileSystem_SecondComponent_NewWorkload(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoWithTwoComponents(t)
	mc := mocks.NewMockInterface(t)
	w := New(mc)

	out := testutil.CaptureStdout(t, func() {
		err := w.Create(CreateParams{
			Mode:          flags.ModeFileSystem,
			RootDir:       repoDir,
			NamespaceName: "test-ns",
			ProjectName:   "myproj",
			ComponentName: "my-worker",
			ImageURL:      "registry/my-worker:v1",
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "Workload written to:")
	assert.Contains(t, out, "my-worker")

	// Verify a workload was created for my-worker with correct content
	workerWorkloadPath := filepath.Join(repoDir, "projects", "myproj", "components", "my-worker", "workload.yaml")
	data, err := os.ReadFile(workerWorkloadPath)
	require.NoError(t, err)

	expectedYAML := `
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: my-worker-workload
  namespace: test-ns
spec:
  owner:
    projectName: myproj
    componentName: my-worker
  container:
    image: registry/my-worker:v1
`
	testutil.AssertYAMLEquals(t, expectedYAML, string(data))
}

// --- Create: API server mode ---

func TestCreate_APIServer_Stdout(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	w := New(mc)

	out := testutil.CaptureStdout(t, func() {
		err := w.Create(CreateParams{
			Mode:          flags.ModeAPIServer,
			NamespaceName: "test-ns",
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			ImageURL:      "registry/my-svc:v1",
		})
		require.NoError(t, err)
	})

	testutil.AssertYAMLEquals(t, expectedBasicWorkloadYAML, out)
}

func TestCreate_APIServer_DefaultMode(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	w := New(mc)

	// Empty Mode defaults to api-server
	out := testutil.CaptureStdout(t, func() {
		err := w.Create(CreateParams{
			NamespaceName: "test-ns",
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			ImageURL:      "registry/my-svc:v1",
		})
		require.NoError(t, err)
	})

	testutil.AssertYAMLEquals(t, expectedBasicWorkloadYAML, out)
}

func TestCreate_APIServer_OutputFile(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	w := New(mc)

	outFile := filepath.Join(t.TempDir(), "workload.yaml")

	out := testutil.CaptureStdout(t, func() {
		err := w.Create(CreateParams{
			Mode:          flags.ModeAPIServer,
			NamespaceName: "test-ns",
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			ImageURL:      "registry/my-svc:v1",
			OutputPath:    outFile,
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "Workload CR written to")

	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "kind: Workload")
	assert.Contains(t, string(data), "registry/my-svc:v1")
}

func TestCreate_APIServer_WithDescriptor(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	w := New(mc)

	// Write a workload descriptor file
	descriptorDir := t.TempDir()
	descriptorFile := filepath.Join(descriptorDir, "workload-descriptor.yaml")
	descriptor := `container:
  image: registry/my-svc:v1
`
	require.NoError(t, os.WriteFile(descriptorFile, []byte(descriptor), 0600))

	out := testutil.CaptureStdout(t, func() {
		err := w.Create(CreateParams{
			Mode:          flags.ModeAPIServer,
			NamespaceName: "test-ns",
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			ImageURL:      "registry/my-svc:v1",
			FilePath:      descriptorFile,
		})
		require.NoError(t, err)
	})

	testutil.AssertYAMLEquals(t, expectedBasicWorkloadYAML, out)
}

// --- Create: validation errors ---

func TestCreate_FileSystem_MissingImage(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	w := New(mc)
	err := w.Create(CreateParams{
		Mode:          flags.ModeFileSystem,
		NamespaceName: "ns",
		ProjectName:   "proj",
		ComponentName: "comp",
	})
	assert.ErrorContains(t, err, "Missing required parameter")
}

func TestCreate_FileSystem_MissingNamespace(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	w := New(mc)
	err := w.Create(CreateParams{
		Mode:          flags.ModeFileSystem,
		ProjectName:   "proj",
		ComponentName: "comp",
		ImageURL:      "img:latest",
	})
	assert.ErrorContains(t, err, "Missing required parameter")
}

func TestCreate_FileSystem_MissingProject(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	w := New(mc)
	err := w.Create(CreateParams{
		Mode:          flags.ModeFileSystem,
		NamespaceName: "ns",
		ComponentName: "comp",
		ImageURL:      "img:latest",
	})
	assert.ErrorContains(t, err, "Missing required parameter")
}

func TestCreate_FileSystem_MissingComponent(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	w := New(mc)
	err := w.Create(CreateParams{
		Mode:          flags.ModeFileSystem,
		NamespaceName: "ns",
		ProjectName:   "proj",
		ImageURL:      "img:latest",
	})
	assert.ErrorContains(t, err, "Missing required parameter")
}
