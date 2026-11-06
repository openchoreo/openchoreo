// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func TestPlaintextCredentials(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "literal", value: "foo123", want: true},
		{name: "empty", value: ""},
		{name: "number", value: 123},
		{name: "structured reference", value: map[string]any{"secretKeyRef": map[string]any{"name": "db", "key": "password"}}},
		{name: "ESO reference", value: "{{ .password }}"},
		{name: "nested ESO reference", value: "{{ .credentials.password }}"},
		{name: "CEL reference", value: "${parameters.password}"},
		{name: "CEL indexed reference", value: "${externalRefs['db-creds'].spec.data[0].password}"},
		{name: "CEL double quoted index", value: `${parameters["password"]}`},
		{name: "reference with whitespace", value: " \n{{ .password }}\n "},
		{name: "mixed ESO literal", value: "literal-{{ .password }}", want: true},
		{name: "mixed CEL literal", value: "${parameters.password}-literal", want: true},
		{name: "multiple references with literal", value: "${parameters.password}-literal-${parameters.token}", want: true},
		{name: "ESO literal expression", value: `{{ "hardcoded" }}`, want: true},
		{name: "ESO literal default", value: `{{ .password | default "hardcoded" }}`, want: true},
		{name: "CEL literal expression", value: "${'hardcoded'}", want: true},
		{name: "CEL literal fallback", value: "${has(parameters.password) ? parameters.password : 'hardcoded'}", want: true},
		{name: "malformed reference", value: "{{ .password", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, paired := range []bool{false, true} {
				object := map[string]any{"password": tt.value}
				field := "password"
				path := ".password"
				if paired {
					object = map[string]any{"key": "DB_PASSWORD", "value": tt.value}
					field = "value"
					path = `.value (key "DB_PASSWORD")`
				}
				if tt.want {
					assert.Equal(t, []string{path}, findPlaintextSecrets(object))
					assert.Equal(t, 1, stripPlaintextSecrets(object))
					assert.NotContains(t, object, field)
				} else {
					assert.Empty(t, findPlaintextSecrets(object))
					assert.Zero(t, stripPlaintextSecrets(object))
					assert.Equal(t, tt.value, object[field])
				}
			}
		})
	}
}

func TestStripPlaintextCredentialsPreservesReferencesInSameResource(t *testing.T) {
	object := map[string]any{
		"spec": map[string]any{
			"dbPassword": "literal-password",
			"password":   "{{ .password }}",
			"apiToken":   "${parameters.apiToken}",
			"env": []any{
				map[string]any{"key": "CLIENT_SECRET", "value": "literal-secret"},
				map[string]any{"key": "DB_PASSWORD", "value": "{{ .password }}"},
				map[string]any{"key": "API_TOKEN", "value": "${parameters.apiToken}"},
			},
			"image": "nginx:latest",
		},
	}
	assert.Equal(t, []string{".spec.dbPassword", `.spec.env[0].value (key "CLIENT_SECRET")`}, findPlaintextSecrets(object))
	assert.Equal(t, 2, stripPlaintextSecrets(object))
	assert.Equal(t, map[string]any{
		"spec": map[string]any{
			"password": "{{ .password }}",
			"apiToken": "${parameters.apiToken}",
			"env": []any{
				map[string]any{"key": "CLIENT_SECRET"},
				map[string]any{"key": "DB_PASSWORD", "value": "{{ .password }}"},
				map[string]any{"key": "API_TOKEN", "value": "${parameters.apiToken}"},
			},
			"image": "nginx:latest",
		},
	}, object)
}

func TestPlaintextCredentialsPreservesWorkloadBindings(t *testing.T) {
	for _, kind := range []string{"Workload", KindComponentRelease} {
		t.Run(kind, func(t *testing.T) {
			bindings := map[string]any{
				"envBindings": map[string]any{
					"password": "DB_PASSWORD",
					"apiToken": "API_TOKEN",
					"key":      "CLIENT_SECRET",
					"value":    "CLIENT_SECRET_ENV",
				},
				"fileBindings": map[string]any{
					"password":   "/etc/database/password",
					"privateKey": "/etc/database/tls.key",
				},
			}
			workload := map[string]any{
				"dependencies": map[string]any{
					"resources": []any{map[string]any{"ref": "database"}, bindings},
				},
				"container": map[string]any{
					"env": []any{map[string]any{"key": "DB_PASSWORD", "value": "literal-password"}},
				},
			}
			spec := workload
			prefix := ".spec"
			if kind == KindComponentRelease {
				spec = map[string]any{"workload": workload}
				prefix += ".workload"
			}
			object := map[string]any{"kind": kind, "spec": spec}
			before, err := serialize(bindings)
			require.NoError(t, err)

			assert.Equal(t, []string{prefix + `.container.env[0].value (key "DB_PASSWORD")`}, findPlaintextSecrets(object))
			assert.Equal(t, 1, stripPlaintextSecrets(object))
			assert.Empty(t, findPlaintextSecrets(object))
			after, err := serialize(bindings)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

func TestPlaintextCredentialsDoesNotExemptArbitraryBindings(t *testing.T) {
	for _, kind := range []string{"", "Workload", KindComponentRelease, KindResource} {
		t.Run(kind, func(t *testing.T) {
			object := map[string]any{
				"kind": kind,
				"spec": map[string]any{
					"parameters": map[string]any{
						"envBindings":  map[string]any{"password": "literal-password"},
						"fileBindings": map[string]any{"privateKey": "literal-key"},
					},
				},
			}
			assert.Equal(t, []string{
				".spec.parameters.envBindings.password",
				".spec.parameters.fileBindings.privateKey",
			}, findPlaintextSecrets(object))
			assert.Equal(t, 2, stripPlaintextSecrets(object))
		})
	}

	// The same path in a different resource kind must not receive the exemption.
	object := map[string]any{
		"kind": KindResource,
		"spec": map[string]any{
			"dependencies": map[string]any{
				"resources": []any{map[string]any{
					"envBindings": map[string]any{"password": "literal-password"},
				}},
			},
		},
	}
	assert.Equal(t, []string{".spec.dependencies.resources[0].envBindings.password"}, findPlaintextSecrets(object))
}

func TestResourceTypeSamplesDoNotContainPlaintextCredentials(t *testing.T) {
	for _, name := range []string{"nats", "postgres", "valkey"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "..", "..", "samples", "getting-started", "cluster-resource-types", name+".yaml")
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			var input map[string]any
			require.NoError(t, yaml.Unmarshal(raw, &input))
			object, err := normalizeObject(input, KindClusterResourceType, "")
			require.NoError(t, err)
			before, err := serialize(object)
			require.NoError(t, err)

			assert.Empty(t, findPlaintextSecrets(object))
			assert.Zero(t, stripPlaintextSecrets(object))

			after, err := serialize(object)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}
