// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// TestCELExtensionWiring verifies that built-in CEL extensions are correctly
// wired up in BaseCELExtensions() and work end-to-end through the template
// engine. These tests exercise upstream cel-go features (not our custom code)
// and serve two purposes:
//   - Smoke tests: catch accidental removal of an extension from BaseCELExtensions()
//   - Documentation: showcase available CEL capabilities for template authors
func TestCELExtensionWiring(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		inputs   string
		want     string
	}{
		// --- ext.Lists() ---
		{
			name: "lists: map comprehension",
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
			name: "lists: filter selects matching elements",
			template: `
active: '${parameters.services.filter(s, s.enabled).map(s, s.name)}'
`,
			inputs: `{
  "parameters": {
    "services": [
      {"name": "web", "enabled": true},
      {"name": "debug", "enabled": false},
      {"name": "api", "enabled": true}
    ]
  }
}`,
			want: `active:
- web
- api
`,
		},
		{
			name:     "lists: filter on empty list returns empty",
			template: `result: '${parameters.items.filter(x, x > 0)}'`,
			inputs:   `{"parameters": {"items": []}}`,
			want:     `result: []`,
		},
		{
			name: "lists: sort on primitives",
			template: `
sorted: '${parameters.names.sort()}'
`,
			inputs: `{"parameters": {"names": ["charlie", "alice", "bob"]}}`,
			want: `sorted:
- alice
- bob
- charlie
`,
		},
		{
			name:     "lists: sort on empty list returns empty",
			template: `result: '${parameters.items.sort()}'`,
			inputs:   `{"parameters": {"items": []}}`,
			want:     `result: []`,
		},
		{
			name: "lists: sortBy on object field",
			template: `
sorted: '${parameters.items.sortBy(item, item.priority)}'
`,
			inputs: `{
  "parameters": {
    "items": [
      {"name": "low", "priority": 3},
      {"name": "high", "priority": 1},
      {"name": "mid", "priority": 2}
    ]
  }
}`,
			want: `sorted:
- name: high
  priority: 1
- name: mid
  priority: 2
- name: low
  priority: 3
`,
		},
		{
			name: "lists: flatten nested lists",
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
			name: "lists: flatten with depth limit",
			template: `
result: '${[[1, [2, 3]], [4]].flatten(1)}'
`,
			inputs: `{}`,
			want: `result:
- 1
- - 2
  - 3
- 4
`,
		},
		{
			name:     "lists: flatten on list of empty lists returns empty",
			template: `result: '${[[], []].flatten()}'`,
			inputs:   `{}`,
			want:     `result: []`,
		},

		// --- ext.Sets() ---
		{
			name: "sets: distinct removes duplicates",
			template: `
unique: '${parameters.items.distinct()}'
`,
			inputs: `{"parameters": {"items": ["a", "b", "a", "c", "b"]}}`,
			want: `unique:
- a
- b
- c
`,
		},

		// --- ext.TwoVarComprehensions() ---
		{
			name: "comprehensions: transformList iterates map to list",
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
			name: "comprehensions: transformMapEntry creates map from list",
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
			name: "comprehensions: transformMap transforms values",
			template: `
uppered: '${parameters.labels.transformMap(k, v, k.upperAscii() + "=" + v)}'
`,
			inputs: `{"parameters": {"labels": {"name": "web", "version": "v1"}}}`,
			want: `uppered:
  name: NAME=web
  version: VERSION=v1
`,
		},
		{
			name: "comprehensions: transformMap with filter condition",
			template: `
external: '${workload.endpoints.transformMap(name, ep, "external" in ep.visibility, ep)}'
`,
			inputs: `{
  "workload": {
    "endpoints": {
      "web": {"port": 8080, "visibility": ["external", "project"]},
      "grpc": {"port": 9090, "visibility": ["project"]},
      "admin": {"port": 9091, "visibility": ["external"]}
    }
  }
}`,
			want: `external:
  admin:
    port: 9091
    visibility:
    - external
  web:
    port: 8080
    visibility:
    - external
    - project
`,
		},
		{
			name: "comprehensions: two-var exists on map",
			template: `
hasExternal: ${workload.endpoints.exists(name, ep, "external" in ep.visibility)}
hasInternal: ${workload.endpoints.exists(name, ep, "internal" in ep.visibility)}
`,
			inputs: `{
  "workload": {
    "endpoints": {
      "web": {"visibility": ["external", "project"]},
      "grpc": {"visibility": ["project"]}
    }
  }
}`,
			want: `hasExternal: true
hasInternal: false
`,
		},

		// --- cel.OptionalTypes() ---
		{
			name: "optional: safe navigation with absent key",
			template: `
annotations: '${{"app": metadata.name, ?"custom": spec.?annotations.?custom}}'
`,
			inputs: `{
  "metadata": {"name": "my-app"},
  "spec": {}
}`,
			want: `annotations:
  app: my-app
`,
		},
		{
			name: "optional: safe navigation with present key",
			template: `
annotations: '${{"app": metadata.name, ?"custom": spec.?annotations.?custom}}'
`,
			inputs: `{
  "metadata": {"name": "my-app"},
  "spec": {"annotations": {"custom": "my-value"}}
}`,
			want: `annotations:
  app: my-app
  custom: my-value
`,
		},
		{
			name: "optional: hasValue and value extract present entries",
			template: `
hosts: |
  ${[parameters.?http, parameters.?https]
    .filter(g, g.hasValue())
    .map(g, g.value().host)}
`,
			inputs: `{
  "parameters": {
    "http": {"host": "example.com"},
    "https": {"host": "secure.example.com"}
  }
}`,
			want: `hosts:
- example.com
- secure.example.com
`,
		},
		{
			name: "optional: hasValue filters absent entries",
			template: `
hosts: |
  ${[parameters.?http, parameters.?https]
    .filter(g, g.hasValue())
    .map(g, g.value().host)}
`,
			inputs: `{
  "parameters": {
    "https": {"host": "secure.example.com"}
  }
}`,
			want: `hosts:
- secure.example.com
`,
		},
		{
			name: "optional: orValue provides default for absent key",
			template: `
path: ${parameters.?basePath.orValue("/")}
`,
			inputs: `{"parameters": {}}`,
			want: `path: /
`,
		},
		{
			name: "optional: orValue with integer default",
			template: `
port: ${parameters.?port.orValue(8080)}
`,
			inputs: `{"parameters": {}}`,
			want: `port: 8080
`,
		},

		// --- ext.Encoders() ---
		{
			name: "encoders: base64 encode",
			template: `
encoded: ${base64.encode(bytes(parameters.value))}
`,
			inputs: `{"parameters": {"value": "hello world"}}`,
			want: `encoded: aGVsbG8gd29ybGQ=
`,
		},
		{
			name: "encoders: base64 decode",
			template: `
decoded: ${string(base64.decode(parameters.encoded))}
`,
			inputs: `{"parameters": {"encoded": "aGVsbG8gd29ybGQ="}}`,
			want: `decoded: hello world
`,
		},

		// --- ext.Strings() ---
		{
			name: "strings: upperAscii",
			template: `
upper: ${parameters.name.upperAscii()}
`,
			inputs: `{"parameters": {"name": "hello"}}`,
			want: `upper: HELLO
`,
		},
		{
			name: "strings: trim",
			template: `
trimmed: ${parameters.padded.trim()}
`,
			inputs: `{"parameters": {"padded": "  spaced  "}}`,
			want: `trimmed: spaced
`,
		},
		{
			name: "strings: replace",
			template: `
replaced: ${parameters.text.replace("old", "new")}
`,
			inputs: `{"parameters": {"text": "the old value is old"}}`,
			want: `replaced: the new value is new
`,
		},
		{
			name: "strings: split",
			template: `
parts: '${parameters.path.split("/")}'
`,
			inputs: `{"parameters": {"path": "a/b/c"}}`,
			want: `parts:
- a
- b
- c
`,
		},
		{
			name: "strings: join",
			template: `
joined: ${parameters.items.join(",")}
`,
			inputs: `{"parameters": {"items": ["x", "y", "z"]}}`,
			want: `joined: x,y,z
`,
		},
		{
			name: "strings: substring",
			template: `
suffix: ${parameters.name.substring(6)}
middle: ${parameters.name.substring(0, 5)}
`,
			inputs: `{"parameters": {"name": "hello-world"}}`,
			want: `suffix: world
middle: hello
`,
		},
		{
			name: "strings: startsWith and endsWith",
			template: `
startsWithHTTPS: ${parameters.url.startsWith("https")}
endsWithSlash: ${parameters.url.endsWith("/")}
`,
			inputs: `{"parameters": {"url": "https://example.com/"}}`,
			want: `startsWithHTTPS: true
endsWithSlash: true
`,
		},

		// --- ext.Math() ---
		{
			name: "math: greatest",
			template: `
max: ${math.greatest([parameters.a, parameters.b, parameters.c])}
`,
			inputs: `{"parameters": {"a": 10, "b": 5, "c": 20}}`,
			want: `max: 20
`,
		},
		{
			name: "math: least",
			template: `
min: ${math.least([parameters.a, parameters.b, parameters.c])}
`,
			inputs: `{"parameters": {"a": 10, "b": 5, "c": 20}}`,
			want: `min: 5
`,
		},

		// --- Core CEL ---
		{
			name: "core: list concatenation with + operator",
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
			name: "core: in operator for map key existence",
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
			name: "core: in operator for list membership",
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
			name: "core: exists macro on map",
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
		{
			name: "core: all macro on map",
			template: `
allPositive: ${workload.endpoints.all(name, workload.endpoints[name].port > 0)}
allHTTP: ${workload.endpoints.all(name, workload.endpoints[name].type == 'HTTP')}
`,
			inputs: `{
  "workload": {
    "endpoints": {
      "web": {"type": "HTTP", "port": 8080},
      "grpc": {"type": "gRPC", "port": 9090}
    }
  }
}`,
			want: `allPositive: true
allHTTP: false
`,
		},
		{
			name: "core: size function on list and map",
			template: `
listSize: ${size(parameters.items)}
mapSize: ${size(parameters.config)}
`,
			inputs: `{
  "parameters": {
    "items": ["a", "b", "c"],
    "config": {"key1": "val1", "key2": "val2"}
  }
}`,
			want: `listSize: 3
mapSize: 2
`,
		},
		{
			name: "core: string type conversion",
			template: `
portStr: ${string(parameters.port)}
`,
			inputs: `{"parameters": {"port": 8080}}`,
			want: `portStr: "8080"
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

			var wantObj, gotObj any
			require.NoError(t, yaml.Unmarshal([]byte(tt.want), &wantObj))
			require.NoError(t, yaml.Unmarshal(got, &gotObj))

			wantBytes, _ := yaml.Marshal(wantObj)
			gotBytes, _ := yaml.Marshal(gotObj)
			assert.Equal(t, string(wantBytes), string(gotBytes))
		})
	}
}
