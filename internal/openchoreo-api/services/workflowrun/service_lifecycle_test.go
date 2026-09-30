// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	k8sMocks "github.com/openchoreo/openchoreo/internal/clients/kubernetes/mocks"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	planeNamespace = "openchoreo-ci"
	argoRunName    = "test-run-abc12"
)

// lifecycleFixture wires a control-plane fake client (Workflow, default
// ClusterWorkflowPlane, WorkflowRun pointing at an Argo Workflow) and a fake
// workflow-plane client holding that Argo Workflow.
type lifecycleFixture struct {
	svc         Service
	planeClient client.Client
}

func newLifecycleFixture(t *testing.T, argoWF *argoproj.Workflow, started bool) lifecycleFixture {
	t.Helper()

	run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)
	if started {
		run.Status.RunReference = &openchoreov1alpha1.ResourceReference{
			APIVersion: "argoproj.io/v1alpha1", Kind: "Workflow", Name: argoRunName, Namespace: planeNamespace,
		}
	}
	cpClient := testutil.NewFakeClient(
		testutil.NewWorkflow(testNamespace, testWorkflowName),
		testutil.NewClusterWorkflowPlane("default"),
		run,
	)

	scheme := runtime.NewScheme()
	require.NoError(t, argoproj.AddToScheme(scheme))
	builder := fake.NewClientBuilder().WithScheme(scheme)
	if argoWF != nil {
		builder = builder.WithObjects(argoWF)
	}
	planeClient := builder.Build()

	provider := k8sMocks.NewMockWorkflowPlaneClientProvider(t)
	provider.EXPECT().ClusterWorkflowPlaneClient(mock.Anything).Return(planeClient, nil).Maybe()

	return lifecycleFixture{
		svc:         NewService(cpClient, provider, nil, testutil.TestLogger()),
		planeClient: planeClient,
	}
}

func (f lifecycleFixture) argoWorkflow(t *testing.T) *argoproj.Workflow {
	t.Helper()
	wf := &argoproj.Workflow{}
	require.NoError(t, f.planeClient.Get(context.Background(), types.NamespacedName{Name: argoRunName, Namespace: planeNamespace}, wf))
	return wf
}

func suspendedArgoWorkflow() *argoproj.Workflow {
	suspend := true
	return &argoproj.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: argoRunName, Namespace: planeNamespace},
		Spec:       argoproj.WorkflowSpec{Suspend: &suspend},
		Status: argoproj.WorkflowStatus{
			Phase: argoproj.WorkflowRunning,
			Nodes: argoproj.Nodes{
				"approve": {ID: "approve", Name: "approve", Type: argoproj.NodeTypeSuspend, Phase: argoproj.NodeRunning},
				"build":   {ID: "build", Name: "build", Type: argoproj.NodeTypePod, Phase: argoproj.NodeSucceeded},
				"done":    {ID: "done", Name: "done", Type: argoproj.NodeTypeSuspend, Phase: argoproj.NodeSucceeded},
			},
		},
	}
}

