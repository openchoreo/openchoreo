// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	authzmocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// Mimics the JWT middleware by attaching a subject the AuthzChecker can resolve.
func wirelogsRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	ctx := auth.SetSubjectContext(req.Context(), &auth.SubjectContext{
		ID:                "user-1",
		Type:              "user",
		EntitlementClaim:  "groups",
		EntitlementValues: []string{"viewers"},
	})
	return req.WithContext(ctx)
}

func TestWirelogsHandler_RejectsMalformedPath(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"missing namespaces segment", "/api/v1/environments/development/wirelogs"},
		{"missing environments segment", "/api/v1/namespaces/ns-a/wirelogs"},
		{"wrong final segment", "/api/v1/namespaces/ns-a/environments/development/foo"},
		{"empty namespace", "/api/v1/namespaces//environments/development/wirelogs"},
		{"empty environment", "/api/v1/namespaces/ns-a/environments//wirelogs"},
		{"extra trailing path", "/api/v1/namespaces/ns-a/environments/development/wirelogs/extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Path parsing runs before authz, so a configured PDP with no
			// expectations doubles as a guard that authz isn't reached.
			pdp := authzmocks.NewMockPDP(t)
			h := &WirelogsHandler{
				authzChecker: svcpkg.NewAuthzChecker(pdp, slog.Default()),
				logger:       slog.Default(),
			}

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, wirelogsRequest(t, tt.path))

			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestWirelogsHandler_AcceptsEnvironmentWideRequest(t *testing.T) {
	// No project/component query params -> entire environment scope.
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(&authz.Decision{Decision: false, Context: &authz.DecisionContext{}}, nil)

	h := &WirelogsHandler{
		authzChecker: svcpkg.NewAuthzChecker(pdp, slog.Default()),
		logger:       slog.Default(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/api/v1/namespaces/ns-a/environments/development/wirelogs"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestWirelogsHandler_AuthzNotConfigured(t *testing.T) {
	h := &WirelogsHandler{logger: slog.Default()}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/api/v1/namespaces/ns-a/environments/development/wirelogs"))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "authorization not configured")
}

func TestWirelogsHandler_Forbidden(t *testing.T) {
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(&authz.Decision{Decision: false, Context: &authz.DecisionContext{Reason: "no wirelogs:view"}}, nil)

	h := &WirelogsHandler{
		authzChecker: svcpkg.NewAuthzChecker(pdp, slog.Default()),
		logger:       slog.Default(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/api/v1/namespaces/ns-a/environments/development/wirelogs?project=demo&component=checkout"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// captureAuthzRequest runs ServeHTTP with a PDP that records the EvaluateRequest
// and denies, so the handler returns 403 before doing any data-plane lookups.
func captureAuthzRequest(t *testing.T, path string) *authz.EvaluateRequest {
	t.Helper()

	var captured *authz.EvaluateRequest
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, req *authz.EvaluateRequest) (*authz.Decision, error) {
			require.NotNil(t, req)
			captured = req
			return &authz.Decision{Decision: false, Context: &authz.DecisionContext{}}, nil
		})

	h := &WirelogsHandler{
		authzChecker: svcpkg.NewAuthzChecker(pdp, slog.Default()),
		logger:       slog.Default(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, path))
	assert.Equal(t, http.StatusForbidden, rec.Code)

	require.NotNil(t, captured)
	return captured
}

func TestWirelogsHandler_AuthzScope_Component(t *testing.T) {
	req := captureAuthzRequest(t, "/api/v1/namespaces/ns-a/environments/development/wirelogs?project=demo&component=checkout")

	assert.Equal(t, authz.ActionViewWirelogs, req.Action, "wirelogs must check its own action, not logs:view")
	assert.Equal(t, "component", req.Resource.Type)
	assert.Equal(t, "checkout", req.Resource.ID)
	assert.Equal(t, "ns-a", req.Resource.Hierarchy.Namespace)
	assert.Equal(t, "demo", req.Resource.Hierarchy.Project)
	assert.Equal(t, "checkout", req.Resource.Hierarchy.Component)
	assert.Equal(t, "ns-a/development", req.Context.Resource.Environment,
		"resource.environment must always be set so CEL conditions can scope per env")
}

func TestWirelogsHandler_AuthzScope_ProjectOnly(t *testing.T) {
	req := captureAuthzRequest(t, "/api/v1/namespaces/ns-a/environments/development/wirelogs?project=demo")

	assert.Equal(t, "project", req.Resource.Type)
	assert.Equal(t, "demo", req.Resource.ID)
	assert.Equal(t, "ns-a", req.Resource.Hierarchy.Namespace)
	assert.Equal(t, "demo", req.Resource.Hierarchy.Project)
	assert.Empty(t, req.Resource.Hierarchy.Component)
}

func TestWirelogsHandler_ComponentWithoutProjectRejected(t *testing.T) {
	pdp := authzmocks.NewMockPDP(t)
	h := &WirelogsHandler{
		authzChecker: svcpkg.NewAuthzChecker(pdp, slog.Default()),
		logger:       slog.Default(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/api/v1/namespaces/ns-a/environments/development/wirelogs?component=checkout"))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "component filter requires project filter")
}

func TestWirelogsHandler_AuthzScope_EnvironmentWide(t *testing.T) {
	req := captureAuthzRequest(t, "/api/v1/namespaces/ns-a/environments/development/wirelogs")

	assert.Equal(t, "environment", req.Resource.Type)
	assert.Equal(t, "development", req.Resource.ID)
	assert.Equal(t, "ns-a", req.Resource.Hierarchy.Namespace)
	assert.Empty(t, req.Resource.Hierarchy.Project)
	assert.Empty(t, req.Resource.Hierarchy.Component)
	assert.Equal(t, "ns-a/development", req.Context.Resource.Environment)
}

func TestBuildGatewayWirelogsURL_AllFilters(t *testing.T) {
	h := &WirelogsHandler{gatewayURL: "https://gw.example.com:8443"}

	got, err := h.buildGatewayWirelogsURL(execPlaneInfo{
		planeType:   "dataplane",
		planeID:     "prod-cluster",
		crNamespace: "team-a",
		crName:      "prod-dp",
	}, "ns-a", "development", "shopfront", "checkout")
	require.NoError(t, err)

	assert.Contains(t, got, "https://gw.example.com:8443/api/wirelogs/dataplane/prod-cluster/team-a/prod-dp")
	assert.Contains(t, got, "namespace=ns-a")
	assert.Contains(t, got, "environment=development")
	assert.Contains(t, got, "project=shopfront")
	assert.Contains(t, got, "component=checkout")
}

func TestBuildGatewayWirelogsURL_OmitsBlankFilters(t *testing.T) {
	h := &WirelogsHandler{gatewayURL: "https://gw.example.com:8443"}

	got, err := h.buildGatewayWirelogsURL(execPlaneInfo{
		planeType:   "dataplane",
		planeID:     "prod-cluster",
		crNamespace: "team-a",
		crName:      "prod-dp",
	}, "ns-a", "development", "", "")
	require.NoError(t, err)

	assert.Contains(t, got, "namespace=ns-a")
	assert.Contains(t, got, "environment=development")
	assert.NotContains(t, got, "project=")
	assert.NotContains(t, got, "component=")
}

// --- ServeHTTP path coverage with a fake k8s client ----------------------

func allowingAuthz(t *testing.T) *svcpkg.AuthzChecker {
	t.Helper()
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(&authz.Decision{Decision: true, Context: &authz.DecisionContext{}}, nil).
		Maybe()
	return svcpkg.NewAuthzChecker(pdp, slog.Default())
}

// seedEnvAndDP returns objects representing an Environment "development" in
// namespace "ns-a" referencing a DataPlane "prod-dp" in the same namespace.
// Mirrors what resolvePlane expects to find via h.k8sClient.Get.
func seedEnvAndDP() []client.Object {
	return []client.Object{
		&openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: "development", Namespace: "ns-a"},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
					Name: "prod-dp",
				},
			},
		},
		&openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "prod-dp", Namespace: "ns-a"},
		},
	}
}

