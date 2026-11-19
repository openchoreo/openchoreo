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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestRun_ExportsFilteredObjects(t *testing.T) {
	f := newFetchFixture(t, false)
	directory := filepath.Join(t.TempDir(), "nested", "export")
	files, err := Run(context.Background(), f.client, Params{OutputDir: directory})
	require.NoError(t, err)
	require.Len(t, files, 58)
	for _, file := range files {
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
			files, err := Run(context.Background(), f.client, Params{OutputDir: directory})
			require.EqualError(t, err, "your role does not have enough permission to perform export")
			assert.Nil(t, files)
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
			files, err := Run(context.Background(), f.client, Params{OutputDir: directory})
			require.ErrorContains(t, err, tc.wantErr)
			assert.NotContains(t, err.Error(), "component:view")
			assert.Nil(t, files)
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
			files, err := Run(context.Background(), tc.wrap(f.client), Params{OutputDir: directory})
			require.ErrorContains(t, err, tc.wantErr)
			assert.Nil(t, files)
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
			files, err := Run(context.Background(), f.client, Params{OutputDir: directory})
			require.Error(t, err)
			if !failProfile {
				require.EqualError(t, err, "your role does not have enough permission to perform export")
			}
			assert.Nil(t, files)
			assert.NotEmpty(t, f.calls, "the initial profile allowed resource fetching")
			assert.Equal(t, 2, f.profileCalls)
			assert.Equal(t, before, snapshotTree(t, directory), "no replacement or pruning after losing permission")
		})
	}
}

func TestRun_AllowsCompleteReadOnlyAccess(t *testing.T) {
	f := newFetchFixture(t, false)
	files, err := Run(context.Background(), f.client, Params{OutputDir: t.TempDir()})
	require.NoError(t, err)
	assert.Len(t, files, 58)
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
	files, err := Run(ctx, f.client, Params{OutputDir: directory})
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, files)
	assert.Empty(t, f.calls)
	assert.NoDirExists(t, directory)
}

func TestRun_RequiresOutputBeforeFetching(t *testing.T) {
	f := newFetchFixture(t, false)
	files, err := Run(context.Background(), f.client, Params{})
	require.ErrorContains(t, err, "output directory is required")
	assert.Nil(t, files)
	assert.Empty(t, f.calls)
}

func TestRun_SerializationErrorDoesNotCreateOutput(t *testing.T) {
	f := newFetchFixture(t, false)
	f.override = func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/v1/namespaces/acme/components" {
			object := fixtureObject(collectionFixture{kind: testComponentKind, path: "components"}, req.URL.Path, 0)
			delete(object, "spec")
			return testutil.JSONResp(http.StatusOK, map[string]any{"items": []any{object}, "pagination": map[string]any{}}), nil
		}
		return nil, nil
	}
	directory := filepath.Join(t.TempDir(), "export")
	files, err := Run(context.Background(), f.client, Params{OutputDir: directory})
	require.ErrorContains(t, err, "spec.owner.projectName")
	assert.Nil(t, files)
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
	files, err := Run(context.Background(), f.client, Params{OutputDir: directory})
	require.ErrorContains(t, err, "already exists")
	assert.Nil(t, files)
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
	files, err := Run(context.Background(), f.client, Params{OutputDir: directory})
	require.NoError(t, err)
	assert.Empty(t, files)
	assert.DirExists(t, directory)
}

func TestRun_DeletingObjectsAreNotSerialized(t *testing.T) {
	f := newFetchFixture(t, false)
	f.override = func(req *http.Request) (*http.Response, error) {
		var collection collectionFixture
		switch req.URL.Path {
		case "/api/v1/clustertraits":
			collection = collectionFixture{kind: "ClusterTrait", path: "clustertraits"}
		case "/api/v1/namespaces/acme/components":
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
	files, err := Run(context.Background(), f.client, Params{OutputDir: directory})
	require.NoError(t, err)
	assert.Len(t, files, 58, "two additional deleting objects must be excluded from the fetched inventory")
	assert.FileExists(t, filepath.Join(directory, "platform-shared", "traits", "clustertraits-0.yaml"))
	assert.NoFileExists(t, filepath.Join(directory, "platform-shared", "traits", "clustertraits-1.yaml"))
	assert.FileExists(t, filepath.Join(directory, "namespaces", "acme", "projects", "store", "components", "components-0", "component.yaml"))
	assert.NoFileExists(t, filepath.Join(directory, "namespaces", "acme", "projects", "store", "components", "components-1", "component.yaml"))
}
