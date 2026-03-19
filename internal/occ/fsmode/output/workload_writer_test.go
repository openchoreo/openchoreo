// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

func buildIndex(t *testing.T, entries ...*index.ResourceEntry) *fsmode.Index {
	t.Helper()
	idx := index.New("/repo")
	for _, e := range entries {
		_ = idx.Add(e)
	}
	return fsmode.WrapIndex(idx)
}

func makeEntry(kind, namespace, name, filePath string, extra map[string]any) *index.ResourceEntry {
	obj := map[string]any{
		"apiVersion": "openchoreo.dev/v1alpha1",
		"kind":       kind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
	}
	for k, v := range extra {
		obj[k] = v
	}
	u := &unstructured.Unstructured{Object: obj}
	return &index.ResourceEntry{Resource: u, FilePath: filePath}
}

func TestWorkloadWriterResolveOutputPath(t *testing.T) {
	tests := []struct {
		name   string
		params WorkloadWriteParams
		idx    func(t *testing.T) *fsmode.Index
		want   string
	}{
		{
			name: "explicit absolute path",
			params: WorkloadWriteParams{
				OutputPath:    "/custom/path/workload.yaml",
				RepoPath:      "/repo",
				ProjectName:   "proj",
				ComponentName: "comp",
				Namespace:     "ns",
			},
			idx:  func(t *testing.T) *fsmode.Index { return buildIndex(t) },
			want: "/custom/path/workload.yaml",
		},
		{
			name: "explicit relative path",
			params: WorkloadWriteParams{
				OutputPath:    "out/workload.yaml",
				RepoPath:      "/repo",
				ProjectName:   "proj",
				ComponentName: "comp",
				Namespace:     "ns",
			},
			idx:  func(t *testing.T) *fsmode.Index { return buildIndex(t) },
			want: "/repo/out/workload.yaml",
		},
		{
			name: "existing workload in index",
			params: WorkloadWriteParams{
				RepoPath:      "/repo",
				ProjectName:   "proj",
				ComponentName: "comp",
				Namespace:     "ns",
			},
			idx: func(t *testing.T) *fsmode.Index {
				return buildIndex(t,
					makeEntry("Workload", "ns", "comp-workload", "/repo/projects/proj/components/comp/workload.yaml", map[string]any{
						"spec": map[string]any{
							"owner": map[string]any{
								"projectName":   "proj",
								"componentName": "comp",
							},
							"container": map[string]any{
								"image": "img:latest",
							},
						},
					}),
				)
			},
			want: "/repo/projects/proj/components/comp/workload.yaml",
		},
		{
			name: "component dir fallback",
			params: WorkloadWriteParams{
				RepoPath:      "/repo",
				ProjectName:   "proj",
				ComponentName: "comp",
				Namespace:     "ns",
			},
			idx: func(t *testing.T) *fsmode.Index {
				return buildIndex(t,
					makeEntry("Component", "ns", "comp", "/repo/projects/proj/components/comp/component.yaml", map[string]any{
						"spec": map[string]any{
							"owner": map[string]any{
								"projectName": "proj",
							},
						},
					}),
				)
			},
			want: "/repo/projects/proj/components/comp/workload.yaml",
		},
		{
			name: "default path when nothing found",
			params: WorkloadWriteParams{
				RepoPath:      "/repo",
				ProjectName:   "proj",
				ComponentName: "comp",
				Namespace:     "ns",
			},
			idx:  func(t *testing.T) *fsmode.Index { return buildIndex(t) },
			want: "/repo/projects/proj/components/comp/workload.yaml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewWorkloadWriter(tt.idx(t))
			got := w.resolveOutputPath(tt.params)
			assert.Equal(t, tt.want, got)
		})
	}
}