// newWirelogsK8sClient builds a controller-runtime fake client seeded with the
// supplied objects, using the package's shared test scheme.
func newWirelogsK8sClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objs...).
		Build()
}

func TestWirelogsHandler_RejectsInvalidNameParams(t *testing.T) {
	// Each case tweaks one path/query segment to fail the RFC1123 regex or
	// length cap, with all others valid.
	cases := []struct {
		name string
		path string
		want string
	}{
		{"namespace too long", "/api/v1/namespaces/" + strings.Repeat("a", 64) + "/environments/development/wirelogs", "invalid namespace parameter"},
		{"namespace uppercase", "/api/v1/namespaces/NS_A/environments/development/wirelogs", "invalid namespace parameter"},
		{"environment uppercase", "/api/v1/namespaces/ns-a/environments/DEV/wirelogs", "invalid environment parameter"},
		{"project uppercase", "/api/v1/namespaces/ns-a/environments/development/wirelogs?project=DEMO", "invalid project parameter"},
		{"component uppercase", "/api/v1/namespaces/ns-a/environments/development/wirelogs?project=demo&component=Checkout", "invalid component parameter"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &WirelogsHandler{
				authzChecker: svcpkg.NewAuthzChecker(authzmocks.NewMockPDP(t), slog.Default()),
				logger:       slog.Default(),
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, wirelogsRequest(t, tc.path))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.want)
		})
	}
}

