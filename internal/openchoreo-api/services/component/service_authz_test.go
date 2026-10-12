// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

// mockService is a local testify mock for the Service interface.
// It exists here instead of generating mocks using mockery and importing component/mocks to avoid a cyclic import
type mockService struct {
	mock.Mock
}

func newMockService(t *testing.T) *mockService {
	m := &mockService{}
	m.Mock.Test(t)
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

func (m *mockService) CreateComponent(ctx context.Context, namespaceName string, component *openchoreov1alpha1.Component) (*openchoreov1alpha1.Component, error) {
	args := m.Called(ctx, namespaceName, component)
	res, _ := args.Get(0).(*openchoreov1alpha1.Component)
	return res, args.Error(1)
}

func (m *mockService) UpdateComponent(ctx context.Context, namespaceName string, component *openchoreov1alpha1.Component) (*openchoreov1alpha1.Component, error) {
	args := m.Called(ctx, namespaceName, component)
	res, _ := args.Get(0).(*openchoreov1alpha1.Component)
	return res, args.Error(1)
}

func (m *mockService) ListComponents(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Component], error) {
	args := m.Called(ctx, namespaceName, projectName, opts)
	res, _ := args.Get(0).(*services.ListResult[openchoreov1alpha1.Component])
	return res, args.Error(1)
}

func (m *mockService) GetComponent(ctx context.Context, namespaceName, componentName string) (*openchoreov1alpha1.Component, error) {
	args := m.Called(ctx, namespaceName, componentName)
	res, _ := args.Get(0).(*openchoreov1alpha1.Component)
	return res, args.Error(1)
}

func (m *mockService) DeleteComponent(ctx context.Context, namespaceName, componentName string) error {
	args := m.Called(ctx, namespaceName, componentName)
	return args.Error(0)
}

func (m *mockService) GenerateRelease(ctx context.Context, namespaceName, componentName string, req *GenerateReleaseRequest) (*openchoreov1alpha1.ComponentRelease, error) {
	args := m.Called(ctx, namespaceName, componentName, req)
	res, _ := args.Get(0).(*openchoreov1alpha1.ComponentRelease)
	return res, args.Error(1)
}

func (m *mockService) GetComponentSchema(ctx context.Context, namespaceName, componentName string) (*extv1.JSONSchemaProps, error) {
	args := m.Called(ctx, namespaceName, componentName)
	res, _ := args.Get(0).(*extv1.JSONSchemaProps)
	return res, args.Error(1)
}

func (m *mockService) GetComponentReleaseSchema(ctx context.Context, namespaceName, releaseName, componentName string) (*extv1.JSONSchemaProps, error) {
	args := m.Called(ctx, namespaceName, releaseName, componentName)
	res, _ := args.Get(0).(*extv1.JSONSchemaProps)
	return res, args.Error(1)
}

func (m *mockService) ListReleaseLines(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ReleaseLine], error) {
	args := m.Called(ctx, namespaceName, componentName, opts)
	res, _ := args.Get(0).(*services.ListResult[openchoreov1alpha1.ReleaseLine])
	return res, args.Error(1)
}

func (m *mockService) GetReleaseLine(ctx context.Context, namespaceName, componentName, releaseLineName string) (*openchoreov1alpha1.ReleaseLine, error) {
	args := m.Called(ctx, namespaceName, componentName, releaseLineName)
	res, _ := args.Get(0).(*openchoreov1alpha1.ReleaseLine)
	return res, args.Error(1)
}

func (m *mockService) CreateReleaseLine(ctx context.Context, namespaceName, componentName string, releaseLine *openchoreov1alpha1.ReleaseLine) (*openchoreov1alpha1.ReleaseLine, error) {
	args := m.Called(ctx, namespaceName, componentName, releaseLine)
	res, _ := args.Get(0).(*openchoreov1alpha1.ReleaseLine)
	return res, args.Error(1)
}

func (m *mockService) UpdateReleaseLine(ctx context.Context, namespaceName, componentName string, releaseLine *openchoreov1alpha1.ReleaseLine) (*openchoreov1alpha1.ReleaseLine, error) {
	args := m.Called(ctx, namespaceName, componentName, releaseLine)
	res, _ := args.Get(0).(*openchoreov1alpha1.ReleaseLine)
	return res, args.Error(1)
}

