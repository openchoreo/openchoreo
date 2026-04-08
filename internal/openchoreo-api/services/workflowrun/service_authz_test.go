// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	authzmocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	ocLabels "github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun"
	wfrmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun/mocks"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

const (
	testNamespace     = "test-ns"
	testWorkflowName  = "test-workflow"
	testRunName       = "test-run"
	testProjectName   = "proj-a"
	testComponentName = "comp-a"
)

func ctxWithSubject() context.Context {
	return auth.SetSubjectContext(context.Background(), &auth.SubjectContext{
		ID:                "user-1",
		Type:              "user",
		EntitlementClaim:  "groups",
		EntitlementValues: []string{"org-admins"},
	})
}

// allowDecision returns a PDP allow decision.
func allowDecision() *authz.Decision {
	return &authz.Decision{Decision: true, Context: &authz.DecisionContext{Reason: "allowed"}}
}

// denyDecision returns a PDP deny decision.
func denyDecision() *authz.Decision {
	return &authz.Decision{Decision: false, Context: &authz.DecisionContext{Reason: "denied"}}
}

// newAuthzService creates a workflowRunServiceWithAuthz with mock Service and mock PDP.
func newAuthzService(t *testing.T, mockSvc *wfrmocks.MockService, mockPDP *authzmocks.MockPDP) workflowrun.Service {
	t.Helper()
	return workflowrun.NewTestServiceWithAuthz(mockSvc, mockPDP, testutil.TestLogger())
}

// newWorkflowRun creates a test WorkflowRun with optional project/component labels.
func newWorkflowRun(name, project, component string) *openchoreov1alpha1.WorkflowRun {
	run := testutil.NewWorkflowRun(testNamespace, testWorkflowName, name)
	if project != "" || component != "" {
		run.Labels = map[string]string{
			ocLabels.LabelKeyProjectName:   project,
			ocLabels.LabelKeyComponentName: component,
		}
	}
	return run
}

// ---------------------------------------------------------------------------
// constructHierarchyForAuthzCheck tests
// ---------------------------------------------------------------------------

func TestConstructHierarchyForAuthzCheck(t *testing.T) {
	t.Run("with project and component labels", func(t *testing.T) {
		labels := map[string]string{
			ocLabels.LabelKeyProjectName:   "my-project",
			ocLabels.LabelKeyComponentName: "my-comp",
		}
		h := workflowrun.ExportConstructHierarchy("ns-1", labels)
		assert.Equal(t, "ns-1", h.Namespace)
		assert.Equal(t, "my-project", h.Project)
		assert.Equal(t, "my-comp", h.Component)
	})

	t.Run("with only project label falls back to namespace", func(t *testing.T) {
		labels := map[string]string{
			ocLabels.LabelKeyProjectName: "my-project",
		}
		h := workflowrun.ExportConstructHierarchy("ns-1", labels)
		assert.Equal(t, "ns-1", h.Namespace)
		assert.Empty(t, h.Project)
		assert.Empty(t, h.Component)
	})

	t.Run("with only component label falls back to namespace", func(t *testing.T) {
		labels := map[string]string{
			ocLabels.LabelKeyComponentName: "my-comp",
		}
		h := workflowrun.ExportConstructHierarchy("ns-1", labels)
		assert.Equal(t, "ns-1", h.Namespace)
		assert.Empty(t, h.Project)
		assert.Empty(t, h.Component)
	})

	t.Run("empty labels falls back to namespace", func(t *testing.T) {
		h := workflowrun.ExportConstructHierarchy("ns-1", map[string]string{})
		assert.Equal(t, "ns-1", h.Namespace)
		assert.Empty(t, h.Project)
		assert.Empty(t, h.Component)
	})

	t.Run("nil labels falls back to namespace", func(t *testing.T) {
		h := workflowrun.ExportConstructHierarchy("ns-1", nil)
		assert.Equal(t, "ns-1", h.Namespace)
		assert.Empty(t, h.Project)
		assert.Empty(t, h.Component)
	})
}