func TestResumeWorkflowRun(t *testing.T) {
	ctx := context.Background()

	// `argo resume` semantics: the suspend flag is cleared AND the active suspend node is
	// marked Succeeded; without the node change Argo would stay parked at the suspend step.
	t.Run("clears spec.suspend and completes active suspend nodes only", func(t *testing.T) {
		f := newLifecycleFixture(t, suspendedArgoWorkflow(), true)

		result, err := f.svc.ResumeWorkflowRun(ctx, testNamespace, testRunName)
		require.NoError(t, err)
		assert.Equal(t, testRunName, result.Name)

		wf := f.argoWorkflow(t)
		assert.Nil(t, wf.Spec.Suspend)
		assert.Equal(t, argoproj.NodeSucceeded, wf.Status.Nodes["approve"].Phase)
		assert.False(t, wf.Status.Nodes["approve"].FinishedAt.Time.IsZero(), "resumed node gets a finish time")
		assert.Equal(t, argoproj.NodeSucceeded, wf.Status.Nodes["build"].Phase, "non-suspend nodes untouched")
		assert.Equal(t, argoproj.NodeSucceeded, wf.Status.Nodes["done"].Phase)
	})

	t.Run("nothing suspended is a conflict, not a silent no-op", func(t *testing.T) {
		wf := suspendedArgoWorkflow()
		wf.Spec.Suspend = nil
		node := wf.Status.Nodes["approve"]
		node.Phase = argoproj.NodeSucceeded
		wf.Status.Nodes["approve"] = node
		f := newLifecycleFixture(t, wf, true)

		_, err := f.svc.ResumeWorkflowRun(ctx, testNamespace, testRunName)
		require.ErrorIs(t, err, ErrWorkflowRunNotSuspended)
	})

	t.Run("run without a live workflow cannot be resumed", func(t *testing.T) {
		f := newLifecycleFixture(t, nil, false)
		_, err := f.svc.ResumeWorkflowRun(ctx, testNamespace, testRunName)
		require.ErrorIs(t, err, ErrWorkflowRunNotStarted)
	})

	t.Run("run reference pointing at a deleted workflow", func(t *testing.T) {
		f := newLifecycleFixture(t, nil, true)
		_, err := f.svc.ResumeWorkflowRun(ctx, testNamespace, testRunName)
		require.ErrorIs(t, err, ErrWorkflowRunNotStarted)
	})

	t.Run("unknown run", func(t *testing.T) {
		f := newLifecycleFixture(t, nil, false)
		_, err := f.svc.ResumeWorkflowRun(ctx, testNamespace, "nope")
		require.ErrorIs(t, err, ErrWorkflowRunNotFound)
	})
}

func TestStopWorkflowRun(t *testing.T) {
	ctx := context.Background()

	t.Run("sets shutdown Stop and records the reason", func(t *testing.T) {
		f := newLifecycleFixture(t, suspendedArgoWorkflow(), true)

		result, err := f.svc.StopWorkflowRun(ctx, testNamespace, testRunName, "superseded by v2")
		require.NoError(t, err)
		assert.Equal(t, "superseded by v2", result.Annotations[AnnotationKeyStopReason])
		assert.Equal(t, workflowRunTypeMeta, result.TypeMeta)

		wf := f.argoWorkflow(t)
		assert.Equal(t, argoproj.ShutdownStrategyStop, wf.Spec.Shutdown)

		stored, err := f.svc.GetWorkflowRun(ctx, testNamespace, testRunName)
		require.NoError(t, err)
		assert.Equal(t, "superseded by v2", stored.Annotations[AnnotationKeyStopReason], "reason must be persisted on the WorkflowRun")
	})

	t.Run("empty reason leaves annotations alone", func(t *testing.T) {
		f := newLifecycleFixture(t, suspendedArgoWorkflow(), true)
		result, err := f.svc.StopWorkflowRun(ctx, testNamespace, testRunName, "")
		require.NoError(t, err)
		_, has := result.Annotations[AnnotationKeyStopReason]
		assert.False(t, has)
	})

	// Stop is idempotent: a second call on an already-stopping workflow must not error,
	// otherwise a retried API call would surface a spurious failure.
	t.Run("already stopping is idempotent", func(t *testing.T) {
		wf := suspendedArgoWorkflow()
		wf.Spec.Shutdown = argoproj.ShutdownStrategyStop
		f := newLifecycleFixture(t, wf, true)
		_, err := f.svc.StopWorkflowRun(ctx, testNamespace, testRunName, "again")
		require.NoError(t, err)
	})

	t.Run("completed run cannot be stopped", func(t *testing.T) {
		wf := suspendedArgoWorkflow()
		wf.Status.Phase = argoproj.WorkflowSucceeded
		f := newLifecycleFixture(t, wf, true)
		_, err := f.svc.StopWorkflowRun(ctx, testNamespace, testRunName, "")
		require.ErrorIs(t, err, ErrWorkflowRunCompleted)
	})

	t.Run("run without a live workflow cannot be stopped", func(t *testing.T) {
		f := newLifecycleFixture(t, nil, false)
		_, err := f.svc.StopWorkflowRun(ctx, testNamespace, testRunName, "")
		require.ErrorIs(t, err, ErrWorkflowRunNotStarted)
	})
}
