// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const testComponentsPath = "/api/v1/namespaces/acme/components"

func TestRun_ExportsFilteredObjects(t *testing.T) {
	f := newFetchFixture(t, false)
	directory := filepath.Join(t.TempDir(), "nested", "export")
	result, err := Run(context.Background(), f.client, Params{OutputDir: directory})
	require.NoError(t, err)
	require.Len(t, result.Files, 58)
	for _, file := range result.Files {
		data, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(file.Path)))
		require.NoError(t, err)
		assert.Equal(t, file.Data, data)
		var document map[string]json.RawMessage
		require.NoError(t, yaml.Unmarshal(data, &document))
		assert.NotContains(t, document, "status", file.Path)
		var metadata map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(document["metadata"], &metadata))
		for field := range metadata {
			assert.Contains(t, []string{"name", "namespace", "labels", "annotations"}, field, file.Path)
		}
		if annotations, exists := metadata["annotations"]; exists {
			var entries map[string]string
			require.NoError(t, json.Unmarshal(annotations, &entries))
			assert.NotContains(t, entries, lastAppliedAnnotation)
			assert.NotContains(t, entries, restartedAtAnnotation)
			assert.Equal(t, "annotation", entries["keep"])
			assert.Equal(t, "existing-release", entries["meta.helm.sh/release-name"])
			assert.Equal(t, "Ignore", entries["kustomize.toolkit.fluxcd.io/ssa"])
		}
	}
	data, err := os.ReadFile(filepath.Join(directory, "namespaces", "acme", "projects", "store", "components", "components-0", "component.yaml"))
	require.NoError(t, err)
	var component struct {
		Spec struct {
			AutoDeploy bool `json:"autoDeploy"`
		} `json:"spec"`
	}
	require.NoError(t, yaml.Unmarshal(data, &component))
	assert.True(t, component.Spec.AutoDeploy, "filtering must not change deployment behavior")
	data, err = os.ReadFile(filepath.Join(directory, "namespaces", "acme", "namespace.yaml"))
	require.NoError(t, err)
	var namespace struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
	}
	require.NoError(t, yaml.Unmarshal(data, &namespace))
	assert.Equal(t, "v1", namespace.APIVersion)
	assert.Equal(t, testNamespaceKind, namespace.Kind)
}

func TestRun_RejectsIncompleteReadAccessBeforeFetching(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]gen.ActionCapability)
	}{
		{"missing component and trait actions", func(caps map[string]gen.ActionCapability) {
			delete(caps, "component:view")
			delete(caps, "trait:view")
		}},
		{"missing release line action", func(caps map[string]gen.ActionCapability) {
			delete(caps, "releaseline:view")
		}},
		{"namespace only", func(caps map[string]gen.ActionCapability) {
			setCapabilityPath(caps, "component:view", "ns/acme")
		}},
		{"project only", func(caps map[string]gen.ActionCapability) {
			setCapabilityPath(caps, "component:view", "ns/acme/project/store")
		}},
		{"conditional global grant", func(caps map[string]gen.ActionCapability) {
			capability := caps["component:view"]
			expressions := []string{"resource.environment == 'development'"}
			(*capability.Allowed)[0].Constraints = &gen.CapabilityConstraints{Expressions: &expressions}
		}},
		{"specific denial with global grant", func(caps map[string]gen.ActionCapability) {
			capability := caps["component:view"]
			name := "ns/acme/project/restricted"
			denied := []gen.CapabilityResource{{Path: &name}}
			capability.Denied = &denied
			caps["component:view"] = capability
		}},
		{"missing grant path", func(caps map[string]gen.ActionCapability) {
			capability := caps["component:view"]
			(*capability.Allowed)[0].Path = nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFetchFixture(t, false)
			profile := fullExportProfile()
			tc.change(*profile.Capabilities)
			f.profileOverride = func(_ *http.Request) (*http.Response, error) {
				return testutil.JSONResp(http.StatusOK, profile), nil
			}
			directory := t.TempDir()
			seedFiles(t, directory, serializedResources(t, manifestResource(testComponentKind)))
			before := snapshotTree(t, directory)
			homeDir, err := os.UserHomeDir()
			require.NoError(t, err)
			configPath := filepath.Join(homeDir, ".openchoreo", "config")
			configBefore, err := os.ReadFile(configPath)
			require.NoError(t, err)
			result, err := Run(context.Background(), f.client, Params{OutputDir: directory})
			require.EqualError(t, err, "your role does not have enough permission to perform export")
			assert.Nil(t, result)
			assert.Empty(t, f.calls, "permission-limited exports must stop before listing resources")
			assert.Equal(t, 1, f.profileCalls)
			assert.Equal(t, before, snapshotTree(t, directory))
			configAfter, err := os.ReadFile(configPath)
			require.NoError(t, err)
			assert.Equal(t, configBefore, configAfter)
		})
	}
}