// ---------------------------------------------------------------------------
// CreateWorkflowRun authz tests
// ---------------------------------------------------------------------------

func TestCreateWorkflowRun_Authz(t *testing.T) {
	t.Run("allowed delegates to internal service", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		run := newWorkflowRun(testRunName, testProjectName, testComponentName)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(allowDecision(), nil)
		mockSvc.EXPECT().CreateWorkflowRun(mock.Anything, testNamespace, run).Return(run, nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		result, err := svc.CreateWorkflowRun(ctxWithSubject(), testNamespace, run)
		require.NoError(t, err)
		assert.Equal(t, testRunName, result.Name)
	})

	t.Run("denied returns forbidden without delegating", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		run := newWorkflowRun(testRunName, testProjectName, testComponentName)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(denyDecision(), nil)
		// mockSvc.CreateWorkflowRun should NOT be called

		svc := newAuthzService(t, mockSvc, mockPDP)
		_, err := svc.CreateWorkflowRun(ctxWithSubject(), testNamespace, run)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("checks correct action and hierarchy from labels", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		run := newWorkflowRun(testRunName, testProjectName, testComponentName)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.MatchedBy(func(req *authz.EvaluateRequest) bool {
			return req.Action == authz.ActionCreateWorkflowRun &&
				req.Resource.Type == workflowrun.ExportResourceType &&
				req.Resource.ID == testRunName &&
				req.Resource.Hierarchy.Namespace == testNamespace &&
				req.Resource.Hierarchy.Project == testProjectName &&
				req.Resource.Hierarchy.Component == testComponentName
		})).Return(allowDecision(), nil)
		mockSvc.EXPECT().CreateWorkflowRun(mock.Anything, testNamespace, run).Return(run, nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		_, err := svc.CreateWorkflowRun(ctxWithSubject(), testNamespace, run)
		require.NoError(t, err)
	})
}

// ---------------------------------------------------------------------------
// UpdateWorkflowRun authz tests
// ---------------------------------------------------------------------------

func TestUpdateWorkflowRun_Authz(t *testing.T) {
	t.Run("allowed delegates to internal service", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		update := newWorkflowRun(testRunName, "", "")
		update.Labels = map[string]string{"env": "prod"}
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(allowDecision(), nil)
		mockSvc.EXPECT().UpdateWorkflowRun(mock.Anything, testNamespace, update).Return(update, nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		result, err := svc.UpdateWorkflowRun(ctxWithSubject(), testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, "prod", result.Labels["env"])
	})

	t.Run("denied returns forbidden without delegating", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		update := newWorkflowRun(testRunName, "", "")
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(denyDecision(), nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		_, err := svc.UpdateWorkflowRun(ctxWithSubject(), testNamespace, update)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("checks correct action", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		update := newWorkflowRun(testRunName, "", "")
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.MatchedBy(func(req *authz.EvaluateRequest) bool {
			return req.Action == authz.ActionUpdateWorkflowRun &&
				req.Resource.Type == workflowrun.ExportResourceType
		})).Return(allowDecision(), nil)
		mockSvc.EXPECT().UpdateWorkflowRun(mock.Anything, testNamespace, update).Return(update, nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		_, err := svc.UpdateWorkflowRun(ctxWithSubject(), testNamespace, update)
		require.NoError(t, err)
	})
}

// ---------------------------------------------------------------------------
// ListWorkflowRuns authz tests
// ---------------------------------------------------------------------------

func TestListWorkflowRuns_Authz(t *testing.T) {
	r1 := newWorkflowRun("run-1", testProjectName, testComponentName)
	r2 := newWorkflowRun("run-2", "proj-b", "comp-b")

	t.Run("all allowed", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockSvc.EXPECT().ListWorkflowRuns(mock.Anything, testNamespace, "", "", "", mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.WorkflowRun]{Items: []openchoreov1alpha1.WorkflowRun{*r1, *r2}}, nil)
		// FilteredList calls Check per item (uses Evaluate, not BatchEvaluate)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(allowDecision(), nil).Times(2)

		svc := newAuthzService(t, mockSvc, mockPDP)
		result, err := svc.ListWorkflowRuns(ctxWithSubject(), testNamespace, "", "", "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
	})

	t.Run("all denied returns empty", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockSvc.EXPECT().ListWorkflowRuns(mock.Anything, testNamespace, "", "", "", mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.WorkflowRun]{Items: []openchoreov1alpha1.WorkflowRun{*r1, *r2}}, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(denyDecision(), nil).Times(2)

		svc := newAuthzService(t, mockSvc, mockPDP)
		result, err := svc.ListWorkflowRuns(ctxWithSubject(), testNamespace, "", "", "", services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("partial authorization filters results", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockSvc.EXPECT().ListWorkflowRuns(mock.Anything, testNamespace, "", "", "", mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.WorkflowRun]{Items: []openchoreov1alpha1.WorkflowRun{*r1, *r2}}, nil)
		// Allow only items in proj-a
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, req *authz.EvaluateRequest) (*authz.Decision, error) {
				allowed := req.Resource.Hierarchy.Project == testProjectName
				return &authz.Decision{Decision: allowed, Context: &authz.DecisionContext{Reason: "filtered"}}, nil
			}).Times(2)

		svc := newAuthzService(t, mockSvc, mockPDP)
		result, err := svc.ListWorkflowRuns(ctxWithSubject(), testNamespace, "", "", "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "run-1", result.Items[0].Name)
	})
}

