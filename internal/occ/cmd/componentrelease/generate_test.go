// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/componentrelease/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	th "github.com/openchoreo/openchoreo/internal/occ/testhelpers"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

// --- Generate: mode validation (no filesystem needed) ---

func TestGenerate_DefaultModeRejectsAPIServer(t *testing.T) {
	mc := mocks.NewMockClient(t)
	cr := New(mc)
	// Empty Mode defaults to "api-server" → rejected
	err := cr.Generate(GenerateParams{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "componentrelease generate only supports file-system mode")
	assert.Contains(t, err.Error(), `"api-server"`)
}

func TestGenerate_ExplicitAPIServerModeRejected(t *testing.T) {
	mc := mocks.NewMockClient(t)
	cr := New(mc)
	err := cr.Generate(GenerateParams{Mode: flags.ModeAPIServer})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "componentrelease generate only supports file-system mode")
}

func TestGenerate_InvalidModeRejected(t *testing.T) {
	mc := mocks.NewMockClient(t)
	cr := New(mc)
	err := cr.Generate(GenerateParams{Mode: "invalid-mode"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "componentrelease generate only supports file-system mode")
	assert.Contains(t, err.Error(), `"invalid-mode"`)
}

// --- Generate: config loading ---

func TestGenerate_NoConfigFile_NoCurrentContext(t *testing.T) {
	th.SetupTestHome(t)
	mc := mocks.NewMockClient(t)
	cr := New(mc)
	// file-system mode, but no config file → LoadStoredConfig returns empty → "no current context set"
	err := cr.Generate(GenerateParams{Mode: flags.ModeFileSystem})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no current context set")
}

// --- Generate: context resolution ---

func TestGenerate_EmptyCurrentContext(t *testing.T) {
	home := th.SetupTestHome(t)
	th.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "",
		Contexts:       []config.Context{},
	})

	mc := mocks.NewMockClient(t)
	cr := New(mc)
	err := cr.Generate(GenerateParams{Mode: flags.ModeFileSystem})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no current context set")
}

func TestGenerate_CurrentContextNotFound(t *testing.T) {
	home := th.SetupTestHome(t)
	th.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "missing-ctx",
		Contexts: []config.Context{
			{Name: "other-ctx", Namespace: "ns"},
		},
	})

	mc := mocks.NewMockClient(t)
	cr := New(mc)
	err := cr.Generate(GenerateParams{Mode: flags.ModeFileSystem})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"missing-ctx" not found in config`)
}

// --- Generate: namespace validation ---

func TestGenerate_EmptyNamespaceInContext(t *testing.T) {
	home := th.SetupTestHome(t)
	th.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts: []config.Context{
			{Name: "my-ctx", Namespace: ""},
		},
	})

	// Need a repo dir with at least one YAML file for index building
	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "dummy.yaml"), []byte("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: x\n"), 0600))

	mc := mocks.NewMockClient(t)
	cr := New(mc)
	err := cr.Generate(GenerateParams{
		Mode:    flags.ModeFileSystem,
		RootDir: repoDir,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace is required in context")
}

// --- Generate: component requires project ---

func TestGenerate_ComponentRequiresProject(t *testing.T) {
	home := th.SetupTestHome(t)
	th.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts: []config.Context{
			{Name: "my-ctx", Namespace: "test-ns"},
		},
	})

	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "dummy.yaml"), []byte("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: x\n"), 0600))

	mc := mocks.NewMockClient(t)
	cr := New(mc)
	err := cr.Generate(GenerateParams{
		Mode:          flags.ModeFileSystem,
		RootDir:       repoDir,
		ComponentName: "my-comp",
		// ProjectName intentionally omitted
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project name is required when specifying --component")
}

// --- Generate: loadReleaseConfig ---

func TestLoadReleaseConfig_NotFound_NotRequired(t *testing.T) {
	mc := mocks.NewMockClient(t)
	cr := New(mc)
	cfg, err := cr.loadReleaseConfig(t.TempDir(), false)
	require.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestLoadReleaseConfig_NotFound_Required(t *testing.T) {
	mc := mocks.NewMockClient(t)
	cr := New(mc)
	dir := t.TempDir()
	_, err := cr.loadReleaseConfig(dir, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "release-config.yaml not found")
	assert.Contains(t, err.Error(), "required for --all or --project operations")
}

func TestLoadReleaseConfig_ValidFile(t *testing.T) {
	mc := mocks.NewMockClient(t)
	cr := New(mc)
	dir := t.TempDir()
	content := `apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseConfig
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release-config.yaml"), []byte(content), 0600))
	cfg, err := cr.loadReleaseConfig(dir, false)
	require.NoError(t, err)
	assert.NotNil(t, cfg)
}

