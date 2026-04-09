// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kinds

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/occ/resources"
	synth "github.com/openchoreo/openchoreo/internal/occ/resources/workload"
)

func TestNewWorkloadResource(t *testing.T) {
	t.Run("with namespace", func(t *testing.T) {
		wr, err := NewWorkloadResource(resources.WorkloadV1Config, "my-ns")
		require.NoError(t, err)
		require.NotNil(t, wr)
		assert.Equal(t, "my-ns", wr.GetNamespace())
		assert.Equal(t, resources.WorkloadV1Config, wr.GetConfig())
	})
	t.Run("without namespace", func(t *testing.T) {
		wr, err := NewWorkloadResource(resources.WorkloadV1Config, "")
		require.NoError(t, err)
		require.NotNil(t, wr)
		assert.Empty(t, wr.GetNamespace())
		assert.Equal(t, resources.WorkloadV1Config, wr.GetConfig())
	})
}
func TestGenerateWorkloadCR_Validation(t *testing.T) {
	wr, err := NewWorkloadResource(resources.WorkloadV1Config, "my-ns")
	require.NoError(t, err)
	tests := []struct {
		name    string
		params  synth.CreateWorkloadParams
		wantErr string
	}{
		{
			name: "missing namespace",
			params: synth.CreateWorkloadParams{
				ProjectName:   "my-project",
				ComponentName: "my-comp",
				ImageURL:      "nginx:latest",
			},
			wantErr: "namespace name is required",
		},
		{
			name: "missing project",
			params: synth.CreateWorkloadParams{
				NamespaceName: "my-ns",
				ComponentName: "my-comp",
				ImageURL:      "nginx:latest",
			},
			wantErr: "project name is required",
		},
		{
			name: "missing component",
			params: synth.CreateWorkloadParams{
				NamespaceName: "my-ns",
				ProjectName:   "my-project",
				ImageURL:      "nginx:latest",
			},
			wantErr: "component name is required",
		},
		{
			name: "missing image URL",
			params: synth.CreateWorkloadParams{
				NamespaceName: "my-ns",
				ProjectName:   "my-project",
				ComponentName: "my-comp",
			},
			wantErr: "image URL is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := wr.GenerateWorkloadCR(tt.params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
func TestGenerateWorkloadCR_BasicSuccess(t *testing.T) {
	wr, err := NewWorkloadResource(resources.WorkloadV1Config, "my-ns")
	require.NoError(t, err)
	params := synth.CreateWorkloadParams{
		NamespaceName: "my-ns",
		ProjectName:   "my-project",
		ComponentName: "my-comp",
		ImageURL:      "nginx:latest",
	}
	workloadCR, err := wr.GenerateWorkloadCR(params)
	require.NoError(t, err)
	require.NotNil(t, workloadCR)
	// Assert TypeMeta
	assert.Equal(t, "openchoreo.dev/v1alpha1", workloadCR.APIVersion)
	assert.Equal(t, "Workload", workloadCR.Kind)
	// Assert ObjectMeta
	assert.Equal(t, "my-comp-workload", workloadCR.Name)
	assert.Equal(t, "my-ns", workloadCR.Namespace)
	// Assert Spec.Owner
	assert.Equal(t, "my-project", workloadCR.Spec.Owner.ProjectName)
	assert.Equal(t, "my-comp", workloadCR.Spec.Owner.ComponentName)
	// Assert Spec.Container
	assert.Equal(t, "nginx:latest", workloadCR.Spec.Container.Image)
	assert.Empty(t, workloadCR.Spec.Container.Command)
	assert.Empty(t, workloadCR.Spec.Container.Args)
	assert.Empty(t, workloadCR.Spec.Container.Env)
	// Assert no endpoints or dependencies on a basic workload
	assert.Empty(t, workloadCR.Spec.Endpoints)
	assert.Nil(t, workloadCR.Spec.Dependencies)
}
func TestGenerateWorkloadCR_WithDescriptorFile(t *testing.T) {
	wr, err := NewWorkloadResource(resources.WorkloadV1Config, "my-ns")
	require.NoError(t, err)
	// Create a temp descriptor file with endpoints
	descriptorYAML := `apiVersion: openchoreo.dev/v1alpha1
metadata:
  name: test-workload
endpoints:
  - name: http-ep
    port: 8080
    type: HTTP
    basePath: /api
    visibility:
      - project
`
	tmpDir := t.TempDir()
	descriptorPath := filepath.Join(tmpDir, "workload.yaml")
	require.NoError(t, os.WriteFile(descriptorPath, []byte(descriptorYAML), 0600))
	params := synth.CreateWorkloadParams{
		NamespaceName: "my-ns",
		ProjectName:   "my-project",
		ComponentName: "my-comp",
		ImageURL:      "nginx:latest",
		FilePath:      descriptorPath,
	}
	workloadCR, err := wr.GenerateWorkloadCR(params)
	require.NoError(t, err)
	require.NotNil(t, workloadCR)
	// Verify metadata
	assert.Equal(t, "my-comp-workload", workloadCR.Name)
	assert.Equal(t, "my-ns", workloadCR.Namespace)
	// Verify endpoint was populated from the descriptor
	require.Contains(t, workloadCR.Spec.Endpoints, "http-ep")
	ep := workloadCR.Spec.Endpoints["http-ep"]
	assert.Equal(t, openchoreov1alpha1.EndpointTypeHTTP, ep.Type)
	assert.Equal(t, int32(8080), ep.Port)
	assert.Equal(t, "/api", ep.BasePath)
	require.Len(t, ep.Visibility, 1)
	assert.Equal(t, openchoreov1alpha1.EndpointVisibilityProject, ep.Visibility[0])
}
func TestCreateWorkload_OutputToFile(t *testing.T) {
	wr, err := NewWorkloadResource(resources.WorkloadV1Config, "my-ns")
	require.NoError(t, err)
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "out.yaml")
	// Capture stdout to avoid polluting test output
	params := synth.CreateWorkloadParams{
		NamespaceName: "my-ns",
		ProjectName:   "my-project",
		ComponentName: "my-comp",
		ImageURL:      "nginx:latest",
		OutputPath:    outPath,
	}
	err = wr.CreateWorkload(params)
	require.NoError(t, err)
	// Verify file was written and contains expected content
	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	yamlStr := string(data)
	assert.Contains(t, yamlStr, "apiVersion: openchoreo.dev/v1alpha1")
	assert.Contains(t, yamlStr, "kind: Workload")
	assert.Contains(t, yamlStr, "name: my-comp-workload")
	assert.Contains(t, yamlStr, "namespace: my-ns")
	assert.Contains(t, yamlStr, "projectName: my-project")
	assert.Contains(t, yamlStr, "componentName: my-comp")
	assert.Contains(t, yamlStr, "image: nginx:latest")
}
func TestCreateWorkload_OutputToStdout(t *testing.T) {
	wr, err := NewWorkloadResource(resources.WorkloadV1Config, "my-ns")
	require.NoError(t, err)
	// Redirect stdout to capture the output
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	params := synth.CreateWorkloadParams{
		NamespaceName: "my-ns",
		ProjectName:   "my-project",
		ComponentName: "my-comp",
		ImageURL:      "busybox:1.36",
		// No OutputPath → writes to stdout
	}
	createErr := wr.CreateWorkload(params)
	w.Close()
	os.Stdout = oldStdout
	// Read captured output
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])
	require.NoError(t, createErr)
	assert.Contains(t, output, "apiVersion: openchoreo.dev/v1alpha1")
	assert.Contains(t, output, "kind: Workload")
	assert.Contains(t, output, "image: busybox:1.36")
}
func TestCreateWorkload_WithDescriptorFile(t *testing.T) {
	wr, err := NewWorkloadResource(resources.WorkloadV1Config, "my-ns")
	require.NoError(t, err)
	// Create a temp descriptor file
	descriptorYAML := `apiVersion: openchoreo.dev/v1alpha1
metadata:
  name: test-workload
endpoints:
  - name: grpc-ep
    port: 9090
    type: gRPC
`
	tmpDir := t.TempDir()
	descriptorPath := filepath.Join(tmpDir, "workload.yaml")
	require.NoError(t, os.WriteFile(descriptorPath, []byte(descriptorYAML), 0600))
	outPath := filepath.Join(tmpDir, "out.yaml")
	params := synth.CreateWorkloadParams{
		NamespaceName: "my-ns",
		ProjectName:   "my-project",
		ComponentName: "my-comp",
		ImageURL:      "myimg:v1",
		FilePath:      descriptorPath,
		OutputPath:    outPath,
	}
	err = wr.CreateWorkload(params)
	require.NoError(t, err)
	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	yamlStr := string(data)
	assert.Contains(t, yamlStr, "grpc-ep")
	assert.Contains(t, yamlStr, "image: myimg:v1")
}
func TestCreateWorkload_Validation(t *testing.T) {
	wr, err := NewWorkloadResource(resources.WorkloadV1Config, "my-ns")
	require.NoError(t, err)
	err = wr.CreateWorkload(synth.CreateWorkloadParams{
		// Missing all required fields
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace name is required")
}
