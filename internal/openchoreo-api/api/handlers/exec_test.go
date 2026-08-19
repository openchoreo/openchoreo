// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newExecHandler(t *testing.T, pdp *testutil.CapturingPDP, objs ...client.Object) *ExecHandler {
	t.Helper()
	return &ExecHandler{
		k8sClient:    fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(objs...).Build(),
		authzChecker: testutil.NewTestAuthzChecker(pdp),
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// The exec authz check must carry the target component in its resource
// hierarchy, otherwise a component-scoped role binding can never match the
// request path and exec is denied. The fake client has no Component, so the
// request fails right after the check — but by then the PDP has captured the
// evaluate request.
func TestExecHandler_AuthzHierarchyIncludesComponent(t *testing.T) {
	pdp := testutil.AllowPDP()
	h := newExecHandler(t, pdp)

	req := httptest.NewRequest(http.MethodGet,
		"/exec/namespaces/default/components/greeter-service?env=development&project=default",
		nil).WithContext(testutil.AuthzContext())
	h.ServeHTTP(httptest.NewRecorder(), req)

	require.Len(t, pdp.Captured, 1, "authz check should run before pod resolution")
	testutil.RequireEvalRequest(t, pdp.Captured[0],
		authz.ActionExecComponent, "component", "greeter-service",
		authz.ResourceHierarchy{Namespace: "default", Project: "default", Component: "greeter-service"})
}
