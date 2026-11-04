// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

const expectedLineWorkloadYAML = `
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: my-svc-next-1ddb7a65-workload
  namespace: test-ns
spec:
  owner:
    projectName: myproj
    componentName: my-svc
    releaseLine: next
  container:
    image: registry/my-svc:next
`

// setupRepoWithReleaseLine extends the repo with the release line "next" of my-svc.
func setupRepoWithReleaseLine(t *testing.T, withLineWorkload bool) string {
	t.Helper()
	repoDir := setupRepoWithWorkload(t)

	testutil.WriteYAML(t, repoDir, "projects/myproj/components/my-svc/release-line-next.yaml", `
apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseLine
metadata:
  name: next
  namespace: test-ns
spec:
  owner:
    projectName: myproj
    componentName: my-svc
`)
	if withLineWorkload {
		testutil.WriteYAML(t, repoDir, "projects/myproj/components/my-svc/next/workload.yaml", `
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: my-svc-next-1ddb7a65-workload
  namespace: test-ns
spec:
  owner:
    projectName: myproj
    componentName: my-svc
    releaseLine: next
  container:
    image: registry/my-svc:next-old
`)
	}
	return repoDir
}

func createInRepo(t *testing.T, repoDir, line, image string) {
	t.Helper()
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "my-ctx",
		Contexts:       []config.Context{{Name: "my-ctx", Namespace: "test-ns"}},
	})
	testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mocks.NewMockInterface(t)).Create(CreateParams{
			Mode:          flags.ModeFileSystem,
			RootDir:       repoDir,
			NamespaceName: "test-ns",
			ProjectName:   "myproj",
			ComponentName: "my-svc",
			ReleaseLine:   line,
			ImageURL:      image,
		}))
	})
}

func readRepoFile(t *testing.T, repoDir, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoDir, path))
	require.NoError(t, err)
	return string(data)
}

// Each line's build updates its own Workload file and leaves the other line's alone.
func TestCreate_FileSystem_ReleaseLineUpdatesItsWorkload(t *testing.T) {
	const mainPath = "projects/myproj/components/my-svc/workload.yaml"
	const linePath = "projects/myproj/components/my-svc/next/workload.yaml"

	repoDir := setupRepoWithReleaseLine(t, true)
	createInRepo(t, repoDir, "next", "registry/my-svc:next-new")
	assert.Contains(t, readRepoFile(t, repoDir, linePath), "registry/my-svc:next-new")
	assert.Contains(t, readRepoFile(t, repoDir, mainPath), "registry/my-svc:v1")

	repoDir = setupRepoWithReleaseLine(t, true)
	createInRepo(t, repoDir, "main", "registry/my-svc:v2")
	assert.Contains(t, readRepoFile(t, repoDir, mainPath), "registry/my-svc:v2")
	assert.Contains(t, readRepoFile(t, repoDir, linePath), "registry/my-svc:next-old")
}

func TestCreate_FileSystem_ReleaseLineNewWorkload(t *testing.T) {
	repoDir := setupRepoWithReleaseLine(t, false)
	createInRepo(t, repoDir, "next", "registry/my-svc:next")

	written := readRepoFile(t, repoDir, "projects/myproj/components/my-svc/workload-next.yaml")
	testutil.AssertYAMLEquals(t, expectedLineWorkloadYAML, written)
	assert.Contains(t, readRepoFile(t, repoDir, "projects/myproj/components/my-svc/workload.yaml"), "registry/my-svc:v1")
}

func TestCreate_FileSystem_UnknownReleaseLine(t *testing.T) {
	repoDir := setupRepoWithReleaseLine(t, false)
	err := New(mocks.NewMockInterface(t)).Create(CreateParams{
		Mode:          flags.ModeFileSystem,
		RootDir:       repoDir,
		NamespaceName: "test-ns",
		ProjectName:   "myproj",
		ComponentName: "my-svc",
		ReleaseLine:   "canary",
		ImageURL:      "registry/my-svc:canary",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `release line "canary" of component "my-svc"`)
}

func TestCreate_APIServer_ReleaseLine(t *testing.T) {
	for line, want := range map[string]string{"next": expectedLineWorkloadYAML, "main": expectedBasicWorkloadYAML} {
		t.Run(line, func(t *testing.T) {
			image := "registry/my-svc:v1"
			if line == "next" {
				image = "registry/my-svc:next"
			}
			out := testutil.CaptureStdout(t, func() {
				require.NoError(t, New(mocks.NewMockInterface(t)).Create(CreateParams{
					Mode:          flags.ModeAPIServer,
					NamespaceName: "test-ns",
					ProjectName:   "myproj",
					ComponentName: "my-svc",
					ReleaseLine:   line,
					ImageURL:      image,
				}))
			})
			testutil.AssertYAMLEquals(t, want, out)
		})
	}
}
