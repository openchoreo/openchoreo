// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode"
	"github.com/openchoreo/openchoreo/internal/occ/fsmode/pipeline"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

const (
	lineNamespace = "test-ns"
	lineProject   = "my-proj"
	lineComponent = "my-comp"
	lineName      = "next"
)

var lineDate = time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)

func addEntry(t *testing.T, idx *index.Index, obj map[string]any, filePath string) {
	t.Helper()
	require.NoError(t, idx.Add(&index.ResourceEntry{Resource: &unstructured.Unstructured{Object: obj}, FilePath: filePath}))
}

// newLineIndex indexes a component with a default line, when withDefault is set, and the release line
// "next", which overrides the log level.
func newLineIndex(t *testing.T, withDefault bool) *index.Index {
	t.Helper()
	idx := index.New("/repo")
	ctSpec := map[string]any{"workloadType": "deployment", "resources": []any{}, "schema": map[string]any{}}
	addEntry(t, idx, map[string]any{
		"apiVersion": "openchoreo.dev/v1alpha1",
		"kind":       "ComponentType",
		"metadata":   map[string]any{"name": "service", "namespace": lineNamespace},
		"spec":       ctSpec,
	}, "/repo/platform/service.yaml")
	addEntry(t, idx, map[string]any{
		"apiVersion": "openchoreo.dev/v1alpha1",
		"kind":       "Component",
		"metadata":   map[string]any{"name": lineComponent, "namespace": lineNamespace},
		"spec": map[string]any{
			"owner":         map[string]any{"projectName": lineProject},
			"componentType": map[string]any{"kind": "ComponentType", "name": "deployment/service"},
			"parameters":    map[string]any{"replicas": 1, "logLevel": "info"},
		},
	}, "/repo/projects/my-proj/components/my-comp/component.yaml")
	addEntry(t, idx, map[string]any{
		"apiVersion": "openchoreo.dev/v1alpha1",
		"kind":       "ReleaseLine",
		"metadata":   map[string]any{"name": lineName, "namespace": lineNamespace},
		"spec": map[string]any{
			"owner":      map[string]any{"projectName": lineProject, "componentName": lineComponent},
			"parameters": map[string]any{"logLevel": "debug"},
		},
	}, "/repo/projects/my-proj/components/my-comp/release-line-next.yaml")
	addWorkload(t, idx, lineNamespace, lineComponent+"-next-5e612006-workload", lineProject, lineComponent, map[string]any{
		"owner":     map[string]any{"projectName": lineProject, "componentName": lineComponent, "releaseLine": lineName},
		"container": map[string]any{"image": "img:v2"},
	}, "/repo/projects/my-proj/components/my-comp/workload-next.yaml")
	if withDefault {
		addWorkload(t, idx, lineNamespace, lineComponent+"-workload", lineProject, lineComponent,
			map[string]any{"container": map[string]any{"image": "img:v1"}},
			"/repo/projects/my-proj/components/my-comp/workload.yaml")
	}
	return idx
}

func generateLineRelease(t *testing.T, idx *index.Index, line, name string) *unstructured.Unstructured {
	t.Helper()
	release, err := NewReleaseGenerator(fsmode.WrapIndex(idx)).GenerateRelease(ReleaseOptions{
		ComponentName: lineComponent,
		ProjectName:   lineProject,
		Namespace:     lineNamespace,
		ReleaseLine:   line,
		ReleaseName:   name,
		Date:          lineDate,
	})
	require.NoError(t, err)
	return release
}