func setCapabilityPath(capabilities map[string]gen.ActionCapability, action, path string) {
	capability := capabilities[action]
	// Give this action its own slice; fixture grants may share their slice.
	allowed := []gen.CapabilityResource{{Path: &path}}
	capability.Allowed = &allowed
	capabilities[action] = capability
}

func TestRun_AuthorizationProfileFailuresCreateNoOutput(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response testutil.RoundTripFunc
		wantErr  string
	}{
		{"forbidden", func(_ *http.Request) (*http.Response, error) {
			return testutil.JSONResp(http.StatusForbidden, map[string]string{"error": "missing component:view"}), nil
		}, "your role does not have enough permission to perform export"},
		{"server error", func(_ *http.Request) (*http.Response, error) {
			return testutil.JSONResp(http.StatusInternalServerError, map[string]string{"error": "policy service unavailable"}), nil
		}, "HTTP 500): policy service unavailable"},
		{"transport error", func(_ *http.Request) (*http.Response, error) {
			return nil, errors.New("authorization server unreachable")
		}, "authorization server unreachable"},
		{"empty response", func(_ *http.Request) (*http.Response, error) {
			return testutil.JSONResp(http.StatusNoContent, nil), nil
		}, "HTTP 204"},
		{"unstructured error", func(_ *http.Request) (*http.Response, error) {
			return testutil.JSONResp(http.StatusBadGateway, map[string]string{"message": "proxy unavailable"}), nil
		}, "HTTP 502"},
		{"missing capabilities", func(_ *http.Request) (*http.Response, error) {
			return testutil.JSONResp(http.StatusOK, gen.UserCapabilitiesResponse{}), nil
		}, "authorization capabilities are missing"},
		{"empty capabilities", func(_ *http.Request) (*http.Response, error) {
			return testutil.JSONResp(http.StatusOK, map[string]any{"capabilities": map[string]any{}}), nil
		}, "your role does not have enough permission to perform export"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFetchFixture(t, false)
			f.profileOverride = tc.response
			directory := filepath.Join(t.TempDir(), "new", "export")
			result, err := Run(context.Background(), f.client, Params{OutputDir: directory})
			require.ErrorContains(t, err, tc.wantErr)
			assert.NotContains(t, err.Error(), "component:view")
			assert.Nil(t, result)
			assert.Empty(t, f.calls)
			assert.NoDirExists(t, filepath.Dir(directory))
		})
	}
}

type exportClientWithMissingGeneratedClient struct {
	client.Interface
}

func (exportClientWithMissingGeneratedClient) GetClient() *gen.ClientWithResponses {
	return nil
}

