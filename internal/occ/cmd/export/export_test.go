// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestRun_ExportsNormalizedComponent(t *testing.T) {
	root := t.TempDir()
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	labels := map[string]string{
		"team":                       "payments",
		"openchoreo.dev/project-uid": "runtime-id",
	}
	component := gen.Component{
		Metadata: gen.ObjectMeta{Name: "api", Uid: stringPointer("uid"), CreationTimestamp: &created, Labels: &labels},
		Spec: &gen.ComponentSpec{
			Owner: struct {
				ProjectName string `json:"projectName"`
			}{ProjectName: "shop"},
		},
		Status: &gen.ComponentStatus{},
	}
	mockClient := mocks.NewMockInterface(t)
	mockClient.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{
		Items: []gen.Namespace{{Metadata: gen.ObjectMeta{Name: "acme"}}},
	}, nil)
	mockClient.EXPECT().ListComponents(mock.Anything, "acme", "", mock.Anything).Return(&gen.ComponentList{
		Items: []gen.Component{component},
	}, nil)

	report, err := runEngine(context.Background(), mockClient, Options{OutputDir: root})
	require.NoError(t, err)
	assert.Equal(t, 1, report.Created)
	assert.Equal(t, 1, report.Exported)
	assert.NotEmpty(t, report.Unsupported)

	path := filepath.Join(root, "namespaces", "acme", "projects", "shop", "components", "api", "component.yaml")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var object map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &object))
	assert.Equal(t, "openchoreo.dev/v1alpha1", object["apiVersion"])
	assert.Equal(t, "Component", object["kind"])
	assert.NotContains(t, object, "status")
	metadata := object["metadata"].(map[string]any)
	assert.Equal(t, "acme", metadata["namespace"])
	assert.NotContains(t, metadata, "uid")
	assert.Equal(t, map[string]any{"team": "payments"}, metadata["labels"])
}

func TestRun_RepeatIsByteIdenticalAndPreservesOtherFiles(t *testing.T) {
	root := t.TempDir()
	unrelated := filepath.Join(root, "README.md")
	require.NoError(t, os.WriteFile(unrelated, []byte("owned by user\n"), 0o600))
	first := mockComponentClient(t, "", "")
	firstReport, err := runEngine(context.Background(), first, Options{OutputDir: root})
	require.NoError(t, err)
	componentPath := filepath.Join(root, "namespaces", "acme", "projects", "shop", "components", "api", "component.yaml")
	want, err := os.ReadFile(componentPath)
	require.NoError(t, err)

	second := mockComponentClient(t, "", "")
	secondReport, err := runEngine(context.Background(), second, Options{OutputDir: root})
	require.NoError(t, err)
	got, err := os.ReadFile(componentPath)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, 1, firstReport.Created)
	assert.Equal(t, 1, secondReport.Unchanged)
	assert.FileExists(t, unrelated)
}

func TestRun_UpdatesExistingComponentFile(t *testing.T) {
	root := t.TempDir()
	first, err := runEngine(context.Background(), mockComponentClient(t, "tier", "one"), Options{OutputDir: root})
	require.NoError(t, err)
	assert.Equal(t, 1, first.Created)

	second, err := runEngine(context.Background(), mockComponentClient(t, "tier", "two"), Options{OutputDir: root})
	require.NoError(t, err)
	assert.Equal(t, 1, second.Updated)

	componentPath := filepath.Join(root, "namespaces", "acme", "projects", "shop", "components", "api", "component.yaml")
	data, err := os.ReadFile(componentPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "two")
}

func TestRun_WithholdsPlaintextSecret(t *testing.T) {
	mockClient := mockComponentClient(t, "dbPassword", "should-not-be-written")
	root := t.TempDir()
	report, err := runEngine(context.Background(), mockClient, Options{OutputDir: root})
	require.NoError(t, err)
	require.Len(t, report.Withheld, 1)
	assert.NotContains(t, report.Withheld[0].Reasons[0], "should-not-be-written")
	assert.NoFileExists(t, filepath.Join(root, "namespaces", "acme", "projects", "shop", "components", "api", "component.yaml"))
}

func TestRun_FetchFailureDoesNotWriteFiles(t *testing.T) {
	mockClient := mocks.NewMockInterface(t)
	mockClient.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(nil, errors.New("API unavailable"))
	root := filepath.Join(t.TempDir(), "output")
	_, err := runEngine(context.Background(), mockClient, Options{OutputDir: root})
	require.ErrorContains(t, err, "nothing was written")
	_, statErr := os.Stat(root)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestRun_RejectsMissingOutputDirectory(t *testing.T) {
	_, err := runEngine(context.Background(), mocks.NewMockInterface(t), Options{})
	assert.ErrorContains(t, err, "output directory is required")
}

func mockComponentClient(t *testing.T, envKey, envValue string) *mocks.MockInterface {
	t.Helper()
	component := gen.Component{
		Metadata: gen.ObjectMeta{Name: "api"},
		Spec: &gen.ComponentSpec{
			Owner: struct {
				ProjectName string `json:"projectName"`
			}{ProjectName: "shop"},
		},
	}
	if envKey != "" {
		parameters := map[string]interface{}{envKey: envValue}
		component.Spec.Parameters = &parameters
	}
	mockClient := mocks.NewMockInterface(t)
	mockClient.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{
		Items: []gen.Namespace{{Metadata: gen.ObjectMeta{Name: "acme"}}},
	}, nil)
	mockClient.EXPECT().ListComponents(mock.Anything, "acme", "", mock.Anything).Return(&gen.ComponentList{
		Items: []gen.Component{component},
	}, nil)
	return mockClient
}

func stringPointer(value string) *string { return &value }
