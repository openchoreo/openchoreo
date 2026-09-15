// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

func TestParseReleaseName(t *testing.T) {
	tests := []struct {
		name          string
		releaseName   string
		wantComponent string
		wantDate      string
		wantVersion   string
		wantErr       string
	}{
		{
			name:          "valid simple name",
			releaseName:   "my-comp-20250101-1",
			wantComponent: "my-comp",
			wantDate:      "20250101",
			wantVersion:   "1",
		},
		{
			name:          "hyphenated component name",
			releaseName:   "my-web-app-20250315-42",
			wantComponent: "my-web-app",
			wantDate:      "20250315",
			wantVersion:   "42",
		},
		{
			name:        "too few parts",
			releaseName: "nodate",
			wantErr:     "invalid release name format",
		},
		{
			name:        "invalid date",
			releaseName: "comp-notadate-1",
			wantErr:     "invalid date part",
		},
		{
			name:        "invalid version",
			releaseName: "comp-20250101-abc",
			wantErr:     "invalid version part",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp, date, version, err := ParseReleaseName(tt.releaseName)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantComponent, comp)
				assert.Equal(t, tt.wantDate, date)
				assert.Equal(t, tt.wantVersion, version)
			}
		})
	}
}

const (
	testNamespace = "team-a"
	testProject   = "proj"
)

// makeTestIndex builds an in-memory fsmode.Index with ComponentRelease entries in
// the default test namespace/project. Each release is owned by the component parsed
// from its release name (falling back to the full name when unparseable) so it is
// indexed for owner-scoped lookups.
func makeTestIndex(t *testing.T, releaseNames []string) *fsmode.Index {
	t.Helper()
	return makeTestIndexNS(t, testNamespace, testProject, releaseNames)
}

// makeTestIndexNS is like makeTestIndex but places releases in a specific namespace/project.
func makeTestIndexNS(t *testing.T, namespace, projectName string, releaseNames []string) *fsmode.Index {
	t.Helper()
	idx := index.New("/test")
	addReleasesNS(t, idx, namespace, projectName, releaseNames)
	return fsmode.WrapIndex(idx)
}

func addReleasesNS(t *testing.T, idx *index.Index, namespace, projectName string, releaseNames []string) {
	t.Helper()
	for _, name := range releaseNames {
		componentName := name
		if comp, _, _, err := ParseReleaseName(name); err == nil {
			componentName = comp
		}
		entry := &index.ResourceEntry{
			Resource: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "openchoreo.dev/v1alpha1",
					"kind":       "ComponentRelease",
					"metadata": map[string]interface{}{
						"name":      name,
						"namespace": namespace,
					},
					"spec": map[string]interface{}{
						"owner": map[string]interface{}{
							"projectName":   projectName,
							"componentName": componentName,
						},
					},
				},
			},
			FilePath: "/test/" + namespace + "/" + name + ".yaml",
		}
		require.NoError(t, idx.Add(entry), "failed to add entry %q", name)
	}
}

// addReleaseWithOwner adds a single ComponentRelease whose metadata.name and owning
// component intentionally differ, so tests can exercise release names that do not follow
// the <component>-<date>-<version> convention (e.g. hand-authored hotfixes).
func addReleaseWithOwner(t *testing.T, idx *index.Index, namespace, projectName, releaseName, ownerComponent string) {
	t.Helper()
	entry := &index.ResourceEntry{
		Resource: &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openchoreo.dev/v1alpha1",
				"kind":       "ComponentRelease",
				"metadata": map[string]any{
					"name":      releaseName,
					"namespace": namespace,
				},
				"spec": map[string]any{
					"owner": map[string]any{
						"projectName":   projectName,
						"componentName": ownerComponent,
					},
				},
			},
		},
		FilePath: "/test/" + namespace + "/" + releaseName + ".yaml",
	}
	require.NoError(t, idx.Add(entry), "failed to add entry %q", releaseName)
}

