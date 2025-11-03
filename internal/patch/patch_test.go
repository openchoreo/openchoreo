// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package patch

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/yaml"
)

func TestApplyPatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		initial    string
		operations []JSONPatchOperation
		want       string
		wantErr    bool
	}{
		{
			name: "add env entry via array filter",
			initial: `
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: app
          image: app:v1
          env:
            - name: A
              value: "1"
`,
			operations: []JSONPatchOperation{
				{
					Op:   "add",
					Path: "/spec/template/spec/containers/[?(@.name=='app')]/env/-",
					Value: map[string]any{
						"name":  "B",
						"value": "2",
					},
				},
			},
			want: `
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: app
          image: app:v1
          env:
            - name: A
              value: "1"
            - name: B
              value: "2"
`,
		},
		{
			name: "replace image using index path",
			initial: `
spec:
  template:
    spec:
      containers:
        - name: app
          image: app:v1
`,
			operations: []JSONPatchOperation{
				{
					Op:    "replace",
					Path:  "/spec/template/spec/containers/0/image",
					Value: "app:v2",
				},
			},
			want: `
spec:
  template:
    spec:
      containers:
        - name: app
          image: app:v2
`,
		},
		{
			name: "remove first env entry",
			initial: `
spec:
  template:
    spec:
      containers:
        - name: app
          env:
            - name: A
              value: "1"
            - name: B
              value: "2"
`,
			operations: []JSONPatchOperation{
				{
					Op:   "remove",
					Path: "/spec/template/spec/containers/[?(@.name=='app')]/env/0",
				},
			},
			want: `
spec:
  template:
    spec:
      containers:
        - name: app
          env:
            - name: B
              value: "2"
`,
		},
		{
			name: "mergeShallow annotations without clobbering existing",
			initial: `
spec:
  template:
    metadata:
      annotations:
        existing: "true"
`,
			operations: []JSONPatchOperation{
				{
					Op:   "mergeShallow",
					Path: "/spec/template/metadata/annotations",
					Value: map[string]any{
						"platform": "enabled",
					},
				},
			},
			want: `
spec:
  template:
    metadata:
      annotations:
        existing: "true"
        platform: enabled
`,
		},
		{
			name: "mergeShallow replaces nested maps instead of deep merging",
			initial: `
spec:
  template:
    metadata:
      annotations:
        nested:
          keep: retained
        sibling: present
`,
			operations: []JSONPatchOperation{
				{
					Op:   "mergeShallow",
					Path: "/spec/template/metadata/annotations",
					Value: map[string]any{
						"nested": map[string]any{
							"added": "new",
						},
					},
				},
			},
			want: `
spec:
  template:
    metadata:
      annotations:
        nested:
          added: new
        sibling: present
`,
		},
		{
			name: "add env entry for multiple matches",
			initial: `
spec:
  template:
    spec:
      containers:
        - name: app
          role: worker
          env: []
        - name: logger
          role: worker
          env: []
`,
			operations: []JSONPatchOperation{
				{
					Op:   "add",
					Path: "/spec/template/spec/containers/[?(@.role=='worker')]/env/-",
					Value: map[string]any{
						"name":  "SHARED",
						"value": "true",
					},
				},
			},
			want: `
spec:
  template:
    spec:
      containers:
        - name: app
          role: worker
          env:
            - name: SHARED
              value: "true"
        - name: logger
          role: worker
          env:
            - name: SHARED
              value: "true"
`,
		},
		{
			name: "add to non-existent path creates parent",
			initial: `
spec:
  template:
    spec: {}
`,
			operations: []JSONPatchOperation{
				{
					Op:   "add",
					Path: "/spec/template/spec/containers/-",
					Value: map[string]any{
						"name":  "app",
						"image": "app:v1",
					},
				},
			},
			want: `
spec:
  template:
    spec:
      containers:
        - name: app
          image: app:v1
`,
		},
		{
			name: "array filter with no matches should error",
			initial: `
spec:
  containers:
    - name: app
      image: app:v1
`,
			operations: []JSONPatchOperation{
				{
					Op:    "replace",
					Path:  "/spec/containers/[?(@.name=='nonexistent')]/image",
					Value: "app:v2",
				},
			},
			wantErr: true,
		},
		{
			name: "add annotation with slash in key (RFC 6901 escape ~1)",
			initial: `
metadata:
  annotations:
    existing: "value"
`,
			operations: []JSONPatchOperation{
				{
					Op:    "add",
					Path:  "/metadata/annotations/app.kubernetes.io~1name",
					Value: "myapp",
				},
			},
			want: `
metadata:
  annotations:
    existing: "value"
    app.kubernetes.io/name: myapp
`,
		},
		{
			name: "replace annotation with slash in key (RFC 6901 escape ~1)",
			initial: `
metadata:
  annotations:
    app.kubernetes.io/name: "oldapp"
`,
			operations: []JSONPatchOperation{
				{
					Op:    "replace",
					Path:  "/metadata/annotations/app.kubernetes.io~1name",
					Value: "newapp",
				},
			},
			want: `
metadata:
  annotations:
    app.kubernetes.io/name: newapp
`,
		},
		{
			name: "remove annotation with slash in key (RFC 6901 escape ~1)",
			initial: `
metadata:
  annotations:
    app.kubernetes.io/name: "myapp"
    other: "value"
`,
			operations: []JSONPatchOperation{
				{
					Op:   "remove",
					Path: "/metadata/annotations/app.kubernetes.io~1name",
				},
			},
			want: `
metadata:
  annotations:
    other: "value"
`,
		},
		{
			name: "add annotation with tilde in key (RFC 6901 escape ~0)",
			initial: `
metadata:
  annotations: {}
`,
			operations: []JSONPatchOperation{
				{
					Op:    "add",
					Path:  "/metadata/annotations/special~0key",
					Value: "value",
				},
			},
			want: `
metadata:
  annotations:
    special~key: value
`,
		},
		{
			name: "add annotation with both tilde and slash (RFC 6901 escapes ~0 and ~1)",
			initial: `
metadata:
  annotations: {}
`,
			operations: []JSONPatchOperation{
				{
					Op:    "add",
					Path:  "/metadata/annotations/app~0test.io~1name",
					Value: "value",
				},
			},
			want: `
metadata:
  annotations:
    app~test.io/name: value
`,
		},
		{
			name: "filter with escaped slash in value (RFC 6901 escape ~1)",
			initial: `
spec:
  containers:
    - name: app
      url: "http://example.com"
      env: []
    - name: logger
      url: "https://logger.com"
      env: []
`,
			operations: []JSONPatchOperation{
				{
					Op:   "add",
					Path: "/spec/containers/[?(@.url=='http:~1~1example.com')]/env/-",
					Value: map[string]any{
						"name":  "MATCHED",
						"value": "true",
					},
				},
			},
			want: `
spec:
  containers:
    - name: app
      url: "http://example.com"
      env:
        - name: MATCHED
          value: "true"
    - name: logger
      url: "https://logger.com"
      env: []
`,
		},
		{
			name: "filter with no matches should error",
			initial: `
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: app
          image: app:v1
`,
			operations: []JSONPatchOperation{
				{
					Op:   "add",
					Path: "/spec/template/spec/containers/[?(@.name=='fluent-bit')]/volumeMounts/-",
					Value: map[string]any{
						"name":      "logs",
						"mountPath": "/var/log",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "replace operation on non-existent filter path should error",
			initial: `
spec:
  containers:
    - name: app
      image: app:v1
`,
			operations: []JSONPatchOperation{
				{
					Op:    "replace",
					Path:  "/spec/containers/[?(@.name=='sidecar')]/image",
					Value: "sidecar:v2",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var resource map[string]any
			if err := yaml.Unmarshal([]byte(tt.initial), &resource); err != nil {
				t.Fatalf("failed to unmarshal initial YAML: %v", err)
			}

			err := ApplyPatches(resource, tt.operations)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("ApplyPatches error = %v", err)
			}

			var wantObj map[string]any
			if err := yaml.Unmarshal([]byte(tt.want), &wantObj); err != nil {
				t.Fatalf("failed to unmarshal expected YAML: %v", err)
			}

			if diff := cmpDiff(wantObj, resource); diff != "" {
				t.Fatalf("resource mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func cmpDiff(expected, actual map[string]any) string {
	wantJSON, _ := json.Marshal(expected)
	gotJSON, _ := json.Marshal(actual)

	var wantNorm, gotNorm any
	_ = json.Unmarshal(wantJSON, &wantNorm)
	_ = json.Unmarshal(gotJSON, &gotNorm)

	if diff := cmp.Diff(wantNorm, gotNorm); diff != "" {
		return diff
	}
	return ""
}