func TestLoadReleaseConfig_InvalidYAML(t *testing.T) {
	mc := mocks.NewMockClient(t)
	cr := New(mc)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release-config.yaml"), []byte("{{invalid"), 0600))
	_, err := cr.loadReleaseConfig(dir, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load release-config.yaml")
}

// --- Generate: printYAML ---

func TestPrintYAML_Success(t *testing.T) {
	mc := mocks.NewMockClient(t)
	cr := New(mc)
	data := map[string]string{"name": "test"}
	out := captureStdout(t, func() {
		require.NoError(t, cr.printYAML(data))
	})
	assert.Contains(t, out, "name: test")
}

// --- Generate: no-op when no scope specified ---

func TestGenerate_NoScope_ReturnsNil(t *testing.T) {
	home := th.SetupTestHome(t)
	th.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts: []config.Context{
			{Name: "my-ctx", Namespace: "test-ns"},
		},
	})

	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "dummy.yaml"), []byte("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: x\n"), 0600))

	mc := mocks.NewMockClient(t)
	cr := New(mc)
	// No All, no ComponentName, no ProjectName → falls through to return nil
	err := cr.Generate(GenerateParams{
		Mode:    flags.ModeFileSystem,
		RootDir: repoDir,
	})
	assert.NoError(t, err)
}

// ===========================================================================
// Happy-path tests for generateForComponent, generateForProject, generateAll
// ===========================================================================

