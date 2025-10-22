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
			name: "array filter with no matches is a no-op",
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
			want: `
spec:
  containers:
    - name: app
      image: app:v1
`,
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
