// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	ocLabels "github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

// hierarchyPDP allows a request only when the evaluated resource hierarchy matches
// allowedProject/allowedComponent, and records every hierarchy it was asked about so
// tests can assert which owner the authz decision was actually made against.
type hierarchyPDP struct {
	allowedProject   string
	allowedComponent string
	seen             []authzcore.ResourceHierarchy
}

func (p *hierarchyPDP) Evaluate(_ context.Context, req *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	p.seen = append(p.seen, req.Resource.Hierarchy)
	allowed := req.Resource.Hierarchy.Project == p.allowedProject &&
		req.Resource.Hierarchy.Component == p.allowedComponent
	return &authzcore.Decision{Decision: allowed, Context: &authzcore.DecisionContext{}}, nil
}

func (p *hierarchyPDP) BatchEvaluate(_ context.Context, _ *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	return nil, nil
}

func (p *hierarchyPDP) GetSubjectProfile(_ context.Context, _ *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, nil
}

func newOwnedWorkflowRun(project, component string) *openchoreov1alpha1.WorkflowRun {
	run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, testRunName)
	run.Labels = map[string]string{
		ocLabels.LabelKeyProjectName:   project,
		ocLabels.LabelKeyComponentName: component,
	}
	return run
}

// TestUpdateWorkflowRunAuthzUsesStoredOwnership is the regression test for the
// authorization bypass fixed by #4596: UpdateWorkflowRun must authorize against the
// ownership labels of the stored WorkflowRun, never against the labels supplied in
// the request body.
func TestUpdateWorkflowRunAuthzUsesStoredOwnership(t *testing.T) {
	ctx := context.Background()

	t.Run("denies when the stored owner is not authorized even if the body claims an authorized owner", func(t *testing.T) {
		stored := newOwnedWorkflowRun("proj-b", "comp-b")
		fakeClient := testutil.NewFakeClient(stored)
		// The caller is only entitled to proj-a/comp-a, not to the stored owner.
		pdp := &hierarchyPDP{allowedProject: "proj-a", allowedComponent: "comp-a"}
		svc := NewServiceWithAuthz(fakeClient, nil, nil, pdp, testutil.TestLogger())

		body := newOwnedWorkflowRun("proj-a", "comp-a")
		body.Annotations = map[string]string{"caller": "proj-a"}

		_, err := svc.UpdateWorkflowRun(ctx, testNamespace, body)
		require.ErrorIs(t, err, services.ErrForbidden)

		require.Len(t, pdp.seen, 1)
		assert.Equal(t, "proj-b", pdp.seen[0].Project, "authz must use the stored project, not the request body")
		assert.Equal(t, "comp-b", pdp.seen[0].Component, "authz must use the stored component, not the request body")
		assert.Equal(t, testNamespace, pdp.seen[0].Namespace)

		reread := &openchoreov1alpha1.WorkflowRun{}
		require.NoError(t, fakeClient.Get(ctx, client.ObjectKey{Name: testRunName, Namespace: testNamespace}, reread))
		assert.Equal(t, "proj-b", reread.Labels[ocLabels.LabelKeyProjectName])
		assert.Equal(t, "comp-b", reread.Labels[ocLabels.LabelKeyComponentName])
		assert.NotContains(t, reread.Annotations, "caller")
	})

	t.Run("positive control: allows when the stored owner is authorized", func(t *testing.T) {
		stored := newOwnedWorkflowRun("proj-b", "comp-b")
		fakeClient := testutil.NewFakeClient(stored)
		pdp := &hierarchyPDP{allowedProject: "proj-b", allowedComponent: "comp-b"}
		svc := NewServiceWithAuthz(fakeClient, nil, nil, pdp, testutil.TestLogger())

		body := newOwnedWorkflowRun("proj-b", "comp-b")
		body.Annotations = map[string]string{"note": "updated"}

		result, err := svc.UpdateWorkflowRun(ctx, testNamespace, body)
		require.NoError(t, err)
		assert.Equal(t, "updated", result.Annotations["note"])

		require.Len(t, pdp.seen, 1)
		assert.Equal(t, "proj-b", pdp.seen[0].Project)
		assert.Equal(t, "comp-b", pdp.seen[0].Component)
	})

	t.Run("nil input is rejected before any authz evaluation", func(t *testing.T) {
		pdp := &hierarchyPDP{allowedProject: "proj-b", allowedComponent: "comp-b"}
		svc := NewServiceWithAuthz(testutil.NewFakeClient(), nil, nil, pdp, testutil.TestLogger())

		_, err := svc.UpdateWorkflowRun(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
		assert.Empty(t, pdp.seen)
	})

	t.Run("missing workflow run is rejected before any authz evaluation", func(t *testing.T) {
		pdp := &hierarchyPDP{allowedProject: "proj-b", allowedComponent: "comp-b"}
		svc := NewServiceWithAuthz(testutil.NewFakeClient(), nil, nil, pdp, testutil.TestLogger())

		body := newOwnedWorkflowRun("proj-b", "comp-b")

		_, err := svc.UpdateWorkflowRun(ctx, testNamespace, body)
		require.ErrorIs(t, err, ErrWorkflowRunNotFound)
		assert.Empty(t, pdp.seen)
	})
}
