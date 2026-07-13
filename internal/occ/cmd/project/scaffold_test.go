// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func rawSchema(t *testing.T, s string) *json.RawMessage {
	t.Helper()
	raw := json.RawMessage(s)
	return &raw
}

// a schema with one required and one defaulted-optional property
const paramsSchema = `{
  "type": "object",
  "properties": {
    "tier": {"type": "string"},
    "replicas": {"type": "integer", "default": 3}
  },
  "required": ["tier"]
}`

func baseParams() ScaffoldParams {
	return ScaffoldParams{
		ProjectName:        "online-store",
		Namespace:          "acme",
		ProjectType:        "web-service",
		DeploymentPipeline: "default",
	}
}

// --- validation ---

func TestScaffold_ValidationErrors(t *testing.T) {
	p := New(mocks.NewMockInterface(t))

	t.Run("missing namespace", func(t *testing.T) {
		params := baseParams()
		params.Namespace = ""
		assert.Error(t, p.Scaffold(params))
	})

	t.Run("missing name", func(t *testing.T) {
		params := baseParams()
		params.ProjectName = ""
		assert.Error(t, p.Scaffold(params))
	})

	t.Run("both type flags", func(t *testing.T) {
		params := baseParams()
		params.ClusterProjectType = "default"
		assert.ErrorContains(t, p.Scaffold(params), "mutually exclusive")
	})

	t.Run("neither type flag", func(t *testing.T) {
		params := baseParams()
		params.ProjectType = ""
		assert.ErrorContains(t, p.Scaffold(params), "one of --projecttype or --clusterprojecttype is required")
	})

	t.Run("empty deployment pipeline", func(t *testing.T) {
		params := baseParams()
		params.DeploymentPipeline = ""
		assert.ErrorContains(t, p.Scaffold(params), "deployment pipeline is required")
	})
}

// --- schema fetch routing ---

func TestScaffold_UsesClusterProjectTypeSchema(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterProjectTypeSchema(mock.Anything, "default").Return(rawSchema(t, paramsSchema), nil)
	mc.EXPECT().GetDeploymentPipeline(mock.Anything, "acme", "default").
		Return(makePipeline(promotionPath("dev", "prod")), nil)

	params := baseParams()
	params.ProjectType = ""
	params.ClusterProjectType = "default"

	p := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, p.Scaffold(params))
	})
	assert.Contains(t, out, "kind: ClusterProjectType")
	assert.Contains(t, out, "name: default")
}

func TestScaffold_SchemaFetchError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectTypeSchema(mock.Anything, "acme", "web-service").
		Return(nil, fmt.Errorf("not found"))

	p := New(mc)
	assert.ErrorContains(t, p.Scaffold(baseParams()), "not found")
}

// --- pipeline resolution (bindings on by default) ---

func TestScaffold_PipelineErrorFails(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectTypeSchema(mock.Anything, "acme", "web-service").Return(rawSchema(t, paramsSchema), nil)
	mc.EXPECT().GetDeploymentPipeline(mock.Anything, "acme", "default").Return(nil, fmt.Errorf("pipeline missing"))

	p := New(mc)
	err := p.Scaffold(baseParams())
	assert.ErrorContains(t, err, "failed to resolve deployment pipeline")
	assert.ErrorContains(t, err, "pipeline missing")
}

// The Project references the pipeline via spec.deploymentPipelineRef, so an
// unresolvable pipeline is an error even when bindings are not requested.
func TestScaffold_PipelineErrorFails_EvenWithNoBindings(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectTypeSchema(mock.Anything, "acme", "web-service").Return(rawSchema(t, paramsSchema), nil)
	mc.EXPECT().GetDeploymentPipeline(mock.Anything, "acme", "default").Return(nil, fmt.Errorf("pipeline missing"))

	params := baseParams()
	params.NoBindings = true

	p := New(mc)
	assert.ErrorContains(t, p.Scaffold(params), "failed to resolve deployment pipeline")
}

// A resolvable pipeline with no environments is not an error: emit the Project
// alone plus a comment explaining why no bindings were generated.
func TestScaffold_PipelineWithNoEnvironments_EmitsProjectWithNote(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectTypeSchema(mock.Anything, "acme", "web-service").Return(rawSchema(t, paramsSchema), nil)
	mc.EXPECT().GetDeploymentPipeline(mock.Anything, "acme", "default").Return(&gen.DeploymentPipeline{}, nil)

	p := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, p.Scaffold(baseParams()))
	})

	assert.Contains(t, out, "kind: Project")
	assert.NotContains(t, out, "kind: ProjectReleaseBinding")
	assert.Contains(t, out, "No ProjectReleaseBindings were generated")
	assert.Contains(t, out, "defines no environments")
}

