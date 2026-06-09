// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/generator"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/pkg/fsindex/cache"
)

// --- Generate: mode validation (no filesystem needed) ---

func TestGenerate_DefaultModeRejectsAPIServer(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	rb := New(mc)
	// Empty Mode defaults to "api-server" → rejected
	err := rb.Generate(GenerateParams{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "releasebinding generate only supports file-system mode")
	assert.Contains(t, err.Error(), `"api-server"`)
}

func TestGenerate_ExplicitAPIServerModeRejected(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	rb := New(mc)
	err := rb.Generate(GenerateParams{Mode: flags.ModeAPIServer})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "releasebinding generate only supports file-system mode")
}

func TestGenerate_InvalidModeRejected(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	rb := New(mc)
	err := rb.Generate(GenerateParams{Mode: "invalid-mode"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "releasebinding generate only supports file-system mode")
	assert.Contains(t, err.Error(), `"invalid-mode"`)
}

// --- Generate: config loading ---

func TestGenerate_NoConfigFile_NoCurrentContext(t *testing.T) {
	testutil.SetupTestHome(t)
	mc := mocks.NewMockInterface(t)
	rb := New(mc)
	// file-system mode, but no config file → LoadStoredConfig returns empty → "no current context set"
	err := rb.Generate(GenerateParams{Mode: flags.ModeFileSystem})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no current context set")
}

// --- Generate: context resolution ---

func TestGenerate_EmptyCurrentContext(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "",
		Contexts:       []config.Context{},
	})

	mc := mocks.NewMockInterface(t)
	rb := New(mc)
	err := rb.Generate(GenerateParams{Mode: flags.ModeFileSystem})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no current context set")
}

func TestGenerate_CurrentContextNotFound(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "missing-ctx",
		Contexts: []config.Context{
			{Name: "other-ctx", Namespace: "ns"},
		},
	})

	mc := mocks.NewMockInterface(t)
	rb := New(mc)
	err := rb.Generate(GenerateParams{Mode: flags.ModeFileSystem})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"missing-ctx" not found in config`)
}

// --- Generate: namespace validation ---

func TestGenerate_EmptyNamespaceInContext(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts: []config.Context{
			{Name: "my-ctx", Namespace: ""},
		},
	})

	// Need a repo dir with at least one YAML file for index building
	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "dummy.yaml"), []byte("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: x\n"), 0600))

	mc := mocks.NewMockInterface(t)
	rb := New(mc)
	err := rb.Generate(GenerateParams{
		Mode:    flags.ModeFileSystem,
		RootDir: repoDir,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace is required in context")
}

// --- Generate: pipeline derivation errors ---

func TestGenerate_PipelineNotFound(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts: []config.Context{
			{Name: "my-ctx", Namespace: "test-ns"},
		},
	})

	// Set up a minimal repo with a Project resource
	repoDir := t.TempDir()
	projectYAML := `apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: my-proj
  namespace: test-ns
spec:
  deploymentPipelineRef:
    name: nonexistent-pipeline
`
	projectDir := filepath.Join(repoDir, "projects", "my-proj")
	require.NoError(t, os.MkdirAll(projectDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "project.yaml"), []byte(projectYAML), 0600))

	mc := mocks.NewMockInterface(t)
	rb := New(mc)
	err := rb.Generate(GenerateParams{
		Mode:        flags.ModeFileSystem,
		RootDir:     repoDir,
		ProjectName: "my-proj",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `deployment pipeline "nonexistent-pipeline" not found`)
}

func TestGenerate_UsePipelineRequiredForAll(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts: []config.Context{
			{Name: "my-ctx", Namespace: "test-ns"},
		},
	})

	repoDir := t.TempDir()
	// empty repo is fine — deriveUsePipeline is checked before index-dependent pipeline lookup
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "dummy.yaml"), []byte("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: x\n"), 0600))

	mc := mocks.NewMockInterface(t)
	rb := New(mc)
	err := rb.Generate(GenerateParams{
		Mode:    flags.ModeFileSystem,
		RootDir: repoDir,
		All:     true,
		// No UsePipeline specified
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--use-pipeline is required when using --all")
}

func TestGenerate_TargetEnvRequiredForAll(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts: []config.Context{
			{Name: "my-ctx", Namespace: "test-ns"},
		},
	})

	// Set up repo with a DeploymentPipeline
	repoDir := t.TempDir()
	pipelineYAML := `apiVersion: openchoreo.dev/v1alpha1
