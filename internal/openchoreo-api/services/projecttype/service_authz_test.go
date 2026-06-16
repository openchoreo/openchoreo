// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projecttype

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projecttype/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newPTAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &projectTypeServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func TestCreateProjectType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ProjectType{ObjectMeta: metav1.ObjectMeta{Name: "my-pt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateProjectType", mock.Anything, testNamespace, resource).Return(resource, nil)
		svc := newPTAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateProjectType(testutil.AuthzContext(), testNamespace, resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "projecttype:create", "projecttype", "my-pt", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newPTAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateProjectType(testutil.AuthzContext(), testNamespace, resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateProjectType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ProjectType{ObjectMeta: metav1.ObjectMeta{Name: "my-pt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateProjectType", mock.Anything, testNamespace, resource).Return(resource, nil)
		svc := newPTAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateProjectType(testutil.AuthzContext(), testNamespace, resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "projecttype:update", "projecttype", "my-pt", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newPTAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateProjectType(testutil.AuthzContext(), testNamespace, resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetProjectType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ProjectType{ObjectMeta: metav1.ObjectMeta{Name: "my-pt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProjectType", mock.Anything, testNamespace, "my-pt").Return(resource, nil)
		svc := newPTAuthzSvc(pdp, mockSvc)
		result, err := svc.GetProjectType(testutil.AuthzContext(), testNamespace, "my-pt")
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "projecttype:view", "projecttype", "my-pt", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newPTAuthzSvc(pdp, mockSvc)
		_, err := svc.GetProjectType(testutil.AuthzContext(), testNamespace, "my-pt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteProjectType_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteProjectType", mock.Anything, testNamespace, "my-pt").Return(nil)
		svc := newPTAuthzSvc(pdp, mockSvc)
		err := svc.DeleteProjectType(testutil.AuthzContext(), testNamespace, "my-pt")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "projecttype:delete", "projecttype", "my-pt", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newPTAuthzSvc(pdp, mockSvc)
		err := svc.DeleteProjectType(testutil.AuthzContext(), testNamespace, "my-pt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetProjectTypeSchema_AuthzCheck(t *testing.T) {
	schemaResult := map[string]any{"type": "object"}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetProjectTypeSchema", mock.Anything, testNamespace, "my-pt").Return(schemaResult, nil)
		svc := newPTAuthzSvc(pdp, mockSvc)
		result, err := svc.GetProjectTypeSchema(testutil.AuthzContext(), testNamespace, "my-pt")
		require.NoError(t, err)
		require.Equal(t, schemaResult, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "projecttype:view", "projecttype", "my-pt", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newPTAuthzSvc(pdp, mockSvc)
		_, err := svc.GetProjectTypeSchema(testutil.AuthzContext(), testNamespace, "my-pt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListProjectTypes_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ProjectType{
		{ObjectMeta: metav1.ObjectMeta{Name: "pt-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pt-2"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListProjectTypes", mock.Anything, testNamespace, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ProjectType]{Items: items}, nil)
		svc := newPTAuthzSvc(pdp, mockSvc)
		result, err := svc.ListProjectTypes(testutil.AuthzContext(), testNamespace, services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "projecttype:view", "projecttype", "pt-1", authzcore.ResourceHierarchy{Namespace: testNamespace})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "projecttype:view", "projecttype", "pt-2", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListProjectTypes", mock.Anything, testNamespace, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ProjectType]{Items: items}, nil)
		svc := newPTAuthzSvc(pdp, mockSvc)
		result, err := svc.ListProjectTypes(testutil.AuthzContext(), testNamespace, services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
