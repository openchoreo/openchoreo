// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestEngineRender(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		inputs   string
		want     string
	}{
		{
			name: "string literal without expressions",
			template: `
plain: hello
`,
			inputs: `{}`,
			want: `plain: hello
`,
		},
		{
			name: "string interpolation and numeric result",
			template: `
message: "${metadata.name} has ${spec.replicas} replicas"
numeric: ${spec.replicas}
`,
			inputs: `{
  "metadata": {"name": "checkout"},
  "spec": {"replicas": 2}
}`,
			want: `message: checkout has 2 replicas
numeric: 2
`,
		},
		{
			name: "map with omit and merge helpers",
			template: `
annotations:
  base: '${merge({"team": "platform"}, metadata.labels)}'
  optional: '${has(spec.flag) && spec.flag ? {"enabled": "true"} : omit()}'
`,
			inputs: `{
  "metadata": {"labels": {"team": "payments", "region": "us"}},
  "spec": {"flag": true}
}`,
			want: `annotations:
  base:
    region: us
    team: payments
  optional:
    enabled: "true"
`,
		},
		{
			name: "array forEach via CEL comprehension",
			template: `
env: '${containers.map(c, {"name": c.name, "image": c.image})}'
`,
			inputs: `{
  "containers": [
    {"name": "app", "image": "app:1.0"},
    {"name": "sidecar", "image": "sidecar:latest"}
  ]
}`,
			want: `env:
- image: app:1.0
  name: app
- image: sidecar:latest
  name: sidecar
`,
		},
		{
			name: "full object literal",
			template: `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${metadata.name}
spec:
  replicas: ${spec.replicas}
  template:
    metadata:
      labels: ${metadata.labels}
`,
			inputs: `{
  "metadata": {"name": "web", "labels": {"app": "web"}},
  "spec": {"replicas": 3}
}`,
			want: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 3
  template:
    metadata:
      labels:
        app: web
`,
		},
		{
			name: "sanitizeK8sResourceName with single argument",
			template: `
name: ${sanitizeK8sResourceName("Hello World!")}
`,
			inputs: `{}`,
			want: `name: hello-world-7f83b165
`,
		},
		{
			name: "sanitizeK8sResourceName with multiple arguments",
			template: `
name: ${sanitizeK8sResourceName("my-app","v1.2.3")}
`,
			inputs: `{}`,
			want: `name: my-app-v1.2.3-4f878dd8
`,
		},
		{
			name: "sanitizeK8sResourceName with many arguments",
			template: `
name: ${sanitizeK8sResourceName("front","end","prod","us-west","99")}
`,
			inputs: `{}`,
			want: `name: front-end-prod-us-west-99-c89a6670
`,
		},
		{
			name: "sanitizeK8sResourceName with dynamic values",
			template: `
name: ${sanitizeK8sResourceName(metadata.name, spec.version)}
`,
			inputs: `{
  "metadata": {"name": "payment-service"},
  "spec": {"version": "v2.0"}
}`,
			want: `name: payment-service-v2.0-bd17faf1
`,
		},
		{
			name: "dynamic map key with string concatenation and number",
			template: `
services:
  ${'port-' + string(metadata.port)}: ${metadata.serviceName}
`,
			inputs: `{
  "metadata": {"port": 8080, "serviceName": "web-service"}
}`,
			want: `services:
  port-8080: web-service
`,
		},
		{
			name: "sha256sum function",
			template: `
hash: ${sha256sum("hello world")}
dynamicHash: ${sha256sum(metadata.value)}
`,
			inputs: `{
  "metadata": {"value": "test data"}
}`,
			want: `hash: b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9
dynamicHash: 916f0027a575074ce72a331777c3478d6513f786a591bd892da1a577bf2335f9
`,
		},
	}

	engine := NewEngine()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var tpl any
			if err := yaml.Unmarshal([]byte(tt.template), &tpl); err != nil {
				t.Fatalf("failed to unmarshal template: %v", err)
			}

			var input map[string]any
			if err := json.Unmarshal([]byte(tt.inputs), &input); err != nil {
				t.Fatalf("failed to unmarshal inputs: %v", err)
			}

			rendered, err := engine.Render(tpl, input)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			cleaned := RemoveOmittedFields(rendered)

			got, err := yaml.Marshal(cleaned)
			if err != nil {
				t.Fatalf("failed to marshal result: %v", err)
			}

			if err := compareYAML(tt.want, string(got)); err != nil {
				t.Fatalf("rendered output mismatch: %v", err)
			}
		})
	}
}

func compareYAML(expected, actual string) error {
	var wantObj, gotObj any
	if err := yaml.Unmarshal([]byte(expected), &wantObj); err != nil {
		return fmt.Errorf("failed to unmarshal expected YAML: %w", err)
	}
	if err := yaml.Unmarshal([]byte(actual), &gotObj); err != nil {
		return fmt.Errorf("failed to unmarshal actual YAML: %w", err)
	}

	wantBytes, _ := yaml.Marshal(wantObj)
	gotBytes, _ := yaml.Marshal(gotObj)

	if string(wantBytes) != string(gotBytes) {
		return fmt.Errorf("want:\n%s\n\ngot:\n%s\n", wantBytes, gotBytes)
	}
	return nil
}

func TestRenderErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		template      string
		inputs        string
		wantErr       bool
		errContains   string
		wantIsMissing bool
	}{
		// String value errors
		{
			name:          "missing map key in string value - runtime error",
			template:      `value: "${data.missingKey}"`,
			inputs:        `{"data": {"existingKey": "value"}}`,
			wantErr:       true,
			errContains:   "no such key",
			wantIsMissing: true,
		},
		{
			name:          "undeclared variable in string value - compile error",
			template:      `value: "${undeclaredVariable}"`,
			inputs:        `{}`,
			wantErr:       true,
			errContains:   "undeclared reference",
			wantIsMissing: true,
		},
		{
			name:          "type error in string value - not missing data",
			template:      `value: "${1 + 'string'}"`,
			inputs:        `{}`,
			wantErr:       true,
			errContains:   "CEL",
			wantIsMissing: false,
		},
		{
			name:        "valid string value expression",
			template:    `value: "${data.key}"`,
			inputs:      `{"data": {"key": "test"}}`,
			wantErr:     false,
			errContains: "",
		},
		// Map key errors
		{
			name: "invalid CEL expression in map key",
			template: `
"${invalid syntax}": value
`,
			inputs:      `{}`,
			wantErr:     true,
			errContains: "CEL compilation error",
		},
		{
			name: "missing data in map key expression",
			template: `
"${metadata.missingField}": value
`,
			inputs:        `{"metadata": {"name": "test"}}`,
			wantErr:       true,
			errContains:   "no such key",
			wantIsMissing: true,
		},
		{
			name: "nested map with missing data in key expression",
			template: `
      outer:
        "${metadata.nonexistent}": value
      `,
			inputs:        `{"metadata": {"name": "test"}}`,
			wantErr:       true,
			errContains:   "no such key",
			wantIsMissing: true,
		},
		{
			name: "undeclared variable in map key",
			template: `
"${undeclaredVar}": value
`,
			inputs:        `{}`,
			wantErr:       true,
			errContains:   "undeclared reference",
			wantIsMissing: true,
		},
		{
			name: "valid dynamic map key",
			template: `
"${metadata.name}": value
`,
			inputs:  `{"metadata": {"name": "test-key"}}`,
			wantErr: false,
		},
		{
			name: "dynamic map key with string concat without type conversion",
			template: `
"${'port-' + metadata.port}": value
`,
			inputs:      `{"metadata": {"port": 8080}}`,
			wantErr:     true,
			errContains: "CEL",
		},
		{
			name: "dynamic map key evaluating to pure number",
			template: `
ports:
  ${metadata.port}: http
`,
			inputs:      `{"metadata": {"port": 8080}}`,
			wantErr:     true,
			errContains: "must evaluate to a string",
		},
		{
			name: "dynamic map key evaluating to boolean",
			template: `
flags:
  ${metadata.enabled}: active
`,
			inputs:      `{"metadata": {"enabled": true}}`,
			wantErr:     true,
			errContains: "must evaluate to a string",
		},
	}

	engine := NewEngine()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var tpl any
			if err := yaml.Unmarshal([]byte(tt.template), &tpl); err != nil {
				t.Fatalf("failed to unmarshal template: %v", err)
			}

			var input map[string]any
			if err := json.Unmarshal([]byte(tt.inputs), &input); err != nil {
				t.Fatalf("failed to unmarshal inputs: %v", err)
			}

			_, err := engine.Render(tpl, input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q but got none", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				if tt.wantIsMissing && !IsMissingDataError(err) {
					t.Errorf("IsMissingDataError() = false, want true for error: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestProgramCaching(t *testing.T) {
	t.Parallel()

	engine := NewEngine()

	// Test 1: Same expression evaluated multiple times should use cached program
	template := `
name: ${metadata.name}
env: ${environment}
replicas: ${replicas}
`
	inputs := map[string]any{
		"metadata":    map[string]any{"name": "test-app"},
		"environment": "production",
		"replicas":    int64(3),
	}

	var tpl any
	if err := yaml.Unmarshal([]byte(template), &tpl); err != nil {
		t.Fatalf("failed to unmarshal template: %v", err)
	}

	// First render - should compile and cache all expressions
	result1, err := engine.Render(tpl, inputs)
	if err != nil {
		t.Fatalf("first render failed: %v", err)
	}

	// Second render with same inputs - should hit cache for all expressions
	result2, err := engine.Render(tpl, inputs)
	if err != nil {
		t.Fatalf("second render failed: %v", err)
	}

	// Results should be identical
	yaml1, _ := yaml.Marshal(result1)
	yaml2, _ := yaml.Marshal(result2)
	if string(yaml1) != string(yaml2) {
		t.Errorf("cached render produced different result:\nfirst:\n%s\nsecond:\n%s", yaml1, yaml2)
	}

	// Test 2: Same expression in different contexts (forEach simulation)
	forEachTemplate := `
item1: ${metadata.name}-${item}
item2: ${metadata.name}-${item}
`
	var forEachTpl any
	if err := yaml.Unmarshal([]byte(forEachTemplate), &forEachTpl); err != nil {
		t.Fatalf("failed to unmarshal forEach template: %v", err)
	}

	// Simulate forEach iterations with different item values
	for i := 1; i <= 5; i++ {
		iterInputs := map[string]any{
			"metadata": map[string]any{"name": "test"},
			"item":     fmt.Sprintf("value-%d", i),
		}

		result, err := engine.Render(forEachTpl, iterInputs)
		if err != nil {
			t.Fatalf("forEach iteration %d failed: %v", i, err)
		}

		// Verify correct rendering
		resultMap := result.(map[string]any)
		expected := fmt.Sprintf("test-value-%d", i)
		if resultMap["item1"] != expected || resultMap["item2"] != expected {
			t.Errorf("iteration %d: expected %q, got item1=%q, item2=%q",
				i, expected, resultMap["item1"], resultMap["item2"])
		}
	}

	// Test 3: Different variable sets create separate cache entries
	template3 := `value: ${x + y}`
	var tpl3 any
	if err := yaml.Unmarshal([]byte(template3), &tpl3); err != nil {
		t.Fatalf("failed to unmarshal template3: %v", err)
	}

	inputs3a := map[string]any{"x": int64(10), "y": int64(20)}
	inputs3b := map[string]any{"x": int64(5), "y": int64(15), "z": int64(100)} // Different var set

	result3a, err := engine.Render(tpl3, inputs3a)
	if err != nil {
		t.Fatalf("render with inputs3a failed: %v", err)
	}

	result3b, err := engine.Render(tpl3, inputs3b)
	if err != nil {
		t.Fatalf("render with inputs3b failed: %v", err)
	}

	// Both should produce correct results
	if result3a.(map[string]any)["value"] != int64(30) {
		t.Errorf("inputs3a: expected 30, got %v", result3a.(map[string]any)["value"])
	}
	if result3b.(map[string]any)["value"] != int64(20) {
		t.Errorf("inputs3b: expected 20, got %v", result3b.(map[string]any)["value"])
	}

	// Test 4: Verify cache actually has entries
	cacheSize := engine.cache.ProgramCacheSize()

	if cacheSize == 0 {
		t.Error("program cache is empty - caching not working")
	}

	t.Logf("Program cache contains %d entries", cacheSize)
}
