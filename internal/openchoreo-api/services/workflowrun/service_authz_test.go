// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	ocLabels "github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun/mocks"
)

// newWR is a helper to create a WorkflowRun with optional project/component labels.
func newWR(name string) *openchoreov1alpha1.WorkflowRun {
	return &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "ns-1",
			Labels: map[string]string{
				ocLabels.LabelKeyProjectName:   "my-proj",
				ocLabels.LabelKeyComponentName: "my-comp",
			},
		},
	}
}

// --- constructHierarchyForAuthzCheck ---

func TestConstructHierarchyForAuthzCheck(t *testing.T) {
	t.Run("with project and component labels", func(t *testing.T) {
		labels := map[string]string{
			ocLabels.LabelKeyProjectName:   "my-proj",
			ocLabels.LabelKeyComponentName: "my-comp",
		}
		h := constructHierarchyForAuthzCheck("ns-1", labels)
		require.Equal(t, authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"}, h)
	})

	t.Run("without labels — namespace fallback", func(t *testing.T) {
		h := constructHierarchyForAuthzCheck("ns-1", map[string]string{})
		require.Equal(t, authzcore.ResourceHierarchy{Namespace: "ns-1"}, h)
	})

	t.Run("only project label — namespace fallback", func(t *testing.T) {
		h := constructHierarchyForAuthzCheck("ns-1", map[string]string{
			ocLabels.LabelKeyProjectName: "my-proj",
		})
		require.Equal(t, authzcore.ResourceHierarchy{Namespace: "ns-1"}, h)
	})
}

// --- CreateWorkflowRun ---

func TestCreateWorkflowRun_AuthzCheck(t *testing.T) {
	wr := newWR("run-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateWorkflowRun", mock.Anything, "ns-1", wr).Return(wr, nil)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.CreateWorkflowRun(testutil.AuthzContext(), "ns-1", wr)
		require.NoError(t, err)
		require.Equal(t, wr, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "workflowrun:create", "workflowrun", "run-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.CreateWorkflowRun(testutil.AuthzContext(), "ns-1", wr)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

// --- UpdateWorkflowRun ---

func TestUpdateWorkflowRun_AuthzCheck(t *testing.T) {
	wr := newWR("run-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateWorkflowRun", mock.Anything, "ns-1", wr).Return(wr, nil)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.UpdateWorkflowRun(testutil.AuthzContext(), "ns-1", wr)
		require.NoError(t, err)
		require.Equal(t, wr, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "workflowrun:update", "workflowrun", "run-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.UpdateWorkflowRun(testutil.AuthzContext(), "ns-1", wr)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

// --- GetWorkflowRun ---

func TestGetWorkflowRun_AuthzCheck(t *testing.T) {
	wr := newWR("run-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetWorkflowRun", mock.Anything, "ns-1", "run-1").Return(wr, nil)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.GetWorkflowRun(testutil.AuthzContext(), "ns-1", "run-1")
		require.NoError(t, err)
		require.Equal(t, wr, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "workflowrun:view", "workflowrun", "run-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetWorkflowRun", mock.Anything, "ns-1", "run-1").Return(wr, nil)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetWorkflowRun(testutil.AuthzContext(), "ns-1", "run-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetWorkflowRun", mock.Anything, "ns-1", "run-1").Return(nil, fetchErr)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetWorkflowRun(testutil.AuthzContext(), "ns-1", "run-1")
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured)
	})
}

// --- GetWorkflowRunLogs ---

func TestGetWorkflowRunLogs_AuthzCheck(t *testing.T) {
	wr := newWR("run-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		entries := []models.WorkflowRunLogEntry{{}}
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetWorkflowRun", mock.Anything, "ns-1", "run-1").Return(wr, nil)
		mockSvc.On("GetWorkflowRunLogs", mock.Anything, "ns-1", "run-1", "task-1", "http://gw", (*int64)(nil)).Return(entries, nil)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.GetWorkflowRunLogs(testutil.AuthzContext(), "ns-1", "run-1", "task-1", "http://gw", nil)
		require.NoError(t, err)
		require.Equal(t, entries, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "workflowrun:view", "workflowrun", "run-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetWorkflowRun", mock.Anything, "ns-1", "run-1").Return(wr, nil)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetWorkflowRunLogs(testutil.AuthzContext(), "ns-1", "run-1", "task-1", "http://gw", nil)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetWorkflowRun", mock.Anything, "ns-1", "run-1").Return(nil, fetchErr)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetWorkflowRunLogs(testutil.AuthzContext(), "ns-1", "run-1", "task-1", "http://gw", nil)
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured)
	})
}