// setupRepoWithComponent creates a temp repo directory containing the minimal
// resources needed for the ComponentRelease generator: a Component, a
// ComponentType and a Workload.
func setupRepoWithComponent(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()

	th.WriteYAML(t, repoDir, "projects/myproj/components/my-svc/component.yaml", `
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

	th.WriteYAML(t, repoDir, "platform/component-types/service.yaml", `
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: service
  namespace: test-ns
spec:
  workloadType: deployment
  resources:
    - id: deployment
      template:
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: my-svc
  schema: {}
`)

	th.WriteYAML(t, repoDir, "projects/myproj/components/my-svc/workload.yaml", `
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
	repoDir := setupRepoWithComponent(t)

	th.WriteYAML(t, repoDir, "projects/myproj/components/my-worker/component.yaml", `
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

	th.WriteYAML(t, repoDir, "projects/myproj/components/my-worker/workload.yaml", `
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
`)

	return repoDir
}

// --- generateForComponent: dry-run ---

func TestGenerate_SingleComponent_DryRun(t *testing.T) {
	home := th.SetupTestHome(t)
	th.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoWithComponent(t)
	mc := mocks.NewMockClient(t)
	cr := New(mc)

	out := captureStdout(t, func() {
		err := cr.Generate(GenerateParams{
			Mode:          flags.ModeFileSystem,
			RootDir:       repoDir,
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			ReleaseName:   "my-svc-release-1",
			DryRun:        true,
		})
		require.NoError(t, err)
	})

	expectedYAML := `
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentRelease
metadata:
  name: my-svc-release-1
  namespace: test-ns
spec:
  componentType:
    kind: ComponentType
    name: service
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: my-svc
      workloadType: deployment
  owner:
    componentName: my-svc
    projectName: myproj
  workload:
    container:
      image: registry/my-svc:v1
`
	th.AssertYAMLEquals(t, expectedYAML, th.ExtractYAML(out))
}

// --- generateForComponent: write to disk ---

func TestGenerate_SingleComponent_Write(t *testing.T) {
	home := th.SetupTestHome(t)
	th.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoWithComponent(t)
	mc := mocks.NewMockClient(t)
	cr := New(mc)

	out := captureStdout(t, func() {
		err := cr.Generate(GenerateParams{
			Mode:          flags.ModeFileSystem,
			RootDir:       repoDir,
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			DryRun:        false,
		})
		require.NoError(t, err)
	})
	// Should print "Generated: <path>"
	assert.Contains(t, out, "Generated:")

	// Verify the YAML file was actually written
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var writtenPath string
	for _, l := range lines {
		if strings.HasPrefix(l, "Generated: ") {
			writtenPath = strings.TrimPrefix(l, "Generated: ")
			break
		}
	}
	require.NotEmpty(t, writtenPath, "expected a Generated: line")
	_, err := os.Stat(writtenPath)
	assert.NoError(t, err, "written file should exist on disk")
}

// --- generateForComponent: write with custom output path ---

func TestGenerate_SingleComponent_CustomOutputPath(t *testing.T) {
	home := th.SetupTestHome(t)
	th.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoWithComponent(t)
	outDir := filepath.Join(t.TempDir(), "custom-out")

	mc := mocks.NewMockClient(t)
	cr := New(mc)

	out := captureStdout(t, func() {
		err := cr.Generate(GenerateParams{
			Mode:          flags.ModeFileSystem,
			RootDir:       repoDir,
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			OutputPath:    outDir,
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "Generated:")
	// The file should be under the custom output directory
	assert.Contains(t, out, outDir)
}

// --- generateForProject: dry-run ---

func TestGenerate_Project_DryRun(t *testing.T) {
	home := th.SetupTestHome(t)
	th.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoWithTwoComponents(t)
	mc := mocks.NewMockClient(t)
	cr := New(mc)

	out := captureStdout(t, func() {
		err := cr.Generate(GenerateParams{
			Mode:        flags.ModeFileSystem,
			RootDir:     repoDir,
			ProjectName: "myproj",
			DryRun:      true,
		})
		require.NoError(t, err)
	})
	// Should print release YAML for both components
	assert.Contains(t, out, "kind: ComponentRelease")
	assert.Contains(t, out, "my-svc")
	assert.Contains(t, out, "my-worker")
	assert.Contains(t, out, "Summary:")
}

// --- generateForProject: write ---

func TestGenerate_Project_Write(t *testing.T) {
	home := th.SetupTestHome(t)
	th.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoWithTwoComponents(t)
	mc := mocks.NewMockClient(t)
	cr := New(mc)

	out := captureStdout(t, func() {
		err := cr.Generate(GenerateParams{
			Mode:        flags.ModeFileSystem,
			RootDir:     repoDir,
			ProjectName: "myproj",
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "Generated:")
	assert.Contains(t, out, "my-svc")
	assert.Contains(t, out, "my-worker")
	assert.Contains(t, out, "Summary:")
	// Verify exactly two generated files exist on disk
	var generatedPaths []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(l, "Generated: ") {
			generatedPaths = append(generatedPaths, strings.TrimPrefix(l, "Generated: "))
		}
	}
	assert.Len(t, generatedPaths, 2, "expected two generated files")
	for _, p := range generatedPaths {
		_, err := os.Stat(p)
		assert.NoError(t, err, "generated file should exist: %s", p)
	}
}

// --- generateAll: dry-run ---

func TestGenerate_All_DryRun(t *testing.T) {
	home := th.SetupTestHome(t)
	th.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoWithTwoComponents(t)
	mc := mocks.NewMockClient(t)
	cr := New(mc)

	out := captureStdout(t, func() {
		err := cr.Generate(GenerateParams{
			Mode:    flags.ModeFileSystem,
			RootDir: repoDir,
			All:     true,
			DryRun:  true,
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "kind: ComponentRelease")
	assert.Contains(t, out, "my-svc")
	assert.Contains(t, out, "my-worker")
	assert.Contains(t, out, "Summary:")
}

// --- generateAll: write ---

func TestGenerate_All_Write(t *testing.T) {
	home := th.SetupTestHome(t)
	th.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoWithTwoComponents(t)
	mc := mocks.NewMockClient(t)
	cr := New(mc)

	out := captureStdout(t, func() {
		err := cr.Generate(GenerateParams{
			Mode:    flags.ModeFileSystem,
			RootDir: repoDir,
			All:     true,
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "Generated:")
	assert.Contains(t, out, "my-svc")
	assert.Contains(t, out, "my-worker")
	assert.Contains(t, out, "Summary:")
	// Verify exactly two generated files exist on disk
	var generatedPaths []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(l, "Generated: ") {
			generatedPaths = append(generatedPaths, strings.TrimPrefix(l, "Generated: "))
		}
	}
	assert.Len(t, generatedPaths, 2, "expected two generated files")
	for _, p := range generatedPaths {
		_, err := os.Stat(p)
		assert.NoError(t, err, "generated file should exist: %s", p)
	}
}

// --- generateForComponent: duplicate release name ---

func TestGenerate_SingleComponent_DuplicateReleaseName(t *testing.T) {
	home := th.SetupTestHome(t)
	th.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoWithComponent(t)

	// First: generate a release with a custom name
	mc := mocks.NewMockClient(t)
	cr := New(mc)

	captureStdout(t, func() {
		err := cr.Generate(GenerateParams{
			Mode:          flags.ModeFileSystem,
			RootDir:       repoDir,
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			ReleaseName:   "fixed-release",
		})
		require.NoError(t, err)
	})

	// Second: try to generate again with the same custom name — should error
	err := cr.Generate(GenerateParams{
		Mode:          flags.ModeFileSystem,
		RootDir:       repoDir,
		ProjectName:   "myproj",
		ComponentName: "my-svc",
		ReleaseName:   "fixed-release",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}
