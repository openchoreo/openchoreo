// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package synth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// outsideMarker is written to a file that lives OUTSIDE the descriptor directory.
// Descriptor file references resolve relative to the descriptor directory, so this
// content must never be inlined into the Workload CR.
const outsideMarker = "CONTENT-OUTSIDE-DESCRIPTOR-DIR"

// pathFixture lays out:
//
//	<root>/outside.txt         <- file outside the descriptor directory
//	<root>/app/workload.yaml   <- the descriptor; baseDir is <root>/app
//
// so a descriptor path of "../outside.txt" resolves out of baseDir into
// <root>/outside.txt. It returns descriptorPath.
func pathFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "outside.txt"), []byte(outsideMarker), 0o600))
	appDir := filepath.Join(root, "app")
	require.NoError(t, os.MkdirAll(appDir, 0o755))
	return filepath.Join(appDir, "workload.yaml")
}

func newTestWorkload() *openchoreov1alpha1.Workload {
	return &openchoreov1alpha1.Workload{
		Spec: openchoreov1alpha1.WorkloadSpec{
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{},
		},
	}
}

// TestAddConfigurationsFromDescriptorParentPath checks the files[].valueFrom.path
// reader: a "../"-prefixed path resolves outside the descriptor directory, so it
// returns an error and does not inline the out-of-directory file.
func TestAddConfigurationsFromDescriptorParentPath(t *testing.T) {
	descriptorPath := pathFixture(t)

	descriptor := &WorkloadDescriptor{
		Configurations: WorkloadDescriptorConfiguration{
			Files: []WorkloadDescriptorFileVar{
				{
					Name:      "cfg",
					MountPath: "/etc/app/cfg",
					ValueFrom: &WorkloadDescriptorEnvVarSource{
						Path: "../outside.txt",
					},
				},
			},
		},
	}

	w := newTestWorkload()
	err := addConfigurationsFromDescriptor(w, descriptor, descriptorPath)

	require.Error(t, err, "a path resolving outside the descriptor directory must return an error")
	for _, f := range w.Spec.Container.Files {
		assert.NotContains(t, f.Value, outsideMarker,
			"content from outside the descriptor directory must not be inlined")
	}
}

// TestAddEndpointsFromDescriptorSchemaFileParentPath checks the same behavior for the
// endpoints[].schemaFile reader.
func TestAddEndpointsFromDescriptorSchemaFileParentPath(t *testing.T) {
	descriptorPath := pathFixture(t)

	descriptor := &WorkloadDescriptor{
		Endpoints: []WorkloadDescriptorEndpoint{
			{
				Name:       "api",
				Type:       "HTTP",
				SchemaFile: "../outside.txt",
			},
		},
	}

	w := newTestWorkload()
	err := addEndpointsFromDescriptor(w, descriptor, descriptorPath)

	require.Error(t, err, "a schema file resolving outside the descriptor directory must return an error")
	for _, ep := range w.Spec.Endpoints {
		if ep.Schema != nil {
			assert.NotContains(t, ep.Schema.Content, outsideMarker,
				"schema content from outside the descriptor directory must not be inlined")
		}
	}
}

// TestAddConfigurationsFromDescriptorSymlink checks a symlink that sits inside the
// descriptor directory but points outside it: the read resolves out of the directory,
// so it returns an error.
func TestAddConfigurationsFromDescriptorSymlink(t *testing.T) {
	descriptorPath := pathFixture(t)
	appDir := filepath.Dir(descriptorPath)
	outsidePath := filepath.Join(filepath.Dir(appDir), "outside.txt")

	if err := os.Symlink(outsidePath, filepath.Join(appDir, "link.txt")); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}

	descriptor := &WorkloadDescriptor{
		Configurations: WorkloadDescriptorConfiguration{
			Files: []WorkloadDescriptorFileVar{
				{
					Name:      "cfg",
					MountPath: "/etc/app/cfg",
					ValueFrom: &WorkloadDescriptorEnvVarSource{
						Path: "link.txt",
					},
				},
			},
		},
	}

	w := newTestWorkload()
	err := addConfigurationsFromDescriptor(w, descriptor, descriptorPath)

	require.Error(t, err, "a symlink pointing outside the descriptor directory must return an error")
	for _, f := range w.Spec.Container.Files {
		assert.NotContains(t, f.Value, outsideMarker,
			"content reached through an out-of-directory symlink must not be inlined")
	}
}

// TestAddConfigurationsFromDescriptorInDirFile confirms a file inside the descriptor
// directory still resolves and is inlined.
func TestAddConfigurationsFromDescriptorInDirFile(t *testing.T) {
	descriptorPath := pathFixture(t)
	appDir := filepath.Dir(descriptorPath)
	const want = "server.port=8080"
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "app.properties"), []byte(want), 0o600))

	descriptor := &WorkloadDescriptor{
		Configurations: WorkloadDescriptorConfiguration{
			Files: []WorkloadDescriptorFileVar{
				{
					Name:      "config",
					MountPath: "/etc/app/config.properties",
					ValueFrom: &WorkloadDescriptorEnvVarSource{
						Path: "app.properties",
					},
				},
			},
		},
	}

	w := newTestWorkload()
	require.NoError(t, addConfigurationsFromDescriptor(w, descriptor, descriptorPath))
	require.Len(t, w.Spec.Container.Files, 1)
	assert.True(t, strings.Contains(w.Spec.Container.Files[0].Value, want),
		"a file inside the descriptor directory must still resolve")
}
