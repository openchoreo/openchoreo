// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
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
			expr: `configurations.toConfigFileList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
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
			},
			want: []map[string]any{
				{
					"name":         "config.yaml",
					"mountPath":    "/etc/config/config.yaml",
					"value":        "key: value",
					"resourceName": generateConfigResourceName("myapp", "app", "config.yaml"),
				},
			},
		},
		{
			name: "single container with multiple config files",
			expr: `configurations.toConfigFileList("prefix")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"configs": map[string]any{
							"files": []any{
								map[string]any{"name": "app.yaml", "mountPath": "/etc/app.yaml", "value": "app config"},
								map[string]any{"name": "logging.properties", "mountPath": "/etc/logging.properties", "value": "log.level=INFO"},
							},
						},
						"secrets": map[string]any{"files": []any{}},
					},
				},
			},
			want: []map[string]any{
				{"name": "app.yaml", "mountPath": "/etc/app.yaml", "value": "app config", "resourceName": generateConfigResourceName("prefix", "app", "app.yaml")},
				{"name": "logging.properties", "mountPath": "/etc/logging.properties", "value": "log.level=INFO", "resourceName": generateConfigResourceName("prefix", "app", "logging.properties")},
			},
		},
		{
			name: "no config files returns empty list",
			expr: `configurations.toConfigFileList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"configs": map[string]any{"files": []any{}},
						"secrets": map[string]any{"files": []any{}},
					},
				},
			},
			want: []map[string]any{},
		},
		{
			name: "empty configurations returns empty list",
			expr: `configurations.toConfigFileList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{},
			},
			want: []map[string]any{},
		},
		{
			name: "config file with remoteRef",
			expr: `configurations.toConfigFileList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
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
			},
			want: []map[string]any{
				{
					"name":         "config.yaml",
					"mountPath":    "/etc/config.yaml",
					"value":        "",
					"resourceName": generateConfigResourceName("myapp", "app", "config.yaml"),
					"remoteRef": map[string]any{
						"key":      "my-config-key",
						"property": "config.yaml",
					},
				},
			},
		},
		{
			name: "dots in filename are replaced with hyphens in resourceName",
			expr: `configurations.toConfigFileList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"configs": map[string]any{
							"files": []any{
								map[string]any{"name": "application.properties", "mountPath": "/etc/application.properties", "value": "prop=value"},
							},
						},
						"secrets": map[string]any{"files": []any{}},
					},
				},
			},
			want: []map[string]any{
				{"name": "application.properties", "mountPath": "/etc/application.properties", "value": "prop=value", "resourceName": generateConfigResourceName("myapp", "app", "application.properties")},
			},
		},
		{
			name: "ignores secret files (only returns config files)",
			expr: `configurations.toConfigFileList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
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
			},
			want: []map[string]any{
				{"name": "config.yaml", "mountPath": "/etc/config.yaml", "value": "config", "resourceName": generateConfigResourceName("myapp", "app", "config.yaml")},
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
	_, err := engine.Render(`${configurations.toConfigFileList("prefix")}`, map[string]any{
		"configurations": map[string]any{},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// This should fail - "other" is not a valid receiver for the macro
	_, err = engine.Render(`${other.toConfigFileList("prefix")}`, map[string]any{
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
		"configurations": map[string]any{
			"app": map[string]any{
				"configs": map[string]any{
					"files": []any{
						map[string]any{"name": "a.yaml", "mountPath": "/a.yaml", "value": "a"},
						map[string]any{"name": "b.yaml", "mountPath": "/b.yaml", "value": "b"},
					},
				},
				"secrets": map[string]any{"files": []any{}},
			},
		},
	}

	t.Run("size() operation", func(t *testing.T) {
		result, err := engine.Render(`${size(configurations.toConfigFileList("prefix"))}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if diff := cmp.Diff(int64(2), result); diff != "" {
			t.Errorf("size() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("map() operation", func(t *testing.T) {
		result, err := engine.Render(`${configurations.toConfigFileList("prefix").map(f, f.name)}`, inputs)
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
		result, err := engine.Render(`${configurations.toConfigFileList("prefix") + [{"name": "inline.yaml", "mountPath": "/inline.yaml"}]}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{
			map[string]any{"name": "a.yaml", "mountPath": "/a.yaml", "value": "a", "resourceName": generateConfigResourceName("prefix", "app", "a.yaml")},
			map[string]any{"name": "b.yaml", "mountPath": "/b.yaml", "value": "b", "resourceName": generateConfigResourceName("prefix", "app", "b.yaml")},
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
			expr: `configurations.toSecretFileList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"configs": map[string]any{"files": []any{}},
						"secrets": map[string]any{
							"files": []any{
								map[string]any{
									"name":      "secret.yaml",
									"mountPath": "/etc/secret/secret.yaml",
									"value":     "password: secret123",
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
					"value":        "password: secret123",
					"resourceName": generateSecretResourceName("myapp", "app", "secret.yaml"),
				},
			},
		},
		{
			name: "single container with multiple secret files",
			expr: `configurations.toSecretFileList("prefix")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"configs": map[string]any{"files": []any{}},
						"secrets": map[string]any{
							"files": []any{
								map[string]any{"name": "db.env", "mountPath": "/etc/db.env", "value": "DB_PASSWORD=secret"},
								map[string]any{"name": "api.key", "mountPath": "/etc/api.key", "value": "API_KEY=abcd1234"},
							},
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "db.env", "mountPath": "/etc/db.env", "value": "DB_PASSWORD=secret", "resourceName": generateSecretResourceName("prefix", "app", "db.env")},
				{"name": "api.key", "mountPath": "/etc/api.key", "value": "API_KEY=abcd1234", "resourceName": generateSecretResourceName("prefix", "app", "api.key")},
			},
		},
		{
			name: "no secret files returns empty list",
			expr: `configurations.toSecretFileList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"configs": map[string]any{"files": []any{}},
						"secrets": map[string]any{"files": []any{}},
					},
				},
			},
			want: []map[string]any{},
		},
		{
			name: "empty configurations returns empty list",
			expr: `configurations.toSecretFileList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{},
			},
			want: []map[string]any{},
		},
		{
			name: "secret file with remoteRef",
			expr: `configurations.toSecretFileList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
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
			},
			want: []map[string]any{
				{
					"name":         "secret.yaml",
					"mountPath":    "/etc/secret.yaml",
					"value":        "",
					"resourceName": generateSecretResourceName("myapp", "app", "secret.yaml"),
					"remoteRef": map[string]any{
						"key":      "my-secret-key",
						"property": "secret.yaml",
					},
				},
			},
		},
		{
			name: "dots in filename are replaced with hyphens in resourceName",
			expr: `configurations.toSecretFileList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"configs": map[string]any{"files": []any{}},
						"secrets": map[string]any{
							"files": []any{
								map[string]any{"name": "database.properties", "mountPath": "/etc/database.properties", "value": "password=secret"},
							},
						},
					},
				},
			},
			want: []map[string]any{
				{"name": "database.properties", "mountPath": "/etc/database.properties", "value": "password=secret", "resourceName": generateSecretResourceName("myapp", "app", "database.properties")},
			},
		},
		{
			name: "ignores config files (only returns secret files)",
			expr: `configurations.toSecretFileList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
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
			},
			want: []map[string]any{
				{"name": "secret.yaml", "mountPath": "/etc/secret.yaml", "value": "secret", "resourceName": generateSecretResourceName("myapp", "app", "secret.yaml")},
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
	_, err := engine.Render(`${configurations.toSecretFileList("prefix")}`, map[string]any{
		"configurations": map[string]any{},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// This should fail - "other" is not a valid receiver for the macro
	_, err = engine.Render(`${other.toSecretFileList("prefix")}`, map[string]any{
		"other": map[string]any{},
	})
	if err == nil {
		t.Error("expected error for non-configurations receiver")
	}
}

func TestToSecretFileListCanBeUsedWithCELOperations(t *testing.T) {
	engine := template.NewEngineWithOptions(template.WithCELExtensions(CELExtensions()...))

	inputs := map[string]any{
		"configurations": map[string]any{
			"app": map[string]any{
				"configs": map[string]any{"files": []any{}},
				"secrets": map[string]any{
					"files": []any{
						map[string]any{"name": "a.secret", "mountPath": "/a.secret", "value": "a"},
						map[string]any{"name": "b.secret", "mountPath": "/b.secret", "value": "b"},
					},
				},
			},
		},
	}

	t.Run("size() operation", func(t *testing.T) {
		result, err := engine.Render(`${size(configurations.toSecretFileList("prefix"))}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if diff := cmp.Diff(int64(2), result); diff != "" {
			t.Errorf("size() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("map() operation", func(t *testing.T) {
		result, err := engine.Render(`${configurations.toSecretFileList("prefix").map(f, f.name)}`, inputs)
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
		result, err := engine.Render(`${configurations.toSecretFileList("prefix") + [{"name": "inline.secret", "mountPath": "/inline.secret"}]}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{
			map[string]any{"name": "a.secret", "mountPath": "/a.secret", "value": "a", "resourceName": generateSecretResourceName("prefix", "app", "a.secret")},
			map[string]any{"name": "b.secret", "mountPath": "/b.secret", "value": "b", "resourceName": generateSecretResourceName("prefix", "app", "b.secret")},
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
			expr: `configurations.toContainerEnvFrom("app", "myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
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
			},
			want: []map[string]any{
				{"configMapRef": map[string]any{"name": "myapp-app-env-configs-4733bd36"}},
				{"secretRef": map[string]any{"name": "myapp-app-env-secrets-0cfffcf8"}},
			},
		},
		{
			name: "container with only config envs",
			expr: `configurations.toContainerEnvFrom("app", "prefix")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"configs": map[string]any{
							"envs": []any{
								map[string]any{"name": "DEBUG", "value": "true"},
								map[string]any{"name": "PORT", "value": "8080"},
							},
						},
						"secrets": map[string]any{"envs": []any{}},
					},
				},
			},
			want: []map[string]any{
				{"configMapRef": map[string]any{"name": "prefix-app-env-configs-be860d65"}},
			},
		},
		{
			name: "container with only secret envs",
			expr: `configurations.toContainerEnvFrom("worker", "myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"worker": map[string]any{
						"configs": map[string]any{"envs": []any{}},
						"secrets": map[string]any{
							"envs": []any{
								map[string]any{"name": "DB_PASSWORD", "remoteRef": map[string]any{"key": "db-secret", "property": "password"}},
							},
						},
					},
				},
			},
			want: []map[string]any{
				{"secretRef": map[string]any{"name": "myapp-worker-env-secrets-6bc17e4e"}},
			},
		},
		{
			name: "container with no envs returns empty list",
			expr: `configurations.toContainerEnvFrom("app", "myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"configs": map[string]any{"envs": []any{}},
						"secrets": map[string]any{"envs": []any{}},
					},
				},
			},
			want: []map[string]any{},
		},
		{
			name: "empty container config returns empty list",
			expr: `configurations.toContainerEnvFrom("empty", "myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"empty": map[string]any{},
				},
			},
			want: []map[string]any{},
		},
		{
			name: "container missing configs section",
			expr: `configurations.toContainerEnvFrom("app", "prefix")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"secrets": map[string]any{
							"envs": []any{
								map[string]any{"name": "SECRET_KEY", "remoteRef": map[string]any{"key": "app-secret", "property": "key"}},
							},
						},
					},
				},
			},
			want: []map[string]any{
				{"secretRef": map[string]any{"name": "prefix-app-env-secrets-b93109bb"}},
			},
		},
		{
			name: "container missing secrets section",
			expr: `configurations.toContainerEnvFrom("app", "prefix")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"configs": map[string]any{
							"envs": []any{
								map[string]any{"name": "CONFIG_VAR", "value": "value"},
							},
						},
					},
				},
			},
			want: []map[string]any{
				{"configMapRef": map[string]any{"name": "prefix-app-env-configs-be860d65"}},
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
	_, err := engine.Render(`${configurations.toContainerEnvFrom("main", "prefix")}`, map[string]any{
		"configurations": map[string]any{
			"main": map[string]any{},
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// toContainerEnvFrom only works on configurations, not arbitrary variables
	_, err = engine.Render(`${someVar.toContainerEnvFrom("main", "prefix")}`, map[string]any{
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
		"configurations": map[string]any{
			"app": map[string]any{
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
		},
	}

	t.Run("size() operation", func(t *testing.T) {
		result, err := engine.Render(`${size(configurations.toContainerEnvFrom("app", "prefix"))}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if diff := cmp.Diff(int64(2), result); diff != "" {
			t.Errorf("size() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("map() operation to extract names", func(t *testing.T) {
		result, err := engine.Render(`${configurations.toContainerEnvFrom("app", "prefix").map(e, has(e.configMapRef) ? e.configMapRef.name : e.secretRef.name)}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{"prefix-app-env-configs-be860d65", "prefix-app-env-secrets-b93109bb"}
		if diff := cmp.Diff(want, result); diff != "" {
			t.Errorf("map() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("concatenation with inline items", func(t *testing.T) {
		result, err := engine.Render(`${configurations.toContainerEnvFrom("app", "prefix") + [{"configMapRef": {"name": "extra-config"}}]}`, inputs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []any{
			map[string]any{"configMapRef": map[string]any{"name": "prefix-app-env-configs-be860d65"}},
			map[string]any{"secretRef": map[string]any{"name": "prefix-app-env-secrets-b93109bb"}},
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
			expr: `configurations.toContainerVolumeMounts("main")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"main": map[string]any{
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
			},
			want: []map[string]any{
				{"name": "main-file-mount-" + generateVolumeHash("/etc/config", "app.properties"), "mountPath": "/etc/config/app.properties", "subPath": "app.properties"},
				{"name": "main-file-mount-" + generateVolumeHash("/etc/config", "config.json"), "mountPath": "/etc/config/config.json", "subPath": "config.json"},
				{"name": "main-file-mount-" + generateVolumeHash("/etc/tls", "tls.crt"), "mountPath": "/etc/tls/tls.crt", "subPath": "tls.crt"},
			},
		},
		{
			name: "container with no files returns empty list",
			expr: `configurations.toContainerVolumeMounts("app")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"configs": map[string]any{"files": []any{}},
						"secrets": map[string]any{"files": []any{}},
					},
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
			name: "multiple containers with config and secret files",
			expr: `configurations.toVolumes("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"main": map[string]any{
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
					"sidecar": map[string]any{
						"configs": map[string]any{
							"files": []any{
								map[string]any{"name": "proxy.conf", "mountPath": "/etc/proxy"},
							},
						},
						"secrets": map[string]any{"files": []any{}},
					},
				},
			},
			want: []map[string]any{
				{
					"name": "main-file-mount-" + generateVolumeHash("/etc/config", "app.properties"),
					"configMap": map[string]any{
						"name": generateConfigResourceName("myapp", "main", "app.properties"),
					},
				},
				{
					"name": "main-file-mount-" + generateVolumeHash("/etc/tls", "tls.crt"),
					"secret": map[string]any{
						"secretName": generateSecretResourceName("myapp", "main", "tls.crt"),
					},
				},
				{
					"name": "sidecar-file-mount-" + generateVolumeHash("/etc/proxy", "proxy.conf"),
					"configMap": map[string]any{
						"name": generateConfigResourceName("myapp", "sidecar", "proxy.conf"),
					},
				},
			},
		},
		{
			name: "no files returns empty list",
			expr: `configurations.toVolumes("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"configs": map[string]any{"files": []any{}},
						"secrets": map[string]any{"files": []any{}},
					},
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

func TestConfigurationsToConfigEnvListMacro(t *testing.T) {
	tests := []struct {
		name   string
		expr   string
		inputs map[string]any
		want   []map[string]any
	}{
		{
			name: "multiple containers with config envs",
			expr: `configurations.toConfigEnvList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"main": map[string]any{
						"configs": map[string]any{
							"envs": []any{
								map[string]any{"name": "LOG_LEVEL", "value": "info"},
								map[string]any{"name": "DEBUG_MODE", "value": "true"},
							},
						},
					},
					"sidecar": map[string]any{
						"configs": map[string]any{
							"envs": []any{
								map[string]any{"name": "PROXY_PORT", "value": "8080"},
							},
						},
					},
				},
			},
			want: []map[string]any{
				{
					"container":    "main",
					"resourceName": generateEnvConfigResourceName("myapp", "main"),
					"envs": []any{
						map[string]any{"name": "LOG_LEVEL", "value": "info"},
						map[string]any{"name": "DEBUG_MODE", "value": "true"},
					},
				},
				{
					"container":    "sidecar",
					"resourceName": generateEnvConfigResourceName("myapp", "sidecar"),
					"envs": []any{
						map[string]any{"name": "PROXY_PORT", "value": "8080"},
					},
				},
			},
		},
		{
			name: "single container with config envs",
			expr: `configurations.toConfigEnvList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"configs": map[string]any{
							"envs": []any{
								map[string]any{"name": "ENV_VAR", "value": "value1"},
							},
						},
					},
				},
			},
			want: []map[string]any{
				{
					"container":    "app",
					"resourceName": generateEnvConfigResourceName("myapp", "app"),
					"envs": []any{
						map[string]any{"name": "ENV_VAR", "value": "value1"},
					},
				},
			},
		},
		{
			name: "container with no config envs returns empty list",
			expr: `configurations.toConfigEnvList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"configs": map[string]any{"envs": []any{}},
					},
				},
			},
			want: []map[string]any{},
		},
		{
			name: "container with only secrets (no configs) returns empty list",
			expr: `configurations.toConfigEnvList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"secrets": map[string]any{
							"envs": []any{
								map[string]any{"name": "SECRET_KEY", "value": "secret"},
							},
						},
					},
				},
			},
			want: []map[string]any{},
		},
		{
			name: "empty configurations returns empty list",
			expr: `configurations.toConfigEnvList("myapp")`,
			inputs: map[string]any{
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

			if diff := cmp.Diff(tt.want, got, cmpopts.SortSlices(func(a, b map[string]any) bool {
				return a["container"].(string) < b["container"].(string)
			})); diff != "" {
				t.Errorf("toConfigEnvList() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConfigurationsToSecretEnvListMacro(t *testing.T) {
	tests := []struct {
		name   string
		expr   string
		inputs map[string]any
		want   []map[string]any
	}{
		{
			name: "multiple containers with secret envs",
			expr: `configurations.toSecretEnvList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"main": map[string]any{
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
					"sidecar": map[string]any{
						"secrets": map[string]any{
							"envs": []any{
								map[string]any{
									"name": "PROXY_SECRET",
									"remoteRef": map[string]any{
										"key": "proxy-secret",
									},
								},
							},
						},
					},
				},
			},
			want: []map[string]any{
				{
					"container":    "main",
					"resourceName": generateEnvSecretResourceName("myapp", "main"),
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
				{
					"container":    "sidecar",
					"resourceName": generateEnvSecretResourceName("myapp", "sidecar"),
					"envs": []any{
						map[string]any{
							"name": "PROXY_SECRET",
							"remoteRef": map[string]any{
								"key": "proxy-secret",
							},
						},
					},
				},
			},
		},
		{
			name: "single container with secret envs",
			expr: `configurations.toSecretEnvList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"secrets": map[string]any{
							"envs": []any{
								map[string]any{
									"name": "SECRET_KEY",
									"remoteRef": map[string]any{
										"key": "secret-key",
									},
								},
							},
						},
					},
				},
			},
			want: []map[string]any{
				{
					"container":    "app",
					"resourceName": generateEnvSecretResourceName("myapp", "app"),
					"envs": []any{
						map[string]any{
							"name": "SECRET_KEY",
							"remoteRef": map[string]any{
								"key": "secret-key",
							},
						},
					},
				},
			},
		},
		{
			name: "container with no secret envs returns empty list",
			expr: `configurations.toSecretEnvList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"secrets": map[string]any{"envs": []any{}},
					},
				},
			},
			want: []map[string]any{},
		},
		{
			name: "container with only configs (no secrets) returns empty list",
			expr: `configurations.toSecretEnvList("myapp")`,
			inputs: map[string]any{
				"configurations": map[string]any{
					"app": map[string]any{
						"configs": map[string]any{
							"envs": []any{
								map[string]any{"name": "CONFIG_VAR", "value": "value"},
							},
						},
					},
				},
			},
			want: []map[string]any{},
		},
		{
			name: "empty configurations returns empty list",
			expr: `configurations.toSecretEnvList("myapp")`,
			inputs: map[string]any{
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

			if diff := cmp.Diff(tt.want, got, cmpopts.SortSlices(func(a, b map[string]any) bool {
				return a["container"].(string) < b["container"].(string)
			})); diff != "" {
				t.Errorf("toSecretEnvList() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// Helper function to generate env config resource names for testing
func generateEnvConfigResourceName(prefix, container string) string {
	return kubernetes.GenerateK8sNameWithLengthLimit(
		kubernetes.MaxResourceNameLength,
		prefix,
		container,
		"env-configs",
	)
}

// Helper function to generate env secret resource names for testing
func generateEnvSecretResourceName(prefix, container string) string {
	return kubernetes.GenerateK8sNameWithLengthLimit(
		kubernetes.MaxResourceNameLength,
		prefix,
		container,
		"env-secrets",
	)
}