func TestRun_RequiresGeneratedClientAccess(t *testing.T) {
	for _, tc := range []struct {
		name    string
		wrap    func(client.Interface) client.Interface
		wantErr string
	}{
		{"missing accessor", func(c client.Interface) client.Interface {
			return struct{ client.Interface }{c}
		}, "API client does not expose the generated client"},
		{"nil generated client", func(c client.Interface) client.Interface {
			return exportClientWithMissingGeneratedClient{c}
		}, "generated API client is missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFetchFixture(t, false)
			directory := t.TempDir()
			seedFiles(t, directory, serializedResources(t, manifestResource(testComponentKind)))
			before := snapshotTree(t, directory)
			result, err := Run(context.Background(), tc.wrap(f.client), Params{OutputDir: directory})
			require.ErrorContains(t, err, tc.wantErr)
			assert.Nil(t, result)
			assert.Zero(t, f.profileCalls)
			assert.Empty(t, f.calls)
			assert.Equal(t, before, snapshotTree(t, directory))
		})
	}
}

func TestRun_RechecksPermissionsBeforeWriting(t *testing.T) {
	for _, failProfile := range []bool{false, true} {
		t.Run(fmt.Sprintf("profile failure %t", failProfile), func(t *testing.T) {
			f := newFetchFixture(t, false)
			f.profileOverride = func(_ *http.Request) (*http.Response, error) {
				profile := fullExportProfile()
				if f.profileCalls > 1 {
					if failProfile {
						return nil, errors.New("authorization profile unavailable")
					}
					delete(*profile.Capabilities, "trait:view")
				}
				return testutil.JSONResp(http.StatusOK, profile), nil
			}
			directory := t.TempDir()
			seedFiles(t, directory, serializedResources(t, manifestResource(testComponentKind)))
			before := snapshotTree(t, directory)
			result, err := Run(context.Background(), f.client, Params{OutputDir: directory})
			require.Error(t, err)
			if !failProfile {
				require.EqualError(t, err, "your role does not have enough permission to perform export")
			}
			assert.Nil(t, result)
			assert.NotEmpty(t, f.calls, "the initial profile allowed resource fetching")
			assert.Equal(t, 2, f.profileCalls)
			assert.Equal(t, before, snapshotTree(t, directory), "no replacement or pruning after losing permission")
		})
	}
}

func TestRun_AllowsCompleteReadOnlyAccess(t *testing.T) {
	f := newFetchFixture(t, false)
	result, err := Run(context.Background(), f.client, Params{OutputDir: t.TempDir()})
	require.NoError(t, err)
	assert.Len(t, result.Files, 58)
	assert.Equal(t, 2, f.profileCalls)
}

func TestUnrestrictedAccess_CombinesGlobalAndSpecificCapabilities(t *testing.T) {
	wildcard, scoped := "*", "ns/acme"
	allowed := []gen.CapabilityResource{{Path: &wildcard}}
	denied := []gen.CapabilityResource{{Path: &scoped}}
	for _, tc := range []struct {
		name         string
		capabilities map[string]gen.ActionCapability
		want         bool
	}{
		{"wildcard grant", map[string]gen.ActionCapability{"*": {Allowed: &allowed}}, true},
		{"wildcard grant with specific denial", map[string]gen.ActionCapability{
			"*": {Allowed: &allowed}, "component:view": {Denied: &denied},
		}, false},
		{"specific grant with wildcard denial", map[string]gen.ActionCapability{
			"*": {Denied: &denied}, "component:view": {Allowed: &allowed},
		}, false},
		{"missing grant", map[string]gen.ActionCapability{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, hasUnrestrictedAccess(tc.capabilities, "component:view"))
		})
	}
}

func TestRun_PropagatesCancellation(t *testing.T) {
	f := newFetchFixture(t, false)
	directory := filepath.Join(t.TempDir(), "export")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := Run(ctx, f.client, Params{OutputDir: directory})
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)
	assert.Empty(t, f.calls)
	assert.NoDirExists(t, directory)
}

func TestRun_RequiresOutputBeforeFetching(t *testing.T) {
	f := newFetchFixture(t, false)
	result, err := Run(context.Background(), f.client, Params{})
	require.ErrorContains(t, err, "output directory is required")
	assert.Nil(t, result)
	assert.Empty(t, f.calls)
}

