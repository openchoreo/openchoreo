// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newProjectAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &projectServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func testProject(ns, name string) *openchoreov1alpha1.Project {
	return &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
}

func TestCreateProject_AuthzCheck(t *testing.T) {
	project := testProject("ns-1", "my-project")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateProject", mock.Anything, "ns-1", project).Return(project, nil)
		svc := newProjectAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateProject(testutil.AuthzContext(), "ns-1", project)
		require.NoError(t, err)
		require.Equal(t, project, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "project:create", "project", "my-project",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-project"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newProjectAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateProject(testutil.AuthzContext(), "ns-1", project)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateProject_AuthzCheck(t *testing.T) {
	project := testProject("ns-1", "my-project")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateProject", mock.Anything, "ns-1", project).Return(project, nil)
		svc := newProjectAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateProject(testutil.AuthzContext(), "ns-1", project)
		require.NoError(t, err)
		require.Equal(t, project, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "project:update", "project", "my-project",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-project"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newProjectAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateProject(testutil.AuthzContext(), "ns-1", project)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetProject_AuthzCheck(t *testing.T) {
	project := testProject("ns-1", "my-project")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProject", mock.Anything, "ns-1", "my-project").Return(project, nil)
		svc := newProjectAuthzSvc(pdp, mockSvc)
		result, err := svc.GetProject(testutil.AuthzContext(), "ns-1", "my-project")
		require.NoError(t, err)
		require.Equal(t, project, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "project:view", "project", "my-project",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-project"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newProjectAuthzSvc(pdp, mockSvc)
		_, err := svc.GetProject(testutil.AuthzContext(), "ns-1", "my-project")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteProject_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteProject", mock.Anything, "ns-1", "my-project").Return(nil)
		svc := newProjectAuthzSvc(pdp, mockSvc)
		err := svc.DeleteProject(testutil.AuthzContext(), "ns-1", "my-project")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "project:delete", "project", "my-project",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-project"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newProjectAuthzSvc(pdp, mockSvc)
		err := svc.DeleteProject(testutil.AuthzContext(), "ns-1", "my-project")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListProjects_AuthzCheck(t *testing.T) {
	projects := []openchoreov1alpha1.Project{
		{ObjectMeta: metav1.ObjectMeta{Name: "proj-1", Namespace: "ns-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "proj-2", Namespace: "ns-1"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListProjects", mock.Anything, "ns-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.Project]{Items: projects}, nil)
		svc := newProjectAuthzSvc(pdp, mockSvc)
		result, err := svc.ListProjects(testutil.AuthzContext(), "ns-1", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "project:view", "project", "proj-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "proj-1"})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "project:view", "project", "proj-2",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "proj-2"})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListProjects", mock.Anything, "ns-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.Project]{Items: projects}, nil)
		svc := newProjectAuthzSvc(pdp, mockSvc)
		result, err := svc.ListProjects(testutil.AuthzContext(), "ns-1", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
