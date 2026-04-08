// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package synth

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/testhelpers"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func TestValidateConversionParams(t *testing.T) {
	tests := []struct {
		name    string
		params  api.CreateWorkloadParams
		wantErr string
	}{
		{
			name: "valid params",
			params: api.CreateWorkloadParams{
				NamespaceName: "ns",
				ProjectName:   "proj",
				ComponentName: "comp",
				ImageURL:      "image:latest",
			},
		},
		{
			name: "missing namespace",
			params: api.CreateWorkloadParams{
				ProjectName:   "proj",
				ComponentName: "comp",
				ImageURL:      "image:latest",
			},
			wantErr: "namespace name is required",
		},
		{
			name: "missing project",
			params: api.CreateWorkloadParams{
				NamespaceName: "ns",
				ComponentName: "comp",
				ImageURL:      "image:latest",
			},
			wantErr: "project name is required",
		},
		{
			name: "missing component",
			params: api.CreateWorkloadParams{
				NamespaceName: "ns",
				ProjectName:   "proj",
				ImageURL:      "image:latest",
			},
			wantErr: "component name is required",
		},
		{
			name: "missing image URL",
			params: api.CreateWorkloadParams{
				NamespaceName: "ns",
				ProjectName:   "proj",
				ComponentName: "comp",
			},
			wantErr: "image URL is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConversionParams(tt.params)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateBaseWorkload(t *testing.T) {
	tests := []struct {
		name         string
		workloadName string
		params       api.CreateWorkloadParams
		wantName     string
		wantNS       string
		wantProject  string
		wantComp     string
		wantImage    string
	}{
		{
			name:         "creates workload with all fields",
			workloadName: "my-workload",
			params: api.CreateWorkloadParams{
				NamespaceName: "test-ns",
				ProjectName:   "test-project",
				ComponentName: "test-component",
				ImageURL:      "gcr.io/test/image:v1",
			},
			wantName:    "my-workload",
			wantNS:      "test-ns",
			wantProject: "test-project",
			wantComp:    "test-component",
			wantImage:   "gcr.io/test/image:v1",
		},
		{
			name:         "creates workload with minimal fields",
			workloadName: "minimal",
			params: api.CreateWorkloadParams{
				NamespaceName: "ns",
				ProjectName:   "proj",
				ComponentName: "comp",
				ImageURL:      "img",
			},
			wantName:    "minimal",
			wantNS:      "ns",
			wantProject: "proj",
			wantComp:    "comp",
			wantImage:   "img",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := createBaseWorkload(tt.workloadName, tt.params)
			require.NotNil(t, w)
			assert.Equal(t, "openchoreo.dev/v1alpha1", w.APIVersion)
			assert.Equal(t, "Workload", w.Kind)
			assert.Equal(t, tt.wantName, w.Name)
			assert.Equal(t, tt.wantNS, w.Namespace)
			assert.Equal(t, tt.wantProject, w.Spec.Owner.ProjectName)
			assert.Equal(t, tt.wantComp, w.Spec.Owner.ComponentName)
			assert.Equal(t, tt.wantImage, w.Spec.Container.Image)
			assert.Nil(t, w.Spec.Endpoints)
			assert.Nil(t, w.Spec.Dependencies)
		})
	}
}