func TestRun_SerializationErrorDoesNotCreateOutput(t *testing.T) {
	f := newFetchFixture(t, false)
	f.override = func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == testComponentsPath {
			object := fixtureObject(collectionFixture{kind: testComponentKind, path: "components"}, req.URL.Path, 0)
			delete(object, "spec")
			return testutil.JSONResp(http.StatusOK, map[string]any{"items": []any{object}, "pagination": map[string]any{}}), nil
		}
		return nil, nil
	}
	directory := filepath.Join(t.TempDir(), "export")
	result, err := Run(context.Background(), f.client, Params{OutputDir: directory})
	require.ErrorContains(t, err, "spec.owner.projectName")
	assert.Nil(t, result)
	assert.NoDirExists(t, directory)
}

func TestRun_ExistingFilePreventsAllManifestWrites(t *testing.T) {
	f := newFetchFixture(t, false)
	directory := t.TempDir()
	existing := filepath.Join(directory, "platform-shared", "workflows", "clusterworkflows-0.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(existing), 0o700))
	require.NoError(t, os.WriteFile(existing, []byte("existing manifest\n"), 0o600))
	readme := filepath.Join(directory, "README.md")
	require.NoError(t, os.WriteFile(readme, []byte("existing repository\n"), 0o600))
	result, err := Run(context.Background(), f.client, Params{OutputDir: directory})
	require.ErrorContains(t, err, "already exists")
	assert.Nil(t, result)
	data, err := os.ReadFile(existing)
	require.NoError(t, err)
	assert.Equal(t, "existing manifest\n", string(data))
	var count int
	require.NoError(t, filepath.WalkDir(directory, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			count++
		}
		return nil
	}))
	assert.Equal(t, 2, count, "a conflict must be detected before any manifest is written")
}

func TestRun_EmptyInventoryCreatesOutputFolder(t *testing.T) {
	f := newFetchFixture(t, false)
	f.empty = true
	directory := filepath.Join(t.TempDir(), "export")
	result, err := Run(context.Background(), f.client, Params{OutputDir: directory})
	require.NoError(t, err)
	assert.Empty(t, result.Files)
	assert.Equal(t, FileCounts{}, result.Counts)
	assert.DirExists(t, directory)
}

func TestRun_DeletingObjectsAreNotSerialized(t *testing.T) {
	f := newFetchFixture(t, false)
	// This case adds a second component to the fixture's single-page response.
	f.collections["/api/v1/namespaces/acme/components/components-1/releaselines"] = requiredComponentCollections[0]
	f.override = func(req *http.Request) (*http.Response, error) {
		var collection collectionFixture
		switch req.URL.Path {
		case "/api/v1/clustertraits":
			collection = collectionFixture{kind: "ClusterTrait", path: "clustertraits"}
		case testComponentsPath:
			collection = collectionFixture{kind: testComponentKind, path: "components"}
		default:
			return nil, nil
		}
		active := fixtureObject(collection, req.URL.Path, 0)
		deleting := fixtureObject(collection, req.URL.Path, 1)
		deleting["metadata"].(map[string]any)["deletionTimestamp"] = "2026-11-17T08:00:00Z"
		return testutil.JSONResp(http.StatusOK, map[string]any{"items": []any{active, deleting}, "pagination": map[string]any{}}), nil
	}
	directory := t.TempDir()
	result, err := Run(context.Background(), f.client, Params{OutputDir: directory})
	require.NoError(t, err)
	assert.Len(t, result.Files, 58, "two additional deleting objects must be excluded from the fetched inventory")
	assert.FileExists(t, filepath.Join(directory, "platform-shared", "traits", "clustertraits-0.yaml"))
	assert.NoFileExists(t, filepath.Join(directory, "platform-shared", "traits", "clustertraits-1.yaml"))
	assert.FileExists(t, filepath.Join(directory, "namespaces", "acme", "projects", "store", "components", "components-0", "component.yaml"))
	assert.NoFileExists(t, filepath.Join(directory, "namespaces", "acme", "projects", "store", "components", "components-1", "component.yaml"))
}

