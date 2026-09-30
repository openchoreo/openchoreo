// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun_test

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	authzmocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun"
	wfrmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun/mocks"
)

// Resume and stop are operator interventions on a live run: both are gated by the single
// workflowrun:resume action, evaluated with the hierarchy and workflow attribute read from
// the stored run (so a caller cannot widen their own scope through the request).
func TestResumeAndStopWorkflowRun_Authz(t *testing.T) {
	run := newWorkflowRun(testRunName, testProjectName, testComponentName)

	isResumeCheck := func(req *authz.EvaluateRequest) bool {
		return req.Action == authz.ActionResumeWorkflowRun &&
			req.Resource.Type == workflowrun.ExportResourceType &&
			req.Resource.ID == testRunName &&
			req.Resource.Hierarchy.Namespace == testNamespace &&
			req.Resource.Hierarchy.Project == testProjectName &&
			req.Resource.Hierarchy.Component == testComponentName &&
			req.Context.Resource.Workflow == testNamespace+"/"+testWorkflowName
	}

	t.Run("resume allowed delegates", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)
		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.MatchedBy(isResumeCheck)).Return(allowDecision(), nil)
		mockSvc.EXPECT().ResumeWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)

		result, err := newAuthzService(t, mockSvc, mockPDP).ResumeWorkflowRun(ctxWithSubject(), testNamespace, testRunName)
		require.NoError(t, err)
		require.Equal(t, run, result)
	})

	t.Run("resume denied does not touch the plane", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)
		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(denyDecision(), nil)

		_, err := newAuthzService(t, mockSvc, mockPDP).ResumeWorkflowRun(ctxWithSubject(), testNamespace, testRunName)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("stop allowed delegates with reason", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)
		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.MatchedBy(isResumeCheck)).Return(allowDecision(), nil)
		mockSvc.EXPECT().StopWorkflowRun(mock.Anything, testNamespace, testRunName, "why").Return(run, nil)

		_, err := newAuthzService(t, mockSvc, mockPDP).StopWorkflowRun(ctxWithSubject(), testNamespace, testRunName, "why")
		require.NoError(t, err)
	})

	t.Run("stop denied", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)
		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(denyDecision(), nil)

		_, err := newAuthzService(t, mockSvc, mockPDP).StopWorkflowRun(ctxWithSubject(), testNamespace, testRunName, "")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found skips authz", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)
		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, "nope").Return(nil, workflowrun.ErrWorkflowRunNotFound)

		_, err := newAuthzService(t, mockSvc, mockPDP).ResumeWorkflowRun(ctxWithSubject(), testNamespace, "nope")
		require.ErrorIs(t, err, workflowrun.ErrWorkflowRunNotFound)
	})
}
