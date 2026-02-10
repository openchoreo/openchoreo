// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
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
			name: "string literal braces inside expression",
			template: `
literal: ${"{"}
`,
			inputs: `{}`,
			want: `literal: "{"
`,
		},
		{
			name: "string literal closed braces inside expression",
			template: `
literal: ${"{metadata.name}=" + metadata.name}
`,
			inputs: `{
  "metadata": {"name": "checkout"}
			}`,
			want: `literal: '{metadata.name}=checkout'
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
  base: '${oc_merge({"team": "platform"}, metadata.labels)}'
  optional: '${has(spec.flag) && spec.flag ? {"enabled": "true"} : oc_omit()}'
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
			name: "variadic oc_merge with three maps",
			template: `
config: '${oc_merge({"a": 1, "b": 2}, {"b": 20, "c": 30}, {"c": 300, "d": 400})}'
`,
			inputs: `{}`,
			want: `config:
  a: 1
  b: 20
  c: 300
  d: 400
`,
		},
		{
			name: "variadic oc_merge with four maps - layer overriding",
			template: `
labels: '${oc_merge(
  {"app": "default", "env": "dev", "version": "v1"},
  metadata.labels,
  parameters.labels,
  {"final": "true"}
)}'
`,
			inputs: `{
  "metadata": {"labels": {"env": "staging", "team": "platform"}},
  "parameters": {"labels": {"version": "v2", "canary": "true"}}
}`,
			want: `labels:
  app: default
  canary: "true"
  env: staging
  final: "true"
  team: platform
  version: v2
`,
		},
		{
			name: "oc_omit() inside map literals with conditional fields",
			template: `
config: |
  ${parameters.sizeLimit != "" || parameters.medium != "" ? {
    "sizeLimit": parameters.sizeLimit != "" ? parameters.sizeLimit : oc_omit(),
    "medium": parameters.medium != "" ? parameters.medium : oc_omit()
  } : {}}
`,
			inputs: `{
  "parameters": {
    "sizeLimit": "1Gi",
    "medium": ""
  }
}`,
			want: `config:
  sizeLimit: 1Gi
`,
		},
		{
			name: "oc_omit() inside map literals removes all omitted fields",
			template: `
volumes:
  - name: cache
    emptyDir: |
      ${parameters.cache.sizeLimit != "" || parameters.cache.medium != "" ? {
        "sizeLimit": parameters.cache.sizeLimit != "" ? parameters.cache.sizeLimit : oc_omit(),
        "medium": parameters.cache.medium != "" ? parameters.cache.medium : oc_omit()
      } : {}}
  - name: workspace
    emptyDir: |
      ${parameters.workspace.sizeLimit != "" || parameters.workspace.medium != "" ? {
        "sizeLimit": parameters.workspace.sizeLimit != "" ? parameters.workspace.sizeLimit : oc_omit(),
        "medium": parameters.workspace.medium != "" ? parameters.workspace.medium : oc_omit()
      } : {}}
`,
			inputs: `{
  "parameters": {
    "cache": {
      "sizeLimit": "",
      "medium": ""
    },
    "workspace": {
      "sizeLimit": "5Gi",
      "medium": "Memory"
    }
  }
}`,
			want: `volumes:
- emptyDir: {}
  name: cache
