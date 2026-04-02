// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

func makeWorkloadEntry(t *testing.T, wl *v1alpha1.Workload) *index.ResourceEntry {
	t.Helper()
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(wl)
	require.NoError(t, err)
	obj := &unstructured.Unstructured{Object: raw}
	obj.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("Workload"))
	return &index.ResourceEntry{Resource: obj}
}

func TestNewWorkload(t *testing.T) {
	tests := []struct {
		name    string
		entry   *index.ResourceEntry
		wantErr bool
	}{
		{
			name: "valid entry",
			entry: makeWorkloadEntry(t, &v1alpha1.Workload{
				Spec: v1alpha1.WorkloadSpec{
					WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
						Container: v1alpha1.Container{Image: "nginx:latest"},
					},
				},
			}),
		},
		{
			name:    "nil entry",
			entry:   nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wl, err := NewWorkload(tt.entry)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, wl)
		})
	}
}

func TestGetContainer(t *testing.T) {
	tests := []struct {
		name      string
		container v1alpha1.Container
		wantNil   bool
		validate  func(t *testing.T, result map[string]interface{})
	}{
		{
			name:      "empty image returns nil",
			container: v1alpha1.Container{},
			wantNil:   true,
		},
		{
			name:      "image only",
			container: v1alpha1.Container{Image: "nginx:latest"},
			validate: func(t *testing.T, result map[string]interface{}) {
				assert.Equal(t, "nginx:latest", result["image"])
				assert.NotContains(t, result, "command")
				assert.NotContains(t, result, "args")
				assert.NotContains(t, result, "env")
				assert.NotContains(t, result, "files")
			},
		},
		{
			name: "with command and args",
			container: v1alpha1.Container{
				Image:   "app:v1",
				Command: []string{"/bin/sh", "-c"},
				Args:    []string{"echo", "hello"},
			},
			validate: func(t *testing.T, result map[string]interface{}) {
				cmd := result["command"].([]interface{})
				require.Len(t, cmd, 2)
				assert.Equal(t, "/bin/sh", cmd[0])
				assert.Equal(t, "-c", cmd[1])

				args := result["args"].([]interface{})
				require.Len(t, args, 2)
				assert.Equal(t, "echo", args[0])
			},
		},
		{
			name: "with env vars - literal and secret ref",
			container: v1alpha1.Container{
				Image: "app:v1",
				Env: []v1alpha1.EnvVar{
					{Key: "PORT", Value: "8080"},
					{Key: "SECRET", ValueFrom: &v1alpha1.EnvVarValueFrom{
						SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "my-secret", Key: "password"},
					}},
				},
			},
			validate: func(t *testing.T, result map[string]interface{}) {
				env := result["env"].([]interface{})
				require.Len(t, env, 2)

				e0 := env[0].(map[string]interface{})
				assert.Equal(t, "PORT", e0["key"])
				assert.Equal(t, "8080", e0["value"])

				e1 := env[1].(map[string]interface{})
				assert.Equal(t, "SECRET", e1["key"])
				assert.NotContains(t, e1, "value")
				vf := e1["valueFrom"].(map[string]interface{})
				skr := vf["secretKeyRef"].(map[string]interface{})
				assert.Equal(t, "my-secret", skr["name"])
				assert.Equal(t, "password", skr["key"])
			},
		},
		{
			name: "with files - literal and secret ref",
			container: v1alpha1.Container{
				Image: "app:v1",
				Files: []v1alpha1.FileVar{
					{Key: "config.yaml", MountPath: "/etc/app/config.yaml", Value: "key: val"},
					{Key: "cert.pem", MountPath: "/etc/certs/cert.pem", ValueFrom: &v1alpha1.EnvVarValueFrom{
						SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "tls-secret", Key: "cert"},
					}},
				},
			},
			validate: func(t *testing.T, result map[string]interface{}) {
				files := result["files"].([]interface{})
				require.Len(t, files, 2)

				f0 := files[0].(map[string]interface{})
				assert.Equal(t, "config.yaml", f0["key"])
				assert.Equal(t, "/etc/app/config.yaml", f0["mountPath"])
				assert.Equal(t, "key: val", f0["value"])

				f1 := files[1].(map[string]interface{})
				assert.Equal(t, "cert.pem", f1["key"])
				assert.NotContains(t, f1, "value")
				vf := f1["valueFrom"].(map[string]interface{})
				skr := vf["secretKeyRef"].(map[string]interface{})
				assert.Equal(t, "tls-secret", skr["name"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wl := &Workload{
				Workload: &v1alpha1.Workload{
					Spec: v1alpha1.WorkloadSpec{
						WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
							Container: tt.container,
						},
					},
				},
			}
			result := wl.GetContainer()
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			tt.validate(t, result)
		})
	}
}

