// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/testutil"
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