func TestRun_ReleaseWindowPreservesHistoryUntilDeleted(t *testing.T) {
	old := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range releaseTestCases {
		t.Run(tc.kind, func(t *testing.T) {
			f := newFetchFixture(t, false)
			present := true
			f.override = func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/api/v1/namespaces/acme/"+tc.collection {
					return nil, nil
				}
				items := []any{releaseTestObject(tc, "acme", "unknown-age", nil)}
				if present {
					items = append(items, releaseTestObject(tc, "acme", "historical", &old),
						releaseTestObject(tc, "acme", "unexported-old", &old))
				}
				deleting := releaseTestObject(tc, "acme", "deleting", &old)
				deleting["metadata"].(map[string]any)["deletionTimestamp"] = fixtureCreationTimestamp
				items = append(items, deleting)
				return testutil.JSONResp(http.StatusOK, map[string]any{"items": items, "pagination": map[string]any{}}), nil
			}
			directory := t.TempDir()
			existing := make([]Resource, 0, 3)
			for _, name := range []string{"historical", "deleted", "deleting"} {
				existing = append(existing, Resource{
					Kind: tc.kind, Namespace: "acme", Name: name, Object: releaseTestObject(tc, "acme", name, &old),
				})
			}
			seedFiles(t, directory, serializedResources(t, existing...))
			base := filepath.Join(directory, "namespaces", "acme", "projects", "store", filepath.FromSlash(tc.directory))
			historicalPath := filepath.Join(base, "historical.yaml")
			before, err := os.ReadFile(historicalPath)
			require.NoError(t, err)
			before = append([]byte("# Preserve this existing history exactly.\n"), before...)
			require.NoError(t, os.WriteFile(historicalPath, before, 0o600))
			infoBefore, err := os.Stat(historicalPath)
			require.NoError(t, err)
			readme := filepath.Join(directory, "README.md")
			require.NoError(t, os.WriteFile(readme, []byte("keep unrelated files\n"), 0o600))

			result, err := Run(context.Background(), f.client, Params{OutputDir: directory})
			require.NoError(t, err)
			selected := len(result.Files)
			assert.Equal(t, FileCounts{Created: selected, Deleted: 2}, result.Counts,
				"preserved history must not be counted as unchanged or updated")
			after, err := os.ReadFile(historicalPath)
			require.NoError(t, err)
			assert.Equal(t, before, after, "aged-out history must not be rewritten")
			infoAfter, err := os.Stat(historicalPath)
			require.NoError(t, err)
			assert.True(t, os.SameFile(infoBefore, infoAfter))
			assert.Equal(t, infoBefore.ModTime(), infoAfter.ModTime())
			assert.NoFileExists(t, filepath.Join(base, "deleted.yaml"))
			assert.NoFileExists(t, filepath.Join(base, "deleting.yaml"))
			assert.NoFileExists(t, filepath.Join(base, "unexported-old.yaml"))
			assert.FileExists(t, filepath.Join(base, "unknown-age.yaml"))

			result, err = Run(context.Background(), f.client, Params{OutputDir: directory, AllReleases: true})
			require.NoError(t, err)
			assert.Equal(t, FileCounts{Created: 1, Updated: 1, Unchanged: selected}, result.Counts)
			assert.Nil(t, result.ReleaseCutoff)
			after, err = os.ReadFile(historicalPath)
			require.NoError(t, err)
			assert.NotEqual(t, before, after, "all-releases resumes synchronization of historical files")
			assert.FileExists(t, filepath.Join(base, "unexported-old.yaml"))

			present = false
			result, err = Run(context.Background(), f.client, Params{OutputDir: directory})
			require.NoError(t, err)
			assert.Equal(t, FileCounts{Unchanged: selected, Deleted: 2}, result.Counts)
			assert.NoFileExists(t, historicalPath, "history is pruned once it is absent from the cluster")
			assert.NoFileExists(t, filepath.Join(base, "unexported-old.yaml"))
			contents, err := os.ReadFile(readme)
			require.NoError(t, err)
			assert.Equal(t, "keep unrelated files\n", string(contents))
		})
	}
}

