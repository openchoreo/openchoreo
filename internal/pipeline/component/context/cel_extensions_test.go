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

func TestConfigurationsToConfigFileListMacro(t *testing.T) {
	tests := []struct {
		name   string
		expr   string
		inputs map[string]any
		want   []map[string]any
	}{
		{
			name: "single container with single config file",
			expr: `configurations.toConfigFileList()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{
						"files": []any{
							map[string]any{
								"name":      "config.yaml",
								"mountPath": "/etc/config/config.yaml",
								"value":     "key: value",
							},
						},
					},
					"secrets": map[string]any{"files": []any{}},
				},
			},
			want: []map[string]any{
				{
					"name":         "config.yaml",
					"mountPath":    "/etc/config/config.yaml",
					"value":        "key: value",
					"resourceName": generateConfigResourceName("app-dev", "config.yaml"),
				},
			},
		},
		{
			name: "single container with multiple config files",
			expr: `configurations.toConfigFileList()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{
						"files": []any{
							map[string]any{"name": "app.yaml", "mountPath": "/etc/app.yaml", "value": "app config"},
							map[string]any{"name": "logging.properties", "mountPath": "/etc/logging.properties", "value": "log.level=INFO"},
						},
					},
					"secrets": map[string]any{"files": []any{}},
				},
			},
			want: []map[string]any{
				{"name": "app.yaml", "mountPath": "/etc/app.yaml", "value": "app config", "resourceName": generateConfigResourceName("app-dev", "app.yaml")},
				{"name": "logging.properties", "mountPath": "/etc/logging.properties", "value": "log.level=INFO", "resourceName": generateConfigResourceName("app-dev", "logging.properties")},
			},
		},
		{
			name: "no config files returns empty list",
			expr: `configurations.toConfigFileList()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{"files": []any{}},
					"secrets": map[string]any{"files": []any{}},
				},
			},
			want: []map[string]any{},
		},
		{
			name: "empty configurations returns empty list",
			expr: `configurations.toConfigFileList()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{},
			},
			want: []map[string]any{},
		},
		{
			name: "config file with remoteRef",
			expr: `configurations.toConfigFileList()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{
						"files": []any{
							map[string]any{
								"name":      "config.yaml",
								"mountPath": "/etc/config.yaml",
								"remoteRef": map[string]any{
									"key":      "my-config-key",
									"property": "config.yaml",
								},
							},
						},
					},
					"secrets": map[string]any{"files": []any{}},
				},
			},
			want: []map[string]any{
				{
					"name":         "config.yaml",
					"mountPath":    "/etc/config.yaml",
					"value":        "",
					"resourceName": generateConfigResourceName("app-dev", "config.yaml"),
					"remoteRef": map[string]any{
						"key":      "my-config-key",
						"property": "config.yaml",
					},
				},
			},
		},
		{
			name: "dots in filename are replaced with hyphens in resourceName",
			expr: `configurations.toConfigFileList()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{
						"files": []any{
							map[string]any{"name": "application.properties", "mountPath": "/etc/application.properties", "value": "prop=value"},
						},
					},
					"secrets": map[string]any{"files": []any{}},
				},
			},
			want: []map[string]any{
				{"name": "application.properties", "mountPath": "/etc/application.properties", "value": "prop=value", "resourceName": generateConfigResourceName("app-dev", "application.properties")},
			},
		},
		{
			name: "ignores secret files (only returns config files)",
			expr: `configurations.toConfigFileList()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{
						"files": []any{
							map[string]any{"name": "config.yaml", "mountPath": "/etc/config.yaml", "value": "config"},
						},
					},
					"secrets": map[string]any{
						"files": []any{
							map[string]any{"name": "secret.yaml", "mountPath": "/etc/secret.yaml", "value": "secret"},
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "config.yaml", "mountPath": "/etc/config.yaml", "value": "config", "resourceName": generateConfigResourceName("app-dev", "config.yaml")},
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

			got, ok := result.([]map[string]any)
			if !ok {
				t.Fatalf("expected []map[string]any, got %T", result)
			}

			if diff := cmp.Diff(tt.want, got, cmpopts.SortSlices(func(a, b map[string]any) bool {
				return a["name"].(string) < b["name"].(string)
			})); diff != "" {
				t.Errorf("toConfigFileList() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestToConfigFileListMacroOnlyExpandsForConfigurations(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))

	// This should work - configurations is the expected receiver
	_, err := engine.Render(`${configurations.toConfigFileList()}`, map[string]any{
		"metadata": map[string]any{
			"componentName":   "app",
			"environmentName": "dev",
		},
		"configurations": map[string]any{},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// This should fail - "other" is not a valid receiver for the macro
	_, err = engine.Render(`${other.toConfigFileList()}`, map[string]any{
		"metadata": map[string]any{
			"componentName":   "app",
			"environmentName": "dev",
		},
		"other": map[string]any{},
	})
	if err == nil {
		t.Error("expected error for non-configurations receiver")
	}

	// Field access should not be affected by the macro
	result, err := engine.Render(`${parameters.configFiles.map(f, f.name)}`, map[string]any{
		"parameters": map[string]any{
			"configFiles": []any{
				map[string]any{"name": "a.yaml"},
				map[string]any{"name": "b.yaml"},
			},
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	want := []any{"a.yaml", "b.yaml"}
	if diff := cmp.Diff(want, result, cmpopts.SortSlices(func(a, b any) bool {
		return a.(string) < b.(string)
	})); diff != "" {
		t.Errorf("field access mismatch (-want +got):\n%s", diff)
	}
}

func TestToConfigFileListCanBeUsedWithCELOperations(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))

	inputs := map[string]any{
		"metadata": map[string]any{
			"componentName":   "app",
			"environmentName": "dev",
		},
		"configurations": map[string]any{
			"configs": map[string]any{
				"files": []any{
					map[string]any{"name": "a.yaml", "mountPath": "/a.yaml", "value": "a"},
					map[string]any{"name": "b.yaml", "mountPath": "/b.yaml", "value": "b"},
				},
			},
			"secrets": map[string]any{"files": []any{}},
		},
	}

	t.Run("size() operation", func(t *testing.T) {
		result, err := engine.Render(`${size(configurations.toConfigFileList())}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if diff := cmp.Diff(int64(2), result); diff != "" {
			t.Errorf("size() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("map() operation", func(t *testing.T) {
		result, err := engine.Render(`${configurations.toConfigFileList().map(f, f.name)}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{"a.yaml", "b.yaml"}
		if diff := cmp.Diff(want, result, cmpopts.SortSlices(func(a, b any) bool {
			return a.(string) < b.(string)
		})); diff != "" {
			t.Errorf("map() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("list concatenation with inline items", func(t *testing.T) {
		result, err := engine.Render(`${configurations.toConfigFileList() + [{"name": "inline.yaml", "mountPath": "/inline.yaml"}]}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{
			map[string]any{"name": "a.yaml", "mountPath": "/a.yaml", "value": "a", "resourceName": generateConfigResourceName("app-dev", "a.yaml")},
			map[string]any{"name": "b.yaml", "mountPath": "/b.yaml", "value": "b", "resourceName": generateConfigResourceName("app-dev", "b.yaml")},
			map[string]any{"name": "inline.yaml", "mountPath": "/inline.yaml"},
		}
		if diff := cmp.Diff(want, result, cmpopts.SortSlices(func(a, b any) bool {
			aMap, aOk := a.(map[string]any)
			bMap, bOk := b.(map[string]any)
			if aOk && bOk {
				return aMap["name"].(string) < bMap["name"].(string)
			}
			return false
		})); diff != "" {
			t.Errorf("concatenation mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestConfigurationsToSecretFileListMacro(t *testing.T) {
	tests := []struct {
		name   string
		expr   string
		inputs map[string]any
		want   []map[string]any
	}{
		{
			name: "single container with single secret file",
			expr: `configurations.toSecretFileList()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{"files": []any{}},
					"secrets": map[string]any{
						"files": []any{
							map[string]any{
								"name":      "secret.yaml",
								"mountPath": "/etc/secret/secret.yaml",
								"remoteRef": map[string]any{
									"key":      "my-secret",
									"property": "password",
								},
							},
						},
					},
				},
			},
			want: []map[string]any{
				{
					"name":         "secret.yaml",
					"mountPath":    "/etc/secret/secret.yaml",
					"resourceName": generateSecretResourceName("app-dev", "secret.yaml"),
					"remoteRef": map[string]any{
						"key":      "my-secret",
						"property": "password",
					},
				},
			},
		},
		{
			name: "single container with multiple secret files",
			expr: `configurations.toSecretFileList()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{"files": []any{}},
					"secrets": map[string]any{
						"files": []any{
							map[string]any{
								"name":      "db.env",
								"mountPath": "/etc/db.env",
								"remoteRef": map[string]any{"key": "db-credentials"},
							},
							map[string]any{
								"name":      "api.key",
								"mountPath": "/etc/api.key",
								"remoteRef": map[string]any{"key": "api-credentials"},
							},
						},
					},
				},
			},
			want: []map[string]any{
				{
					"name":         "db.env",
					"mountPath":    "/etc/db.env",
					"resourceName": generateSecretResourceName("app-dev", "db.env"),
					"remoteRef":    map[string]any{"key": "db-credentials"},
				},
				{
					"name":         "api.key",
					"mountPath":    "/etc/api.key",
					"resourceName": generateSecretResourceName("app-dev", "api.key"),
					"remoteRef":    map[string]any{"key": "api-credentials"},
				},
			},
		},
		{
			name: "no secret files returns empty list",
			expr: `configurations.toSecretFileList()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{"files": []any{}},
					"secrets": map[string]any{"files": []any{}},
				},
			},
			want: []map[string]any{},
		},
		{
			name: "empty configurations returns empty list",
			expr: `configurations.toSecretFileList()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{},
			},
			want: []map[string]any{},
		},
		{
			name: "secret file with remoteRef properties",
			expr: `configurations.toSecretFileList()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{"files": []any{}},
					"secrets": map[string]any{
						"files": []any{
							map[string]any{
								"name":      "secret.yaml",
								"mountPath": "/etc/secret.yaml",
								"remoteRef": map[string]any{
									"key":      "my-secret-key",
									"property": "secret.yaml",
								},
							},
						},
					},
				},
			},
			want: []map[string]any{
				{
					"name":         "secret.yaml",
					"mountPath":    "/etc/secret.yaml",
					"resourceName": generateSecretResourceName("app-dev", "secret.yaml"),
					"remoteRef": map[string]any{
						"key":      "my-secret-key",
						"property": "secret.yaml",
					},
				},
			},
		},
		{
			name: "ignores config files (only returns secret files)",
			expr: `configurations.toSecretFileList()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{
						"files": []any{
							map[string]any{"name": "config.yaml", "mountPath": "/etc/config.yaml", "value": "config"},
						},
					},
					"secrets": map[string]any{
						"files": []any{
							map[string]any{
								"name":      "secret.yaml",
								"mountPath": "/etc/secret.yaml",
								"remoteRef": map[string]any{"key": "app-secret"},
							},
						},
					},
				},
			},
			want: []map[string]any{
				{
					"name":         "secret.yaml",
					"mountPath":    "/etc/secret.yaml",
					"resourceName": generateSecretResourceName("app-dev", "secret.yaml"),
					"remoteRef":    map[string]any{"key": "app-secret"},
				},
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

			got, ok := result.([]map[string]any)
			if !ok {
				t.Fatalf("expected []map[string]any, got %T", result)
			}

			if diff := cmp.Diff(tt.want, got, cmpopts.SortSlices(func(a, b map[string]any) bool {
				return a["name"].(string) < b["name"].(string)
			})); diff != "" {
				t.Errorf("toSecretFileList() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestToSecretFileListMacroOnlyExpandsForConfigurations(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))

	// This should work - configurations is the expected receiver
	_, err := engine.Render(`${configurations.toSecretFileList()}`, map[string]any{
		"metadata": map[string]any{
			"componentName":   "app",
			"environmentName": "dev",
		},
		"configurations": map[string]any{},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// This should fail - "other" is not a valid receiver for the macro
	_, err = engine.Render(`${other.toSecretFileList()}`, map[string]any{
		"metadata": map[string]any{
			"componentName":   "app",
			"environmentName": "dev",
		},
		"other": map[string]any{},
	})
	if err == nil {
		t.Error("expected error for non-configurations receiver")
	}
}

func TestToSecretFileListCanBeUsedWithCELOperations(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))

	inputs := map[string]any{
		"metadata": map[string]any{
			"componentName":   "app",
			"environmentName": "dev",
		},
		"configurations": map[string]any{
			"configs": map[string]any{"files": []any{}},
			"secrets": map[string]any{
				"files": []any{
					map[string]any{
						"name":      "a.secret",
						"mountPath": "/a.secret",
						"remoteRef": map[string]any{"key": "a-secret"},
					},
					map[string]any{
						"name":      "b.secret",
						"mountPath": "/b.secret",
						"remoteRef": map[string]any{"key": "b-secret"},
					},
				},
			},
		},
	}

	t.Run("size() operation", func(t *testing.T) {
		result, err := engine.Render(`${size(configurations.toSecretFileList())}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if diff := cmp.Diff(int64(2), result); diff != "" {
			t.Errorf("size() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("map() operation", func(t *testing.T) {
		result, err := engine.Render(`${configurations.toSecretFileList().map(f, f.name)}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{"a.secret", "b.secret"}
		if diff := cmp.Diff(want, result, cmpopts.SortSlices(func(a, b any) bool {
			return a.(string) < b.(string)
		})); diff != "" {
			t.Errorf("map() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("list concatenation with inline items", func(t *testing.T) {
		result, err := engine.Render(`${configurations.toSecretFileList() + [{"name": "inline.secret", "mountPath": "/inline.secret"}]}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{
			map[string]any{"name": "a.secret", "mountPath": "/a.secret", "resourceName": generateSecretResourceName("app-dev", "a.secret"), "remoteRef": map[string]any{"key": "a-secret"}},
			map[string]any{"name": "b.secret", "mountPath": "/b.secret", "resourceName": generateSecretResourceName("app-dev", "b.secret"), "remoteRef": map[string]any{"key": "b-secret"}},
			map[string]any{"name": "inline.secret", "mountPath": "/inline.secret"},
		}
		if diff := cmp.Diff(want, result, cmpopts.SortSlices(func(a, b any) bool {
			aMap, aOk := a.(map[string]any)
			bMap, bOk := b.(map[string]any)
			if aOk && bOk {
				return aMap["name"].(string) < bMap["name"].(string)
			}
			return false
		})); diff != "" {
			t.Errorf("concatenation mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestContainerConfigEnvFromMacro(t *testing.T) {
	tests := []struct {
		name   string
		expr   string
		inputs map[string]any
		want   []map[string]any
	}{
		{
			name: "container with both config and secret envs",
			expr: `configurations.toContainerEnvFrom()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{
						"envs": []any{
							map[string]any{"name": "LOG_LEVEL", "value": "info"},
						},
					},
					"secrets": map[string]any{
						"envs": []any{
							map[string]any{"name": "API_KEY", "remoteRef": map[string]any{"key": "api-secret", "property": "key"}},
						},
					},
				},
			},
			want: []map[string]any{
				{"configMapRef": map[string]any{"name": generateEnvResourceName("app-dev", "env-configs")}},
				{"secretRef": map[string]any{"name": generateEnvResourceName("app-dev", "env-secrets")}},
			},
		},
		{
			name: "container with only config envs",
			expr: `configurations.toContainerEnvFrom()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{
						"envs": []any{
							map[string]any{"name": "DEBUG", "value": "true"},
							map[string]any{"name": "PORT", "value": "8080"},
						},
					},
					"secrets": map[string]any{"envs": []any{}},
				},
			},
			want: []map[string]any{
				{"configMapRef": map[string]any{"name": generateEnvResourceName("app-dev", "env-configs")}},
			},
		},
		{
			name: "container with only secret envs",
			expr: `configurations.toContainerEnvFrom()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{"envs": []any{}},
					"secrets": map[string]any{
						"envs": []any{
							map[string]any{"name": "DB_PASSWORD", "remoteRef": map[string]any{"key": "db-secret", "property": "password"}},
						},
					},
				},
			},
			want: []map[string]any{
				{"secretRef": map[string]any{"name": generateEnvResourceName("app-dev", "env-secrets")}},
			},
		},
		{
			name: "container with no envs returns empty list",
			expr: `configurations.toContainerEnvFrom()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{"envs": []any{}},
					"secrets": map[string]any{"envs": []any{}},
				},
			},
			want: []map[string]any{},
		},
		{
			name: "empty container config returns empty list",
			expr: `configurations.toContainerEnvFrom()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{},
			},
			want: []map[string]any{},
		},
		{
			name: "container missing configs section",
			expr: `configurations.toContainerEnvFrom()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"secrets": map[string]any{
						"envs": []any{
							map[string]any{"name": "SECRET_KEY", "remoteRef": map[string]any{"key": "app-secret", "property": "key"}},
						},
					},
				},
			},
			want: []map[string]any{
				{"secretRef": map[string]any{"name": generateEnvResourceName("app-dev", "env-secrets")}},
			},
		},
		{
			name: "container missing secrets section",
			expr: `configurations.toContainerEnvFrom()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{
						"envs": []any{
							map[string]any{"name": "CONFIG_VAR", "value": "value"},
						},
					},
				},
			},
			want: []map[string]any{
				{"configMapRef": map[string]any{"name": generateEnvResourceName("app-dev", "env-configs")}},
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

			got, ok := result.([]map[string]any)
			if !ok {
				t.Fatalf("expected []map[string]any, got %T", result)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("envFrom() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestEnvFromMacroValidation(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))

	// This should work - accessing container config from configurations
	_, err := engine.Render(`${configurations.toContainerEnvFrom()}`, map[string]any{
		"metadata": map[string]any{
			"componentName":   "app",
			"environmentName": "dev",
		},
		"configurations": map[string]any{
			"configs": map[string]any{"envs": []any{}},
			"secrets": map[string]any{"envs": []any{}},
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// toContainerEnvFrom only works on configurations, not arbitrary variables
	_, err = engine.Render(`${someVar.toContainerEnvFrom()}`, map[string]any{
		"metadata": map[string]any{
			"componentName":   "app",
			"environmentName": "dev",
		},
		"someVar": map[string]any{
			"configs": map[string]any{"envs": []any{}},
			"secrets": map[string]any{"envs": []any{}},
		},
	})
	if err == nil {
		t.Error("expected error for non-configurations target, got nil")
	}
}

func TestEnvFromCanBeUsedWithCELOperations(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))

	inputs := map[string]any{
		"metadata": map[string]any{
			"componentName":   "app",
			"environmentName": "dev",
		},
		"configurations": map[string]any{
			"configs": map[string]any{
				"envs": []any{
					map[string]any{"name": "CONFIG1", "value": "value1"},
				},
			},
			"secrets": map[string]any{
				"envs": []any{
					map[string]any{"name": "SECRET1", "remoteRef": map[string]any{"key": "secret", "property": "key"}},
				},
			},
		},
	}

	t.Run("size() operation", func(t *testing.T) {
		result, err := engine.Render(`${size(configurations.toContainerEnvFrom())}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if diff := cmp.Diff(int64(2), result); diff != "" {
			t.Errorf("size() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("map() operation to extract names", func(t *testing.T) {
		result, err := engine.Render(`${configurations.toContainerEnvFrom().map(e, has(e.configMapRef) ? e.configMapRef.name : e.secretRef.name)}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{generateEnvResourceName("app-dev", "env-configs"), generateEnvResourceName("app-dev", "env-secrets")}
		if diff := cmp.Diff(want, result); diff != "" {
			t.Errorf("map() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("concatenation with inline items", func(t *testing.T) {
		result, err := engine.Render(`${configurations.toContainerEnvFrom() + [{"configMapRef": {"name": "extra-config"}}]}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{
			map[string]any{"configMapRef": map[string]any{"name": generateEnvResourceName("app-dev", "env-configs")}},
			map[string]any{"secretRef": map[string]any{"name": generateEnvResourceName("app-dev", "env-secrets")}},
			map[string]any{"configMapRef": map[string]any{"name": "extra-config"}},
		}
		if diff := cmp.Diff(want, result); diff != "" {
			t.Errorf("concatenation mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestContainerConfigVolumeMountsMacro(t *testing.T) {
	tests := []struct {
		name   string
		expr   string
		inputs map[string]any
		want   []map[string]any
	}{
		{
			name: "container with both config and secret files",
			expr: `configurations.toContainerVolumeMounts()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{
						"files": []any{
							map[string]any{"name": "app.properties", "mountPath": "/etc/config"},
							map[string]any{"name": "config.json", "mountPath": "/etc/config"},
						},
					},
					"secrets": map[string]any{
						"files": []any{
							map[string]any{"name": "tls.crt", "mountPath": "/etc/tls"},
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "file-mount-" + generateVolumeHash("/etc/config", "app.properties"), "mountPath": "/etc/config/app.properties", "subPath": "app.properties"},
				{"name": "file-mount-" + generateVolumeHash("/etc/config", "config.json"), "mountPath": "/etc/config/config.json", "subPath": "config.json"},
				{"name": "file-mount-" + generateVolumeHash("/etc/tls", "tls.crt"), "mountPath": "/etc/tls/tls.crt", "subPath": "tls.crt"},
			},
		},
		{
			name: "container with no files returns empty list",
			expr: `configurations.toContainerVolumeMounts()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{"files": []any{}},
					"secrets": map[string]any{"files": []any{}},
				},
			},
			want: []map[string]any{},
		},
	}

	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Render("${"+tt.expr+"}", tt.inputs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, ok := result.([]map[string]any)
			if !ok {
				t.Fatalf("expected []map[string]any, got %T", result)
			}

			if diff := cmp.Diff(tt.want, got, cmpopts.SortSlices(func(a, b map[string]any) bool {
				return a["name"].(string) < b["name"].(string)
			})); diff != "" {
				t.Errorf("volumeMounts() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConfigurationsToVolumesMacro(t *testing.T) {
	tests := []struct {
		name   string
		expr   string
		inputs map[string]any
		want   []map[string]any
	}{
		{
			name: "single container with config and secret files",
			expr: `configurations.toVolumes()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{
						"files": []any{
							map[string]any{"name": "app.properties", "mountPath": "/etc/config"},
						},
					},
					"secrets": map[string]any{
						"files": []any{
							map[string]any{"name": "tls.crt", "mountPath": "/etc/tls"},
						},
					},
				},
			},
			want: []map[string]any{
				{
					"name": "file-mount-" + generateVolumeHash("/etc/config", "app.properties"),
					"configMap": map[string]any{
						"name": generateConfigResourceName("app-dev", "app.properties"),
					},
				},
				{
					"name": "file-mount-" + generateVolumeHash("/etc/tls", "tls.crt"),
					"secret": map[string]any{
						"secretName": generateSecretResourceName("app-dev", "tls.crt"),
					},
				},
			},
		},
		{
			name: "no files returns empty list",
			expr: `configurations.toVolumes()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{"files": []any{}},
					"secrets": map[string]any{"files": []any{}},
				},
			},
			want: []map[string]any{},
		},
	}

	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Render("${"+tt.expr+"}", tt.inputs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, ok := result.([]map[string]any)
			if !ok {
				t.Fatalf("expected []map[string]any, got %T", result)
			}

			if diff := cmp.Diff(tt.want, got, cmpopts.SortSlices(func(a, b map[string]any) bool {
				return a["name"].(string) < b["name"].(string)
			})); diff != "" {
				t.Errorf("volumes() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConfigurationsToConfigEnvsByContainerMacro(t *testing.T) {
	tests := []struct {
		name   string
		expr   string
		inputs map[string]any
		want   []map[string]any
	}{
		{
			name: "single container with config envs",
			expr: `configurations.toConfigEnvsByContainer()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{
						"envs": []any{
							map[string]any{"name": "LOG_LEVEL", "value": "info"},
							map[string]any{"name": "DEBUG_MODE", "value": "true"},
						},
					},
					"secrets": map[string]any{"envs": []any{}},
				},
			},
			want: []map[string]any{
				{
					"resourceName": generateEnvResourceName("app-dev", "env-configs"),
					"envs": []any{
						map[string]any{"name": "LOG_LEVEL", "value": "info"},
						map[string]any{"name": "DEBUG_MODE", "value": "true"},
					},
				},
			},
		},
		{
			name: "container with no config envs returns empty list",
			expr: `configurations.toConfigEnvsByContainer()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{"envs": []any{}},
					"secrets": map[string]any{"envs": []any{}},
				},
			},
			want: []map[string]any{},
		},
		{
			name: "container with only secrets (no configs) returns empty list",
			expr: `configurations.toConfigEnvsByContainer()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"secrets": map[string]any{
						"envs": []any{
							map[string]any{"name": "SECRET_KEY", "value": "secret"},
						},
					},
				},
			},
			want: []map[string]any{},
		},
		{
			name: "empty configurations returns empty list",
			expr: `configurations.toConfigEnvsByContainer()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{},
			},
			want: []map[string]any{},
		},
	}

	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Render("${"+tt.expr+"}", tt.inputs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, ok := result.([]map[string]any)
			if !ok {
				t.Fatalf("expected []map[string]any, got %T", result)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("toConfigEnvsByContainer() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConfigurationsToSecretEnvsByContainerMacro(t *testing.T) {
	tests := []struct {
		name   string
		expr   string
		inputs map[string]any
		want   []map[string]any
	}{
		{
			name: "single container with secret envs",
			expr: `configurations.toSecretEnvsByContainer()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{"envs": []any{}},
					"secrets": map[string]any{
						"envs": []any{
							map[string]any{
								"name": "DB_PASSWORD",
								"remoteRef": map[string]any{
									"key":      "db-password",
									"property": "password",
								},
							},
							map[string]any{
								"name": "API_KEY",
								"remoteRef": map[string]any{
									"key": "api-key",
								},
							},
						},
					},
				},
			},
			want: []map[string]any{
				{
					"resourceName": generateEnvResourceName("app-dev", "env-secrets"),
					"envs": []any{
						map[string]any{
							"name": "DB_PASSWORD",
							"remoteRef": map[string]any{
								"key":      "db-password",
								"property": "password",
							},
						},
						map[string]any{
							"name": "API_KEY",
							"remoteRef": map[string]any{
								"key": "api-key",
							},
						},
					},
				},
			},
		},
		{
			name: "container with no secret envs returns empty list",
			expr: `configurations.toSecretEnvsByContainer()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{"envs": []any{}},
					"secrets": map[string]any{"envs": []any{}},
				},
			},
			want: []map[string]any{},
		},
		{
			name: "container with only configs (no secrets) returns empty list",
			expr: `configurations.toSecretEnvsByContainer()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{
					"configs": map[string]any{
						"envs": []any{
							map[string]any{"name": "CONFIG_VAR", "value": "value"},
						},
					},
				},
			},
			want: []map[string]any{},
		},
		{
			name: "empty configurations returns empty list",
			expr: `configurations.toSecretEnvsByContainer()`,
			inputs: map[string]any{
				"metadata": map[string]any{
					"componentName":   "app",
					"environmentName": "dev",
				},
				"configurations": map[string]any{},
			},
			want: []map[string]any{},
		},
	}

	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Render("${"+tt.expr+"}", tt.inputs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, ok := result.([]map[string]any)
			if !ok {
				t.Fatalf("expected []map[string]any, got %T", result)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("toSecretEnvsByContainer() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestWorkloadEndpointsToServicePortsMacro(t *testing.T) {
	tests := []struct {
		name   string
		expr   string
		inputs map[string]any
		want   []map[string]any
	}{
		{
			name: "single HTTP endpoint",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"http": map[string]any{
							"type": "HTTP",
							"port": int64(8080),
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "http", "port": int64(8080), "targetPort": int64(8080), "protocol": "TCP"},
			},
		},
		{
			name: "multiple endpoints with different types",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"http": map[string]any{
							"type": "HTTP",
							"port": int64(8080),
						},
						"grpc": map[string]any{
							"type": "gRPC",
							"port": int64(9090),
						},
						"metrics": map[string]any{
							"type": "REST",
							"port": int64(9091),
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "http", "port": int64(8080), "targetPort": int64(8080), "protocol": "TCP"},
				{"name": "grpc", "port": int64(9090), "targetPort": int64(9090), "protocol": "TCP"},
				{"name": "metrics", "port": int64(9091), "targetPort": int64(9091), "protocol": "TCP"},
			},
		},
		{
			name: "empty endpoints returns empty list",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{},
				},
			},
			want: []map[string]any{},
		},
		{
			name: "TCP endpoint maps to TCP protocol",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"custom": map[string]any{
							"type": "TCP",
							"port": int64(5432),
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "custom", "port": int64(5432), "targetPort": int64(5432), "protocol": "TCP"},
			},
		},
		{
			name: "UDP endpoint maps to UDP protocol",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"dns": map[string]any{
							"type": "UDP",
							"port": int64(53),
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "dns", "port": int64(53), "targetPort": int64(53), "protocol": "UDP"},
			},
		},
		{
			name: "GraphQL endpoint maps to TCP protocol",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"graphql": map[string]any{
							"type": "GraphQL",
							"port": int64(8000),
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "graphql", "port": int64(8000), "targetPort": int64(8000), "protocol": "TCP"},
			},
		},
		{
			name: "Websocket endpoint maps to TCP protocol",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"websocket": map[string]any{
							"type": "Websocket",
							"port": int64(8080),
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "websocket", "port": int64(8080), "targetPort": int64(8080), "protocol": "TCP"},
			},
		},
		{
			name: "targetPort differs from port",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"http": map[string]any{
							"type":       "HTTP",
							"port":       int64(80),
							"targetPort": int64(8080),
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "http", "port": int64(80), "targetPort": int64(8080), "protocol": "TCP"},
			},
		},
		{
			name: "endpoint name with underscores converts to hyphens",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"api_endpoint": map[string]any{
							"type": "HTTP",
							"port": int64(8080),
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "api-endpoint", "port": int64(8080), "targetPort": int64(8080), "protocol": "TCP"},
			},
		},
		{
			name: "endpoint name with mixed case converts to lowercase",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"HttpAPI": map[string]any{
							"type": "HTTP",
							"port": int64(8080),
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "httpapi", "port": int64(8080), "targetPort": int64(8080), "protocol": "TCP"},
			},
		},
		{
			name: "endpoint name with invalid characters removed",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"api@endpoint!": map[string]any{
							"type": "HTTP",
							"port": int64(8080),
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "apiendpoint", "port": int64(8080), "targetPort": int64(8080), "protocol": "TCP"},
			},
		},
		{
			name: "endpoint name with leading/trailing hyphens trimmed",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"-api-": map[string]any{
							"type": "HTTP",
							"port": int64(8080),
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "api", "port": int64(8080), "targetPort": int64(8080), "protocol": "TCP"},
			},
		},
		{
			name: "endpoint name longer than 15 characters truncated",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"verylongendpointname": map[string]any{
							"type": "HTTP",
							"port": int64(8080),
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "verylongendpoin", "port": int64(8080), "targetPort": int64(8080), "protocol": "TCP"},
			},
		},
		{
			name: "endpoint name with only invalid characters uses port number",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"@@@": map[string]any{
							"type": "HTTP",
							"port": int64(8080),
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "port-8080", "port": int64(8080), "targetPort": int64(8080), "protocol": "TCP"},
			},
		},
		{
			name: "duplicate names after sanitization get unique suffixes",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"http": map[string]any{
							"type": "HTTP",
							"port": int64(8080),
						},
						"http_": map[string]any{
							"type": "HTTP",
							"port": int64(8081),
						},
						"HTTP": map[string]any{
							"type": "HTTP",
							"port": int64(8082),
						},
					},
				},
			},
			// With alphabetical sorting: "HTTP" (8082) -> "http", then "http" (8080) -> "http-2", then "http_" (8081) -> "http-3"
			want: []map[string]any{
				{"name": "http", "port": int64(8082), "targetPort": int64(8082), "protocol": "TCP"},
				{"name": "http-2", "port": int64(8080), "targetPort": int64(8080), "protocol": "TCP"},
				{"name": "http-3", "port": int64(8081), "targetPort": int64(8081), "protocol": "TCP"},
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

			got, ok := result.([]map[string]any)
			if !ok {
				t.Fatalf("expected []map[string]any, got %T", result)
			}

			if diff := cmp.Diff(tt.want, got, cmpopts.SortSlices(func(a, b map[string]any) bool {
				return a["name"].(string) < b["name"].(string)
			})); diff != "" {
				t.Errorf("toServicePorts() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestWorkloadEndpointsToServicePortsMacroErrors(t *testing.T) {
	tests := []struct {
		name        string
		expr        string
		inputs      map[string]any
		expectError string
	}{
		{
			name: "endpoint not an object returns error",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"http": "invalid",
					},
				},
			},
			expectError: "endpoint 'http' must be an object",
		},
		{
			name: "endpoint missing port field returns error",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"http": map[string]any{
							"type": "HTTP",
						},
					},
				},
			},
			expectError: "endpoint 'http' is missing required 'port' field",
		},
		{
			name: "endpoint with non-numeric port returns error",
			expr: `workload.toServicePorts()`,
			inputs: map[string]any{
				"workload": map[string]any{
					"endpoints": map[string]any{
						"http": map[string]any{
							"type": "HTTP",
							"port": "8080",
						},
					},
				},
			},
			expectError: "endpoint 'http' must have a numeric integer port",
		},
	}

	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := engine.Render("${"+tt.expr+"}", tt.inputs)
			if err == nil {
				t.Fatal("expected error but got none")
			}
			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got: %v", tt.expectError, err)
			}
		})
	}
}