func TestWirelogsHandler_AuthzCheckInternalError(t *testing.T) {
	// PDP returning a non-Forbidden error must map to 500, not 403.
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(nil, errors.New("policy engine down"))

	h := &WirelogsHandler{
		authzChecker: svcpkg.NewAuthzChecker(pdp, slog.Default()),
		logger:       slog.Default(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/api/v1/namespaces/ns-a/environments/development/wirelogs"))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "authorization check failed")
}

func TestWirelogsHandler_PlaneResolve_EnvironmentNotFound(t *testing.T) {
	// Empty fake client → resolvePlane's Get returns NotFound.
	h := &WirelogsHandler{
		k8sClient:    newWirelogsK8sClient(t),
		authzChecker: allowingAuthz(t),
		logger:       slog.Default(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/api/v1/namespaces/ns-a/environments/development/wirelogs"))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "failed to resolve data plane")
	assert.Contains(t, rec.Body.String(), "environment \"development\" not found")
}

func TestWirelogsHandler_PlaneResolve_EnvironmentMissingDataPlaneRef(t *testing.T) {
	// Environment exists but has no DataPlaneRef → resolvePlane returns the
	// dedicated "no data plane reference" error before doing a DataPlane lookup.
	env := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Name: "development", Namespace: "ns-a"},
	}
	h := &WirelogsHandler{
		k8sClient:    newWirelogsK8sClient(t, env),
		authzChecker: allowingAuthz(t),
		logger:       slog.Default(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/api/v1/namespaces/ns-a/environments/development/wirelogs"))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "no data plane reference")
}

// wirelogsNoFlushRecorder hides ResponseRecorder.Flush from the http.Flusher
// type assertion so the handler's "streaming unsupported" branch triggers.
type wirelogsNoFlushRecorder struct {
	rec *httptest.ResponseRecorder
}

func (n *wirelogsNoFlushRecorder) Header() http.Header         { return n.rec.Header() }
func (n *wirelogsNoFlushRecorder) Write(b []byte) (int, error) { return n.rec.Write(b) }
func (n *wirelogsNoFlushRecorder) WriteHeader(code int)        { n.rec.WriteHeader(code) }

func TestWirelogsHandler_NoFlusherSupport(t *testing.T) {
	h := &WirelogsHandler{
		k8sClient:    newWirelogsK8sClient(t, seedEnvAndDP()...),
		authzChecker: allowingAuthz(t),
		logger:       slog.Default(),
	}

	rec := &wirelogsNoFlushRecorder{rec: httptest.NewRecorder()}
	h.ServeHTTP(rec, wirelogsRequest(t, "/api/v1/namespaces/ns-a/environments/development/wirelogs"))

	assert.Equal(t, http.StatusInternalServerError, rec.rec.Code)
	assert.Contains(t, rec.rec.Body.String(), "streaming unsupported")
}

func TestWirelogsHandler_GatewayDialError(t *testing.T) {
	// Start and immediately close a test server so the URL routes nowhere.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close()

	h := &WirelogsHandler{
		k8sClient:    newWirelogsK8sClient(t, seedEnvAndDP()...),
		authzChecker: allowingAuthz(t),
		logger:       slog.Default(),
		gatewayURL:   srv.URL,
		httpClient:   srv.Client(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/api/v1/namespaces/ns-a/environments/development/wirelogs"))

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "failed to connect to data plane")
}

func TestWirelogsHandler_GatewayNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no agent available: not connected", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	h := &WirelogsHandler{
		k8sClient:    newWirelogsK8sClient(t, seedEnvAndDP()...),
		authzChecker: allowingAuthz(t),
		logger:       slog.Default(),
		gatewayURL:   srv.URL,
		httpClient:   srv.Client(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/api/v1/namespaces/ns-a/environments/development/wirelogs"))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code,
		"503 from the gateway must be propagated, not collapsed to 502")
	assert.Contains(t, rec.Body.String(), "no agent available")
}

func TestWirelogsHandler_GatewayWeirdStatusCoercedTo502(t *testing.T) {
	// Out-of-range status codes from the upstream must be normalised to 502 so
	// http.Error doesn't panic / emit an unparsable status to the client.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(299)
		_, _ = w.Write([]byte("weird"))
	}))
	defer srv.Close()

	h := &WirelogsHandler{
		k8sClient:    newWirelogsK8sClient(t, seedEnvAndDP()...),
		authzChecker: allowingAuthz(t),
		logger:       slog.Default(),
		gatewayURL:   srv.URL,
		httpClient:   srv.Client(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/api/v1/namespaces/ns-a/environments/development/wirelogs"))

	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestWirelogsHandler_HappyPath(t *testing.T) {
	// Stand up a fake gateway that emits two SSE events and closes the stream.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// resolvePlane → resolveExecPlaneInfo: empty Spec.PlaneID falls back to
		// the DataPlane name, so the URL path must encode "prod-dp" twice
		// (planeID and crName) and "ns-a" as crNamespace.
		require.Equal(t, "/api/wirelogs/dataplane/prod-dp/ns-a/prod-dp", r.URL.Path)

		// Forwarded query params should reach the gateway intact.
		require.Equal(t, "ns-a", r.URL.Query().Get("namespace"))
		require.Equal(t, "development", r.URL.Query().Get("environment"))
		require.Equal(t, "shopfront", r.URL.Query().Get("project"))

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = w.Write([]byte("data: {\"flow\":\"a\"}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = w.Write([]byte("data: {\"flow\":\"b\"}\n\n"))
	}))
	defer srv.Close()

	h := &WirelogsHandler{
		k8sClient:    newWirelogsK8sClient(t, seedEnvAndDP()...),
		authzChecker: allowingAuthz(t),
		logger:       slog.Default(),
		gatewayURL:   srv.URL,
		httpClient:   srv.Client(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t,
		"/api/v1/namespaces/ns-a/environments/development/wirelogs?project=shopfront"))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache, no-transform", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "no", rec.Header().Get("X-Accel-Buffering"))
	body := rec.Body.String()
	assert.Contains(t, body, "data: {\"flow\":\"a\"}")
	assert.Contains(t, body, "data: {\"flow\":\"b\"}")
}

func TestNewWirelogsHandler_InitialisesHTTPClient(t *testing.T) {
	// The handler must own a *http.Client distinct from the gatewayClient's so
	// the gateway client's request-level timeout doesn't kill a long-lived SSE
	// stream. We can't reach in to verify the timeout difference directly, but
	// we can at least assert the field is populated and the logger tag stuck.
	h := NewWirelogsHandler(nil, nil, "https://gw.example.com", nil, nil, slog.Default())
	require.NotNil(t, h)
	assert.NotNil(t, h.httpClient, "httpClient must be initialized")
	assert.Equal(t, "https://gw.example.com", h.gatewayURL)
	assert.NotNil(t, h.logger)

	// Client.Timeout must stay zero — an absolute request deadline would kill
	// the long-lived SSE stream. Pre-stream phases are bounded via Transport.
	assert.Zero(t, h.httpClient.Timeout,
		"Client.Timeout must be unset; it would abort the long-lived SSE stream")

	tr, ok := h.httpClient.Transport.(*http.Transport)
	require.True(t, ok, "Transport must be *http.Transport")
	assert.NotZero(t, tr.TLSHandshakeTimeout, "TLSHandshakeTimeout must be set to bound the handshake")
	assert.NotZero(t, tr.ResponseHeaderTimeout, "ResponseHeaderTimeout must be set so a stuck gateway can't block before the stream starts")
	assert.NotNil(t, tr.DialContext, "DialContext must be set to bound TCP connect")
}
