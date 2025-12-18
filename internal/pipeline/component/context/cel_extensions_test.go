// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
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