- emptyDir:
    medium: Memory
    sizeLimit: 5Gi
  name: workspace
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
			name: "oc_generate_name with single argument",
			template: `
name: ${oc_generate_name("Hello World!")}
`,
			inputs: `{}`,
			want: `name: hello-world-7f83b165
`,
		},
		{
			name: "oc_generate_name with multiple arguments",
			template: `
name: ${oc_generate_name("my-app", "v1.2.3")}
`,
			inputs: `{}`,
			want: `name: my-app-v1.2.3-4f878dd8
`,
		},
		{
			name: "oc_generate_name with many arguments",
			template: `
name: ${oc_generate_name("front", "-", "end", "-", "prod", "-", "us-west", "-", "99")}
`,
			inputs: `{}`,
			want: `name: front--end--prod--us-west--99-d5cf2aae
`,
		},
		{
			name: "oc_generate_name with dynamic values",
			template: `
name: ${oc_generate_name(metadata.name, "-", spec.version)}
`,
			inputs: `{
  "metadata": {"name": "payment-service"},
  "spec": {"version": "v2.0"}
}`,
			want: `name: payment-service--v2.0-38fcb255
`,
		},
		{
			name: "list transformation with map comprehension",
			template: `
env: '${parameters.envVars.map(e, {"name": e.key, "value": e.value})}'
`,
			inputs: `{
  "parameters": {
    "envVars": [
      {"key": "PORT", "value": "8080"},
      {"key": "HOST", "value": "0.0.0.0"},
      {"key": "DEBUG", "value": "true"}
    ]
  }
}`,
			want: `env:
- name: PORT
  value: "8080"
- name: HOST
  value: 0.0.0.0
- name: DEBUG
  value: "true"
`,
		},
		{
			name: "transformMapEntry to create map with dynamic keys from list",
			template: `
envMap: '${parameters.envVars.transformMapEntry(_, v, {v.name: v.value})}'
`,
			inputs: `{
  "parameters": {
    "envVars": [
      {"name": "PORT", "value": "8080"},
      {"name": "HOST", "value": "0.0.0.0"},
      {"name": "DEBUG", "value": "true"}
    ]
  }
}`,
			want: `envMap:
  DEBUG: "true"
  HOST: 0.0.0.0
  PORT: "8080"
`,
		},
		{
			name: "list concatenation with + operator",
			template: `
items: '${parameters.defaults + parameters.custom}'
`,
			inputs: `{
  "parameters": {
    "defaults": ["item1", "item2"],
    "custom": ["item3", "item4"]
  }
}`,
			want: `items:
- item1
- item2
- item3
- item4
`,
		},
		{
			name: "flatten nested lists",
			template: `
items: '${[[1, 2], [3, 4], [5]].flatten()}'
`,
			inputs: `{}`,
			want: `items:
- 1
- 2
- 3
- 4
- 5
`,
		},
		{
			name: "flatten with transformList",
			template: `
files: |
  ${containers.transformList(name, c,
    has(c.files) ? c.files.map(f, {
      "container": name,
      "name": f.name
    }) : []
  ).flatten()}
`,
			inputs: `{
  "containers": {
    "app": {
      "files": [
        {"name": "config.yaml"},
        {"name": "secrets.yaml"}
      ]
    }
  }
}`,
			want: `files:
- container: app
  name: config.yaml
- container: app
  name: secrets.yaml
`,
		},
		{
			name: "optional types with safe navigation",
			template: `
metadata:
  annotations: '${{"app": metadata.name, ?"custom": spec.?annotations.?custom}}'
`,
			inputs: `{
  "metadata": {"name": "my-app"},
  "spec": {}
}`,
			want: `metadata:
  annotations:
    app: my-app
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
			name: "oc_omit() in list - removes omitted elements",
			template: `
items: |
  ${[
    "first",
    parameters.includeSecond ? "second" : oc_omit(),
    "third",
    parameters.includeFourth ? "fourth" : oc_omit()
  ]}
`,
			inputs: `{
  "parameters": {
    "includeSecond": false,
    "includeFourth": true
  }
}`,
			want: `items:
- first
- third
- fourth
`,
		},
		{
			name: "oc_omit() in nested maps within list",
			template: `
resources: |
  ${[
    {
      "name": "cpu-request",
      "value": parameters.cpuRequest != "" ? parameters.cpuRequest : oc_omit()
    },
    {
      "name": "memory-request",
      "value": parameters.memRequest != "" ? parameters.memRequest : oc_omit()
    },
    {
      "name": "cpu-limit",
      "value": parameters.cpuLimit != "" ? parameters.cpuLimit : oc_omit()
    }
  ]}
`,
			inputs: `{
  "parameters": {
    "cpuRequest": "100m",
    "memRequest": "",
    "cpuLimit": "500m"
  }
}`,
			want: `resources:
- name: cpu-request
  value: 100m
- name: memory-request
- name: cpu-limit
  value: 500m
`,
		},
		{
			name: "oc_omit() in deeply nested structures",
			template: `
config: |
  ${{
    "level1": {
      "level2": {
        "enabled": parameters.enabled,
        "setting1": parameters.setting1 != "" ? parameters.setting1 : oc_omit(),
        "setting2": parameters.setting2 != "" ? parameters.setting2 : oc_omit()
      },
      "optional": parameters.optionalEnabled ? {
        "value": parameters.optionalValue
      } : oc_omit()
    }
  }}
`,
			inputs: `{
  "parameters": {
    "enabled": true,
    "setting1": "value1",
    "setting2": "",
    "optionalEnabled": false,
    "optionalValue": "test"
  }
}`,
			want: `config:
  level1:
    level2:
      enabled: true
      setting1: value1
`,
		},
		{
			name: "oc_omit() in list of lists with nested structures",
			template: `
matrix: |
  ${[
    [
      "always-present",
      parameters.includeOptional1 ? "optional1" : oc_omit()
    ],
    parameters.includeRow2 ? [
      "row2-item1",
      "row2-item2"
    ] : oc_omit(),
    [
      parameters.includeOptional2 ? "optional2" : oc_omit(),
      "always-present-2"
    ]
  ]}
`,
			inputs: `{
  "parameters": {
    "includeOptional1": true,
    "includeRow2": false,
    "includeOptional2": false
  }
}`,
			want: `matrix:
- - always-present
  - optional1
- - always-present-2
`,
		},
		{
			name: "oc_omit() in map of maps with nested omissions",
			template: `
settings: |
  ${{
    "database": {
      "host": parameters.dbHost,
      "port": parameters.dbPort != 0 ? parameters.dbPort : oc_omit(),
      "ssl": parameters.dbSSL != "" ? {
        "enabled": true,
        "cert": parameters.dbSSL
      } : oc_omit()
    },
    "cache": parameters.cacheEnabled ? {
      "host": parameters.cacheHost,
      "ttl": parameters.cacheTTL != 0 ? parameters.cacheTTL : oc_omit()
    } : oc_omit()
  }}
`,
			inputs: `{
  "parameters": {
    "dbHost": "localhost",
    "dbPort": 0,
    "dbSSL": "",
    "cacheEnabled": true,
    "cacheHost": "redis",
    "cacheTTL": 0
  }
}`,
			want: `settings:
  cache:
    host: redis
  database:
    host: localhost
`,
		},
		{
			name: "oc_omit() with list comprehension and filtering",
			template: `
volumes: |
  ${[
    {"name": "config", "configMap": {"name": "app-config"}},
    parameters.tmpStorage ? {"name": "tmp", "emptyDir": {}} : oc_omit(),
    parameters.secretName != "" ? {"name": "secret", "secret": {"secretName": parameters.secretName}} : oc_omit()
  ]}
`,
			inputs: `{
  "parameters": {
    "tmpStorage": true,
    "secretName": ""
  }
}`,
			want: `volumes:
- configMap:
    name: app-config
  name: config
- emptyDir: {}
  name: tmp
`,
		},
		{
			name: "oc_omit() in mixed list with maps and primitives",
			template: `
mixed: |
  ${[
    "string-value",
    42,
    parameters.includeMap ? {"key": "value"} : oc_omit(),
    true,
    parameters.includeNumber ? 99 : oc_omit(),
    {"nested": parameters.includeNested ? "present" : oc_omit()}
  ]}
`,
			inputs: `{
  "parameters": {
    "includeMap": false,
    "includeNumber": true,
    "includeNested": false
  }
}`,
			want: `mixed:
- string-value
- 42
- true
- 99
- {}
`,
		},
		{
			name: "hash function",
			template: `
hash: ${oc_hash("hello world")}
dynamicHash: ${oc_hash(metadata.value)}
`,
			inputs: `{
  "metadata": {"value": "test data"}
}`,
			want: `hash: d58b3fa7
dynamicHash: 578fbe87
`,
		},
		{
			name: "base64 encode and decode",
			template: `
encoded: ${base64.encode(bytes(parameters.value))}
decoded: ${string(base64.decode(parameters.encoded))}
`,
			inputs: `{
  "parameters": {
    "value": "hello world",
    "encoded": "aGVsbG8gd29ybGQ="
  }
}`,
			want: `encoded: aGVsbG8gd29ybGQ=
decoded: hello world
`,
		},
		{
			name: "in operator for map key existence",
			template: `
endpointNameFound: ${parameters.endpointName in workload.endpoints}
missingNameFound: ${parameters.missingName in workload.endpoints}
`,
			inputs: `{
  "parameters": {
    "endpointName": "web",
    "missingName": "nonexistent"
  },
  "workload": {
    "endpoints": {
      "web": {"type": "HTTP", "port": 8080},
      "grpc": {"type": "gRPC", "port": 9090}
    }
  }
}`,
			want: `endpointNameFound: true
missingNameFound: false
`,
		},
		{
			name: "in operator for list membership",
			template: `
found: ${"a" in parameters.items}
missingItemFound: ${"z" in parameters.items}
`,
			inputs: `{
  "parameters": {
    "items": ["a", "b", "c"]
  }
}`,
			want: `found: true
missingItemFound: false
`,
		},
		{
			name: "exists macro on map iterates over keys",
			template: `
hasHTTP: ${workload.endpoints.exists(name, workload.endpoints[name].type == 'HTTP')}
hasUDP: ${workload.endpoints.exists(name, workload.endpoints[name].type == 'UDP')}
`,
			inputs: `{
  "workload": {
    "endpoints": {
      "web": {"type": "HTTP", "port": 8080},
      "grpc": {"type": "gRPC", "port": 9090}
    }
  }
}`,
			want: `hasHTTP: true
hasUDP: false
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
			name:        "nested expression without quoting",
			template:    `value: ${outer(${inner})}`,
			inputs:      `{"inner": "value"}`,
			wantErr:     true,
			errContains: "nested CEL expressions",
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
		// Variadic merge error cases
		{
			name:        "oc_merge with no arguments",
			template:    `value: ${oc_merge()}`,
			inputs:      `{}`,
			wantErr:     true,
			errContains: "oc_merge requires at least 2 arguments",
		},
		{
			name: "oc_merge with single argument",
			template: `
value: '${oc_merge({"a": 1})}'
`,
			inputs:      `{}`,
			wantErr:     true,
			errContains: "oc_merge requires at least 2 arguments",
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

func TestFindCELExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    []CELMatch
		wantErr bool
	}{
		{
			name:  "Simple expression",
			input: "${resource.field}",
			want: []CELMatch{
				{FullExpr: "${resource.field}", InnerExpr: "resource.field"},
			},
			wantErr: false,
		},
		{
			name:  "Expression with function",
			input: "${length(resource.list)}",
			want: []CELMatch{
				{FullExpr: "${length(resource.list)}", InnerExpr: "length(resource.list)"},
			},
			wantErr: false,
		},
		{
			name:  "Expression with prefix",
			input: "prefix-${resource.field}",
			want: []CELMatch{
				{FullExpr: "${resource.field}", InnerExpr: "resource.field"},
			},
			wantErr: false,
		},
		{
			name:  "Expression with suffix",
			input: "${resource.field}-suffix",
			want: []CELMatch{
				{FullExpr: "${resource.field}", InnerExpr: "resource.field"},
			},
			wantErr: false,
		},
		{
			name:  "Multiple expressions",
			input: "${resource1.field}-middle-${resource2.field}",
			want: []CELMatch{
				{FullExpr: "${resource1.field}", InnerExpr: "resource1.field"},
				{FullExpr: "${resource2.field}", InnerExpr: "resource2.field"},
			},
			wantErr: false,
		},
		{
			name:  "Expression with map access",
			input: "${resource.map['key']}",
			want: []CELMatch{
				{FullExpr: "${resource.map['key']}", InnerExpr: "resource.map['key']"},
			},
			wantErr: false,
		},
		{
			name:  "Expression with list index",
			input: "${resource.list[0]}",
			want: []CELMatch{
				{FullExpr: "${resource.list[0]}", InnerExpr: "resource.list[0]"},
			},
			wantErr: false,
		},
		{
			name:  "Complex expression with operators",
			input: "${resource.field == 'value' && resource.number > 5}",
			want: []CELMatch{
				{FullExpr: "${resource.field == 'value' && resource.number > 5}", InnerExpr: "resource.field == 'value' && resource.number > 5"},
			},
			wantErr: false,
		},
		{
			name:    "No expressions",
			input:   "plain string",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "Empty string",
			input:   "",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "Incomplete expression - no closing brace",
			input:   "${incomplete",
			want:    nil,
			wantErr: false,
		},
		{
			name:  "Expression with escaped quotes",
			input: `${resource.field == "escaped\"quote"}`,
			want: []CELMatch{
				{FullExpr: `${resource.field == "escaped\"quote"}`, InnerExpr: `resource.field == "escaped\"quote"`},
			},
			wantErr: false,
		},
		{
			name:  "Multiple expressions with whitespace",
			input: "  ${resource1.field}  ${resource2.field}  ",
			want: []CELMatch{
				{FullExpr: "${resource1.field}", InnerExpr: "resource1.field"},
				{FullExpr: "${resource2.field}", InnerExpr: "resource2.field"},
			},
			wantErr: false,
		},
		{
			name:  "Expression with newlines",
			input: "${resource.list.map(\n  x,\n  x * 2\n)}",
			want: []CELMatch{
				{FullExpr: "${resource.list.map(\n  x,\n  x * 2\n)}", InnerExpr: "resource.list.map(\n  x,\n  x * 2\n)"},
			},
			wantErr: false,
		},
		{
			name:    "Nested expression without quotes - should error",
			input:   "${outer(${inner})}",
			want:    nil,
			wantErr: true,
		},
		{
			name:  "Expression with nested-like string literal in double quotes",
			input: `${outer("${inner}")}`,
			want: []CELMatch{
				{FullExpr: `${outer("${inner}")}`, InnerExpr: `outer("${inner}")`},
			},
			wantErr: false,
		},
		{
			name:  "Expression with nested-like string literal in single quotes",
			input: "${outer('${inner}')}",
			want: []CELMatch{
				{FullExpr: "${outer('${inner}')}", InnerExpr: "outer('${inner}')"},
			},
			wantErr: false,
		},
		{
			name:  "String literal with closing braces inside double quotes",
			input: `${"text with }} inside"}`,
			want: []CELMatch{
				{FullExpr: `${"text with }} inside"}`, InnerExpr: `"text with }} inside"`},
			},
			wantErr: false,
		},
		{
			name:  "String literal with opening brace inside double quotes",
			input: `${"text with { inside"}`,
			want: []CELMatch{
				{FullExpr: `${"text with { inside"}`, InnerExpr: `"text with { inside"`},
			},
			wantErr: false,
		},
		{
			name:  "String literal with opening brace inside single quotes",
			input: "${'text with { inside'}",
			want: []CELMatch{
				{FullExpr: "${'text with { inside'}", InnerExpr: "'text with { inside'"},
			},
			wantErr: false,
		},
		{
			name:  "Expression with dictionary building",
			input: "${true ? {'key': 'value'} : {'key': 'value2'}}",
			want: []CELMatch{
				{FullExpr: "${true ? {'key': 'value'} : {'key': 'value2'}}", InnerExpr: "true ? {'key': 'value'} : {'key': 'value2'}"},
			},
			wantErr: false,
		},
		{
			name:  "Multiple expressions with dictionary building",
			input: "${true ? {'key': 'value'} : {'key': 'value2'}} somewhat ${resource.field} then ${false ? {'key': {'nestedKey':'value'}} : {'key': 'value2'}}",
			want: []CELMatch{
				{FullExpr: "${true ? {'key': 'value'} : {'key': 'value2'}}", InnerExpr: "true ? {'key': 'value'} : {'key': 'value2'}"},
				{FullExpr: "${resource.field}", InnerExpr: "resource.field"},
				{FullExpr: "${false ? {'key': {'nestedKey':'value'}} : {'key': 'value2'}}", InnerExpr: "false ? {'key': {'nestedKey':'value'}} : {'key': 'value2'}"},
			},
			wantErr: false,
		},
		{
			name:    "Multiple incomplete expressions - nested at start",
			input:   "${incomplete1 ${incomplete2",
			want:    nil,
			wantErr: true,
		},
		{
			name:  "Mixed complete and incomplete - incomplete at end",
			input: "${complete} ${complete2} ${incomplete",
			want: []CELMatch{
				{FullExpr: "${complete}", InnerExpr: "complete"},
				{FullExpr: "${complete2}", InnerExpr: "complete2"},
			},
			wantErr: false,
		},
		{
			name:    "Mixed incomplete and complete - nested incomplete",
			input:   "${incomplete ${complete}",
			want:    nil,
			wantErr: true,
		},
		{
			name:  "String literal with just opening brace - the fix case",
			input: `${"{"}`,
			want: []CELMatch{
				{FullExpr: `${"{"}`, InnerExpr: `"{"`},
			},
			wantErr: false,
		},
		{
			name:  "String literal with closing brace",
			input: `${"}"}`,
			want: []CELMatch{
				{FullExpr: `${"}"}`, InnerExpr: `"}"`},
			},
			wantErr: false,
		},
		{
			name:  "String literal braces with concatenation",
			input: `${"{metadata.name}=" + metadata.name}`,
			want: []CELMatch{
				{FullExpr: `${"{metadata.name}=" + metadata.name}`, InnerExpr: `"{metadata.name}=" + metadata.name`},
			},
			wantErr: false,
		},
		{
			name:  "Merge function with nested braces",
			input: "${merge({a: 1}, {b: 2})}",
			want: []CELMatch{
				{FullExpr: "${merge({a: 1}, {b: 2})}", InnerExpr: "merge({a: 1}, {b: 2})"},
			},
			wantErr: false,
		},
		{
			name:  "Complex nested braces with strings",
			input: `${data.map(x, {"key": x, "template": "value-{}" + x})}`,
			want: []CELMatch{
				{FullExpr: `${data.map(x, {"key": x, "template": "value-{}" + x})}`, InnerExpr: `data.map(x, {"key": x, "template": "value-{}" + x})`},
			},
			wantErr: false,
		},
		{
			name:  "Escaped backslash before quote",
			input: `${field == "test\\\"value"}`,
			want: []CELMatch{
				{FullExpr: `${field == "test\\\"value"}`, InnerExpr: `field == "test\\\"value"`},
			},
			wantErr: false,
		},
		{
			name:  "Single quote inside double quote",
			input: `${field == "test'value"}`,
			want: []CELMatch{
				{FullExpr: `${field == "test'value"}`, InnerExpr: `field == "test'value"`},
			},
			wantErr: false,
		},
		{
			name:  "Double quote inside single quote",
			input: `${field == 'test"value'}`,
			want: []CELMatch{
				{FullExpr: `${field == 'test"value'}`, InnerExpr: `field == 'test"value'`},
			},
			wantErr: false,
		},
		{
			name:  "Escaped single quote in single quote string",
			input: `${field == 'test\'value'}`,
			want: []CELMatch{
				{FullExpr: `${field == 'test\'value'}`, InnerExpr: `field == 'test\'value'`},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := FindCELExpressions(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("FindCELExpressions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("FindCELExpressions() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