// ---------------------------------------------------------------------------
// GetWorkflowRun authz tests
// ---------------------------------------------------------------------------

func TestGetWorkflowRun_Authz(t *testing.T) {
	run := newWorkflowRun(testRunName, testProjectName, testComponentName)

	t.Run("allowed delegates to internal service", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		// Authz wrapper calls GetWorkflowRun to fetch the resource, then checks authz
		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(allowDecision(), nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		result, err := svc.GetWorkflowRun(ctxWithSubject(), testNamespace, testRunName)
		require.NoError(t, err)
		assert.Equal(t, testRunName, result.Name)
	})

	t.Run("denied returns forbidden", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(denyDecision(), nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		_, err := svc.GetWorkflowRun(ctxWithSubject(), testNamespace, testRunName)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found does not check authz", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, "nonexistent").
			Return(nil, workflowrun.ErrWorkflowRunNotFound)
		// PDP.Evaluate should NOT be called

		svc := newAuthzService(t, mockSvc, mockPDP)
		_, err := svc.GetWorkflowRun(ctxWithSubject(), testNamespace, "nonexistent")
		require.ErrorIs(t, err, workflowrun.ErrWorkflowRunNotFound)
	})

	t.Run("checks view action with hierarchy from fetched labels", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.MatchedBy(func(req *authz.EvaluateRequest) bool {
			return req.Action == authz.ActionViewWorkflowRun &&
				req.Resource.Hierarchy.Namespace == testNamespace &&
				req.Resource.Hierarchy.Project == testProjectName &&
				req.Resource.Hierarchy.Component == testComponentName
		})).Return(allowDecision(), nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		_, err := svc.GetWorkflowRun(ctxWithSubject(), testNamespace, testRunName)
		require.NoError(t, err)
	})
}

// ---------------------------------------------------------------------------
// DeleteWorkflowRun authz tests
// ---------------------------------------------------------------------------