func TestToServicePortsMacroOnlyExpandsForWorkloadEndpoints(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))

	// This should work - workload.endpoints is the expected receiver
	_, err := engine.Render(`${workload.toServicePorts()}`, map[string]any{
		"workload": map[string]any{
			"endpoints": map[string]any{},
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// This should fail - "other" is not a valid receiver for the macro
	_, err = engine.Render(`${other.toServicePorts()}`, map[string]any{
		"other": map[string]any{
			"endpoints": map[string]any{},
		},
	})
	if err == nil {
		t.Error("expected error for non-workload receiver")
	}

	// This should fail - direct call on non-endpoints field
	_, err = engine.Render(`${workload.containers.toServicePorts()}`, map[string]any{
		"workload": map[string]any{
			"containers": map[string]any{},
		},
	})
	if err == nil {
		t.Error("expected error for non-endpoints field")
	}
}

func TestToServicePortsCanBeUsedWithCELOperations(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))

	inputs := map[string]any{
		"workload": map[string]any{
			"endpoints": map[string]any{
				"http": map[string]any{
					"type": "HTTP",
					"port": int64(8080),
				},
				"grpc": map[string]any{
					"type": "gRPC",
					"port": int64(9090),
				},
			},
		},
	}

	t.Run("size() operation", func(t *testing.T) {
		result, err := engine.Render(`${size(workload.toServicePorts())}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if diff := cmp.Diff(int64(2), result); diff != "" {
			t.Errorf("size() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("map() operation to extract port names", func(t *testing.T) {
		result, err := engine.Render(`${workload.toServicePorts().map(p, p.name)}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{"http", "grpc"}
		if diff := cmp.Diff(want, result, cmpopts.SortSlices(func(a, b any) bool {
			return a.(string) < b.(string)
		})); diff != "" {
			t.Errorf("map() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("map() operation to extract port numbers", func(t *testing.T) {
		result, err := engine.Render(`${workload.toServicePorts().map(p, p.port)}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{int64(8080), int64(9090)}
		if diff := cmp.Diff(want, result, cmpopts.SortSlices(func(a, b any) bool {
			return a.(int64) < b.(int64)
		})); diff != "" {
			t.Errorf("map() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("filter() operation by protocol", func(t *testing.T) {
		udpInputs := map[string]any{
			"workload": map[string]any{
				"endpoints": map[string]any{
					"http": map[string]any{
						"type": "HTTP",
						"port": int64(8080),
					},
					"dns": map[string]any{
						"type": "UDP",
						"port": int64(53),
					},
				},
			},
		}
		result, err := engine.Render(`${workload.toServicePorts().filter(p, p.protocol == "UDP")}`, udpInputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{
			map[string]any{"name": "dns", "port": int64(53), "targetPort": int64(53), "protocol": "UDP"},
		}
		if diff := cmp.Diff(want, result); diff != "" {
			t.Errorf("filter() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("list concatenation with inline items", func(t *testing.T) {
		result, err := engine.Render(`${workload.toServicePorts() + [{"name": "admin", "port": 9999, "targetPort": 9999, "protocol": "TCP"}]}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{
			map[string]any{"name": "http", "port": int64(8080), "targetPort": int64(8080), "protocol": "TCP"},
			map[string]any{"name": "grpc", "port": int64(9090), "targetPort": int64(9090), "protocol": "TCP"},
			map[string]any{"name": "admin", "port": int64(9999), "targetPort": int64(9999), "protocol": "TCP"},
		}
		if diff := cmp.Diff(want, result, cmpopts.SortSlices(func(a, b any) bool {
			aMap, aOk := a.(map[string]any)
			bMap, bOk := b.(map[string]any)
			if aOk && bOk {
				return aMap["name"].(string) < bMap["name"].(string)
			}
			return false
		})); diff != "" {
			t.Errorf("concatenation mismatch (-want +got):\n%s", diff)
		}
	})
}
