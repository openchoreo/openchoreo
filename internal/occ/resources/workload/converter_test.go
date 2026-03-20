// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package synth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
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

func TestConvertWorkloadCRToYAML(t *testing.T) {
	tests := []struct {
		name         string
		workload     *openchoreov1alpha1.Workload
		wantContains []string
		wantErr      bool
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
			wantContains: []string{
				"apiVersion: openchoreo.dev/v1alpha1",
				"kind: Workload",
				"name: test-workload",
				"namespace: test-ns",
				"projectName: test-project",
				"componentName: test-component",
				"image: gcr.io/test/image:v1",
				"http:",
				"port: 8080",
			},
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
			wantContains: []string{
				"apiVersion: openchoreo.dev/v1alpha1",
				"kind: Workload",
				"name: simple-workload",
				"image: img:latest",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlBytes, err := ConvertWorkloadCRToYAML(tt.workload)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			yamlStr := string(yamlBytes)
			for _, want := range tt.wantContains {
				assert.Contains(t, yamlStr, want)
			}
		})
	}
}