// The explanatory note survives --skip-comments; it describes the output shape,
// not a schema field.
func TestScaffold_NoEnvironmentsNote_SurvivesSkipComments(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectTypeSchema(mock.Anything, "acme", "web-service").Return(rawSchema(t, paramsSchema), nil)
	mc.EXPECT().GetDeploymentPipeline(mock.Anything, "acme", "default").Return(&gen.DeploymentPipeline{}, nil)

	params := baseParams()
	params.SkipComments = true

	p := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, p.Scaffold(params))
	})
	assert.Contains(t, out, "No ProjectReleaseBindings were generated")
	assert.NotContains(t, out, "Generated by occ project scaffold")
}

// Opting out of bindings explicitly does not produce the note.
func TestScaffold_NoBindings_OmitsNote(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectTypeSchema(mock.Anything, "acme", "web-service").Return(rawSchema(t, paramsSchema), nil)
	mc.EXPECT().GetDeploymentPipeline(mock.Anything, "acme", "default").Return(&gen.DeploymentPipeline{}, nil)

	params := baseParams()
	params.NoBindings = true

	p := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, p.Scaffold(params))
	})
	assert.NotContains(t, out, "No ProjectReleaseBindings were generated")
	assert.NotContains(t, out, "kind: ProjectReleaseBinding")
}

func TestScaffold_EmitsBindingPerEnvironment(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectTypeSchema(mock.Anything, "acme", "web-service").Return(rawSchema(t, paramsSchema), nil)
	mc.EXPECT().GetDeploymentPipeline(mock.Anything, "acme", "default").
		Return(makePipeline(promotionPath("dev", "staging"), promotionPath("staging", "prod")), nil)

	p := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, p.Scaffold(baseParams()))
	})

	// Project doc
	assert.Contains(t, out, "kind: Project")
	assert.Contains(t, out, "name: online-store")
	assert.Contains(t, out, "kind: ProjectType")
	assert.Contains(t, out, "namespace: acme")

	// One binding per env, in pipeline order, deduped
	assert.Contains(t, out, "kind: ProjectReleaseBinding")
	assert.Contains(t, out, "name: online-store-dev")
	assert.Contains(t, out, "name: online-store-staging")
	assert.Contains(t, out, "name: online-store-prod")
	assert.Equal(t, 3, countOccurrences(out, "kind: ProjectReleaseBinding"))

	// projectRelease is left unset for the controller to seed
	assert.NotContains(t, out, "projectRelease:")
}

func TestScaffold_NoBindings_StillValidatesPipeline(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectTypeSchema(mock.Anything, "acme", "web-service").Return(rawSchema(t, paramsSchema), nil)
	// The pipeline is resolved even with --no-bindings.
	mc.EXPECT().GetDeploymentPipeline(mock.Anything, "acme", "default").
		Return(makePipeline(promotionPath("dev", "prod")), nil)

	params := baseParams()
	params.NoBindings = true

	p := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, p.Scaffold(params))
	})
	assert.Contains(t, out, "kind: Project")
	assert.NotContains(t, out, "kind: ProjectReleaseBinding")
}

// --- parameter rendering ---

func TestScaffold_RendersParameters(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectTypeSchema(mock.Anything, "acme", "web-service").Return(rawSchema(t, paramsSchema), nil)
	mc.EXPECT().GetDeploymentPipeline(mock.Anything, "acme", "default").
		Return(makePipeline(promotionPath("dev", "prod")), nil)

	params := baseParams()
	params.NoBindings = true

	p := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, p.Scaffold(params))
	})

	assert.Contains(t, out, "parameters:")
	assert.Contains(t, out, "tier:")     // required -> placeholder
	assert.Contains(t, out, "replicas:") // optional w/ default -> commented
}

func TestScaffold_NilSchema_OmitsParameters(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectTypeSchema(mock.Anything, "acme", "web-service").
		Return(rawSchema(t, `{"type":"object"}`), nil)
	mc.EXPECT().GetDeploymentPipeline(mock.Anything, "acme", "default").
		Return(makePipeline(promotionPath("dev", "prod")), nil)

	params := baseParams()
	params.NoBindings = true

	p := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, p.Scaffold(params))
	})
	assert.Contains(t, out, "kind: Project")
	assert.NotContains(t, out, "parameters:")
}

// --- output file ---

func TestScaffold_WritesOutputFile(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectTypeSchema(mock.Anything, "acme", "web-service").Return(rawSchema(t, paramsSchema), nil)
	mc.EXPECT().GetDeploymentPipeline(mock.Anything, "acme", "default").
		Return(makePipeline(promotionPath("dev", "prod")), nil)

	outPath := filepath.Join(t.TempDir(), "project.yaml")
	params := baseParams()
	params.NoBindings = true
	params.OutputPath = outPath

	p := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, p.Scaffold(params))
	})
	assert.Contains(t, out, "Project YAML written to")
	assert.FileExists(t, outPath)
}

func countOccurrences(s, substr string) int {
	count := 0
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			count++
		}
	}
	return count
}
