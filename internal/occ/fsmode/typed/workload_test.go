// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"testing"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

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
			name: "Single REST endpoint with required fields only",
			endpoints: map[string]v1alpha1.WorkloadEndpoint{
				"http": {
					Type: v1alpha1.EndpointTypeREST,
					Port: 8080,
				},
			},
			wantKeys: []string{"http"},
			validate: func(t *testing.T, result map[string]interface{}) {
				ep := result["http"].(map[string]interface{})
				if ep["type"] != "REST" {
					t.Errorf("type = %v, want REST", ep["type"])
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
					Type: v1alpha1.EndpointTypeREST,
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

func TestGetConnections(t *testing.T) {
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
					Endpoint:   "tcp",
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
				if conn["endpoint"] != "tcp" {
					t.Errorf("endpoint = %v, want tcp", conn["endpoint"])
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
					Namespace:  "other-ns",
					Project:    "other-project",
					Component:  "redis",
					Endpoint:   "tcp",
					Visibility: v1alpha1.EndpointVisibilityInternal,
					EnvironmentMapping: v1alpha1.EnvironmentMapping{
						"dev":     "staging",
						"staging": "prod",
					},
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
				if conn["namespace"] != "other-ns" {
					t.Errorf("namespace = %v, want other-ns", conn["namespace"])
				}
				if conn["project"] != "other-project" {
					t.Errorf("project = %v, want other-project", conn["project"])
				}
				if conn["visibility"] != "internal" {
					t.Errorf("visibility = %v, want internal", conn["visibility"])
				}

				envMapping := conn["environmentMapping"].(map[string]interface{})
				if len(envMapping) != 2 {
					t.Fatalf("environmentMapping length = %d, want 2", len(envMapping))
				}
				if envMapping["dev"] != "staging" {
					t.Errorf("environmentMapping[dev] = %v, want staging", envMapping["dev"])
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
					Endpoint:   "tcp",
					Visibility: v1alpha1.EndpointVisibilityProject,
					EnvBindings: v1alpha1.ConnectionEnvBindings{
						Address: "DB_URL",
					},
				},
				{
					Component:  "nats",
					Endpoint:   "tcp",
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
			wl := &Workload{
				Workload: &v1alpha1.Workload{
					Spec: v1alpha1.WorkloadSpec{
						WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
							Container:   v1alpha1.Container{Image: "test:latest"},
							Connections: tt.connections,
						},
					},
				},
			}
			result := wl.GetConnections()
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
