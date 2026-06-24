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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
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

// These tests assert the exec authz check carries the target environment in its
// ABAC context. The fake client has no Component, so the request fails right
// after the check — but by then the PDP has captured the evaluate request.

func TestExecHandler_AuthzEnvironmentContext_ExplicitEnv(t *testing.T) {
	pdp := testutil.AllowPDP()
	h := newExecHandler(t, pdp)

	req := httptest.NewRequest(http.MethodGet,
		"/exec/namespaces/default/components/greeter-service?env=development&project=default",
		nil).WithContext(testutil.AuthzContext())
	h.ServeHTTP(httptest.NewRecorder(), req)

	require.Len(t, pdp.Captured, 1, "authz check should run before pod resolution")
	testutil.RequireEvalRequest(t, pdp.Captured[0],
		authz.ActionExecComponent, "component", "greeter-service",
		authz.ResourceHierarchy{Namespace: "default", Project: "default"})
	require.Equal(t,
		services.FormatDualScopedResourceName("default", "development", false),
		pdp.Captured[0].Context.Resource.Environment)
}

// When `env` is omitted, the environment is derived from the project's deployment
// pipeline (the root env) and must still reach the ABAC context.
func TestExecHandler_AuthzEnvironmentContext_DerivedEnv(t *testing.T) {
	pdp := testutil.AllowPDP()
	proj := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "default"},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default-pipeline"},
		},
	}
	// development → production, so "development" is the root (never a target).
	pipeline := &openchoreov1alpha1.DeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "default-pipeline"},
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{{
				SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: "development"},
				TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: "production"}},
			}},
		},
	}
	h := newExecHandler(t, pdp, proj, pipeline)

	req := httptest.NewRequest(http.MethodGet,
		"/exec/namespaces/default/components/greeter-service?project=default",
		nil).WithContext(testutil.AuthzContext())
	h.ServeHTTP(httptest.NewRecorder(), req)

	require.Len(t, pdp.Captured, 1)
	require.Equal(t,
		services.FormatDualScopedResourceName("default", "development", false),
		pdp.Captured[0].Context.Resource.Environment,
		"pipeline-derived environment must reach the ABAC context when env is omitted")
}