func TestAddDependenciesFromDescriptor(t *testing.T) {
	baseWorkload := func() *openchoreov1alpha1.Workload {
		return &openchoreov1alpha1.Workload{
			Spec: openchoreov1alpha1.WorkloadSpec{
				WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{},
			},
		}
	}

	tests := []struct {
		name           string
		descriptor     *WorkloadDescriptor
		wantEndpoints  int
		wantNilDeps    bool
		wantErr        string
		wantComponent  string
		wantVisibility openchoreov1alpha1.EndpointVisibility
	}{
		{
			name: "valid dependencies with project visibility",
			descriptor: &WorkloadDescriptor{
				Dependencies: &WorkloadDescriptorDependencies{
					Endpoints: []WorkloadDescriptorConnection{
						{
							Project:    "proj-a",
							Component:  "svc-b",
							Name:       "http-ep",
							Visibility: "project",
							EnvBindings: WorkloadDescriptorConnectionEnvBindings{
								Address: "SVC_B_URL",
								Host:    "SVC_B_HOST",
								Port:    "SVC_B_PORT",
							},
						},
						{
							Component:  "svc-c",
							Name:       "grpc-ep",
							Visibility: "namespace",
							EnvBindings: WorkloadDescriptorConnectionEnvBindings{
								Address: "SVC_C_URL",
							},
						},
					},
				},
			},
			wantEndpoints:  2,
			wantComponent:  "svc-b",
			wantVisibility: openchoreov1alpha1.EndpointVisibilityProject,
		},
		{
			name: "invalid visibility returns error",
			descriptor: &WorkloadDescriptor{
				Dependencies: &WorkloadDescriptorDependencies{
					Endpoints: []WorkloadDescriptorConnection{
						{
							Component:  "svc-a",
							Name:       "http-ep",
							Visibility: "external",
							EnvBindings: WorkloadDescriptorConnectionEnvBindings{
								Address: "SVC_A_URL",
							},
						},
					},
				},
			},
			wantErr: "invalid dependency endpoint visibility",
		},
		{
			name: "missing component returns error",
			descriptor: &WorkloadDescriptor{
				Dependencies: &WorkloadDescriptorDependencies{
					Endpoints: []WorkloadDescriptorConnection{
						{
							Name:       "http-ep",
							Visibility: "project",
							EnvBindings: WorkloadDescriptorConnectionEnvBindings{
								Address: "URL",
							},
						},
					},
				},
			},
			wantErr: "component is required",
		},
		{
			name: "missing name returns error",
			descriptor: &WorkloadDescriptor{
				Dependencies: &WorkloadDescriptorDependencies{
					Endpoints: []WorkloadDescriptorConnection{
						{
							Component:  "svc-a",
							Visibility: "project",
							EnvBindings: WorkloadDescriptorConnectionEnvBindings{
								Address: "URL",
							},
						},
					},
				},
			},
			wantErr: "name is required",
		},
		{
			name: "missing visibility returns error",
			descriptor: &WorkloadDescriptor{
				Dependencies: &WorkloadDescriptorDependencies{
					Endpoints: []WorkloadDescriptorConnection{
						{
							Component: "svc-a",
							Name:      "http-ep",
							EnvBindings: WorkloadDescriptorConnectionEnvBindings{
								Address: "URL",
							},
						},
					},
				},
			},
			wantErr: "visibility is required",
		},
		{
			name: "empty dependency endpoints",
			descriptor: &WorkloadDescriptor{
				Dependencies: &WorkloadDescriptorDependencies{
					Endpoints: []WorkloadDescriptorConnection{},
				},
			},
			wantNilDeps: true,
		},
		{
			name:        "nil dependencies",
			descriptor:  &WorkloadDescriptor{},
			wantNilDeps: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := baseWorkload()
			err := addDependenciesFromDescriptor(w, tt.descriptor)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.wantNilDeps {
				assert.Nil(t, w.Spec.Dependencies)
				return
			}
			require.NotNil(t, w.Spec.Dependencies)
			assert.Len(t, w.Spec.Dependencies.Endpoints, tt.wantEndpoints)
			assert.Equal(t, tt.wantComponent, w.Spec.Dependencies.Endpoints[0].Component)
			assert.Equal(t, tt.wantVisibility, w.Spec.Dependencies.Endpoints[0].Visibility)
			// verify env bindings are mapped
			assert.Equal(t, "SVC_B_URL", w.Spec.Dependencies.Endpoints[0].EnvBindings.Address)
			assert.Equal(t, "SVC_B_HOST", w.Spec.Dependencies.Endpoints[0].EnvBindings.Host)
			assert.Equal(t, "SVC_B_PORT", w.Spec.Dependencies.Endpoints[0].EnvBindings.Port)
		})
	}
}