// addLineReleaseBinding indexes a binding and writes it to dir, where an update reads it from.
func addLineReleaseBinding(t *testing.T, idx *index.Index, dir, name, line, env, releaseName string) {
	t.Helper()
	owner := map[string]any{"projectName": lineProject, "componentName": lineComponent}
	if line != "" {
		owner["releaseLine"] = line
	}
	obj := map[string]any{
		"apiVersion": "openchoreo.dev/v1alpha1",
		"kind":       "ReleaseBinding",
		"metadata":   map[string]any{"name": name, "namespace": lineNamespace},
		"spec":       map[string]any{"owner": owner, "environment": env, "releaseName": releaseName},
	}
	data, err := yaml.Marshal(obj)
	require.NoError(t, err)
	path := filepath.Join(dir, name+".yaml")
	require.NoError(t, os.WriteFile(path, data, 0600))
	addEntry(t, idx, obj, path)
}

func TestGenerateRelease_ReleaseLine(t *testing.T) {
	idx := newLineIndex(t, true)

	tests := []struct {
		name         string
		line         string
		wantName     string
		wantImage    string
		wantLine     any
		wantLogLevel string
	}{
		{name: "default line", wantName: "my-comp-20261021-1", wantImage: "img:v1", wantLogLevel: "info"},
		{name: "default line keyword", line: "default", wantName: "my-comp-20261021-1", wantImage: "img:v1", wantLogLevel: "info"},
		{name: "release line", line: lineName, wantName: "my-comp-next-5e612006-20261021-1", wantImage: "img:v2", wantLine: lineName, wantLogLevel: "debug"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			release := generateLineRelease(t, idx, tt.line, "")

			assert.Equal(t, tt.wantName, release.GetName())
			image, _, _ := unstructured.NestedString(release.Object, "spec", "workload", "container", "image")
			assert.Equal(t, tt.wantImage, image)
			line, _, _ := unstructured.NestedFieldNoCopy(release.Object, "spec", "owner", "releaseLine")
			assert.Equal(t, tt.wantLine, line)
			params, _, _ := unstructured.NestedMap(release.Object, "spec", "componentProfile", "parameters")
			assert.Equal(t, tt.wantLogLevel, params["logLevel"])
			assert.EqualValues(t, 1, params["replicas"], "the line keeps the component's other parameters")
		})
	}

	t.Run("unknown release line", func(t *testing.T) {
		_, err := NewReleaseGenerator(fsmode.WrapIndex(idx)).GenerateRelease(ReleaseOptions{
			ComponentName: lineComponent, ProjectName: lineProject, Namespace: lineNamespace, ReleaseLine: "canary",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `release line "canary" of component "my-comp"`)
	})
}