func TestRun_MissingReferencedReleaseLeavesFilesUnchanged(t *testing.T) {
	f := newFetchFixture(t, false)
	tc := releaseTestCases[0]
	f.override = func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/v1/namespaces/acme/"+tc.bindings {
			return testutil.JSONResp(http.StatusOK, map[string]any{
				"items": []any{bindingTestObject(tc, "missing-release")}, "pagination": map[string]any{},
			}), nil
		}
		return nil, nil
	}
	directory := t.TempDir()
	seedFiles(t, directory, serializedResources(t, manifestResource(testComponentKind)))
	before := snapshotTree(t, directory)
	result, err := Run(context.Background(), f.client, Params{OutputDir: directory})
	require.ErrorContains(t, err, "binding-referenced release is missing")
	assert.Nil(t, result)
	assert.Equal(t, before, snapshotTree(t, directory))
}

func TestRun_ExportsReleaseLineConfigurationAndSynchronizesItsFile(t *testing.T) {
	f := newFetchFixture(t, false)
	present := true
	replicas := 2
	f.override = func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case testComponentsPath:
			component := fixtureObject(collectionFixture{kind: testComponentKind, path: "components"}, req.URL.Path, 0)
			component["metadata"].(map[string]any)["name"] = "backend"
			return testutil.JSONResp(http.StatusOK, map[string]any{"items": []any{component}, "pagination": map[string]any{}}), nil
		case "/api/v1/namespaces/acme/components/backend/releaselines":
			items := []any{releaseLineTestObject("acme", "backend", "main")}
			if present {
				line := releaseLineTestObject("acme", "backend", "backend-v2")
				line["metadata"].(map[string]any)["creationTimestamp"] = "2000-01-01T00:00:00Z"
				line["spec"].(map[string]any)["autoDeploy"] = false
				line["spec"].(map[string]any)["parameters"] = map[string]any{"replicas": replicas}
				line["spec"].(map[string]any)["workflow"] = map[string]any{"parameters": map[string]any{"branch": "v2"}}
				line["status"] = map[string]any{"observedGeneration": 3}
				items = append(items, line)
			}
			return testutil.JSONResp(http.StatusOK, map[string]any{"items": items, "pagination": map[string]any{}}), nil
		default:
			return nil, nil
		}
	}
	directory := t.TempDir()
	result, err := Run(context.Background(), f.client, Params{OutputDir: directory})
	require.NoError(t, err)
	assert.Equal(t, 59, result.Counts.Created)
	base := filepath.Join(directory, "namespaces", "acme", "projects", "store", "components", "backend", "release-lines")
	name := filepath.Join(base, "backend-v2.yaml")
	data, err := os.ReadFile(name)
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, yaml.Unmarshal(data, &document))
	assert.Equal(t, testReleaseLineKind, document["kind"])
	assert.Equal(t, map[string]any{
		"owner":      map[string]any{"projectName": "store", "componentName": "backend"},
		"autoDeploy": false,
		"parameters": map[string]any{"replicas": float64(2)},
		"workflow":   map[string]any{"parameters": map[string]any{"branch": "v2"}},
	}, document["spec"])
	assert.NotContains(t, document, "status")
	assert.NotContains(t, document["metadata"], "creationTimestamp")
	assert.NoFileExists(t, filepath.Join(base, "main.yaml"))

	replicas = 3
	result, err = Run(context.Background(), f.client, Params{OutputDir: directory})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Counts.Updated)
	data, err = os.ReadFile(name)
	require.NoError(t, err)
	assert.Contains(t, string(data), "replicas: 3")

	present = false
	result, err = Run(context.Background(), f.client, Params{OutputDir: directory})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Counts.Deleted)
	assert.NoFileExists(t, name)
	assert.NoDirExists(t, base)
}