func (m *mockService) DeleteReleaseLine(ctx context.Context, namespaceName, componentName, releaseLineName string) error {
	args := m.Called(ctx, namespaceName, componentName, releaseLineName)
	return args.Error(0)
}

func testComp() *openchoreov1alpha1.Component {
	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: "my-comp", Namespace: "ns-1"},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner:         openchoreov1alpha1.ComponentOwner{ProjectName: "my-proj"},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"},
		},
	}
}

var compHierarchy = authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"}

// --- CreateComponent ---

func TestCreateComponent_AuthzCheck(t *testing.T) {
	comp := testComp()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := newMockService(t)
		mockSvc.On("CreateComponent", mock.Anything, "ns-1", comp).Return(comp, nil)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.CreateComponent(testutil.AuthzContext(), "ns-1", comp)
		require.NoError(t, err)
		require.Equal(t, comp, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "component:create", "component", "my-comp", compHierarchy)
		require.Equal(t, "ns-1/web-app", pdp.Captured[0].Context.Resource.ComponentType)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := newMockService(t)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.CreateComponent(testutil.AuthzContext(), "ns-1", comp)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

// --- UpdateComponent ---

func TestUpdateComponent_AuthzCheck(t *testing.T) {
	comp := testComp()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := newMockService(t)
		mockSvc.On("UpdateComponent", mock.Anything, "ns-1", comp).Return(comp, nil)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.UpdateComponent(testutil.AuthzContext(), "ns-1", comp)
		require.NoError(t, err)
		require.Equal(t, comp, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "component:update", "component", "my-comp", compHierarchy)
		require.Equal(t, "ns-1/web-app", pdp.Captured[0].Context.Resource.ComponentType)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := newMockService(t)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.UpdateComponent(testutil.AuthzContext(), "ns-1", comp)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

// --- ListComponents ---

func TestListComponents_AuthzCheck(t *testing.T) {
	comps := []openchoreov1alpha1.Component{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "comp-1", Namespace: "ns-1"},
			Spec:       openchoreov1alpha1.ComponentSpec{Owner: openchoreov1alpha1.ComponentOwner{ProjectName: "my-proj"}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "comp-2", Namespace: "ns-1"},
			Spec:       openchoreov1alpha1.ComponentSpec{Owner: openchoreov1alpha1.ComponentOwner{ProjectName: "my-proj"}},
		},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := newMockService(t)
		mockSvc.On("ListComponents", mock.Anything, "ns-1", "my-proj", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.Component]{Items: comps}, nil)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListComponents(testutil.AuthzContext(), "ns-1", "my-proj", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "component:view", "component", "comp-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "comp-1"})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "component:view", "component", "comp-2",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "comp-2"})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := newMockService(t)
		mockSvc.On("ListComponents", mock.Anything, "ns-1", "my-proj", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.Component]{Items: comps}, nil)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListComponents(testutil.AuthzContext(), "ns-1", "my-proj", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}

// --- GetComponent ---

func TestGetComponent_AuthzCheck(t *testing.T) {
	fetched := testComp()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(fetched, nil)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.GetComponent(testutil.AuthzContext(), "ns-1", "my-comp")
		require.NoError(t, err)
		require.Equal(t, fetched, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "component:view", "component", "my-comp", compHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(fetched, nil)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetComponent(testutil.AuthzContext(), "ns-1", "my-comp")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(nil, fetchErr)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetComponent(testutil.AuthzContext(), "ns-1", "my-comp")
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured, "authz should not be called when fetch fails")
	})
}

// --- DeleteComponent ---

func TestDeleteComponent_AuthzCheck(t *testing.T) {
	fetched := testComp()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(fetched, nil)
		mockSvc.On("DeleteComponent", mock.Anything, "ns-1", "my-comp").Return(nil)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteComponent(testutil.AuthzContext(), "ns-1", "my-comp")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "component:delete", "component", "my-comp", compHierarchy)
		require.Equal(t, "ns-1/web-app", pdp.Captured[0].Context.Resource.ComponentType)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(fetched, nil)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteComponent(testutil.AuthzContext(), "ns-1", "my-comp")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(nil, fetchErr)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteComponent(testutil.AuthzContext(), "ns-1", "my-comp")
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured, "authz should not be called when fetch fails")
	})
}