// --- GetWorkflowRunEvents ---

func TestGetWorkflowRunEvents_AuthzCheck(t *testing.T) {
	wr := newWR("run-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		events := []models.WorkflowRunEventEntry{{}}
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetWorkflowRun", mock.Anything, "ns-1", "run-1").Return(wr, nil)
		mockSvc.On("GetWorkflowRunEvents", mock.Anything, "ns-1", "run-1", "task-1", "http://gw").Return(events, nil)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.GetWorkflowRunEvents(testutil.AuthzContext(), "ns-1", "run-1", "task-1", "http://gw")
		require.NoError(t, err)
		require.Equal(t, events, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "workflowrun:view", "workflowrun", "run-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetWorkflowRun", mock.Anything, "ns-1", "run-1").Return(wr, nil)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetWorkflowRunEvents(testutil.AuthzContext(), "ns-1", "run-1", "task-1", "http://gw")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetWorkflowRun", mock.Anything, "ns-1", "run-1").Return(nil, fetchErr)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetWorkflowRunEvents(testutil.AuthzContext(), "ns-1", "run-1", "task-1", "http://gw")
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured)
	})
}

// --- GetWorkflowRunStatus ---

func TestGetWorkflowRunStatus_AuthzCheck(t *testing.T) {
	wr := newWR("run-1")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		status := &models.WorkflowRunStatusResponse{}
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetWorkflowRun", mock.Anything, "ns-1", "run-1").Return(wr, nil)
		mockSvc.On("GetWorkflowRunStatus", mock.Anything, "ns-1", "run-1", "http://gw").Return(status, nil)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.GetWorkflowRunStatus(testutil.AuthzContext(), "ns-1", "run-1", "http://gw")
		require.NoError(t, err)
		require.Equal(t, status, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "workflowrun:view", "workflowrun", "run-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetWorkflowRun", mock.Anything, "ns-1", "run-1").Return(wr, nil)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetWorkflowRunStatus(testutil.AuthzContext(), "ns-1", "run-1", "http://gw")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetWorkflowRun", mock.Anything, "ns-1", "run-1").Return(nil, fetchErr)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetWorkflowRunStatus(testutil.AuthzContext(), "ns-1", "run-1", "http://gw")
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured)
	})
}

// --- TriggerWorkflow ---

func TestTriggerWorkflow_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		resp := &models.WorkflowRunTriggerResponse{}
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("TriggerWorkflow", mock.Anything, "ns-1", "my-proj", "my-comp", "abc123").Return(resp, nil)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.TriggerWorkflow(testutil.AuthzContext(), "ns-1", "my-proj", "my-comp", "abc123")
		require.NoError(t, err)
		require.Equal(t, resp, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "workflowrun:create", "workflowrun", "my-comp",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.TriggerWorkflow(testutil.AuthzContext(), "ns-1", "my-proj", "my-comp", "abc123")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

// --- ListWorkflowRuns ---

func TestListWorkflowRuns_AuthzCheck(t *testing.T) {
	wr1 := newWR("run-1")
	wr2 := newWR("run-2")

	t.Run("all allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListWorkflowRuns", mock.Anything, "ns-1", "my-proj", "my-comp", "wf-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.WorkflowRun]{
			Items: []openchoreov1alpha1.WorkflowRun{*wr1, *wr2},
		}, nil)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListWorkflowRuns(testutil.AuthzContext(), "ns-1", "my-proj", "my-comp", "wf-1", services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListWorkflowRuns", mock.Anything, "ns-1", "my-proj", "my-comp", "wf-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.WorkflowRun]{
			Items: []openchoreov1alpha1.WorkflowRun{*wr1, *wr2},
		}, nil)
		svc := &workflowRunServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListWorkflowRuns(testutil.AuthzContext(), "ns-1", "my-proj", "my-comp", "wf-1", services.ListOptions{})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
