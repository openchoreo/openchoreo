// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	authzmocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun"
	wfrmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun/mocks"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

func TestGetWorkflowRunLogs_RecordsRequestAndCount(t *testing.T) {
	run := newWorkflowRun(testRunName, testProjectName, testComponentName)
	mockSvc := wfrmocks.NewMockService(t)
	mockPDP := authzmocks.NewMockPDP(t)
	since := int64(600)
	mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, testRunName).Return(run, nil)
	mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(allowDecision(), nil)
	mockSvc.EXPECT().GetWorkflowRunLogs(mock.Anything, testNamespace, testRunName, "build", &since).
		Return([]models.WorkflowRunLogEntry{{Log: "GITHUB_TOKEN=should-not-leak"}, {Log: "b"}, {Log: "c"}}, nil)

	ctx, data := audit.NewAuditContext(ctxWithSubject(), &audit.Resource{}, audit.RequestInfo{})
	svc := newAuthzService(t, mockSvc, mockPDP)
	_, err := svc.GetWorkflowRunLogs(ctx, testNamespace, testRunName, "build", &since)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{"task": "build", "sinceSeconds": int64(600)}, data.Parameters)
	require.NotNil(t, data.Returned)
	assert.Equal(t, audit.Returned{Count: 3}, *data.Returned)
}

func TestGetWorkflowRunLogs_NotFoundRecordsRequestOnly(t *testing.T) {
	mockSvc := wfrmocks.NewMockService(t)
	mockSvc.EXPECT().GetWorkflowRun(mock.Anything, testNamespace, "nonexistent").
		Return(nil, workflowrun.ErrWorkflowRunNotFound)

	ctx, data := audit.NewAuditContext(ctxWithSubject(), &audit.Resource{}, audit.RequestInfo{})
	svc := newAuthzService(t, mockSvc, authzmocks.NewMockPDP(t))
	_, err := svc.GetWorkflowRunLogs(ctx, testNamespace, "nonexistent", "build", nil)
	require.ErrorIs(t, err, workflowrun.ErrWorkflowRunNotFound)

	assert.Equal(t, map[string]any{"task": "build"}, data.Parameters)
	assert.Nil(t, data.Returned)
}