// --- GenerateRelease ---

func TestGenerateRelease_AuthzCheck(t *testing.T) {
	fetched := testComp()
	genReq := &GenerateReleaseRequest{ReleaseName: "rel-1"}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		cr := &openchoreov1alpha1.ComponentRelease{}
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(fetched, nil)
		mockSvc.On("GenerateRelease", mock.Anything, "ns-1", "my-comp", genReq).Return(cr, nil)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.GenerateRelease(testutil.AuthzContext(), "ns-1", "my-comp", genReq)
		require.NoError(t, err)
		require.Equal(t, cr, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "componentrelease:create", "componentrelease", "my-comp", compHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(fetched, nil)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GenerateRelease(testutil.AuthzContext(), "ns-1", "my-comp", genReq)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(nil, fetchErr)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GenerateRelease(testutil.AuthzContext(), "ns-1", "my-comp", genReq)
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured, "authz should not be called when fetch fails")
	})
}

// --- GetComponentSchema ---

func TestGetComponentSchema_AuthzCheck(t *testing.T) {
	fetched := testComp()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		schema := &extv1.JSONSchemaProps{}
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(fetched, nil)
		mockSvc.On("GetComponentSchema", mock.Anything, "ns-1", "my-comp").Return(schema, nil)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.GetComponentSchema(testutil.AuthzContext(), "ns-1", "my-comp")
		require.NoError(t, err)
		require.Equal(t, schema, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "component:view", "component", "my-comp", compHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(fetched, nil)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetComponentSchema(testutil.AuthzContext(), "ns-1", "my-comp")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(nil, fetchErr)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetComponentSchema(testutil.AuthzContext(), "ns-1", "my-comp")
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured, "authz should not be called when fetch fails")
	})
}

// --- ListReleaseLines ---

func TestListReleaseLines_AuthzCheck(t *testing.T) {
	fetched := testComp()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		opts := services.ListOptions{Limit: 10}
		lines := &services.ListResult[openchoreov1alpha1.ReleaseLine]{Items: []openchoreov1alpha1.ReleaseLine{{ObjectMeta: metav1.ObjectMeta{Name: "main"}}}}
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(fetched, nil)
		mockSvc.On("ListReleaseLines", mock.Anything, "ns-1", "my-comp", opts).Return(lines, nil)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListReleaseLines(testutil.AuthzContext(), "ns-1", "my-comp", opts)
		require.NoError(t, err)
		require.Equal(t, lines, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "releaseline:view", "releaseline", "", compHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(fetched, nil)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.ListReleaseLines(testutil.AuthzContext(), "ns-1", "my-comp", services.ListOptions{})
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(nil, fetchErr)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.ListReleaseLines(testutil.AuthzContext(), "ns-1", "my-comp", services.ListOptions{})
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured, "authz should not be called when fetch fails")
	})
}

// --- Release line CRUD ---