func TestReadWorkloadDescriptorDependencies(t *testing.T) {
	tests := []struct {
		name          string
		yaml          string
		wantNilDeps   bool
		wantEndpoints int
		wantComponent string
	}{
		{
			name: "parses dependencies with endpoints",
			yaml: `apiVersion: openchoreo.dev/v1alpha1
metadata:
  name: my-service
dependencies:
  endpoints:
    - component: postgres
      name: tcp
      visibility: project
      envBindings:
        address: DATABASE_URL
    - component: redis
      name: tcp
      visibility: namespace
      envBindings:
        address: REDIS_URL
        host: REDIS_HOST
`,
			wantEndpoints: 2,
			wantComponent: "postgres",
		},
		{
			name: "no dependencies section",
			yaml: `apiVersion: openchoreo.dev/v1alpha1
metadata:
  name: my-service
`,
			wantNilDeps: true,
		},
		{
			name: "old connections field is ignored",
			yaml: `apiVersion: openchoreo.dev/v1alpha1
metadata:
  name: my-service
connections:
  - component: postgres
    name: tcp
    visibility: project
    envBindings:
      address: DATABASE_URL
`,
			wantNilDeps: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.yaml)
			descriptor, err := readWorkloadDescriptorFromReader(reader)
			require.NoError(t, err)
			if tt.wantNilDeps {
				assert.True(t, descriptor.Dependencies == nil || len(descriptor.Dependencies.Endpoints) == 0)
				return
			}
			require.NotNil(t, descriptor.Dependencies)
			assert.Len(t, descriptor.Dependencies.Endpoints, tt.wantEndpoints)
			assert.Equal(t, tt.wantComponent, descriptor.Dependencies.Endpoints[0].Component)
		})
	}
}

