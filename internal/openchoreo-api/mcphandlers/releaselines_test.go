// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	ocLabels "github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	componentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component"
	componentmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component/mocks"
	releasebindingmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding/mocks"
	workflowrunmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun/mocks"
	workloadmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workload/mocks"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

const testLine = "next"

func rawJSON(t *testing.T, v any) *runtime.RawExtension {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return &runtime.RawExtension{Raw: raw}
}

func makeLine(t *testing.T, workflowParams map[string]any) *openchoreov1alpha1.ReleaseLine {
	t.Helper()
	line := &openchoreov1alpha1.ReleaseLine{
		ObjectMeta: metav1.ObjectMeta{Name: testLine, Namespace: testNS},
		Spec: openchoreov1alpha1.ReleaseLineSpec{
			Owner: openchoreov1alpha1.ReleaseLineOwner{ProjectName: testProject, ComponentName: testComponent},
		},
	}
	if workflowParams != nil {
		line.Spec.Workflow = &openchoreov1alpha1.ReleaseLineWorkflow{Parameters: rawJSON(t, workflowParams)}
	}
	return line
}

func TestTriggerWorkflowRun_ReleaseLine(t *testing.T) {
	ctx := context.Background()
	comp := &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: testComponent},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner:               openchoreov1alpha1.ComponentOwner{ProjectName: testProject},
			ReleaseLinesEnabled: true,
			Workflow: &openchoreov1alpha1.ComponentWorkflowConfig{
				Name: "build-wf",
				Parameters: rawJSON(t, map[string]any{
					"repository": map[string]any{"url": "https://example.com/repo", "branch": "main"},
				}),
			},
		},
	}

	t.Run("merges the line's workflow parameters and labels the run", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		wfrSvc := workflowrunmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(comp, nil)
		compSvc.EXPECT().GetReleaseLine(mock.Anything, testNS, testComponent, testLine).
			Return(makeLine(t, map[string]any{"repository": map[string]any{"branch": "next"}}), nil)
		wfrSvc.EXPECT().
			CreateWorkflowRun(mock.Anything, testNS, mock.MatchedBy(func(wr *openchoreov1alpha1.WorkflowRun) bool {
				var params map[string]map[string]any
				_ = json.Unmarshal(wr.Spec.Workflow.Parameters.Raw, &params)
				return wr.Labels[ocLabels.LabelKeyReleaseLine] == testLine &&
					wr.Labels[ocLabels.LabelKeyComponentName] == testComponent &&
					params["repository"]["branch"] == "next" &&
					params["repository"]["url"] == "https://example.com/repo"
			})).
			Return(&openchoreov1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{Name: testComponent + "-workflow-abc"}}, nil)

		h := newTestHandler(withComponentService(compSvc), withWorkflowRunService(wfrSvc))
		_, err := h.TriggerWorkflowRun(ctx, testNS, testProject, testComponent, "", testLine)
		require.NoError(t, err)
	})

	t.Run("unknown line propagates the service error", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(comp, nil)
		compSvc.EXPECT().GetReleaseLine(mock.Anything, testNS, testComponent, testLine).
			Return(nil, componentsvc.ErrReleaseLineNotFound)

		h := newTestHandler(withComponentService(compSvc))
		_, err := h.TriggerWorkflowRun(ctx, testNS, testProject, testComponent, "", testLine)
		require.ErrorIs(t, err, componentsvc.ErrReleaseLineNotFound)
	})
}

func TestCreateReleaseBinding_ReleaseLine(t *testing.T) {
	ctx := context.Background()
	line := testLine
	rbSvc := releasebindingmocks.NewMockService(t)
	rbSvc.EXPECT().
		CreateReleaseBinding(mock.Anything, testNS, mock.MatchedBy(func(rb *openchoreov1alpha1.ReleaseBinding) bool {
			return rb.Name == testComponent+"-"+testLine+"-"+testEnvironmentName &&
				rb.Spec.Owner.ReleaseLine == testLine &&
				rb.Spec.Owner.ComponentName == testComponent
		})).
		Return(&openchoreov1alpha1.ReleaseBinding{ObjectMeta: metav1.ObjectMeta{Name: testComponent + "-" + testLine + "-dev"}}, nil)

	req := &gen.ReleaseBindingSpec{Environment: testEnvironmentName}
	req.Owner.ComponentName = testComponent
	req.Owner.ProjectName = testProject
	req.Owner.ReleaseLine = &line

	h := newTestHandler(withReleaseBindingService(rbSvc))
	_, err := h.CreateReleaseBinding(ctx, testNS, req)
	require.NoError(t, err)
}

func TestCreateWorkload_ReleaseLine(t *testing.T) {
	ctx := context.Background()
	compSvc := componentmocks.NewMockService(t)
	wlSvc := workloadmocks.NewMockService(t)
	compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(&openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: testComponent},
		Spec:       openchoreov1alpha1.ComponentSpec{Owner: openchoreov1alpha1.ComponentOwner{ProjectName: testProject}},
	}, nil)
	wlSvc.EXPECT().
		CreateWorkload(mock.Anything, testNS, mock.MatchedBy(func(w *openchoreov1alpha1.Workload) bool {
			return w.Name == testComponent+"-"+testLine+"-workload" && w.Spec.Owner.ReleaseLine == testLine
		})).
		Return(&openchoreov1alpha1.Workload{ObjectMeta: metav1.ObjectMeta{Name: testComponent + "-" + testLine + "-workload"}}, nil)

	h := newTestHandler(withComponentService(compSvc), withWorkloadService(wlSvc))
	_, err := h.CreateWorkload(ctx, testNS, testComponent, testLine,
		map[string]any{"container": map[string]any{"image": "nginx"}})
	require.NoError(t, err)
}