func TestStringsToInterfaceSlice(t *testing.T) {
	result := stringsToInterfaceSlice([]string{"a", "b", "c"})
	require.Len(t, result, 3)
	assert.Equal(t, "a", result[0])
	assert.Equal(t, "b", result[1])
	assert.Equal(t, "c", result[2])
}

func TestGetEndpoints(t *testing.T) {
	tests := []struct {
		name      string
		endpoints map[string]v1alpha1.WorkloadEndpoint
		wantNil   bool
		wantKeys  []string
		validate  func(t *testing.T, result map[string]interface{})
	}{
		{
			name:      "Nil endpoints returns nil",
			endpoints: nil,
			wantNil:   true,
		},
		{
			name:      "Empty endpoints returns nil",
			endpoints: map[string]v1alpha1.WorkloadEndpoint{},
			wantNil:   true,
		},
		{
			name: "Single HTTP endpoint with required fields only",
			endpoints: map[string]v1alpha1.WorkloadEndpoint{
				"http": {
					Type: v1alpha1.EndpointTypeHTTP,
					Port: 8080,
				},
			},
			wantKeys: []string{"http"},
			validate: func(t *testing.T, result map[string]interface{}) {
				ep := result["http"].(map[string]interface{})
				if ep["type"] != "HTTP" {
					t.Errorf("type = %v, want HTTP", ep["type"])
				}
				if ep["port"] != int64(8080) {
					t.Errorf("port = %v, want 8080", ep["port"])
				}
				// Optional fields should be absent
				if _, ok := ep["targetPort"]; ok {
					t.Error("targetPort should not be set when zero")
				}
				if _, ok := ep["displayName"]; ok {
					t.Error("displayName should not be set when empty")
				}
				if _, ok := ep["basePath"]; ok {
					t.Error("basePath should not be set when empty")
				}
				if _, ok := ep["visibility"]; ok {
					t.Error("visibility should not be set when empty")
				}
				if _, ok := ep["schema"]; ok {
					t.Error("schema should not be set when nil")
				}
			},
		},
		{
			name: "Endpoint with all optional fields",
			endpoints: map[string]v1alpha1.WorkloadEndpoint{
				"api": {
					Type:        v1alpha1.EndpointTypeHTTP,
					Port:        9090,
					TargetPort:  8080,
					DisplayName: "My API",
					BasePath:    "/api/v1",
					Visibility:  []v1alpha1.EndpointVisibility{v1alpha1.EndpointVisibilityExternal, v1alpha1.EndpointVisibilityProject},
					Schema: &v1alpha1.Schema{
						Type:    "openapi",
						Content: "openapi: 3.0.0",
					},
				},
			},
			wantKeys: []string{"api"},
			validate: func(t *testing.T, result map[string]interface{}) {
				ep := result["api"].(map[string]interface{})
				if ep["type"] != "HTTP" {
					t.Errorf("type = %v, want HTTP", ep["type"])
				}
				if ep["port"] != int64(9090) {
					t.Errorf("port = %v, want 9090", ep["port"])
				}
				if ep["targetPort"] != int64(8080) {
					t.Errorf("targetPort = %v, want 8080", ep["targetPort"])
				}
				if ep["displayName"] != "My API" {
					t.Errorf("displayName = %v, want My API", ep["displayName"])
				}
				if ep["basePath"] != "/api/v1" {
					t.Errorf("basePath = %v, want /api/v1", ep["basePath"])
				}

				vis := ep["visibility"].([]interface{})
				if len(vis) != 2 {
					t.Fatalf("visibility length = %d, want 2", len(vis))
				}
				if vis[0] != "external" {
					t.Errorf("visibility[0] = %v, want external", vis[0])
				}
				if vis[1] != "project" {
					t.Errorf("visibility[1] = %v, want project", vis[1])
				}

				schema := ep["schema"].(map[string]interface{})
				if schema["type"] != "openapi" {
					t.Errorf("schema.type = %v, want openapi", schema["type"])
				}
				if schema["content"] != "openapi: 3.0.0" {
					t.Errorf("schema.content = %v, want openapi: 3.0.0", schema["content"])
				}
			},
		},
		{
			name: "Multiple endpoints",
			endpoints: map[string]v1alpha1.WorkloadEndpoint{
				"http": {
					Type: v1alpha1.EndpointTypeHTTP,
					Port: 8080,
				},
				"grpc": {
					Type: v1alpha1.EndpointTypeGRPC,
					Port: 9090,
				},
				"tcp": {
					Type: v1alpha1.EndpointTypeTCP,
					Port: 5432,
				},
			},
			wantKeys: []string{"http", "grpc", "tcp"},
			validate: func(t *testing.T, result map[string]interface{}) {
				if len(result) != 3 {
					t.Errorf("expected 3 endpoints, got %d", len(result))
				}
				http := result["http"].(map[string]interface{})
				if http["port"] != int64(8080) {
					t.Errorf("http port = %v, want 8080", http["port"])
				}
				grpc := result["grpc"].(map[string]interface{})
				if grpc["type"] != "gRPC" {
					t.Errorf("grpc type = %v, want gRPC", grpc["type"])
				}
				tcp := result["tcp"].(map[string]interface{})
				if tcp["type"] != "TCP" {
					t.Errorf("tcp type = %v, want TCP", tcp["type"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wl := &Workload{
				Workload: &v1alpha1.Workload{
					Spec: v1alpha1.WorkloadSpec{
						WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
							Container: v1alpha1.Container{Image: "test:latest"},
							Endpoints: tt.endpoints,
						},
					},
				},
			}
			result := wl.GetEndpoints()
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("expected key %q in result", key)
				}
			}
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestGetDependencies(t *testing.T) {
	tests := []struct {
		name        string
		connections []v1alpha1.WorkloadConnection
		wantNil     bool
		wantLen     int
		validate    func(t *testing.T, result []interface{})
	}{
		{
			name:        "Nil connections returns nil",
			connections: nil,
			wantNil:     true,
		},
		{
			name:        "Empty connections returns nil",
			connections: []v1alpha1.WorkloadConnection{},
			wantNil:     true,
		},
		{
			name: "Single connection with required fields only",
			connections: []v1alpha1.WorkloadConnection{
				{
					Component:  "postgres",
					Name:       "tcp",
					Visibility: v1alpha1.EndpointVisibilityProject,
					EnvBindings: v1alpha1.ConnectionEnvBindings{
						Address: "DATABASE_URL",
					},
				},
			},
			wantLen: 1,
			validate: func(t *testing.T, result []interface{}) {
				conn := result[0].(map[string]interface{})
				if conn["component"] != "postgres" {
					t.Errorf("component = %v, want postgres", conn["component"])
				}
				if conn["name"] != "tcp" {
					t.Errorf("name = %v, want tcp", conn["name"])
				}
				if conn["visibility"] != "project" {
					t.Errorf("visibility = %v, want project", conn["visibility"])
				}
				// Optional fields should be absent
				if _, ok := conn["namespace"]; ok {
					t.Error("namespace should not be set when empty")
				}
				if _, ok := conn["project"]; ok {
					t.Error("project should not be set when empty")
				}
				if _, ok := conn["environmentMapping"]; ok {
					t.Error("environmentMapping should not be set when empty")
				}

				envBindings := conn["envBindings"].(map[string]interface{})
				if envBindings["address"] != "DATABASE_URL" {
					t.Errorf("envBindings.address = %v, want DATABASE_URL", envBindings["address"])
				}
			},
		},
		{
			name: "Connection with all optional fields",
			connections: []v1alpha1.WorkloadConnection{
				{
					Project:    "other-project",
					Component:  "redis",
					Name:       "tcp",
					Visibility: v1alpha1.EndpointVisibilityNamespace,
					EnvBindings: v1alpha1.ConnectionEnvBindings{
						Address:  "REDIS_URL",
						Host:     "REDIS_HOST",
						Port:     "REDIS_PORT",
						BasePath: "REDIS_PATH",
					},
				},
			},
			wantLen: 1,
			validate: func(t *testing.T, result []interface{}) {
				conn := result[0].(map[string]interface{})
				if conn["project"] != "other-project" {
					t.Errorf("project = %v, want other-project", conn["project"])
				}
				if conn["visibility"] != "namespace" {
					t.Errorf("visibility = %v, want namespace", conn["visibility"])
				}

				envBindings := conn["envBindings"].(map[string]interface{})
				if envBindings["address"] != "REDIS_URL" {
					t.Errorf("envBindings.address = %v, want REDIS_URL", envBindings["address"])
				}
				if envBindings["host"] != "REDIS_HOST" {
					t.Errorf("envBindings.host = %v, want REDIS_HOST", envBindings["host"])
				}
				if envBindings["port"] != "REDIS_PORT" {
					t.Errorf("envBindings.port = %v, want REDIS_PORT", envBindings["port"])
				}
				if envBindings["basePath"] != "REDIS_PATH" {
					t.Errorf("envBindings.basePath = %v, want REDIS_PATH", envBindings["basePath"])
				}
			},
		},
		{
			name: "Multiple connections preserves order",
			connections: []v1alpha1.WorkloadConnection{
				{
					Component:  "postgres",
					Name:       "tcp",
					Visibility: v1alpha1.EndpointVisibilityProject,
					EnvBindings: v1alpha1.ConnectionEnvBindings{
						Address: "DB_URL",
					},
				},
				{
					Component:  "nats",
					Name:       "tcp",
					Visibility: v1alpha1.EndpointVisibilityProject,
					EnvBindings: v1alpha1.ConnectionEnvBindings{
						Address: "NATS_URL",
					},
				},
			},
			wantLen: 2,
			validate: func(t *testing.T, result []interface{}) {
				first := result[0].(map[string]interface{})
				if first["component"] != "postgres" {
					t.Errorf("first connection component = %v, want postgres", first["component"])
				}
				second := result[1].(map[string]interface{})
				if second["component"] != "nats" {
					t.Errorf("second connection component = %v, want nats", second["component"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := v1alpha1.WorkloadTemplateSpec{
				Container: v1alpha1.Container{Image: "test:latest"},
			}
			if len(tt.connections) > 0 {
				spec.Dependencies = &v1alpha1.WorkloadDependencies{
					Endpoints: tt.connections,
				}
			}
			wl := &Workload{
				Workload: &v1alpha1.Workload{
					Spec: v1alpha1.WorkloadSpec{
						WorkloadTemplateSpec: spec,
					},
				},
			}
			result := wl.GetDependencies()
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if len(result) != tt.wantLen {
				t.Fatalf("length = %d, want %d", len(result), tt.wantLen)
			}
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}