kind: DeploymentPipeline
metadata:
  name: my-pipeline
  namespace: test-ns
spec:
  promotionPaths:
    - sourceEnvironmentRef:
        name: dev
      targetEnvironmentRefs:
        - name: staging
`
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, "pipelines"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "pipelines", "my-pipeline.yaml"), []byte(pipelineYAML), 0600))

	mc := mocks.NewMockInterface(t)
	rb := New(mc)
	err := rb.Generate(GenerateParams{
		Mode:        flags.ModeFileSystem,
		RootDir:     repoDir,
		All:         true,
		UsePipeline: "my-pipeline",
		// No TargetEnv specified + --all → error
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--target-env is required when using --all")
}

func TestGenerate_InvalidTargetEnv(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts: []config.Context{
			{Name: "my-ctx", Namespace: "test-ns"},
		},
	})

	// Set up repo with a DeploymentPipeline
	repoDir := t.TempDir()
	pipelineYAML := `apiVersion: openchoreo.dev/v1alpha1
kind: DeploymentPipeline
metadata:
  name: my-pipeline
  namespace: test-ns
spec:
  promotionPaths:
    - sourceEnvironmentRef:
        name: dev
      targetEnvironmentRefs:
        - name: staging
`
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, "pipelines"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "pipelines", "my-pipeline.yaml"), []byte(pipelineYAML), 0600))

	mc := mocks.NewMockInterface(t)
	rb := New(mc)
	err := rb.Generate(GenerateParams{
		Mode:        flags.ModeFileSystem,
		RootDir:     repoDir,
		ProjectName: "some-proj",
		UsePipeline: "my-pipeline",
		TargetEnv:   "nonexistent-env",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid target environment")
}

// --- Generate: loadReleaseConfig ---

func TestLoadReleaseConfig_NotFound_NotRequired(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	rb := New(mc)
	cfg, err := rb.loadReleaseConfig(t.TempDir(), false)
	require.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestLoadReleaseConfig_NotFound_Required(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	rb := New(mc)
	dir := t.TempDir()
	_, err := rb.loadReleaseConfig(dir, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "release-config.yaml not found")
	assert.Contains(t, err.Error(), "required for --all or --project operations")
}

func TestLoadReleaseConfig_ValidFile(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	rb := New(mc)
	dir := t.TempDir()
	content := `apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseConfig
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release-config.yaml"), []byte(content), 0600))
	cfg, err := rb.loadReleaseConfig(dir, false)
	require.NoError(t, err)
	assert.NotNil(t, cfg)
}

func TestLoadReleaseConfig_InvalidYAML(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	rb := New(mc)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release-config.yaml"), []byte("{{invalid"), 0600))
	_, err := rb.loadReleaseConfig(dir, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load release-config.yaml")
}

// --- Generate: printYAML ---

func TestPrintYAML_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	rb := New(mc)
	data := map[string]string{"name": "test"}
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, rb.printYAML(data))
	})
	assert.Contains(t, out, "name: test")
}

// ===========================================================================
// Happy-path tests for generateForComponent, generateForProject, generateAll
// ===========================================================================

// setupRepoForBinding creates a temp repo with a Component, ComponentType,
// Workload, a matching ComponentRelease, a DeploymentPipeline, and a Project.
// The ComponentRelease is generated programmatically via the ReleaseGenerator
// so that the hash-based spec comparison succeeds.
func setupRepoForBinding(t *testing.T) string {
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

	// DeploymentPipeline: dev → staging
	testutil.WriteYAML(t, repoDir, "platform/pipelines/my-pipeline.yaml", `
apiVersion: openchoreo.dev/v1alpha1
kind: DeploymentPipeline
metadata:
  name: my-pipeline
  namespace: test-ns
spec:
  promotionPaths:
    - sourceEnvironmentRef:
        name: dev
      targetEnvironmentRefs:
        - name: staging
`)

	// Project references the pipeline
	testutil.WriteYAML(t, repoDir, "projects/myproj/project.yaml", `
apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: myproj
  namespace: test-ns
spec:
  deploymentPipelineRef:
    name: my-pipeline
`)

	// Generate a matching ComponentRelease programmatically
	generateMatchingRelease(t, repoDir, "test-ns", "myproj", "my-svc", "my-svc-release-1",
		"projects/myproj/components/my-svc/releases/my-svc-release-1.yaml")

	return repoDir
}