func TestGenerateReleaseName(t *testing.T) {
	date := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)

	t.Run("explicit version skips auto-detect", func(t *testing.T) {
		idx := makeTestIndex(t, nil)
		name, err := GenerateReleaseName("my-comp", date, "5", testNamespace, testProject, idx)
		require.NoError(t, err)
		assert.Equal(t, "my-comp-20250315-5", name)
	})

	t.Run("auto-detect version with no existing releases", func(t *testing.T) {
		idx := makeTestIndex(t, nil)
		name, err := GenerateReleaseName("my-comp", date, "", testNamespace, testProject, idx)
		require.NoError(t, err)
		assert.Equal(t, "my-comp-20250315-1", name)
	})

	t.Run("auto-detect version increments from latest", func(t *testing.T) {
		idx := makeTestIndex(t, []string{
			"my-comp-20250315-1",
			"my-comp-20250315-3",
			"my-comp-20250315-2",
		})
		name, err := GenerateReleaseName("my-comp", date, "", testNamespace, testProject, idx)
		require.NoError(t, err)
		assert.Equal(t, "my-comp-20250315-4", name)
	})

	t.Run("auto-detect ignores different component", func(t *testing.T) {
		idx := makeTestIndex(t, []string{
			"other-comp-20250315-10",
		})
		name, err := GenerateReleaseName("my-comp", date, "", testNamespace, testProject, idx)
		require.NoError(t, err)
		assert.Equal(t, "my-comp-20250315-1", name)
	})

	t.Run("auto-detect ignores different date", func(t *testing.T) {
		idx := makeTestIndex(t, []string{
			"my-comp-20250314-5",
		})
		name, err := GenerateReleaseName("my-comp", date, "", testNamespace, testProject, idx)
		require.NoError(t, err)
		assert.Equal(t, "my-comp-20250315-1", name)
	})

	t.Run("auto-detect ignores off-convention releases owned by the component", func(t *testing.T) {
		// A hand-authored hotfix owned by my-comp but named differently must not pull
		// the auto-generated version up to 10.
		idx := index.New("/test")
		addReleaseWithOwner(t, idx, testNamespace, testProject, "hotfix-20250315-9", "my-comp")
		ocIndex := fsmode.WrapIndex(idx)
		name, err := GenerateReleaseName("my-comp", date, "", testNamespace, testProject, ocIndex)
		require.NoError(t, err)
		assert.Equal(t, "my-comp-20250315-1", name)
	})

	t.Run("auto-detect ignores same-named component in another namespace", func(t *testing.T) {
		idx := index.New("/test")
		addReleasesNS(t, idx, "team-a", testProject, []string{"my-comp-20250315-1"})
		addReleasesNS(t, idx, "team-b", testProject, []string{
			"my-comp-20250315-7",
			"my-comp-20250315-8",
		})
		ocIndex := fsmode.WrapIndex(idx)
		name, err := GenerateReleaseName("my-comp", date, "", "team-a", testProject, ocIndex)
		require.NoError(t, err)
		assert.Equal(t, "my-comp-20250315-2", name, "team-a version must not be affected by team-b releases")
	})

	t.Run("zero date uses current date", func(t *testing.T) {
		idx := makeTestIndex(t, nil)
		before := time.Now().Format("20060102")
		name, err := GenerateReleaseName("my-comp", time.Time{}, "1", testNamespace, testProject, idx)
		require.NoError(t, err)
		after := time.Now().Format("20060102")
		valid := name == "my-comp-"+before+"-1" || name == "my-comp-"+after+"-1"
		assert.True(t, valid, "expected name with date %s or %s, got %s", before, after, name)
	})
}

func TestGetLatestVersionForDate(t *testing.T) {
	t.Run("no releases returns 0", func(t *testing.T) {
		idx := makeTestIndex(t, nil)
		assert.Equal(t, "0", getLatestVersionForDate("comp", "20250315", testNamespace, testProject, idx))
	})

	t.Run("finds highest version for matching component and date", func(t *testing.T) {
		idx := makeTestIndex(t, []string{
			"comp-20250315-1",
			"comp-20250315-3",
			"comp-20250315-2",
		})
		assert.Equal(t, "3", getLatestVersionForDate("comp", "20250315", testNamespace, testProject, idx))
	})

	t.Run("ignores different component", func(t *testing.T) {
		idx := makeTestIndex(t, []string{
			"other-20250315-10",
		})
		assert.Equal(t, "0", getLatestVersionForDate("comp", "20250315", testNamespace, testProject, idx))
	})

	t.Run("ignores different date", func(t *testing.T) {
		idx := makeTestIndex(t, []string{
			"comp-20250314-5",
		})
		assert.Equal(t, "0", getLatestVersionForDate("comp", "20250315", testNamespace, testProject, idx))
	})

	t.Run("ignores malformed release names", func(t *testing.T) {
		idx := makeTestIndex(t, []string{
			"not-a-valid-release",
			"comp-20250315-2",
		})
		assert.Equal(t, "2", getLatestVersionForDate("comp", "20250315", testNamespace, testProject, idx))
	})

	t.Run("scopes version to namespace", func(t *testing.T) {
		idx := index.New("/test")
		addReleasesNS(t, idx, "team-a", testProject, []string{"comp-20250315-2"})
		addReleasesNS(t, idx, "team-b", testProject, []string{"comp-20250315-9"})
		ocIndex := fsmode.WrapIndex(idx)
		assert.Equal(t, "2", getLatestVersionForDate("comp", "20250315", "team-a", testProject, ocIndex))
		assert.Equal(t, "9", getLatestVersionForDate("comp", "20250315", "team-b", testProject, ocIndex))
	})

	t.Run("ignores off-convention release names owned by the component", func(t *testing.T) {
		// Owned by "comp" but named "hotfix-...": must not count toward comp's versioning.
		idx := index.New("/test")
		addReleaseWithOwner(t, idx, testNamespace, testProject, "hotfix-20250315-9", "comp")
		addReleasesNS(t, idx, testNamespace, testProject, []string{"comp-20250315-2"})
		ocIndex := fsmode.WrapIndex(idx)
		assert.Equal(t, "2", getLatestVersionForDate("comp", "20250315", testNamespace, testProject, ocIndex))
	})
}

func TestIncrementVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "1 to 2", version: "1", want: "2"},
		{name: "0 to 1", version: "0", want: "1"},
		{name: "non-numeric defaults to 1", version: "abc", want: "1"},
		{name: "99 to 100", version: "99", want: "100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IncrementVersion(tt.version))
		})
	}
}
