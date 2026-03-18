// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			name: "oc_dns_label with single argument",
			template: `
name: ${oc_dns_label("Hello World!")}
`,
			inputs: `{}`,
			want: `name: hello-world-7f83b165
`,
		},
		{
			name: "oc_dns_label with multiple arguments",
			template: `
name: ${oc_dns_label("my-app", "v1.2.3")}
`,
			inputs: `{}`,
			want: `name: my-app-v1.2.3-4f878dd8
`,
		},
		{
			name: "oc_dns_label truncates to 63 characters",
			template: `
name: ${oc_dns_label("very-long-endpoint-name", "very-long-component-name", "very-long-env-name")}
`,
			inputs: `{}`,
			want: `name: very-long-endpoint-very-long-compone-very-long-env-nam-23ab5d45
`,
		},
		{
			name: "oc_dns_label with list argument",
			template: `
name: ${oc_dns_label(["http", "my-service", "prod", "default"])}
`,
			inputs: `{}`,
			want: `name: http-my-service-prod-default-800d0dd2
`,
		},
		{
			name: "oc_dns_label with dynamic values",
			template: `
name: ${oc_dns_label(metadata.endpointName, metadata.componentName, spec.environment)}
`,
			inputs: `{
  "metadata": {"endpointName": "http", "componentName": "payment-service"},
  "spec": {"environment": "prod"}
}`,
			want: `name: http-payment-service-prod-779ef0c2
`,
		},
		{
			name: "oc_dns_label with special characters",
			template: `
name: ${oc_dns_label("My App!")}
`,
			inputs: `{}`,
			want: `name: my-app-6e06c02a
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
	}

	engine := NewEngine()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var tpl any
			require.NoError(t, yaml.Unmarshal([]byte(tt.template), &tpl))

			var input map[string]any
			require.NoError(t, json.Unmarshal([]byte(tt.inputs), &input))

			rendered, err := engine.Render(tpl, input)
			require.NoError(t, err)

			cleaned := RemoveOmittedFields(rendered)

			got, err := yaml.Marshal(cleaned)
			require.NoError(t, err)

			require.NoError(t, compareYAML(tt.want, string(got)))
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
		name        string
		template    string
		inputs      string
		wantErr     bool
		errContains string
	}{
		{
			name:        "missing map key in string value - runtime error",
			template:    `value: "${data.missingKey}"`,
			inputs:      `{"data": {"existingKey": "value"}}`,
			wantErr:     true,
			errContains: "no such key",
		},
		{
			name:        "nested expression without quoting",
			template:    `value: ${outer(${inner})}`,
			inputs:      `{"inner": "value"}`,
			wantErr:     true,
			errContains: "nested CEL expressions",
		},
		{
			name:        "undeclared variable in string value - compile error",
			template:    `value: "${undeclaredVariable}"`,
			inputs:      `{}`,
			wantErr:     true,
			errContains: "undeclared reference",
		},
		{
			name:        "type error in string value - not missing data",
			template:    `value: "${1 + 'string'}"`,
			inputs:      `{}`,
			wantErr:     true,
			errContains: "CEL",
		},
		{
			name:     "valid string value expression",
			template: `value: "${data.key}"`,
			inputs:   `{"data": {"key": "test"}}`,
			wantErr:  false,
		},
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
			inputs:      `{"metadata": {"name": "test"}}`,
			wantErr:     true,
			errContains: "no such key",
		},
		{
			name: "nested map with missing data in key expression",
			template: `
      outer:
        "${metadata.nonexistent}": value
      `,
			inputs:      `{"metadata": {"name": "test"}}`,
			wantErr:     true,
			errContains: "no such key",
		},
		{
			name: "undeclared variable in map key",
			template: `
"${undeclaredVar}": value
`,
			inputs:      `{}`,
			wantErr:     true,
			errContains: "undeclared reference",
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
			require.NoError(t, yaml.Unmarshal([]byte(tt.template), &tpl))

			var input map[string]any
			require.NoError(t, json.Unmarshal([]byte(tt.inputs), &input))

			_, err := engine.Render(tpl, input)

			if tt.wantErr {
				require.Error(t, err, "expected error containing %q", tt.errContains)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
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
		},
		{
			name:  "Expression with function",
			input: "${length(resource.list)}",
			want: []CELMatch{
				{FullExpr: "${length(resource.list)}", InnerExpr: "length(resource.list)"},
			},
		},
		{
			name:  "Expression with prefix",
			input: "prefix-${resource.field}",
			want: []CELMatch{
				{FullExpr: "${resource.field}", InnerExpr: "resource.field"},
			},
		},
		{
			name:  "Expression with suffix",
			input: "${resource.field}-suffix",
			want: []CELMatch{
				{FullExpr: "${resource.field}", InnerExpr: "resource.field"},
			},
		},
		{
			name:  "Multiple expressions",
			input: "${resource1.field}-middle-${resource2.field}",
			want: []CELMatch{
				{FullExpr: "${resource1.field}", InnerExpr: "resource1.field"},
				{FullExpr: "${resource2.field}", InnerExpr: "resource2.field"},
			},
		},
		{
			name:  "Expression with map access",
			input: "${resource.map['key']}",
			want: []CELMatch{
				{FullExpr: "${resource.map['key']}", InnerExpr: "resource.map['key']"},
			},
		},
		{
			name:  "Expression with list index",
			input: "${resource.list[0]}",
			want: []CELMatch{
				{FullExpr: "${resource.list[0]}", InnerExpr: "resource.list[0]"},
			},
		},
		{
			name:  "Complex expression with operators",
			input: "${resource.field == 'value' && resource.number > 5}",
			want: []CELMatch{
				{FullExpr: "${resource.field == 'value' && resource.number > 5}", InnerExpr: "resource.field == 'value' && resource.number > 5"},
			},
		},
		{
			name:  "No expressions",
			input: "plain string",
			want:  nil,
		},
		{
			name:  "Empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "Incomplete expression - no closing brace",
			input: "${incomplete",
			want:  nil,
		},
		{
			name:  "Expression with escaped quotes",
			input: `${resource.field == "escaped\"quote"}`,
			want: []CELMatch{
				{FullExpr: `${resource.field == "escaped\"quote"}`, InnerExpr: `resource.field == "escaped\"quote"`},
			},
		},
		{
			name:  "Multiple expressions with whitespace",
			input: "  ${resource1.field}  ${resource2.field}  ",
			want: []CELMatch{
				{FullExpr: "${resource1.field}", InnerExpr: "resource1.field"},
				{FullExpr: "${resource2.field}", InnerExpr: "resource2.field"},
			},
		},
		{
			name:  "Expression with newlines",
			input: "${resource.list.map(\n  x,\n  x * 2\n)}",
			want: []CELMatch{
				{FullExpr: "${resource.list.map(\n  x,\n  x * 2\n)}", InnerExpr: "resource.list.map(\n  x,\n  x * 2\n)"},
			},
		},
		{
			name:    "Nested expression without quotes - should error",
			input:   "${outer(${inner})}",
			wantErr: true,
		},
		{
			name:  "Expression with nested-like string literal in double quotes",
			input: `${outer("${inner}")}`,
			want: []CELMatch{
				{FullExpr: `${outer("${inner}")}`, InnerExpr: `outer("${inner}")`},
			},
		},
		{
			name:  "Expression with nested-like string literal in single quotes",
			input: "${outer('${inner}')}",
			want: []CELMatch{
				{FullExpr: "${outer('${inner}')}", InnerExpr: "outer('${inner}')"},
			},
		},
		{
			name:  "String literal with closing braces inside double quotes",
			input: `${"text with }} inside"}`,
			want: []CELMatch{
				{FullExpr: `${"text with }} inside"}`, InnerExpr: `"text with }} inside"`},
			},
		},
		{
			name:  "String literal with opening brace inside double quotes",
			input: `${"text with { inside"}`,
			want: []CELMatch{
				{FullExpr: `${"text with { inside"}`, InnerExpr: `"text with { inside"`},
			},
		},
		{
			name:  "String literal with opening brace inside single quotes",
			input: "${'text with { inside'}",
			want: []CELMatch{
				{FullExpr: "${'text with { inside'}", InnerExpr: "'text with { inside'"},
			},
		},
		{
			name:  "Expression with dictionary building",
			input: "${true ? {'key': 'value'} : {'key': 'value2'}}",
			want: []CELMatch{
				{FullExpr: "${true ? {'key': 'value'} : {'key': 'value2'}}", InnerExpr: "true ? {'key': 'value'} : {'key': 'value2'}"},
			},
		},
		{
			name:  "Multiple expressions with dictionary building",
			input: "${true ? {'key': 'value'} : {'key': 'value2'}} somewhat ${resource.field} then ${false ? {'key': {'nestedKey':'value'}} : {'key': 'value2'}}",
			want: []CELMatch{
				{FullExpr: "${true ? {'key': 'value'} : {'key': 'value2'}}", InnerExpr: "true ? {'key': 'value'} : {'key': 'value2'}"},
				{FullExpr: "${resource.field}", InnerExpr: "resource.field"},
				{FullExpr: "${false ? {'key': {'nestedKey':'value'}} : {'key': 'value2'}}", InnerExpr: "false ? {'key': {'nestedKey':'value'}} : {'key': 'value2'}"},
			},
		},
		{
			name:    "Multiple incomplete expressions - nested at start",
			input:   "${incomplete1 ${incomplete2",
			wantErr: true,
		},
		{
			name:  "Mixed complete and incomplete - incomplete at end",
			input: "${complete} ${complete2} ${incomplete",
			want: []CELMatch{
				{FullExpr: "${complete}", InnerExpr: "complete"},
				{FullExpr: "${complete2}", InnerExpr: "complete2"},
			},
		},
		{
			name:    "Mixed incomplete and complete - nested incomplete",
			input:   "${incomplete ${complete}",
			wantErr: true,
		},
		{
			name:  "String literal with just opening brace - the fix case",
			input: `${"{"}`,
			want: []CELMatch{
				{FullExpr: `${"{"}`, InnerExpr: `"{"`},
			},
		},
		{
			name:  "String literal with closing brace",
			input: `${"}"}`,
			want: []CELMatch{
				{FullExpr: `${"}"}`, InnerExpr: `"}"`},
			},
		},
		{
			name:  "String literal braces with concatenation",
			input: `${"{metadata.name}=" + metadata.name}`,
			want: []CELMatch{
				{FullExpr: `${"{metadata.name}=" + metadata.name}`, InnerExpr: `"{metadata.name}=" + metadata.name`},
			},
		},
		{
			name:  "Merge function with nested braces",
			input: "${merge({a: 1}, {b: 2})}",
			want: []CELMatch{
				{FullExpr: "${merge({a: 1}, {b: 2})}", InnerExpr: "merge({a: 1}, {b: 2})"},
			},
		},
		{
			name:  "Complex nested braces with strings",
			input: `${data.map(x, {"key": x, "template": "value-{}" + x})}`,
			want: []CELMatch{
				{FullExpr: `${data.map(x, {"key": x, "template": "value-{}" + x})}`, InnerExpr: `data.map(x, {"key": x, "template": "value-{}" + x})`},
			},
		},
		{
			name:  "Escaped backslash before quote",
			input: `${field == "test\\\"value"}`,
			want: []CELMatch{
				{FullExpr: `${field == "test\\\"value"}`, InnerExpr: `field == "test\\\"value"`},
			},
		},
		{
			name:  "Single quote inside double quote",
			input: `${field == "test'value"}`,
			want: []CELMatch{
				{FullExpr: `${field == "test'value"}`, InnerExpr: `field == "test'value"`},
			},
		},
		{
			name:  "Double quote inside single quote",
			input: `${field == 'test"value'}`,
			want: []CELMatch{
				{FullExpr: `${field == 'test"value'}`, InnerExpr: `field == 'test"value'`},
			},
		},
		{
			name:  "Escaped single quote in single quote string",
			input: `${field == 'test\'value'}`,
			want: []CELMatch{
				{FullExpr: `${field == 'test\'value'}`, InnerExpr: `field == 'test\'value'`},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := FindCELExpressions(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("FindCELExpressions() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRenderString_InterpolationTypes(t *testing.T) {
	t.Parallel()

	engine := NewEngine()

	tests := []struct {
		name   string
		tpl    string
		inputs string
		want   string
	}{
		{
			name:   "int64 interpolation",
			tpl:    `msg: "count: ${spec.count}"`,
			inputs: `{"spec": {"count": 42}}`,
			want:   `msg: "count: 42"`,
		},
		{
			name:   "float64 interpolation",
			tpl:    `msg: "ratio: ${spec.ratio}"`,
			inputs: `{"spec": {"ratio": 3.14}}`,
			want:   `msg: "ratio: 3.14"`,
		},
		{
			name:   "bool interpolation",
			tpl:    `msg: "enabled: ${spec.enabled}"`,
			inputs: `{"spec": {"enabled": true}}`,
			want:   `msg: "enabled: true"`,
		},
		{
			name:   "complex type interpolation (map to JSON)",
			tpl:    `msg: "config: ${spec.config}"`,
			inputs: `{"spec": {"config": {"key": "value"}}}`,
			want:   `msg: 'config: {"key":"value"}'`,
		},
		{
			name:   "mixed types in single string",
			tpl:    `msg: "${spec.name}-${spec.count}-${spec.enabled}"`,
			inputs: `{"spec": {"name": "app", "count": 3, "enabled": false}}`,
			want:   `msg: app-3-false`,
		},
		{
			name:   "repeated placeholder",
			tpl:    `msg: "${x}-${x}"`,
			inputs: `{"x": "hello"}`,
			want:   `msg: hello-hello`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var tpl any
			require.NoError(t, yaml.Unmarshal([]byte(tt.tpl), &tpl))

			var input map[string]any
			require.NoError(t, json.Unmarshal([]byte(tt.inputs), &input))

			rendered, err := engine.Render(tpl, input)
			require.NoError(t, err)

			got, err := yaml.Marshal(rendered)
			require.NoError(t, err)

			var wantObj, gotObj any
			require.NoError(t, yaml.Unmarshal([]byte(tt.want), &wantObj))
			require.NoError(t, yaml.Unmarshal(got, &gotObj))

			wantBytes, _ := yaml.Marshal(wantObj)
			gotBytes, _ := yaml.Marshal(gotObj)
			assert.Equal(t, string(wantBytes), string(gotBytes))
		})
	}
}

func TestRender_ErrorPropagation(t *testing.T) {
	t.Parallel()

	engine := NewEngine()

	tests := []struct {
		name        string
		tpl         string
		inputs      string
		errContains string
	}{
		{
			name:        "error in array item propagates",
			tpl:         `{"items": ["${data.missing}"]}`,
			inputs:      `{"data": {"existing": "val"}}`,
			errContains: "no such key",
		},
		{
			name:        "error in interpolation mode propagates",
			tpl:         `{"msg": "prefix-${data.missing}-suffix"}`,
			inputs:      `{"data": {"existing": "val"}}`,
			errContains: "no such key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var tpl any
			require.NoError(t, json.Unmarshal([]byte(tt.tpl), &tpl))

			var input map[string]any
			require.NoError(t, json.Unmarshal([]byte(tt.inputs), &input))

			_, err := engine.Render(tpl, input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestRender_OmitSentinelExclusion(t *testing.T) {
	t.Parallel()

	engine := NewEngine()

	t.Run("omit in map value", func(t *testing.T) {
		t.Parallel()

		tpl := map[string]any{
			"keep":   "value",
			"remove": "${oc_omit()}",
		}
		result, err := engine.Render(tpl, map[string]any{})
		require.NoError(t, err)

		resultMap, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "value", resultMap["keep"])
		_, hasRemove := resultMap["remove"]
		assert.False(t, hasRemove, "omitted key should not be present")
	})

	t.Run("omit in array element", func(t *testing.T) {
		t.Parallel()

		tpl := []any{"first", "${oc_omit()}", "third"}
		result, err := engine.Render(tpl, map[string]any{})
		require.NoError(t, err)

		resultSlice, ok := result.([]any)
		require.True(t, ok)
		assert.Equal(t, []any{"first", "third"}, resultSlice)
	})
}

func TestRender_DefaultPassthrough(t *testing.T) {
	t.Parallel()

	engine := NewEngine()

	result, err := engine.Render(int64(42), map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, int64(42), result)

	result, err = engine.Render(true, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, true, result)

	result, err = engine.Render(nil, map[string]any{})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestConcurrentRender(t *testing.T) {
	t.Parallel()

	engine := NewEngine()

	templateYAML := `
name: ${metadata.name}
replicas: ${spec.replicas}
`
	var tpl any
	require.NoError(t, yaml.Unmarshal([]byte(templateYAML), &tpl))

	const goroutines = 20
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			inputs := map[string]any{
				"metadata": map[string]any{"name": "app"},
				"spec":     map[string]any{"replicas": int64(idx)},
			}
			result, err := engine.Render(tpl, inputs)
			if err != nil {
				errs <- err
				return
			}
			resultMap, ok := result.(map[string]any)
			if !ok {
				errs <- fmt.Errorf("goroutine %d: expected map[string]any, got %T", idx, result)
				return
			}
			if resultMap["name"] != "app" {
				errs <- fmt.Errorf("goroutine %d: expected name=app, got %v", idx, resultMap["name"])
			}
			if resultMap["replicas"] != int64(idx) {
				errs <- fmt.Errorf("goroutine %d: expected replicas=%d, got %v", idx, idx, resultMap["replicas"])
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent render error: %v", err)
	}
}