// generateMatchingRelease builds the index from YAML files on disk,
// uses the ReleaseGenerator to produce a ComponentRelease whose spec
// matches the current component state, then writes it to disk.
func generateMatchingRelease(t *testing.T, repoDir, namespace, project, component, releaseName, relPath string) {
	t.Helper()
	pi, err := cache.LoadOrBuild(repoDir)
	require.NoError(t, err)

	ocIndex := fsmode.WrapIndex(pi.Index)
	gen := generator.NewReleaseGenerator(ocIndex)
	release, err := gen.GenerateRelease(generator.ReleaseOptions{
		ComponentName: component,
		ProjectName:   project,
		Namespace:     namespace,
		ReleaseName:   releaseName,
	})
	require.NoError(t, err)

	data, err := sigsyaml.Marshal(release.Object)
	require.NoError(t, err)
	testutil.WriteYAML(t, repoDir, relPath, string(data))
}

// setupRepoForBindingTwoComponents extends the single-component repo with a second component.
func setupRepoForBindingTwoComponents(t *testing.T) string {
	t.Helper()
	repoDir := setupRepoForBinding(t)

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

	testutil.WriteYAML(t, repoDir, "projects/myproj/components/my-worker/workload.yaml", `
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

	// Generate matching ComponentRelease for my-worker
	generateMatchingRelease(t, repoDir, "test-ns", "myproj", "my-worker", "my-worker-release-1",
		"projects/myproj/components/my-worker/releases/my-worker-release-1.yaml")

	return repoDir
}

// --- generateForComponent: dry-run ---

func TestGenerate_SingleComponent_DryRun(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoForBinding(t)
	mc := mocks.NewMockInterface(t)
	rb := New(mc)

	out := testutil.CaptureStdout(t, func() {
		err := rb.Generate(GenerateParams{
			Mode:          flags.ModeFileSystem,
			RootDir:       repoDir,
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			TargetEnv:     "dev",
			DryRun:        true,
		})
		require.NoError(t, err)
	})

	expectedYAML := `
apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseBinding
metadata:
  name: my-svc-dev
  namespace: test-ns
spec:
  environment: dev
  owner:
    componentName: my-svc
    projectName: myproj
  releaseName: my-svc-release-1
`
	testutil.AssertYAMLEquals(t, expectedYAML, testutil.ExtractYAML(out))
}

// --- generateForComponent: write to disk ---

func TestGenerate_SingleComponent_Write(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoForBinding(t)
	mc := mocks.NewMockInterface(t)
	rb := New(mc)

	out := testutil.CaptureStdout(t, func() {
		err := rb.Generate(GenerateParams{
			Mode:          flags.ModeFileSystem,
			RootDir:       repoDir,
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			TargetEnv:     "dev",
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "Created:")

	// Verify the YAML file was actually written
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var writtenPath string
	for _, l := range lines {
		if strings.HasPrefix(l, "Created: ") {
			writtenPath = strings.TrimPrefix(l, "Created: ")
			break
		}
	}
	require.NotEmpty(t, writtenPath, "expected a Created: line")
	_, err := os.Stat(writtenPath)
	assert.NoError(t, err, "written file should exist on disk")
}

// --- generateForComponent: write with custom output path ---

func TestGenerate_SingleComponent_CustomOutputPath(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoForBinding(t)
	outDir := filepath.Join(t.TempDir(), "custom-out")

	mc := mocks.NewMockInterface(t)
	rb := New(mc)

	out := testutil.CaptureStdout(t, func() {
		err := rb.Generate(GenerateParams{
			Mode:          flags.ModeFileSystem,
			RootDir:       repoDir,
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			TargetEnv:     "dev",
			OutputPath:    outDir,
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "Created:")
	assert.Contains(t, out, outDir)
}

// --- generateForProject: dry-run ---

func TestGenerate_Project_DryRun(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoForBindingTwoComponents(t)
	mc := mocks.NewMockInterface(t)
	rb := New(mc)

	out := testutil.CaptureStdout(t, func() {
		err := rb.Generate(GenerateParams{
			Mode:        flags.ModeFileSystem,
			RootDir:     repoDir,
			ProjectName: "myproj",
			TargetEnv:   "dev",
			DryRun:      true,
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "kind: ReleaseBinding")
	assert.Contains(t, out, "my-svc")
	assert.Contains(t, out, "my-worker")
	assert.Contains(t, out, "Summary:")
}

// --- generateForProject: write ---

func TestGenerate_Project_Write(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoForBindingTwoComponents(t)
	mc := mocks.NewMockInterface(t)
	rb := New(mc)

	out := testutil.CaptureStdout(t, func() {
		err := rb.Generate(GenerateParams{
			Mode:        flags.ModeFileSystem,
			RootDir:     repoDir,
			ProjectName: "myproj",
			TargetEnv:   "dev",
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "Created:")
	assert.Contains(t, out, "my-svc")
	assert.Contains(t, out, "my-worker")
	assert.Contains(t, out, "Summary:")
	// Verify exactly two files were created on disk
	var createdPaths []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(l, "Created: ") {
			createdPaths = append(createdPaths, strings.TrimPrefix(l, "Created: "))
		}
	}
	assert.Len(t, createdPaths, 2, "expected two created files")
	for _, p := range createdPaths {
		_, statErr := os.Stat(p)
		assert.NoError(t, statErr, "created file should exist: %s", p)
	}
}

// --- generateAll: dry-run ---

func TestGenerate_All_DryRun(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoForBindingTwoComponents(t)
	mc := mocks.NewMockInterface(t)
	rb := New(mc)

	out := testutil.CaptureStdout(t, func() {
		err := rb.Generate(GenerateParams{
			Mode:        flags.ModeFileSystem,
			RootDir:     repoDir,
			All:         true,
			UsePipeline: "my-pipeline",
			TargetEnv:   "dev",
			DryRun:      true,
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "kind: ReleaseBinding")
	assert.Contains(t, out, "my-svc")
	assert.Contains(t, out, "my-worker")
	assert.Contains(t, out, "Summary:")
}

// --- generateAll: write ---

func TestGenerate_All_Write(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoForBindingTwoComponents(t)
	mc := mocks.NewMockInterface(t)
	rb := New(mc)

	out := testutil.CaptureStdout(t, func() {
		err := rb.Generate(GenerateParams{
			Mode:        flags.ModeFileSystem,
			RootDir:     repoDir,
			All:         true,
			UsePipeline: "my-pipeline",
			TargetEnv:   "dev",
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "Created:")
	assert.Contains(t, out, "my-svc")
	assert.Contains(t, out, "my-worker")
	assert.Contains(t, out, "Summary:")
	// Verify exactly two files were created on disk
	var createdPaths []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(l, "Created: ") {
			createdPaths = append(createdPaths, strings.TrimPrefix(l, "Created: "))
		}
	}
	assert.Len(t, createdPaths, 2, "expected two created files")
	for _, p := range createdPaths {
		_, statErr := os.Stat(p)
		assert.NoError(t, statErr, "created file should exist: %s", p)
	}
}

// --- generate: pipeline derived from project ---

func TestGenerate_PipelineDerivedFromProject(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoForBinding(t)
	mc := mocks.NewMockInterface(t)
	rb := New(mc)

	// Don't specify UsePipeline — it should be derived from the Project's deploymentPipelineRef
	out := testutil.CaptureStdout(t, func() {
		err := rb.Generate(GenerateParams{
			Mode:          flags.ModeFileSystem,
			RootDir:       repoDir,
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			TargetEnv:     "dev",
			DryRun:        true,
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "kind: ReleaseBinding")
	assert.Contains(t, out, "deploymentPipelineRef: my-pipeline")
}

// --- generate: target env defaults to root ---

func TestGenerate_TargetEnvDefaultsToRoot(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})

	repoDir := setupRepoForBinding(t)
	mc := mocks.NewMockInterface(t)
	rb := New(mc)

	// Don't specify TargetEnv — it should default to pipeline root ("dev")
	out := testutil.CaptureStdout(t, func() {
		err := rb.Generate(GenerateParams{
			Mode:          flags.ModeFileSystem,
			RootDir:       repoDir,
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			DryRun:        true,
		})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "kind: ReleaseBinding")
	assert.Contains(t, out, "environment: dev")
}
