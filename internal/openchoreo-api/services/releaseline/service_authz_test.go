// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releaseline/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

var compHierarchy = authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"}

func newAuthzService(internal Service, pdp authzcore.PDP) *releaseLineServiceWithAuthz {
	return &releaseLineServiceWithAuthz{
		internal:  internal,
		k8sClient: testutil.NewFakeClient(testutil.NewComponent("ns-1", "my-proj", "my-comp")),
		authz:     testutil.NewTestAuthzChecker(pdp),
	}
}

func TestListReleaseLines_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		opts := services.ListOptions{Limit: 10}
		lines := &services.ListResult[openchoreov1alpha1.ReleaseLine]{Items: []openchoreov1alpha1.ReleaseLine{{ObjectMeta: metav1.ObjectMeta{Name: "main"}}}}
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListReleaseLines", mock.Anything, "ns-1", "my-comp", opts).Return(lines, nil)

		result, err := newAuthzService(mockSvc, pdp).ListReleaseLines(testutil.AuthzContext(), "ns-1", "my-comp", opts)
		require.NoError(t, err)
		require.Equal(t, lines, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "releaseline:view", "releaseline", "", compHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		mockSvc := mocks.NewMockService(t)
		_, err := newAuthzService(mockSvc, testutil.DenyPDP()).ListReleaseLines(testutil.AuthzContext(), "ns-1", "my-comp", services.ListOptions{})
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("unknown component", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		_, err := newAuthzService(mockSvc, pdp).ListReleaseLines(testutil.AuthzContext(), "ns-1", "missing", services.ListOptions{})
		require.ErrorIs(t, err, ErrComponentNotFound)
		require.Empty(t, pdp.Captured, "authz should not be called when the component is missing")
	})
}

func TestReleaseLineCRUD_AuthzCheck(t *testing.T) {
	line := &openchoreov1alpha1.ReleaseLine{ObjectMeta: metav1.ObjectMeta{Name: "next"}}
	cases := []struct {
		name   string
		action string
		method string
		args   []any
		ret    []any
		call   func(svc *releaseLineServiceWithAuthz) error
	}{
		{
			name: "get", action: "releaseline:view", method: "GetReleaseLine",
			args: []any{"ns-1", "my-comp", "next"}, ret: []any{line, nil},
			call: func(svc *releaseLineServiceWithAuthz) error {
				_, err := svc.GetReleaseLine(testutil.AuthzContext(), "ns-1", "my-comp", "next")
				return err
			},
		},
		{
			name: "create", action: "releaseline:create", method: "CreateReleaseLine",
			args: []any{"ns-1", "my-comp", line}, ret: []any{line, nil},
			call: func(svc *releaseLineServiceWithAuthz) error {
				_, err := svc.CreateReleaseLine(testutil.AuthzContext(), "ns-1", "my-comp", line)
				return err
			},
		},
		{
			name: "update", action: "releaseline:update", method: "UpdateReleaseLine",
			args: []any{"ns-1", "my-comp", line}, ret: []any{line, nil},
			call: func(svc *releaseLineServiceWithAuthz) error {
				_, err := svc.UpdateReleaseLine(testutil.AuthzContext(), "ns-1", "my-comp", line)
				return err
			},
		},
		{
			name: "delete", action: "releaseline:delete", method: "DeleteReleaseLine",
			args: []any{"ns-1", "my-comp", "next"}, ret: []any{nil},
			call: func(svc *releaseLineServiceWithAuthz) error {
				return svc.DeleteReleaseLine(testutil.AuthzContext(), "ns-1", "my-comp", "next")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+" allowed", func(t *testing.T) {
			pdp := testutil.AllowPDP()
			mockSvc := mocks.NewMockService(t)
			mockSvc.On(tc.method, append([]any{mock.Anything}, tc.args...)...).Return(tc.ret...)

			require.NoError(t, tc.call(newAuthzService(mockSvc, pdp)))
			require.Len(t, pdp.Captured, 1)
			testutil.RequireEvalRequest(t, pdp.Captured[0], tc.action, "releaseline", "next", compHierarchy)
			require.Equal(t, "next", pdp.Captured[0].Context.Resource.ReleaseLine)
		})

		t.Run(tc.name+" denied", func(t *testing.T) {
			mockSvc := mocks.NewMockService(t)
			require.ErrorIs(t, tc.call(newAuthzService(mockSvc, testutil.DenyPDP())), services.ErrForbidden)
			mockSvc.AssertNotCalled(t, tc.method)
		})
	}
}