func TestReleaseLineCRUD_AuthzCheck(t *testing.T) {
	line := &openchoreov1alpha1.ReleaseLine{ObjectMeta: metav1.ObjectMeta{Name: "next"}}
	cases := []struct {
		name   string
		action string
		method string
		args   []any
		ret    []any
		call   func(svc *componentServiceWithAuthz) error
	}{
		{
			name: "get", action: "releaseline:view", method: "GetReleaseLine",
			args: []any{"ns-1", "my-comp", "next"}, ret: []any{line, nil},
			call: func(svc *componentServiceWithAuthz) error {
				_, err := svc.GetReleaseLine(testutil.AuthzContext(), "ns-1", "my-comp", "next")
				return err
			},
		},
		{
			name: "create", action: "releaseline:create", method: "CreateReleaseLine",
			args: []any{"ns-1", "my-comp", line}, ret: []any{line, nil},
			call: func(svc *componentServiceWithAuthz) error {
				_, err := svc.CreateReleaseLine(testutil.AuthzContext(), "ns-1", "my-comp", line)
				return err
			},
		},
		{
			name: "update", action: "releaseline:update", method: "UpdateReleaseLine",
			args: []any{"ns-1", "my-comp", line}, ret: []any{line, nil},
			call: func(svc *componentServiceWithAuthz) error {
				_, err := svc.UpdateReleaseLine(testutil.AuthzContext(), "ns-1", "my-comp", line)
				return err
			},
		},
		{
			name: "delete", action: "releaseline:delete", method: "DeleteReleaseLine",
			args: []any{"ns-1", "my-comp", "next"}, ret: []any{nil},
			call: func(svc *componentServiceWithAuthz) error {
				return svc.DeleteReleaseLine(testutil.AuthzContext(), "ns-1", "my-comp", "next")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+" allowed", func(t *testing.T) {
			pdp := testutil.AllowPDP()
			mockSvc := newMockService(t)
			mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(testComp(), nil)
			mockSvc.On(tc.method, append([]any{mock.Anything}, tc.args...)...).Return(tc.ret...)
			svc := &componentServiceWithAuthz{internal: mockSvc, authz: testutil.NewTestAuthzChecker(pdp)}

			require.NoError(t, tc.call(svc))
			require.Len(t, pdp.Captured, 1)
			testutil.RequireEvalRequest(t, pdp.Captured[0], tc.action, "releaseline", "next", compHierarchy)
		})

		t.Run(tc.name+" denied", func(t *testing.T) {
			mockSvc := newMockService(t)
			mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(testComp(), nil)
			svc := &componentServiceWithAuthz{internal: mockSvc, authz: testutil.NewTestAuthzChecker(testutil.DenyPDP())}

			require.ErrorIs(t, tc.call(svc), services.ErrForbidden)
			mockSvc.AssertNotCalled(t, tc.method)
		})
	}
}

// --- GetComponentReleaseSchema ---

func TestGetComponentReleaseSchema_AuthzCheck(t *testing.T) {
	fetched := testComp()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		schema := &extv1.JSONSchemaProps{}
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(fetched, nil)
		mockSvc.On("GetComponentReleaseSchema", mock.Anything, "ns-1", "rel-1", "my-comp").Return(schema, nil)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.GetComponentReleaseSchema(testutil.AuthzContext(), "ns-1", "rel-1", "my-comp")
		require.NoError(t, err)
		require.Equal(t, schema, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "component:view", "component", "my-comp", compHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(fetched, nil)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetComponentReleaseSchema(testutil.AuthzContext(), "ns-1", "rel-1", "my-comp")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := newMockService(t)
		mockSvc.On("GetComponent", mock.Anything, "ns-1", "my-comp").Return(nil, fetchErr)
		svc := &componentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetComponentReleaseSchema(testutil.AuthzContext(), "ns-1", "rel-1", "my-comp")
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured, "authz should not be called when fetch fails")
	})
}

func TestFormatComponentTypeAttr(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		ref       openchoreov1alpha1.ComponentTypeRef
		want      string
	}{
		{
			name:      "namespace-scoped ComponentType is prefixed with namespace and strips workloadType",
			namespace: "ns-1",
			ref:       openchoreov1alpha1.ComponentTypeRef{Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType, Name: "deployment/web-app"},
			want:      "ns-1/web-app",
		},
		{
			name:      "empty kind defaults to namespace-scoped (matches CRD default)",
			namespace: "ns-1",
			ref:       openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"},
			want:      "ns-1/web-app",
		},
		{
			name:      "ClusterComponentType is not namespace-prefixed and strips workloadType",
			namespace: "ns-1",
			ref:       openchoreov1alpha1.ComponentTypeRef{Kind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType, Name: "deployment/web-app"},
			want:      "web-app",
		},
		{
			name:      "name without workloadType prefix is used as-is (defensive)",
			namespace: "ns-1",
			ref:       openchoreov1alpha1.ComponentTypeRef{Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType, Name: "web-app"},
			want:      "ns-1/web-app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, formatComponentTypeAttr(tt.namespace, tt.ref))
		})
	}
}