func TestGenerateBulkReleases_ReleaseLines(t *testing.T) {
	today := time.Now().Format("20060102")
	tests := []struct {
		name        string
		withDefault bool
		want        map[string]string
	}{
		{name: "default and release line", withDefault: true, want: map[string]string{"": "my-comp-" + today + "-1", lineName: "my-comp-next-5e612006-" + today + "-1"}},
		{name: "component without a default line workload", want: map[string]string{lineName: "my-comp-next-5e612006-" + today + "-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := NewReleaseGenerator(fsmode.WrapIndex(newLineIndex(t, tt.withDefault)))
			result, err := gen.GenerateBulkReleases(BulkReleaseOptions{All: true, Namespace: lineNamespace, Version: "1"})
			require.NoError(t, err)
			assert.Empty(t, result.Errors)

			got := map[string]string{}
			for _, info := range result.Releases {
				got[info.ReleaseLine] = info.ReleaseName
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGenerateBinding_ReleaseLine(t *testing.T) {
	pipelineInfo := &pipeline.PipelineInfo{
		Name:            "test-pipeline",
		RootEnvironment: "dev",
		Environments:    []string{"dev", "staging"},
		PromotionPaths:  map[string][]string{"dev": {"staging"}, "staging": {}},
		EnvPosition:     map[string]int{"dev": 0, "staging": 1},
	}

	// Each line has a release matching its current state, and a binding in dev.
	idx := newLineIndex(t, true)
	for _, r := range []struct{ line, name string }{{"", "my-comp-20261021-1"}, {lineName, "my-comp-next-5e612006-20261021-1"}} {
		addEntry(t, idx, generateLineRelease(t, idx, r.line, r.name).Object, "/repo/projects/my-proj/components/my-comp/releases/"+r.name+".yaml")
	}
	dir := t.TempDir()
	addLineReleaseBinding(t, idx, dir, "my-comp-dev", "", "dev", "my-comp-20261021-1")
	addLineReleaseBinding(t, idx, dir, "my-comp-next-5e612006-dev", lineName, "dev", "my-comp-next-5e612006-20261021-1")
	gen := NewBindingGenerator(fsmode.WrapIndex(idx))

	t.Run("root environment of the release line", func(t *testing.T) {
		info, err := gen.GenerateBindingWithInfo(BindingOptions{
			ProjectName: lineProject, ComponentName: lineComponent, ReleaseLine: lineName,
			TargetEnv: "dev", PipelineInfo: pipelineInfo, Namespace: lineNamespace,
		})
		require.NoError(t, err)
		assert.Equal(t, "my-comp-next-5e612006-20261021-1", info.ReleaseName)
		assert.Equal(t, lineName, info.ReleaseLine)
		assert.True(t, info.IsUpdate)
		assert.Equal(t, filepath.Join(dir, "my-comp-next-5e612006-dev.yaml"), info.ExistingFilePath)
	})

	for _, tt := range []struct {
		name, line, wantName, wantRelease string
		wantLine                          any
	}{
		{name: "promote the default line", wantName: "my-comp-staging", wantRelease: "my-comp-20261021-1"},
		{name: "promote the release line", line: lineName, wantName: "my-comp-next-5e612006-staging", wantRelease: "my-comp-next-5e612006-20261021-1", wantLine: lineName},
	} {
		t.Run(tt.name, func(t *testing.T) {
			binding, err := gen.GenerateBinding(BindingOptions{
				ProjectName: lineProject, ComponentName: lineComponent, ReleaseLine: tt.line,
				TargetEnv: "staging", PipelineInfo: pipelineInfo, Namespace: lineNamespace,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, binding.GetName())
			assert.Equal(t, tt.wantRelease, getNestedString(binding.Object, "spec", "releaseName"))
			line, _, _ := unstructured.NestedFieldNoCopy(binding.Object, "spec", "owner", "releaseLine")
			assert.Equal(t, tt.wantLine, line)
		})
	}

	t.Run("explicit release of another line", func(t *testing.T) {
		_, err := gen.GenerateBinding(BindingOptions{
			ProjectName: lineProject, ComponentName: lineComponent, ReleaseLine: lineName,
			ComponentRelease: "my-comp-20261021-1", TargetEnv: "staging", PipelineInfo: pipelineInfo, Namespace: lineNamespace,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `does not belong to release line "next" of component my-proj/my-comp`)
	})

	t.Run("unknown release line", func(t *testing.T) {
		_, err := gen.GenerateBinding(BindingOptions{
			ProjectName: lineProject, ComponentName: lineComponent, ReleaseLine: "canary",
			TargetEnv: "dev", PipelineInfo: pipelineInfo, Namespace: lineNamespace,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `release line "canary" of component my-proj/my-comp not found`)
	})

	t.Run("bulk binds every line", func(t *testing.T) {
		result, err := gen.GenerateBulkBindings(BulkBindingOptions{
			All: true, TargetEnv: "staging", PipelineInfo: pipelineInfo, Namespace: lineNamespace,
		})
		require.NoError(t, err)
		assert.Empty(t, result.Errors)

		got := map[string]string{}
		for _, info := range result.Bindings {
			got[info.BindingName] = info.ReleaseName
		}
		assert.Equal(t, map[string]string{
			"my-comp-staging":               "my-comp-20261021-1",
			"my-comp-next-5e612006-staging": "my-comp-next-5e612006-20261021-1",
		}, got)
	})
}
