// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
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

func TestRun_ExportsFetchedObjects(t *testing.T) {
	f := newFetchFixture(t, false)
	directory := filepath.Join(t.TempDir(), "nested", "export")
	files, err := Run(context.Background(), f.client, Params{OutputDir: directory})
	require.NoError(t, err)
	require.Len(t, files, 58)
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(file.Path)))
		require.NoError(t, err)
		assert.Equal(t, file.Data, data)
	}
	data, err := os.ReadFile(filepath.Join(directory, "namespaces", "acme", "projects", "store", "components", "components-0", "component.yaml"))
	require.NoError(t, err)
	var component struct {
		Metadata struct {
			UID string `json:"uid"`
		} `json:"metadata"`
		Status struct {
			ObservedGeneration int `json:"observedGeneration"`
		} `json:"status"`
	}
	require.NoError(t, yaml.Unmarshal(data, &component))
	assert.Equal(t, "keep-uid", component.Metadata.UID)
	assert.Equal(t, 7, component.Status.ObservedGeneration)
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
