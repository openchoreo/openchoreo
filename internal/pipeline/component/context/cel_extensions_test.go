// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/openchoreo/openchoreo/internal/template"
)

func derivedInputs(configs ContainerConfigurations, workload WorkloadData, deps ConnectionsContextData) map[string]any {
	derived := BuildDerivedContext(configs, workload, deps, "app-dev", nil)
	derivedMap, err := structToMap(&derived)
	if err != nil {
		panic("structToMap failed for DerivedContext: " + err.Error())
	}
	return map[string]any{"derived": derivedMap}
}

func configInputs(configs ContainerConfigurations) map[string]any {
	return derivedInputs(configs, WorkloadData{}, ConnectionsContextData{})
}

func workloadInputs(workload WorkloadData) map[string]any {
	return derivedInputs(ContainerConfigurations{}, workload, ConnectionsContextData{})
}

func depInputs(deps ConnectionsContextData) map[string]any {
	return derivedInputs(ContainerConfigurations{}, WorkloadData{}, deps)
}

func TestConfigurationsToConfigFileListMacro(t *testing.T) {
	tests := []struct {
		name   string
		expr   string
		inputs map[string]any
		want   []any
	}{
		{
			name: "single config file",
			expr: `configurations.toConfigFileList()`,
			inputs: configInputs(ContainerConfigurations{
				Configs: ConfigurationItems{
					Files: []FileConfiguration{{Name: "config.yaml", MountPath: "/etc/config/config.yaml", Value: "key: value"}},
				},
			}),
			want: []any{
				map[string]any{"name": "config.yaml", "mountPath": "/etc/config/config.yaml", "value": "key: value", "resourceName": generateConfigResourceName("app-dev", "config.yaml")},
			},
		},
		{
			name: "multiple config files",
			expr: `configurations.toConfigFileList()`,
			inputs: configInputs(ContainerConfigurations{
				Configs: ConfigurationItems{
					Files: []FileConfiguration{
						{Name: "app.yaml", MountPath: "/etc/app.yaml", Value: "app config"},
						{Name: "logging.properties", MountPath: "/etc/logging.properties", Value: "log.level=INFO"},
					},
				},
			}),
			want: []any{
				map[string]any{"name": "app.yaml", "mountPath": "/etc/app.yaml", "value": "app config", "resourceName": generateConfigResourceName("app-dev", "app.yaml")},
				map[string]any{"name": "logging.properties", "mountPath": "/etc/logging.properties", "value": "log.level=INFO", "resourceName": generateConfigResourceName("app-dev", "logging.properties")},
			},
		},
		{
			name:   "no config files returns empty list",
			expr:   `configurations.toConfigFileList()`,
			inputs: configInputs(ContainerConfigurations{Configs: ConfigurationItems{Files: []FileConfiguration{}}}),
			want:   []any{},
		},
		{
			name:   "empty configurations returns empty list",
			expr:   `configurations.toConfigFileList()`,
			inputs: configInputs(ContainerConfigurations{}),
			want:   []any{},
		},
		{
			name: "config file with remoteRef",
			expr: `configurations.toConfigFileList()`,
			inputs: configInputs(ContainerConfigurations{
				Configs: ConfigurationItems{
					Files: []FileConfiguration{{Name: "config.yaml", MountPath: "/etc/config.yaml", RemoteRef: &RemoteRefData{Key: "my-config-key", Property: "config.yaml"}}},
				},
			}),
			want: []any{
				map[string]any{
					"name": "config.yaml", "mountPath": "/etc/config.yaml", "value": "",
					"resourceName": generateConfigResourceName("app-dev", "config.yaml"),
					"remoteRef":    map[string]any{"key": "my-config-key", "property": "config.yaml"},
				},
			},
		},
		{
			name: "ignores secret files",
			expr: `configurations.toConfigFileList()`,
			inputs: configInputs(ContainerConfigurations{
				Configs: ConfigurationItems{Files: []FileConfiguration{{Name: "config.yaml", MountPath: "/etc/config.yaml", Value: "config"}}},
				Secrets: ConfigurationItems{Files: []FileConfiguration{{Name: "secret.yaml", MountPath: "/etc/secret.yaml", RemoteRef: &RemoteRefData{Key: "s"}}}},
			}),
			want: []any{
				map[string]any{"name": "config.yaml", "mountPath": "/etc/config.yaml", "value": "config", "resourceName": generateConfigResourceName("app-dev", "config.yaml")},
			},
		},
	}

	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Render("${"+tt.expr+"}", tt.inputs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, result, cmpopts.SortSlices(func(a, b any) bool {
				return a.(map[string]any)["name"].(string) < b.(map[string]any)["name"].(string)
			})); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestToConfigFileListMacroOnlyExpandsForConfigurations(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))

	_, err := engine.Render(`${configurations.toConfigFileList()}`, configInputs(ContainerConfigurations{}))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	inputs := configInputs(ContainerConfigurations{})
	inputs["other"] = map[string]any{}
	_, err = engine.Render(`${other.toConfigFileList()}`, inputs)
	if err == nil {
		t.Error("expected error for non-configurations receiver")
	}
}

