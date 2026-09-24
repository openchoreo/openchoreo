// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	k8sMocks "github.com/openchoreo/openchoreo/internal/clients/kubernetes/mocks"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

var errLifecycleBoom = errors.New("boom")

// lifecycleErrorOpts lets a test break one dependency of the resume/stop path.
type lifecycleErrorOpts struct {
	omitWorkflow bool
	providerErr  error
	cpFuncs      interceptor.Funcs
	planeFuncs   interceptor.Funcs
	argoWF       *argoproj.Workflow
}

func newLifecycleErrorService(t *testing.T, o lifecycleErrorOpts) Service {
	t.Helper()

	run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)
	run.Status.RunReference = &openchoreov1alpha1.ResourceReference{
		APIVersion: "argoproj.io/v1alpha1", Kind: "Workflow", Name: argoRunName, Namespace: planeNamespace,
	}
	cpObjs := []client.Object{testutil.NewClusterWorkflowPlane("default"), run}
	if !o.omitWorkflow {
		cpObjs = append(cpObjs, testutil.NewWorkflow(testNamespace, testWorkflowName))
	}
	cpClient := fake.NewClientBuilder().
		WithScheme(testutil.NewScheme()).
		WithObjects(cpObjs...).
		WithInterceptorFuncs(o.cpFuncs).
		Build()

	scheme := runtime.NewScheme()
	require.NoError(t, argoproj.AddToScheme(scheme))
	argoWF := o.argoWF
	if argoWF == nil {
		argoWF = suspendedArgoWorkflow()
	}
	planeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(argoWF).
		WithInterceptorFuncs(o.planeFuncs).
		Build()

	provider := k8sMocks.NewMockWorkflowPlaneClientProvider(t)
	if o.providerErr != nil {
		provider.EXPECT().ClusterWorkflowPlaneClient(mock.Anything).Return(nil, o.providerErr).Maybe()
	} else {
		provider.EXPECT().ClusterWorkflowPlaneClient(mock.Anything).Return(planeClient, nil).Maybe()
	}

	return NewService(cpClient, provider, nil, testutil.TestLogger())
}

func failingUpdate(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
	return errLifecycleBoom
}

// Failures reaching the workflow plane must surface as wrapped errors (500), never as the
// sentinel "not started"/"not suspended" conflicts, which would tell the operator the run
// is in the wrong state when the plane was simply unreachable.
func TestWorkflowRunLifecycle_PlaneErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("resume fails when the workflow cannot be resolved", func(t *testing.T) {
		svc := newLifecycleErrorService(t, lifecycleErrorOpts{omitWorkflow: true})
		_, err := svc.ResumeWorkflowRun(ctx, testNamespace, testRunName)
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrWorkflowRunNotStarted)
		assert.Contains(t, err.Error(), "failed to resolve workflow")
	})

	t.Run("resume fails when the plane client cannot be built", func(t *testing.T) {
		svc := newLifecycleErrorService(t, lifecycleErrorOpts{providerErr: errLifecycleBoom})
		_, err := svc.ResumeWorkflowRun(ctx, testNamespace, testRunName)
		require.ErrorIs(t, err, errLifecycleBoom)
		assert.Contains(t, err.Error(), "failed to create workflow plane client")
	})

	t.Run("resume wraps a non-not-found Argo get error", func(t *testing.T) {
		svc := newLifecycleErrorService(t, lifecycleErrorOpts{planeFuncs: interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errLifecycleBoom
			},
		}})
		_, err := svc.ResumeWorkflowRun(ctx, testNamespace, testRunName)
		require.ErrorIs(t, err, errLifecycleBoom)
		assert.NotErrorIs(t, err, ErrWorkflowRunNotStarted)
		assert.Contains(t, err.Error(), planeNamespace+"/"+argoRunName)
	})

	t.Run("resume wraps an Argo update error", func(t *testing.T) {
		svc := newLifecycleErrorService(t, lifecycleErrorOpts{planeFuncs: interceptor.Funcs{Update: failingUpdate}})
		_, err := svc.ResumeWorkflowRun(ctx, testNamespace, testRunName)
		require.ErrorIs(t, err, errLifecycleBoom)
		assert.Contains(t, err.Error(), "failed to resume workflow")
	})

	t.Run("stop of an unknown run is not found", func(t *testing.T) {
		svc := newLifecycleErrorService(t, lifecycleErrorOpts{})
		_, err := svc.StopWorkflowRun(ctx, testNamespace, "nope", "")
		require.ErrorIs(t, err, ErrWorkflowRunNotFound)
	})

	t.Run("stop fails when the plane client cannot be built", func(t *testing.T) {
		svc := newLifecycleErrorService(t, lifecycleErrorOpts{providerErr: errLifecycleBoom})
		_, err := svc.StopWorkflowRun(ctx, testNamespace, testRunName, "")
		require.ErrorIs(t, err, errLifecycleBoom)
	})

	t.Run("stop wraps an Argo update error", func(t *testing.T) {
		svc := newLifecycleErrorService(t, lifecycleErrorOpts{planeFuncs: interceptor.Funcs{Update: failingUpdate}})
		_, err := svc.StopWorkflowRun(ctx, testNamespace, testRunName, "")
		require.ErrorIs(t, err, errLifecycleBoom)
		assert.Contains(t, err.Error(), "failed to stop workflow")
	})

	// The stop reason is the audit trail for why a run was cut short; if it cannot be
	// persisted the call must fail rather than report success with the reason lost.
	t.Run("stop fails when the reason cannot be recorded", func(t *testing.T) {
		svc := newLifecycleErrorService(t, lifecycleErrorOpts{cpFuncs: interceptor.Funcs{
			Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
				return errLifecycleBoom
			},
		}})
		_, err := svc.StopWorkflowRun(ctx, testNamespace, testRunName, "superseded")
		require.ErrorIs(t, err, errLifecycleBoom)
		assert.Contains(t, err.Error(), "failed to record stop reason")
	})
}

// Argo's terminal phases are all "completed" for stop purposes; Failed and Error must be
// rejected like Succeeded so a stop never rewrites a finished workflow.
func TestStopWorkflowRun_TerminalPhases(t *testing.T) {
	for _, phase := range []argoproj.WorkflowPhase{argoproj.WorkflowFailed, argoproj.WorkflowError} {
		t.Run(string(phase), func(t *testing.T) {
			wf := suspendedArgoWorkflow()
			wf.Status.Phase = phase
			svc := newLifecycleErrorService(t, lifecycleErrorOpts{
				argoWF:     wf,
				planeFuncs: interceptor.Funcs{Update: failingUpdate},
			})
			_, err := svc.StopWorkflowRun(context.Background(), testNamespace, testRunName, "")
			require.ErrorIs(t, err, ErrWorkflowRunCompleted)
		})
	}
}