func TestDeleteWorkflowRun_Authz(t *testing.T) {
	run := newWorkflowRun(testRunName, testProjectName, testComponentName)

	t.Run("allowed delegates to internal service", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(allowDecision(), nil)
		mockSvc.EXPECT().DeleteWorkflowRun(mock.Anything, testNamespace, testRunName).Return(nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		err := svc.DeleteWorkflowRun(ctxWithSubject(), testNamespace, testRunName)
		require.NoError(t, err)
	})

	t.Run("denied returns forbidden without deleting", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(denyDecision(), nil)
		// mockSvc.DeleteWorkflowRun should NOT be called

		svc := newAuthzService(t, mockSvc, mockPDP)
		err := svc.DeleteWorkflowRun(ctxWithSubject(), testNamespace, testRunName)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found does not check authz", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, "nonexistent").
			Return(nil, workflowrun.ErrWorkflowRunNotFound)
		// PDP.Evaluate should NOT be called

		svc := newAuthzService(t, mockSvc, mockPDP)
		err := svc.DeleteWorkflowRun(ctxWithSubject(), testNamespace, "nonexistent")
		require.ErrorIs(t, err, workflowrun.ErrWorkflowRunNotFound)
	})

	t.Run("checks delete action with hierarchy from fetched labels", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.MatchedBy(func(req *authz.EvaluateRequest) bool {
			return req.Action == authz.ActionDeleteWorkflowRun &&
				req.Resource.Type == workflowrun.ExportResourceType &&
				req.Resource.ID == testRunName &&
				req.Resource.Hierarchy.Namespace == testNamespace &&
				req.Resource.Hierarchy.Project == testProjectName &&
				req.Resource.Hierarchy.Component == testComponentName
		})).Return(allowDecision(), nil)
		mockSvc.EXPECT().DeleteWorkflowRun(mock.Anything, testNamespace, testRunName).Return(nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		err := svc.DeleteWorkflowRun(ctxWithSubject(), testNamespace, testRunName)
		require.NoError(t, err)
	})
}

// ---------------------------------------------------------------------------
// GetWorkflowRunLogs authz tests
// ---------------------------------------------------------------------------

func TestGetWorkflowRunLogs_Authz(t *testing.T) {
	run := newWorkflowRun(testRunName, testProjectName, testComponentName)

	t.Run("denied before fetching logs", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		// Authz wrapper fetches the run first via GetWorkflowRun, then checks authz
		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(denyDecision(), nil)
		// mockSvc.GetWorkflowRunLogs should NOT be called

		svc := newAuthzService(t, mockSvc, mockPDP)
		_, err := svc.GetWorkflowRunLogs(ctxWithSubject(), testNamespace, testRunName, "", nil)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found does not check authz", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, "nonexistent").
			Return(nil, workflowrun.ErrWorkflowRunNotFound)

		svc := newAuthzService(t, mockSvc, mockPDP)
		_, err := svc.GetWorkflowRunLogs(ctxWithSubject(), testNamespace, "nonexistent", "", nil)
		require.ErrorIs(t, err, workflowrun.ErrWorkflowRunNotFound)
	})

	t.Run("allowed delegates to internal service", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		logEntries := []models.WorkflowRunLogEntry{{Timestamp: "2026-01-01T00:00:00Z", Log: "test log"}}
		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(allowDecision(), nil)
		mockSvc.EXPECT().GetWorkflowRunLogs(mock.Anything, testNamespace, testRunName, "task-1", (*int64)(nil)).
			Return(logEntries, nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		result, err := svc.GetWorkflowRunLogs(ctxWithSubject(), testNamespace, testRunName, "task-1", nil)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "test log", result[0].Log)
	})
}

// ---------------------------------------------------------------------------
// GetWorkflowRunEvents authz tests
// ---------------------------------------------------------------------------

func TestGetWorkflowRunEvents_Authz(t *testing.T) {
	run := newWorkflowRun(testRunName, testProjectName, testComponentName)

	t.Run("denied before fetching events", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(denyDecision(), nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		_, err := svc.GetWorkflowRunEvents(ctxWithSubject(), testNamespace, testRunName, "")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found does not check authz", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, "nonexistent").
			Return(nil, workflowrun.ErrWorkflowRunNotFound)

		svc := newAuthzService(t, mockSvc, mockPDP)
		_, err := svc.GetWorkflowRunEvents(ctxWithSubject(), testNamespace, "nonexistent", "")
		require.ErrorIs(t, err, workflowrun.ErrWorkflowRunNotFound)
	})

	t.Run("allowed delegates to internal service", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		events := []models.WorkflowRunEventEntry{{Timestamp: "2026-01-01T00:00:00Z", Type: "Normal", Reason: "Started", Message: "pod started"}}
		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(allowDecision(), nil)
		mockSvc.EXPECT().GetWorkflowRunEvents(mock.Anything, testNamespace, testRunName, "task-1").
			Return(events, nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		result, err := svc.GetWorkflowRunEvents(ctxWithSubject(), testNamespace, testRunName, "task-1")
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "Started", result[0].Reason)
	})
}