func TestToConfigFileListCanBeUsedWithCELOperations(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	inputs := configInputs(ContainerConfigurations{
		Configs: ConfigurationItems{
			Files: []FileConfiguration{
				{Name: "a.yaml", MountPath: "/a.yaml", Value: "a"},
				{Name: "b.yaml", MountPath: "/b.yaml", Value: "b"},
			},
		},
	})

	t.Run("size", func(t *testing.T) {
		result, err := engine.Render(`${size(configurations.toConfigFileList())}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if diff := cmp.Diff(int64(2), result); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("map", func(t *testing.T) {
		result, err := engine.Render(`${configurations.toConfigFileList().map(f, f.name)}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if diff := cmp.Diff([]any{"a.yaml", "b.yaml"}, result, cmpopts.SortSlices(func(a, b any) bool { return a.(string) < b.(string) })); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestConfigurationsToSecretFileListMacro(t *testing.T) {
	tests := []struct {
		name   string
		inputs map[string]any
		want   []any
	}{
		{
			name: "single secret file",
			inputs: configInputs(ContainerConfigurations{
				Secrets: ConfigurationItems{
					Files: []FileConfiguration{{Name: "secret.yaml", MountPath: "/etc/secret.yaml", RemoteRef: &RemoteRefData{Key: "my-secret", Property: "password"}}},
				},
			}),
			want: []any{
				map[string]any{
					"name": "secret.yaml", "mountPath": "/etc/secret.yaml",
					"resourceName": generateSecretResourceName("app-dev", "secret.yaml", &RemoteRefData{Key: "my-secret", Property: "password"}),
					"remoteRef":    map[string]any{"key": "my-secret", "property": "password"},
				},
			},
		},
		{
			name:   "empty returns empty list",
			inputs: configInputs(ContainerConfigurations{}),
			want:   []any{},
		},
		{
			name: "ignores config files",
			inputs: configInputs(ContainerConfigurations{
				Configs: ConfigurationItems{Files: []FileConfiguration{{Name: "config.yaml", MountPath: "/c", Value: "c"}}},
				Secrets: ConfigurationItems{Files: []FileConfiguration{{Name: "secret.yaml", MountPath: "/s", RemoteRef: &RemoteRefData{Key: "k"}}}},
			}),
			want: []any{
				map[string]any{"name": "secret.yaml", "mountPath": "/s", "resourceName": generateSecretResourceName("app-dev", "secret.yaml", &RemoteRefData{Key: "k"}), "remoteRef": map[string]any{"key": "k"}},
			},
		},
	}

	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Render(`${configurations.toSecretFileList()}`, tt.inputs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, result, cmpopts.SortSlices(func(a, b any) bool {
				return a.(map[string]any)["name"].(string) < b.(map[string]any)["name"].(string)
			})); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestContainerConfigEnvFromMacro(t *testing.T) {
	tests := []struct {
		name   string
		inputs map[string]any
		want   []any
	}{
		{
			name: "both config and secret envs",
			inputs: configInputs(ContainerConfigurations{
				Configs: ConfigurationItems{Envs: []EnvConfiguration{{Name: "LOG_LEVEL", Value: "info"}}},
				Secrets: ConfigurationItems{Envs: []EnvConfiguration{{Name: "API_KEY", RemoteRef: &RemoteRefData{Key: "api-secret", Property: "key"}}}},
			}),
			want: []any{
				map[string]any{"configMapRef": map[string]any{"name": generateEnvResourceName("app-dev")}},
				map[string]any{"secretRef": map[string]any{"name": generateSecretEnvResourceName("app-dev", []EnvConfiguration{{Name: "API_KEY", RemoteRef: &RemoteRefData{Key: "api-secret", Property: "key"}}})}},
			},
		},
		{
			name: "only config envs",
			inputs: configInputs(ContainerConfigurations{
				Configs: ConfigurationItems{Envs: []EnvConfiguration{{Name: "DEBUG", Value: "true"}}},
			}),
			want: []any{
				map[string]any{"configMapRef": map[string]any{"name": generateEnvResourceName("app-dev")}},
			},
		},
		{
			name:   "no envs returns empty",
			inputs: configInputs(ContainerConfigurations{}),
			want:   []any{},
		},
	}

	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Render(`${configurations.toContainerEnvFrom()}`, tt.inputs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, result); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestEnvFromMacroValidation(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))

	_, err := engine.Render(`${configurations.toContainerEnvFrom()}`, configInputs(ContainerConfigurations{}))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	inputs := configInputs(ContainerConfigurations{})
	inputs["someVar"] = map[string]any{}
	_, err = engine.Render(`${someVar.toContainerEnvFrom()}`, inputs)
	if err == nil {
		t.Error("expected error for non-configurations target")
	}
}

func TestEnvFromCanBeUsedWithCELOperations(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	inputs := configInputs(ContainerConfigurations{
		Configs: ConfigurationItems{Envs: []EnvConfiguration{{Name: "C1", Value: "v1"}}},
		Secrets: ConfigurationItems{Envs: []EnvConfiguration{{Name: "S1", RemoteRef: &RemoteRefData{Key: "s", Property: "k"}}}},
	})

	t.Run("size", func(t *testing.T) {
		result, err := engine.Render(`${size(configurations.toContainerEnvFrom())}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if diff := cmp.Diff(int64(2), result); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("map extract names", func(t *testing.T) {
		result, err := engine.Render(`${configurations.toContainerEnvFrom().map(e, has(e.configMapRef) ? e.configMapRef.name : e.secretRef.name)}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{
			generateEnvResourceName("app-dev"),
			generateSecretEnvResourceName("app-dev", []EnvConfiguration{{Name: "S1", RemoteRef: &RemoteRefData{Key: "s", Property: "k"}}}),
		}
		if diff := cmp.Diff(want, result); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestContainerConfigVolumeMountsMacro(t *testing.T) {
	tests := []struct {
		name   string
		inputs map[string]any
		want   []any
	}{
		{
			name: "config and secret files",
			inputs: configInputs(ContainerConfigurations{
				Configs: ConfigurationItems{Files: []FileConfiguration{
					{Name: "app.properties", MountPath: "/etc/config"},
					{Name: "config.json", MountPath: "/etc/config"},
				}},
				Secrets: ConfigurationItems{Files: []FileConfiguration{
					{Name: "tls.crt", MountPath: "/etc/tls", RemoteRef: &RemoteRefData{Key: "tls"}},
				}},
			}),
			want: []any{
				map[string]any{"name": "file-mount-" + generateVolumeHash("/etc/config", "app.properties"), "mountPath": "/etc/config/app.properties", "subPath": "app.properties"},
				map[string]any{"name": "file-mount-" + generateVolumeHash("/etc/config", "config.json"), "mountPath": "/etc/config/config.json", "subPath": "config.json"},
				map[string]any{"name": "file-mount-" + generateVolumeHash("/etc/tls", "tls.crt"), "mountPath": "/etc/tls/tls.crt", "subPath": "tls.crt"},
			},
		},
		{
			name:   "no files returns empty",
			inputs: configInputs(ContainerConfigurations{}),
			want:   []any{},
		},
	}

	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Render(`${configurations.toContainerVolumeMounts()}`, tt.inputs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, result, cmpopts.SortSlices(func(a, b any) bool {
				return a.(map[string]any)["name"].(string) < b.(map[string]any)["name"].(string)
			})); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConfigurationsToVolumesMacro(t *testing.T) {
	tests := []struct {
		name   string
		inputs map[string]any
		want   []any
	}{
		{
			name: "config and secret files",
			inputs: configInputs(ContainerConfigurations{
				Configs: ConfigurationItems{Files: []FileConfiguration{{Name: "app.properties", MountPath: "/etc/config"}}},
				Secrets: ConfigurationItems{Files: []FileConfiguration{{Name: "tls.crt", MountPath: "/etc/tls", RemoteRef: &RemoteRefData{Key: "tls"}}}},
			}),
			want: []any{
				map[string]any{"name": "file-mount-" + generateVolumeHash("/etc/config", "app.properties"), "configMap": map[string]any{"name": generateConfigResourceName("app-dev", "app.properties")}},
				map[string]any{"name": "file-mount-" + generateVolumeHash("/etc/tls", "tls.crt"), "secret": map[string]any{"secretName": generateSecretResourceName("app-dev", "tls.crt", &RemoteRefData{Key: "tls"})}},
			},
		},
		{
			name:   "no files returns empty",
			inputs: configInputs(ContainerConfigurations{}),
			want:   []any{},
		},
	}

	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Render(`${configurations.toVolumes()}`, tt.inputs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, result, cmpopts.SortSlices(func(a, b any) bool {
				return a.(map[string]any)["name"].(string) < b.(map[string]any)["name"].(string)
			})); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConfigurationsToVolumesMacro_DeterministicOrder(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	inputs := configInputs(ContainerConfigurations{
		Configs: ConfigurationItems{Files: []FileConfiguration{
			{Name: "b.yaml", MountPath: "/m"},
			{Name: "a.yaml", MountPath: "/m"},
		}},
	})

	var prev []any
	for i := 0; i < 10; i++ {
		result, err := engine.Render(`${configurations.toVolumes()}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := result.([]any)
		if prev != nil {
			if diff := cmp.Diff(prev, got); diff != "" {
				t.Fatalf("non-deterministic order on iteration %d:\n%s", i, diff)
			}
		}
		prev = got
	}
}

func TestConfigurationsToConfigEnvsByContainerMacro(t *testing.T) {
	tests := []struct {
		name   string
		inputs map[string]any
		want   []any
	}{
		{
			name: "with config envs",
			inputs: configInputs(ContainerConfigurations{
				Configs: ConfigurationItems{Envs: []EnvConfiguration{
					{Name: "LOG_LEVEL", Value: "info"},
					{Name: "DEBUG_MODE", Value: "true"},
				}},
			}),
			want: []any{
				map[string]any{
					"resourceName": generateEnvResourceName("app-dev"),
					"envs": []any{
						map[string]any{"name": "LOG_LEVEL", "value": "info"},
						map[string]any{"name": "DEBUG_MODE", "value": "true"},
					},
				},
			},
		},
		{
			name:   "no config envs returns empty",
			inputs: configInputs(ContainerConfigurations{}),
			want:   []any{},
		},
		{
			name: "only secrets returns empty",
			inputs: configInputs(ContainerConfigurations{
				Secrets: ConfigurationItems{Envs: []EnvConfiguration{{Name: "S", Value: "v"}}},
			}),
			want: []any{},
		},
	}

	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Render(`${configurations.toConfigEnvsByContainer()}`, tt.inputs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, result); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConfigurationsToSecretEnvsByContainerMacro(t *testing.T) {
	tests := []struct {
		name   string
		inputs map[string]any
		want   []any
	}{
		{
			name: "with secret envs",
			inputs: configInputs(ContainerConfigurations{
				Secrets: ConfigurationItems{Envs: []EnvConfiguration{
					{Name: "DB_PASSWORD", RemoteRef: &RemoteRefData{Key: "db-password", Property: "password"}},
				}},
			}),
			want: []any{
				map[string]any{
					"resourceName": generateSecretEnvResourceName("app-dev", []EnvConfiguration{{Name: "DB_PASSWORD", RemoteRef: &RemoteRefData{Key: "db-password", Property: "password"}}}),
					"envs": []any{
						map[string]any{"name": "DB_PASSWORD", "value": "", "remoteRef": map[string]any{"key": "db-password", "property": "password"}},
					},
				},
			},
		},
		{
			name:   "no secret envs returns empty",
			inputs: configInputs(ContainerConfigurations{}),
			want:   []any{},
		},
	}

	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Render(`${configurations.toSecretEnvsByContainer()}`, tt.inputs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, result); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDependenciesEnvVarsMacro(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	inputs := depInputs(ConnectionsContextData{
		EnvVars: []EnvVarEntry{
			{Name: "SVC_A_URL", Value: "http://svc-a:8080"},
			{Name: "SVC_B_URL", Value: "grpc://svc-b:9090"},
		},
	})

	result, err := engine.Render(`${dependencies.toContainerEnvs()}`, inputs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []any{
		map[string]any{"name": "SVC_A_URL", "value": "http://svc-a:8080"},
		map[string]any{"name": "SVC_B_URL", "value": "grpc://svc-b:9090"},
	}
	if diff := cmp.Diff(want, result); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestDependenciesVolumeMacros(t *testing.T) {
	t.Run("toContainerVolumeMounts", func(t *testing.T) {
		engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
		inputs := depInputs(ConnectionsContextData{
			VolumeMounts: []VolumeMountEntry{
				{Name: "r-abc", MountPath: "/etc/db", SubPath: "password"},
				{Name: "r-def", MountPath: "/etc/tls", SubPath: "ca.crt"},
			},
		})

		result, err := engine.Render(`${dependencies.toContainerVolumeMounts()}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{
			map[string]any{"name": "r-abc", "mountPath": "/etc/db", "subPath": "password"},
			map[string]any{"name": "r-def", "mountPath": "/etc/tls", "subPath": "ca.crt"},
		}
		if diff := cmp.Diff(want, result); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("toVolumes", func(t *testing.T) {
		engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
		inputs := depInputs(ConnectionsContextData{
			Volumes: []VolumeEntry{
				{Name: "r-abc", Secret: &SecretVolume{SecretName: "db-conn"}},
				{Name: "r-def", ConfigMap: &ConfigMapVolume{Name: "db-tls"}},
			},
		})

		result, err := engine.Render(`${dependencies.toVolumes()}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{
			map[string]any{"name": "r-abc", "secret": map[string]any{"secretName": "db-conn"}},
			map[string]any{"name": "r-def", "configMap": map[string]any{"name": "db-tls"}},
		}
		if diff := cmp.Diff(want, result); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestWorkloadEndpointsToServicePortsMacro(t *testing.T) {
	tests := []struct {
		name   string
		inputs map[string]any
		want   []any
	}{
		{
			name: "single HTTP endpoint",
			inputs: workloadInputs(WorkloadData{
				Endpoints: map[string]EndpointData{
					"http": {Port: 8080, TargetPort: 8080, Type: "HTTP"},
				},
			}),
			want: []any{
				map[string]any{"name": "http", "port": float64(8080), "targetPort": float64(8080), "protocol": "TCP"},
			},
		},
		{
			name: "multiple endpoints sorted by name",
			inputs: workloadInputs(WorkloadData{
				Endpoints: map[string]EndpointData{
					"grpc":  {Port: 9090, TargetPort: 9090, Type: "gRPC"},
					"admin": {Port: 9091, TargetPort: 9091, Type: "HTTP"},
				},
			}),
			want: []any{
				map[string]any{"name": "admin", "port": float64(9091), "targetPort": float64(9091), "protocol": "TCP"},
				map[string]any{"name": "grpc", "port": float64(9090), "targetPort": float64(9090), "protocol": "TCP"},
			},
		},
		{
			name: "UDP endpoint",
			inputs: workloadInputs(WorkloadData{
				Endpoints: map[string]EndpointData{
					"dns": {Port: 53, TargetPort: 53, Type: "UDP"},
				},
			}),
			want: []any{
				map[string]any{"name": "dns", "port": float64(53), "targetPort": float64(53), "protocol": "UDP"},
			},
		},
		{
			name: "duplicate port+protocol deduplicated",
			inputs: workloadInputs(WorkloadData{
				Endpoints: map[string]EndpointData{
					"api": {Port: 8080, TargetPort: 8080, Type: "HTTP"},
					"web": {Port: 8080, TargetPort: 8080, Type: "HTTP"},
				},
			}),
			want: []any{
				map[string]any{"name": "api", "port": float64(8080), "targetPort": float64(8080), "protocol": "TCP"},
			},
		},
		{
			name:   "no endpoints returns empty",
			inputs: workloadInputs(WorkloadData{}),
			want:   []any{},
		},
		{
			name: "targetPort defaults to port when zero",
			inputs: workloadInputs(WorkloadData{
				Endpoints: map[string]EndpointData{
					"http": {Port: 8080, TargetPort: 0, Type: "HTTP"},
				},
			}),
			want: []any{
				map[string]any{"name": "http", "port": float64(8080), "targetPort": float64(8080), "protocol": "TCP"},
			},
		},
		{
			name: "port name sanitized",
			inputs: workloadInputs(WorkloadData{
				Endpoints: map[string]EndpointData{
					"My_HTTP_Endpoint": {Port: 8080, TargetPort: 8080, Type: "HTTP"},
				},
			}),
			want: []any{
				map[string]any{"name": "my-http-endpoin", "port": float64(8080), "targetPort": float64(8080), "protocol": "TCP"},
			},
		},
	}

	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Render(`${workload.toServicePorts()}`, tt.inputs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, result); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestToServicePortsMacroOnlyExpandsForWorkloadEndpoints(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))

	_, err := engine.Render(`${workload.toServicePorts()}`, workloadInputs(WorkloadData{}))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	inputs := workloadInputs(WorkloadData{})
	inputs["other"] = map[string]any{}
	_, err = engine.Render(`${other.toServicePorts()}`, inputs)
	if err == nil {
		t.Error("expected error for non-workload receiver")
	}
}

func TestToServicePortsCanBeUsedWithCELOperations(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	inputs := workloadInputs(WorkloadData{
		Endpoints: map[string]EndpointData{
			"http": {Port: 8080, TargetPort: 8080, Type: "HTTP"},
			"grpc": {Port: 9090, TargetPort: 9090, Type: "gRPC"},
		},
	})

	t.Run("size", func(t *testing.T) {
		result, err := engine.Render(`${size(workload.toServicePorts())}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if diff := cmp.Diff(int64(2), result); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("map port names", func(t *testing.T) {
		result, err := engine.Render(`${workload.toServicePorts().map(p, p.name)}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{"grpc", "http"}
		if diff := cmp.Diff(want, result, cmpopts.SortSlices(func(a, b any) bool { return a.(string) < b.(string) })); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestNewDependenciesContextData(t *testing.T) {
	t.Run("merges endpoint env vars", func(t *testing.T) {
		data := ConnectionsData{
			Items: []ConnectionItem{
				{Namespace: "ns1", Project: "proj1", Component: "svc-a", Endpoint: "http", Visibility: "project", EnvVars: []EnvVarEntry{{Name: "SVC_A_URL", Value: "http://svc-a:8080"}}},
				{Namespace: "ns1", Project: "proj1", Component: "svc-b", Endpoint: "grpc", Visibility: "namespace", EnvVars: []EnvVarEntry{{Name: "SVC_B_URL", Value: "grpc://svc-b:9090"}, {Name: "SVC_B_HOST", Value: "svc-b"}}},
			},
		}
		ctx := newDependenciesContextData(data)
		wantEnvVars := []EnvVarEntry{{Name: "SVC_A_URL", Value: "http://svc-a:8080"}, {Name: "SVC_B_URL", Value: "grpc://svc-b:9090"}, {Name: "SVC_B_HOST", Value: "svc-b"}}
		if diff := cmp.Diff(wantEnvVars, ctx.EnvVars); diff != "" {
			t.Errorf("merged envVars mismatch:\n%s", diff)
		}
	})

	t.Run("empty returns empty slices", func(t *testing.T) {
		ctx := newDependenciesContextData(ConnectionsData{})
		if len(ctx.EnvVars) != 0 || ctx.EnvVars == nil {
			t.Errorf("expected empty non-nil envVars, got %v", ctx.EnvVars)
		}
	})

	t.Run("nil item envVars normalized", func(t *testing.T) {
		data := ConnectionsData{
			Items: []ConnectionItem{
				{Namespace: "ns1", Project: "proj1", Component: "svc-a", Endpoint: "http", Visibility: "project", EnvVars: nil},
			},
		}
		ctx := newDependenciesContextData(data)
		if ctx.Items[0].EnvVars == nil {
			t.Error("expected items[0].EnvVars to be empty slice, got nil")
		}
	})
}

func TestSanitizePortName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"http", "http"},
		{"HTTP", "http"},
		{"my_endpoint", "my-endpoint"},
		{"my-endpoint-with-a-very-long-name", "my-endpoint-wit"},
		{"---invalid---", "invalid"},
		{"", ""},
		{"special!@#chars", "specialchars"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := sanitizePortName(tt.input); got != tt.want {
				t.Errorf("sanitizePortName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapEndpointTypeToProtocol(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"TCP", "TCP"},
		{"UDP", "UDP"},
		{"HTTP", "TCP"},
		{"gRPC", "TCP"},
		{"", "TCP"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := mapEndpointTypeToProtocol(tt.input); got != tt.want {
				t.Errorf("mapEndpointTypeToProtocol(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDependenciesMacroReceiverGuards(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	inputs := depInputs(ConnectionsContextData{
		EnvVars:      []EnvVarEntry{},
		VolumeMounts: []VolumeMountEntry{},
		Volumes:      []VolumeEntry{},
	})

	// toContainerEnvs only expands for dependencies
	inputs["other"] = map[string]any{}
	_, err := engine.Render(`${other.toContainerEnvs()}`, inputs)
	if err == nil {
		t.Error("expected error for non-dependencies receiver on toContainerEnvs")
	}

	// toContainerVolumeMounts also works on configurations
	_, err = engine.Render(`${dependencies.toContainerVolumeMounts()}`, inputs)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// toVolumes also works on configurations
	_, err = engine.Render(`${dependencies.toVolumes()}`, inputs)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConfigurationsVolumeMountsConcatWithDependencies(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	inputs := derivedInputs(
		ContainerConfigurations{
			Configs: ConfigurationItems{Files: []FileConfiguration{{Name: "app.yaml", MountPath: "/etc/config"}}},
		},
		WorkloadData{},
		ConnectionsContextData{
			VolumeMounts: []VolumeMountEntry{{Name: "dep-vol", MountPath: "/dep", SubPath: "key"}},
		},
	)

	result, err := engine.Render(`${configurations.toContainerVolumeMounts() + dependencies.toContainerVolumeMounts()}`, inputs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := result.([]any)
	if len(got) != 2 {
		t.Errorf("expected 2 volume mounts, got %d", len(got))
	}
}

func TestBuildDerivedContext_EmptyInputs(t *testing.T) {
	derived := BuildDerivedContext(ContainerConfigurations{}, WorkloadData{}, ConnectionsContextData{}, "app-dev", nil)

	if derived.ConfigFileList == nil {
		t.Error("ConfigFileList should be non-nil")
	}
	if derived.SecretFileList == nil {
		t.Error("SecretFileList should be non-nil")
	}
	if derived.ServicePorts == nil {
		t.Error("ServicePorts should be non-nil")
	}
	if derived.DependencyEnvVars == nil {
		t.Error("DependencyEnvVars should be non-nil")
	}
	if derived.DependencyVolumeMounts == nil {
		t.Error("DependencyVolumeMounts should be non-nil")
	}
	if derived.DependencyVolumes == nil {
		t.Error("DependencyVolumes should be non-nil")
	}
}

func TestMacroDoesNotExpandOnWrongReceiver(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	inputs := configInputs(ContainerConfigurations{})
	inputs["other"] = map[string]any{}

	macros := []string{
		`other.toConfigFileList()`,
		`other.toSecretFileList()`,
		`other.toContainerEnvFrom()`,
		`other.toConfigEnvsByContainer()`,
		`other.toSecretEnvsByContainer()`,
	}

	for _, macro := range macros {
		t.Run(macro, func(t *testing.T) {
			_, err := engine.Render("${"+macro+"}", inputs)
			if err == nil {
				t.Fatalf("expected error for %s on wrong receiver", macro)
			}
			if !strings.Contains(err.Error(), "undeclared reference") && !strings.Contains(err.Error(), "unrecognized") {
				t.Logf("error (expected macro guard): %v", err)
			}
		})
	}
}

func TestWorkloadToEndpointResourcesMacro(t *testing.T) {
	derived := BuildDerivedContext(ContainerConfigurations{}, WorkloadData{}, ConnectionsContextData{}, "app-dev",
		EndpointResourceMap{
			"grpc": {{Kind: "gRPC", Service: "greeter.Greeter", Method: "SayHello"}},
		})
	derivedMap, err := structToMap(&derived)
	if err != nil {
		t.Fatalf("structToMap failed: %v", err)
	}
	inputs := map[string]any{"derived": derivedMap}

	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))

	// The macro rewrites workload.toEndpointResources(<name>) to an optional index
	// into derived.endpointResources; mapping yields the extracted route fields.
	got, err := engine.Render(`${workload.toEndpointResources("grpc").orValue([]).map(r, r.service + "/" + r.method)}`, inputs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff := cmp.Diff([]any{"greeter.Greeter/SayHello"}, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	// A missing endpoint name yields optional.none -> orValue fallback.
	missing, err := engine.Render(`${workload.toEndpointResources("missing").orValue([]).size()}`, inputs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if missing != int64(0) {
		t.Errorf("expected 0 for missing key, got %v", missing)
	}

	// hasValue() distinguishes present vs absent endpoints.
	present, err := engine.Render(`${workload.toEndpointResources("grpc").hasValue() && !workload.toEndpointResources("missing").hasValue()}`, inputs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if present != true {
		t.Errorf("expected true, got %v", present)
	}
}

// TestWorkloadToEndpointResourcesMacroEmptyMap pins the regression where a template
// uses the macro but no endpoint produced resources (e.g. all schemas missing or
// malformed): the backing map must still be present in the marshaled context so the
// macro degrades to optional.none instead of failing the render with
// "no such key: endpointResources".
func TestWorkloadToEndpointResourcesMacroEmptyMap(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))

	for name, endpointResources := range map[string]EndpointResourceMap{
		"empty map": {},
		"nil map":   nil, // normalized to an empty map by BuildDerivedContext
	} {
		derived := BuildDerivedContext(ContainerConfigurations{}, WorkloadData{}, ConnectionsContextData{}, "app-dev", endpointResources)
		derivedMap, err := structToMap(&derived)
		if err != nil {
			t.Fatalf("%s: structToMap failed: %v", name, err)
		}
		if _, ok := derivedMap["endpointResources"]; !ok {
			t.Fatalf("%s: endpointResources must be present in the marshaled derived context", name)
		}
		inputs := map[string]any{"derived": derivedMap}

		got, err := engine.Render(`${workload.toEndpointResources("grpc").orValue([]).size()}`, inputs)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}
		if got != int64(0) {
			t.Errorf("%s: expected 0, got %v", name, got)
		}

		has, err := engine.Render(`${workload.toEndpointResources("grpc").hasValue()}`, inputs)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}
		if has != false {
			t.Errorf("%s: expected false, got %v", name, has)
		}
	}
}
