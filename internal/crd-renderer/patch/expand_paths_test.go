// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package patch

import (
	"fmt"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestExpandPaths(t *testing.T) {
	t.Parallel()

	baseRoot := `
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
        - name: sidecar
          image: sidecar:v1
          env: []
`

	tests := []struct {
		name string
		root string
		path string
		want []string
	}{
		{
			name: "simple index",
			root: baseRoot,
			path: "/spec/template/spec/containers/0/env/0",
			want: []string{"/spec/template/spec/containers/0/env/0"},
		},
		{
			name: "append dash",
			root: baseRoot,
			path: "/spec/template/spec/containers/-",
			want: []string{"/spec/template/spec/containers/-"},
		},
		{
			name: "filter single match",
			root: baseRoot,
			path: "/spec/template/spec/containers/[?(@.name=='app')]/env/-",
			want: []string{"/spec/template/spec/containers/0/env/-"},
		},
		{
			name: "nested filters",
			root: baseRoot,
			path: "/spec/template/spec/containers/[?(@.name=='app')]/env/[?(@.name=='A')]/value",
			want: []string{"/spec/template/spec/containers/0/env/0/value"},
		},
		{
			name: "no match filter",
			root: baseRoot,
			path: "/spec/template/spec/containers/[?(@.name=='missing')]/env/-",
			want: []string{},
		},
		{
			name: "filter multiple matches",
			root: `
spec:
  template:
    spec:
      containers:
        - name: app
          role: worker
        - name: logger
          role: worker
        - name: sidecar
          role: helper
`,
			path: "/spec/template/spec/containers/[?(@.role=='worker')]",
			want: []string{
				"/spec/template/spec/containers/0",
				"/spec/template/spec/containers/1",
			},
		},
		{
			name: "two filters expanding to four paths",
			root: `
spec:
  template:
    spec:
      containers:
        - name: worker-a
          role: worker
          env:
            - name: SHARED
              value: "true"
            - name: SHARED
              value: "alt"
        - name: worker-b
          role: worker
          env:
            - name: SHARED
              value: "true"
            - name: SHARED
              value: "alt"
        - name: helper
          role: helper
          env:
            - name: SHARED
              value: "false"
`,
			path: "/spec/template/spec/containers/[?(@.role=='worker')]/env/[?(@.name=='SHARED')]/value",
			want: []string{
				"/spec/template/spec/containers/0/env/0/value",
				"/spec/template/spec/containers/0/env/1/value",
				"/spec/template/spec/containers/1/env/0/value",
				"/spec/template/spec/containers/1/env/1/value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var root map[string]any
			if err := yaml.Unmarshal([]byte(tt.root), &root); err != nil {
				t.Fatalf("failed to unmarshal root YAML: %v", err)
			}

			got, err := expandPaths(root, tt.path)
			if err != nil {
				t.Fatalf("expandPaths error = %v", err)
			}

			if diff := cmpDiffStrings(tt.want, got); diff != "" {
				t.Fatalf("expandPaths mismatch:\n%s", diff)
			}
		})
	}
}

func cmpDiffStrings(want, got []string) string {
	if len(want) != len(got) {
		return fmt.Sprintf("length mismatch: want %d, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if want[i] != got[i] {
			return fmt.Sprintf("index %d: want %q, got %q", i, want[i], got[i])
		}
	}
	return ""
}