// ---------------------------------------------------------------------------
// GetWorkflowRunStatus authz tests
// ---------------------------------------------------------------------------

func TestGetWorkflowRunStatus_Authz(t *testing.T) {
	run := newWorkflowRun(testRunName, testProjectName, testComponentName)

	t.Run("denied before fetching status", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(denyDecision(), nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		_, err := svc.GetWorkflowRunStatus(ctxWithSubject(), testNamespace, testRunName)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found does not check authz", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, "nonexistent").
			Return(nil, workflowrun.ErrWorkflowRunNotFound)

		svc := newAuthzService(t, mockSvc, mockPDP)
		_, err := svc.GetWorkflowRunStatus(ctxWithSubject(), testNamespace, "nonexistent")
		require.ErrorIs(t, err, workflowrun.ErrWorkflowRunNotFound)
	})

	t.Run("allowed delegates to internal service", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		statusResp := &models.WorkflowRunStatusResponse{Status: workflowrun.ExportStatusPending}
		mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(allowDecision(), nil)
		mockSvc.EXPECT().GetWorkflowRunStatus(mock.Anything, testNamespace, testRunName).Return(statusResp, nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		result, err := svc.GetWorkflowRunStatus(ctxWithSubject(), testNamespace, testRunName)
		require.NoError(t, err)
		assert.Equal(t, workflowrun.ExportStatusPending, result.Status)
	})
}

// ---------------------------------------------------------------------------
// TriggerWorkflow authz tests
// ---------------------------------------------------------------------------

func TestTriggerWorkflow_Authz(t *testing.T) {
	t.Run("denied returns forbidden without delegating", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(denyDecision(), nil)
		// mockSvc.TriggerWorkflow should NOT be called

		svc := newAuthzService(t, mockSvc, mockPDP)
		_, err := svc.TriggerWorkflow(ctxWithSubject(), testNamespace, "proj", "my-comp", "abc1234f")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("checks correct action and component-level hierarchy", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		mockPDP.EXPECT().Evaluate(mock.Anything, mock.MatchedBy(func(req *authz.EvaluateRequest) bool {
			return req.Action == authz.ActionCreateWorkflowRun &&
				req.Resource.Type == workflowrun.ExportResourceType &&
				req.Resource.ID == "my-comp" &&
				req.Resource.Hierarchy.Namespace == testNamespace &&
				req.Resource.Hierarchy.Project == "proj" &&
				req.Resource.Hierarchy.Component == "my-comp"
		})).Return(denyDecision(), nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		_, _ = svc.TriggerWorkflow(ctxWithSubject(), testNamespace, "proj", "my-comp", "abc1234f")
	})

	t.Run("allowed delegates to internal service", func(t *testing.T) {
		mockSvc := wfrmocks.NewMockService(t)
		mockPDP := authzmocks.NewMockPDP(t)

		triggerResp := &models.WorkflowRunTriggerResponse{
			Name:          "my-comp-run-abc12345",
			ComponentName: "my-comp",
			ProjectName:   "proj",
			NamespaceName: testNamespace,
			Status:        workflowrun.ExportStatusPending,
		}
		mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(allowDecision(), nil)
		mockSvc.EXPECT().TriggerWorkflow(mock.Anything, testNamespace, "proj", "my-comp", "abc1234f").
			Return(triggerResp, nil)

		svc := newAuthzService(t, mockSvc, mockPDP)
		result, err := svc.TriggerWorkflow(ctxWithSubject(), testNamespace, "proj", "my-comp", "abc1234f")
		require.NoError(t, err)
		assert.Equal(t, "my-comp", result.ComponentName)
		assert.Equal(t, workflowrun.ExportStatusPending, result.Status)
	})
}
