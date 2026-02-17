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
		name    string
		root    string
		path    string
		want    []string
		wantErr bool
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
		{
			name: "filter with escaped slash in value",
			root: `
spec:
  containers:
    - name: web
      url: "http://example.com"
    - name: api
      url: "https://api.example.com"
    - name: local
      url: "localhost"
`,
			path: "/spec/containers/[?(@.url=='http:~1~1example.com')]",
			want: []string{
				"/spec/containers/0",
			},
		},
		{
			name: "filter with escaped tilde in value",
			root: `
spec:
  items:
    - tag: "version~1"
      value: "old"
    - tag: "version~2"
      value: "new"
`,
			path: "/spec/items/[?(@.tag=='version~01')]",
			want: []string{
				"/spec/items/0",
			},
		},
		{
			name: "filter with nested field - configMap.name",
			root: `
spec:
  volumes:
    - name: config-vol
      configMap:
        name: app-config
    - name: data-vol
      persistentVolumeClaim:
        claimName: data
    - name: settings-vol
      configMap:
        name: settings
`,
			path: "/spec/volumes/[?(@.configMap.name=='app-config')]/name",
			want: []string{
				"/spec/volumes/0/name",
			},
		},
		{
			name: "filter with nested field - resources.limits.memory",
			root: `
spec:
  containers:
    - name: app
      image: app:v1
      resources:
        limits:
          memory: 2Gi
          cpu: 1000m
    - name: sidecar
      image: sidecar:v1
      resources:
        limits:
          memory: 512Mi
          cpu: 100m
    - name: worker
      image: worker:v1
      resources:
        limits:
          memory: 2Gi
          cpu: 2000m
`,
			path: "/spec/containers/[?(@.resources.limits.memory=='2Gi')]/image",
			want: []string{
				"/spec/containers/0/image",
				"/spec/containers/2/image",
			},
		},
		{
			name: "filter with nested field - persistentVolumeClaim.claimName",
			root: `
spec:
  volumes:
    - name: config
      configMap:
        name: my-config
    - name: data
      persistentVolumeClaim:
        claimName: data-pvc
    - name: cache
      emptyDir: {}
`,
			path: "/spec/volumes/[?(@.persistentVolumeClaim.claimName=='data-pvc')]",
			want: []string{
				"/spec/volumes/1",
			},
		},
		{
			name: "wildcard expands all array elements",
			root: `
spec:
  rules:
    - host: a.example.com
    - host: b.example.com
    - host: c.example.com
`,
			path: "/spec/rules/[*]/host",
			want: []string{
				"/spec/rules/0/host",
				"/spec/rules/1/host",
				"/spec/rules/2/host",
			},
		},
		{
			name: "wildcard combined with filter",
			root: `
spec:
  rules:
    - backendRefs:
        - name: svc-a
          port: 80
        - name: svc-b
          port: 80
    - backendRefs:
        - name: svc-a
          port: 8080
        - name: svc-c
          port: 80
`,
			path: "/spec/rules/[*]/backendRefs/[?(@.name=='svc-a')]/port",
			want: []string{
				"/spec/rules/0/backendRefs/0/port",
				"/spec/rules/1/backendRefs/0/port",
			},
		},
		{
			name: "wildcard on empty array",
			root: `
spec:
  rules: []
`,
			path: "/spec/rules/[*]/host",
			want: []string{},
		},
		{
			name: "filter on non-array errors",
			root: `
spec:
  containers:
    name: app
`,
			path:    "/spec/containers/[?(@.name=='app')]/image",
			wantErr: true,
		},
		{
			name: "wildcard on non-array errors",
			root: `
spec:
  rules:
    host: example.com
`,
			path:    "/spec/rules/[*]/host",
			wantErr: true,
		},
		{
			name: "wildcard with append marker",
			root: `
spec:
  rules:
    - backendRefs:
        - name: svc-a
    - backendRefs:
        - name: svc-b
`,
			path: "/spec/rules/[*]/backendRefs/-",
			want: []string{
				"/spec/rules/0/backendRefs/-",
				"/spec/rules/1/backendRefs/-",
			},
		},
		{
			name: "filter with deep nested field - metadata.labels.app",
			root: `
spec:
  pods:
    - name: web-1
      metadata:
        labels:
          app: web
          tier: frontend
    - name: api-1
      metadata:
        labels:
          app: api
          tier: backend
    - name: web-2
      metadata:
        labels:
          app: web
          tier: frontend
`,
			path: "/spec/pods/[?(@.metadata.labels.app=='web')]/name",
			want: []string{
				"/spec/pods/0/name",
				"/spec/pods/2/name",
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
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
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