func TestCreateComponentRelease_ReleaseLine(t *testing.T) {
	ctx := context.Background()
	compSvc := componentmocks.NewMockService(t)
	compSvc.EXPECT().
		GenerateRelease(mock.Anything, testNS, testComponent, &componentsvc.GenerateReleaseRequest{ReleaseLine: testLine}).
		Return(&openchoreov1alpha1.ComponentRelease{ObjectMeta: metav1.ObjectMeta{Name: testComponent + "-" + testLine + "-abc"}}, nil)

	h := newTestHandler(withComponentService(compSvc))
	result, err := h.CreateComponentRelease(ctx, testNS, testComponent, "", testLine)
	require.NoError(t, err)
	assert.Equal(t, testComponent+"-"+testLine+"-abc", result.(map[string]any)["name"])
}

func TestCreateComponent_ReleaseLinesEnabled(t *testing.T) {
	ctx := context.Background()
	enabled := true
	compSvc := componentmocks.NewMockService(t)
	compSvc.EXPECT().
		CreateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
			return c.Spec.ReleaseLinesEnabled
		})).
		Return(&openchoreov1alpha1.Component{ObjectMeta: metav1.ObjectMeta{Name: testComponent}}, nil)

	h := newTestHandler(withComponentService(compSvc))
	_, err := h.CreateComponent(ctx, testNS, testProject, &gen.CreateComponentRequest{
		Name:                testComponent,
		ReleaseLinesEnabled: &enabled,
	})
	require.NoError(t, err)
}

func TestReleaseLineHandlers(t *testing.T) {
	ctx := context.Background()

	t.Run("list returns summaries with environments", func(t *testing.T) {
		line := makeLine(t, nil)
		line.Status.LatestRelease = &openchoreov1alpha1.LatestRelease{Name: testComponent + "-next-abc"}
		line.Status.Environments = []openchoreov1alpha1.ReleaseLineEnvironment{
			{Environment: "dev", ReleaseBinding: testComponent + "-next-dev", Release: testComponent + "-next-abc", Ready: true},
		}
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().ListReleaseLines(mock.Anything, testNS, testComponent, mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.ReleaseLine]{Items: []openchoreov1alpha1.ReleaseLine{*line}}, nil)

		h := newTestHandler(withComponentService(compSvc))
		result, err := h.ListReleaseLines(ctx, testNS, testComponent, tools.ListOpts{})
		require.NoError(t, err)
		items := result.(map[string]any)["releaseLines"].([]map[string]any)
		require.Len(t, items, 1)
		assert.Equal(t, testLine, items[0]["name"])
		assert.Equal(t, testComponent+"-next-abc", items[0]["latestRelease"])
		envs := items[0]["environments"].([]map[string]any)
		assert.Equal(t, "dev", envs[0]["environment"])
		assert.Equal(t, true, envs[0]["ready"])
	})

	t.Run("get returns the line's overrides", func(t *testing.T) {
		line := makeLine(t, map[string]any{"branch": "next"})
		line.Spec.Parameters = rawJSON(t, map[string]any{"logLevel": "warn"})
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().GetReleaseLine(mock.Anything, testNS, testComponent, testLine).Return(line, nil)

		h := newTestHandler(withComponentService(compSvc))
		result, err := h.GetReleaseLine(ctx, testNS, testComponent, testLine)
		require.NoError(t, err)
		m := result.(map[string]any)
		assert.Equal(t, map[string]any{"logLevel": "warn"}, m["parameters"])
		assert.Equal(t, map[string]any{"parameters": map[string]any{"branch": "next"}}, m["workflow"])
	})

	t.Run("create converts the API line", func(t *testing.T) {
		auto := false
		params := map[string]any{"logLevel": "warn"}
		line := &gen.ReleaseLine{
			Metadata: gen.ObjectMeta{Name: testLine},
			Spec:     &gen.ReleaseLineSpec{AutoDeploy: &auto, Parameters: &params},
		}
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().
			CreateReleaseLine(mock.Anything, testNS, testComponent, mock.MatchedBy(func(rl *openchoreov1alpha1.ReleaseLine) bool {
				return rl.Name == testLine && rl.Spec.AutoDeploy != nil && !*rl.Spec.AutoDeploy &&
					string(rl.Spec.Parameters.Raw) == `{"logLevel":"warn"}`
			})).
			Return(makeLine(t, nil), nil)

		h := newTestHandler(withComponentService(compSvc))
		result, err := h.CreateReleaseLine(ctx, testNS, testComponent, line)
		require.NoError(t, err)
		assert.Equal(t, "created", result.(map[string]any)["action"])
	})

	t.Run("update replaces the line", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().
			UpdateReleaseLine(mock.Anything, testNS, testComponent, mock.MatchedBy(func(rl *openchoreov1alpha1.ReleaseLine) bool {
				return rl.Name == testLine && rl.Spec.Parameters == nil
			})).
			Return(makeLine(t, nil), nil)

		h := newTestHandler(withComponentService(compSvc))
		result, err := h.UpdateReleaseLine(ctx, testNS, testComponent, &gen.ReleaseLine{Metadata: gen.ObjectMeta{Name: testLine}})
		require.NoError(t, err)
		assert.Equal(t, "updated", result.(map[string]any)["action"])
	})

	t.Run("delete propagates errors", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().DeleteReleaseLine(mock.Anything, testNS, testComponent, testLine).
			Return(errors.New("boom"))

		h := newTestHandler(withComponentService(compSvc))
		_, err := h.DeleteReleaseLine(ctx, testNS, testComponent, testLine)
		require.Error(t, err)
	})
}