func TestAddEndpointsFromDescriptorVisibilityValidation(t *testing.T) {
	baseWorkload := func() *openchoreov1alpha1.Workload {
		return &openchoreov1alpha1.Workload{
			Spec: openchoreov1alpha1.WorkloadSpec{
				WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{},
			},
		}
	}

	tests := []struct {
		name    string
		desc    *WorkloadDescriptor
		wantErr string
	}{
		{
			name: "valid visibility values accepted",
			desc: &WorkloadDescriptor{
				Endpoints: []WorkloadDescriptorEndpoint{
					{Name: "ep1", Port: 8080, Type: "HTTP", Visibility: []string{"project", "external"}},
				},
			},
		},
		{
			name: "invalid visibility rejected",
			desc: &WorkloadDescriptor{
				Endpoints: []WorkloadDescriptorEndpoint{
					{Name: "ep1", Port: 8080, Type: "HTTP", Visibility: []string{"public"}},
				},
			},
			wantErr: "invalid endpoint visibility",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := baseWorkload()
			err := addEndpointsFromDescriptor(w, tt.desc, "/tmp/workload.yaml")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestConvertEnvVarSource(t *testing.T) {
	tests := []struct {
		name       string
		source     *WorkloadDescriptorEnvVarSource
		wantNil    bool
		wantSecret string
		wantKey    string
	}{
		{
			name: "secret key ref",
			source: &WorkloadDescriptorEnvVarSource{
				SecretKeyRef: &WorkloadDescriptorSecretKeyRef{
					Name: "my-secret",
					Key:  "password",
				},
			},
			wantSecret: "my-secret",
			wantKey:    "password",
		},
		{
			name:    "nil source",
			source:  nil,
			wantNil: true,
		},
		{
			name:   "source without secret ref",
			source: &WorkloadDescriptorEnvVarSource{Path: "/some/path"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertEnvVarSource(tt.source)
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			if tt.wantSecret != "" {
				require.NotNil(t, result.SecretKeyRef)
				assert.Equal(t, tt.wantSecret, result.SecretKeyRef.Name)
				assert.Equal(t, tt.wantKey, result.SecretKeyRef.Key)
			} else {
				assert.Nil(t, result.SecretKeyRef)
			}
		})
	}
}

func TestCreateBasicWorkload(t *testing.T) {
	tests := []struct {
		name    string
		params  api.CreateWorkloadParams
		wantErr string
	}{
		{
			name: "creates workload from params",
			params: api.CreateWorkloadParams{
				NamespaceName: "test-ns",
				ProjectName:   "test-project",
				ComponentName: "test-component",
				ImageURL:      "gcr.io/test/image:v1",
			},
		},
		{
			name: "fails on missing image",
			params: api.CreateWorkloadParams{
				NamespaceName: "ns",
				ProjectName:   "proj",
				ComponentName: "comp",
			},
			wantErr: "image URL is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := CreateBasicWorkload(tt.params)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, w)
			assert.Equal(t, tt.params.ComponentName+"-workload", w.Name)
			assert.Equal(t, tt.params.NamespaceName, w.Namespace)
			assert.Equal(t, tt.params.ProjectName, w.Spec.Owner.ProjectName)
			assert.Equal(t, tt.params.ComponentName, w.Spec.Owner.ComponentName)
			assert.Equal(t, tt.params.ImageURL, w.Spec.Container.Image)
		})
	}
}

func TestReadWorkloadDescriptor(t *testing.T) {
	t.Run("reads valid descriptor from file", func(t *testing.T) {
		dir := t.TempDir()
		content := `apiVersion: openchoreo.dev/v1alpha1
metadata:
  name: my-service
endpoints:
  - name: http
    port: 8080
    type: REST
`
		testhelpers.WriteYAML(t, dir, "workload.yaml", content)

		desc, err := readWorkloadDescriptor(filepath.Join(dir, "workload.yaml"))
		require.NoError(t, err)
		assert.Equal(t, "my-service", desc.Metadata.Name)
		assert.Len(t, desc.Endpoints, 1)
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := readWorkloadDescriptor("/nonexistent/workload.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open file")
	})
}

func TestReadSchemaFile(t *testing.T) {
	t.Run("reads schema content", func(t *testing.T) {
		dir := t.TempDir()
		testhelpers.WriteYAML(t, dir, "schema.json", `{"openapi":"3.0.0"}`)

		content, err := readSchemaFile(filepath.Join(dir, "schema.json"))
		require.NoError(t, err)
		assert.Equal(t, `{"openapi":"3.0.0"}`, content)
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := readSchemaFile("/nonexistent/schema.json")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read schema file")
	})
}

func TestAddEndpointsFromDescriptorWithSchemaFile(t *testing.T) {
	dir := t.TempDir()
	descriptorPath := filepath.Join(dir, "workload.yaml")

	// Write a schema file in the same directory
	schemaContent := `openapi: "3.0.0"
info:
  title: Test API
  version: "1.0"
`
	testhelpers.WriteYAML(t, dir, "openapi.yaml", schemaContent)

	w := &openchoreov1alpha1.Workload{
		Spec: openchoreov1alpha1.WorkloadSpec{
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{},
		},
	}
	desc := &WorkloadDescriptor{
		Endpoints: []WorkloadDescriptorEndpoint{
			{
				Name:       "api",
				Port:       8080,
				Type:       "REST",
				SchemaFile: "openapi.yaml",
				Visibility: []string{"external"},
			},
		},
	}

	err := addEndpointsFromDescriptor(w, desc, descriptorPath)
	require.NoError(t, err)
	require.Contains(t, w.Spec.Endpoints, "api")
	ep := w.Spec.Endpoints["api"]
	require.NotNil(t, ep.Schema)
	assert.Equal(t, "REST", ep.Schema.Type)
	assert.Equal(t, schemaContent, ep.Schema.Content)
}

func TestAddEndpointsFromDescriptorSchemaFileMissing(t *testing.T) {
	dir := t.TempDir()
	descriptorPath := filepath.Join(dir, "workload.yaml")

	w := &openchoreov1alpha1.Workload{
		Spec: openchoreov1alpha1.WorkloadSpec{
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{},
		},
	}
	desc := &WorkloadDescriptor{
		Endpoints: []WorkloadDescriptorEndpoint{
			{
				Name:       "api",
				Port:       8080,
				Type:       "REST",
				SchemaFile: "nonexistent.yaml",
				Visibility: []string{"external"},
			},
		},
	}

	err := addEndpointsFromDescriptor(w, desc, descriptorPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read schema file")
}

func TestAddConfigurationsFromDescriptor(t *testing.T) {
	dir := t.TempDir()
	descriptorPath := filepath.Join(dir, "workload.yaml")
	configContent := "server.port=8080\nserver.host=0.0.0.0\n"
	testhelpers.WriteYAML(t, dir, "app.properties", configContent)

	tests := []struct {
		name       string
		descriptor *WorkloadDescriptor
		wantEnvLen int
		wantFiles  int
		wantErr    string
		verify     func(t *testing.T, w *openchoreov1alpha1.Workload)
	}{
		{
			name: "env vars with inline values",
			descriptor: &WorkloadDescriptor{
				Configurations: WorkloadDescriptorConfiguration{
					Env: []WorkloadDescriptorEnvVar{
						{Name: "APP_PORT", Value: "8080"},
						{Name: "APP_ENV", Value: "prod"},
					},
				},
			},
			wantEnvLen: 2,
			verify: func(t *testing.T, w *openchoreov1alpha1.Workload) {
				assert.Equal(t, "APP_PORT", w.Spec.Container.Env[0].Key)
				assert.Equal(t, "8080", w.Spec.Container.Env[0].Value)
				assert.Equal(t, "APP_ENV", w.Spec.Container.Env[1].Key)
				assert.Equal(t, "prod", w.Spec.Container.Env[1].Value)
			},
		},
		{
			name: "env var from secret ref",
			descriptor: &WorkloadDescriptor{
				Configurations: WorkloadDescriptorConfiguration{
					Env: []WorkloadDescriptorEnvVar{
						{
							Name: "DB_PASSWORD",
							ValueFrom: &WorkloadDescriptorEnvVarSource{
								SecretKeyRef: &WorkloadDescriptorSecretKeyRef{
									Name: "db-secret",
									Key:  "password",
								},
							},
						},
					},
				},
			},
			wantEnvLen: 1,
			verify: func(t *testing.T, w *openchoreov1alpha1.Workload) {
				env := w.Spec.Container.Env[0]
				assert.Equal(t, "DB_PASSWORD", env.Key)
				assert.Empty(t, env.Value)
				require.NotNil(t, env.ValueFrom)
				require.NotNil(t, env.ValueFrom.SecretKeyRef)
				assert.Equal(t, "db-secret", env.ValueFrom.SecretKeyRef.Name)
				assert.Equal(t, "password", env.ValueFrom.SecretKeyRef.Key)
			},
		},
		{
			name: "file with inline value",
			descriptor: &WorkloadDescriptor{
				Configurations: WorkloadDescriptorConfiguration{
					Files: []WorkloadDescriptorFileVar{
						{Name: "config", MountPath: "/etc/app/config.yaml", Value: "key: value"},
					},
				},
			},
			wantFiles: 1,
			verify: func(t *testing.T, w *openchoreov1alpha1.Workload) {
				f := w.Spec.Container.Files[0]
				assert.Equal(t, "config", f.Key)
				assert.Equal(t, "/etc/app/config.yaml", f.MountPath)
				assert.Equal(t, "key: value", f.Value)
				assert.Nil(t, f.ValueFrom)
			},
		},
		{
			name: "file from secret ref",
			descriptor: &WorkloadDescriptor{
				Configurations: WorkloadDescriptorConfiguration{
					Files: []WorkloadDescriptorFileVar{
						{
							Name:      "tls-cert",
							MountPath: "/etc/tls/cert.pem",
							ValueFrom: &WorkloadDescriptorEnvVarSource{
								SecretKeyRef: &WorkloadDescriptorSecretKeyRef{
									Name: "tls-secret",
									Key:  "cert",
								},
							},
						},
					},
				},
			},
			wantFiles: 1,
			verify: func(t *testing.T, w *openchoreov1alpha1.Workload) {
				f := w.Spec.Container.Files[0]
				assert.Equal(t, "tls-cert", f.Key)
				require.NotNil(t, f.ValueFrom)
				require.NotNil(t, f.ValueFrom.SecretKeyRef)
				assert.Equal(t, "tls-secret", f.ValueFrom.SecretKeyRef.Name)
			},
		},
		{
			name: "file from path",
			descriptor: &WorkloadDescriptor{
				Configurations: WorkloadDescriptorConfiguration{
					Files: []WorkloadDescriptorFileVar{
						{
							Name:      "app-config",
							MountPath: "/etc/app/config.properties",
							ValueFrom: &WorkloadDescriptorEnvVarSource{
								Path: "app.properties",
							},
						},
					},
				},
			},
			wantFiles: 1,
			verify: func(t *testing.T, w *openchoreov1alpha1.Workload) {
				f := w.Spec.Container.Files[0]
				assert.Equal(t, "app-config", f.Key)
				assert.Equal(t, configContent, f.Value)
				assert.Nil(t, f.ValueFrom)
			},
		},
		{
			name: "file from missing path returns error",
			descriptor: &WorkloadDescriptor{
				Configurations: WorkloadDescriptorConfiguration{
					Files: []WorkloadDescriptorFileVar{
						{
							Name:      "missing",
							MountPath: "/etc/app/missing.conf",
							ValueFrom: &WorkloadDescriptorEnvVarSource{
								Path: "nonexistent.conf",
							},
						},
					},
				},
			},
			wantErr: "failed to read file",
		},
		{
			name:       "empty configurations",
			descriptor: &WorkloadDescriptor{},
			verify: func(t *testing.T, w *openchoreov1alpha1.Workload) {
				assert.Nil(t, w.Spec.Container.Env)
				assert.Nil(t, w.Spec.Container.Files)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &openchoreov1alpha1.Workload{
				Spec: openchoreov1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{},
				},
			}
			err := addConfigurationsFromDescriptor(w, tt.descriptor, descriptorPath)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.wantEnvLen > 0 {
				assert.Len(t, w.Spec.Container.Env, tt.wantEnvLen)
			}
			if tt.wantFiles > 0 {
				assert.Len(t, w.Spec.Container.Files, tt.wantFiles)
			}
			if tt.verify != nil {
				tt.verify(t, w)
			}
		})
	}
}

func TestConvertWorkloadDescriptorToWorkloadCR(t *testing.T) {
	dir := t.TempDir()
	descriptorPath := filepath.Join(dir, "workload.yaml")

	// Write a schema file
	testhelpers.WriteYAML(t, dir, "openapi.yaml", `openapi: "3.0.0"`)

	descriptorContent := `apiVersion: openchoreo.dev/v1alpha1
metadata:
  name: my-service
endpoints:
  - name: http
    port: 8080
    type: REST
    basePath: /api
    visibility:
      - external
    schemaFile: openapi.yaml
dependencies:
  endpoints:
    - component: db-service
      name: tcp
      visibility: project
      envBindings:
        address: DB_URL
configurations:
  env:
    - name: LOG_LEVEL
      value: info
  files:
    - name: cfg
      mountPath: /etc/app/cfg.yaml
      value: "key: val"
`
	testhelpers.WriteYAML(t, dir, "workload.yaml", descriptorContent)

	params := api.CreateWorkloadParams{
		NamespaceName: "test-ns",
		ProjectName:   "test-project",
		ComponentName: "test-comp",
		ImageURL:      "gcr.io/img:v1",
	}

	t.Run("full conversion", func(t *testing.T) {
		w, err := ConvertWorkloadDescriptorToWorkloadCR(descriptorPath, params)
		require.NoError(t, err)
		require.NotNil(t, w)

		yamlBytes, err := ConvertWorkloadCRToYAML(w)
		require.NoError(t, err)

		wantYAML := `apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: test-comp-workload
  namespace: test-ns
spec:
  owner:
    projectName: test-project
    componentName: test-comp
  container:
    image: gcr.io/img:v1
    env:
      - key: LOG_LEVEL
        value: info
    files:
      - key: cfg
        mountPath: /etc/app/cfg.yaml
        value: "key: val"
  endpoints:
    http:
      port: 8080
      type: REST
      basePath: /api
      schema:
        type: REST
        content: 'openapi: "3.0.0"'
      visibility:
        - external
  dependencies:
    endpoints:
      - component: db-service
        name: tcp
        visibility: project
        envBindings:
          address: DB_URL
`
		testhelpers.AssertYAMLEquals(t, wantYAML, string(yamlBytes))
	})

	t.Run("invalid params", func(t *testing.T) {
		_, err := ConvertWorkloadDescriptorToWorkloadCR(descriptorPath, api.CreateWorkloadParams{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "namespace name is required")
	})

	t.Run("missing descriptor file", func(t *testing.T) {
		_, err := ConvertWorkloadDescriptorToWorkloadCR("/nonexistent/workload.yaml", params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read workload descriptor")
	})

	t.Run("descriptor with invalid endpoint visibility propagates error", func(t *testing.T) {
		badDir := t.TempDir()
		badContent := `apiVersion: openchoreo.dev/v1alpha1
metadata:
  name: bad-service
endpoints:
  - name: ep
    port: 80
    type: REST
    visibility:
      - bogus
`
		testhelpers.WriteYAML(t, badDir, "workload.yaml", badContent)
		_, err := ConvertWorkloadDescriptorToWorkloadCR(filepath.Join(badDir, "workload.yaml"), params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `invalid endpoint visibility "bogus" for endpoint "ep"`)
	})

	t.Run("descriptor with invalid dependency propagates error", func(t *testing.T) {
		badDir := t.TempDir()
		badContent := `apiVersion: openchoreo.dev/v1alpha1
metadata:
  name: bad-deps
dependencies:
  endpoints:
    - name: tcp
      visibility: project
      envBindings:
        address: URL
`
		testhelpers.WriteYAML(t, badDir, "workload.yaml", badContent)
		_, err := ConvertWorkloadDescriptorToWorkloadCR(filepath.Join(badDir, "workload.yaml"), params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "component is required")
	})

	t.Run("descriptor with missing config file propagates error", func(t *testing.T) {
		badDir := t.TempDir()
		badContent := `apiVersion: openchoreo.dev/v1alpha1
metadata:
  name: bad-cfg
configurations:
  files:
    - name: cfg
      mountPath: /etc/cfg
      valueFrom:
        path: does-not-exist.conf
`
		testhelpers.WriteYAML(t, badDir, "workload.yaml", badContent)
		_, err := ConvertWorkloadDescriptorToWorkloadCR(filepath.Join(badDir, "workload.yaml"), params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read file")
	})
}

func TestConvertWorkloadCRToYAML(t *testing.T) {
	tests := []struct {
		name     string
		workload *openchoreov1alpha1.Workload
		wantYAML string
	}{
		{
			name: "valid workload with endpoints",
			workload: &openchoreov1alpha1.Workload{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "openchoreo.dev/v1alpha1",
					Kind:       "Workload",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: openchoreov1alpha1.WorkloadSpec{
					Owner: openchoreov1alpha1.WorkloadOwner{
						ProjectName:   "test-project",
						ComponentName: "test-component",
					},
					WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
						Container: openchoreov1alpha1.Container{
							Image: "gcr.io/test/image:v1",
						},
						Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
							"http": {
								Port: 8080,
								Type: "REST",
							},
						},
					},
				},
			},
			wantYAML: `apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: test-workload
  namespace: test-ns
spec:
  owner:
    projectName: test-project
    componentName: test-component
  container:
    image: gcr.io/test/image:v1
  endpoints:
    http:
      port: 8080
      type: REST
`,
		},
		{
			name: "valid workload without endpoints",
			workload: &openchoreov1alpha1.Workload{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "openchoreo.dev/v1alpha1",
					Kind:       "Workload",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "simple-workload",
				},
				Spec: openchoreov1alpha1.WorkloadSpec{
					Owner: openchoreov1alpha1.WorkloadOwner{
						ProjectName:   "proj",
						ComponentName: "comp",
					},
					WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
						Container: openchoreov1alpha1.Container{
							Image: "img:latest",
						},
					},
				},
			},
			wantYAML: `apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: simple-workload
spec:
  owner:
    projectName: proj
    componentName: comp
  container:
    image: img:latest
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlBytes, err := ConvertWorkloadCRToYAML(tt.workload)
			require.NoError(t, err)
			testhelpers.AssertYAMLEquals(t, tt.wantYAML, string(yamlBytes))
		})
	}
}
