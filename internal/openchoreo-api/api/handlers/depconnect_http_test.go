// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/depconnect"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	authmw "github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// hierarchyPath mirrors the real PDP (internal/authz/casbin): the request path is built
// from the hierarchy alone — Resource.ID is not part of it — and a policy matches only
// when the policy path is a prefix of the request path. Modeling it this way means a
// request whose hierarchy is too coarse fails here exactly as it does in production.
func hierarchyPath(h authzcore.ResourceHierarchy) string {
	path := ""
	if h.Namespace != "" {
		path = "ns/" + h.Namespace
	}
	if h.Project != "" {
		path += "/project/" + h.Project
	}
	if h.Component != "" {
		path += "/component/" + h.Component
	}
	return strings.Trim(path, "/")
}

// recordingPDP records every authorization request and allows one whose hierarchy falls
// under any of grants (policy resource paths, as a role binding produces). No grants
// means a single cluster-wide wildcard.
type recordingPDP struct {
	grants   []string
	err      error
	requests []authzcore.EvaluateRequest
}

func (p *recordingPDP) decide(req authzcore.EvaluateRequest) authzcore.Decision {
	p.requests = append(p.requests, req)
	grants := p.grants
	if len(grants) == 0 {
		grants = []string{"*"}
	}
	path := hierarchyPath(req.Resource.Hierarchy)
	for _, g := range grants {
		if g == "*" || path == g || strings.HasPrefix(path, g+"/") {
			return authzcore.Decision{Decision: true, Context: &authzcore.DecisionContext{}}
		}
	}
	return authzcore.Decision{Decision: false, Context: &authzcore.DecisionContext{}}
}

func (p *recordingPDP) Evaluate(_ context.Context, req *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	if p.err != nil {
		return nil, p.err
	}
	d := p.decide(*req)
	return &d, nil
}

func (p *recordingPDP) BatchEvaluate(_ context.Context, req *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	if p.err != nil {
		return nil, p.err
	}
	resp := &authzcore.BatchEvaluateResponse{Decisions: make([]authzcore.Decision, 0, len(req.Requests))}
	for _, r := range req.Requests {
		resp.Decisions = append(resp.Decisions, p.decide(r))
	}
	return resp, nil
}

func (p *recordingPDP) GetSubjectProfile(context.Context, *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, nil
}

// checkedProviders returns the "project/component" of every authorization request made.
func (p *recordingPDP) checkedProviders() []string {
	out := make([]string, 0, len(p.requests))
	for _, r := range p.requests {
		out = append(out, r.Resource.Hierarchy.Project+"/"+r.Resource.ID)
	}
	return out
}

// providerBinding builds a deployed provider component publishing one endpoint.
func providerBinding(project, component string) *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: project + "-" + component + "-development", Namespace: "default"},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: project, ComponentName: component},
			Environment: "development",
		},
		Status: openchoreov1alpha1.ReleaseBindingStatus{
			Endpoints: []openchoreov1alpha1.EndpointURLStatus{{
				Name: httpStr,
				ServiceURL: &openchoreov1alpha1.EndpointURL{
					Scheme: httpStr, Host: component + ".dp-" + project + ".svc.cluster.local", Port: 8080, Path: "/api",
				},
			}},
		},
	}
}

// newResolveHandler wires a DepConnectHandler over two deployed providers: one in the
// caller's own project (doclet/backend-api) and one in another project
// (finance/ledger-svc), so cross-project authorization is exercised.
func newResolveHandler(t *testing.T, pdp authzcore.PDP) (*DepConnectHandler, ed25519.PrivateKey) {
	t.Helper()
	env, dp := testEnvironmentAndDataPlane()
	cl := fake.NewClientBuilder().WithScheme(depConnectScheme(t)).
		WithObjects(env, dp, providerBinding("doclet", "backend-api"), providerBinding("finance", "ledger-svc")).
		Build()

	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &DepConnectHandler{
		k8sClient:    cl,
		authzChecker: svcpkg.NewAuthzChecker(pdp, logger),
		signer:       &capabilitySigner{privKey: priv, keyID: "k1", issuer: "cp", ttl: 30 * time.Minute},
		logger:       logger,
	}, priv
}

func endpointDep(project, component, envVar string) depconnect.EndpointDep {
	return depconnect.EndpointDep{
		Project: project, Component: component, Name: httpStr, Visibility: "namespace",
		EnvBindings: depconnect.EndpointEnvBindings{Address: envVar},
	}
}

// validResolveRequest depends on one component in its own project and one in another.
func validResolveRequest() depconnect.ResolveRequest {
	return depconnect.ResolveRequest{
		Namespace:   "default",
		Project:     "doclet",
		Component:   "doclet-document",
		Environment: "development",
		Endpoints: []depconnect.EndpointDep{
			endpointDep("doclet", "backend-api", "BACKEND_API_URL"),
			endpointDep("finance", "ledger-svc", "LEDGER_URL"),
		},
	}
}

// postResolve drives the handler through ServeHTTP as an authenticated caller.
func postResolve(t *testing.T, h *DepConnectHandler, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf []byte
	switch v := body.(type) {
	case string:
		buf = []byte(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		buf = b
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dep-connect:resolve", bytes.NewReader(buf))
	ctx := authmw.SetSubjectContext(req.Context(), &authmw.SubjectContext{ID: "user:alice", Type: "user"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

func decodeResolve(t *testing.T, rec *httptest.ResponseRecorder) depconnect.ResolveResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp depconnect.ResolveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func unconnectableRefs(resp depconnect.ResolveResponse) []string {
	out := make([]string, 0, len(resp.Unconnectable))
	for _, u := range resp.Unconnectable {
		out = append(out, u.Ref)
	}
	return out
}

// TestResolveHTTPAuthorizesEachDependency pins the authorization model: one check per
// dependency component, in that component's own project, and none against the caller's
// self-declared component (which need not exist).
func TestResolveHTTPAuthorizesEachDependency(t *testing.T) {
	pdp := &recordingPDP{}
	h, priv := newResolveHandler(t, pdp)

	resp := decodeResolve(t, postResolve(t, h, validResolveRequest()))
	if len(resp.Targets) != 2 {
		t.Fatalf("expected both dependencies to resolve, got %+v (unconnectable: %v)", resp.Targets, unconnectableRefs(resp))
	}

	checked := pdp.checkedProviders()
	for _, want := range []string{"doclet/backend-api", "finance/ledger-svc"} {
		if !slices.Contains(checked, want) {
			t.Errorf("no authorization check for dependency %s; checked %v", want, checked)
		}
	}
	if slices.Contains(checked, "doclet/doclet-document") {
		t.Error("authorized the caller's own component; it is self-declared and may not exist")
	}
	if len(checked) != 2 {
		t.Errorf("expected exactly one check per dependency, got %v", checked)
	}

	for _, r := range pdp.requests {
		if r.Action != authzcore.ActionConnectComponent {
			t.Errorf("action = %q, want %q", r.Action, authzcore.ActionConnectComponent)
		}
		if r.Resource.Type != "component" {
			t.Errorf("resource type = %q, want component", r.Resource.Type)
		}
		if r.Resource.Hierarchy.Namespace != "default" {
			t.Errorf("namespace = %q, want default", r.Resource.Hierarchy.Namespace)
		}
		// component:connect is environment-conditioned; without this, per-env policies pass silently.
		if want := "default/development"; r.Context.Resource.Environment != want {
			t.Errorf("environment attribute = %q, want %q", r.Context.Resource.Environment, want)
		}
	}

	claims, err := depconnect.VerifyCapability(resp.Capability, priv.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatalf("verify capability: %v", err)
	}
	if claims.Subject != "user:alice" {
		t.Errorf("capability subject = %q, want user:alice", claims.Subject)
	}
}

// TestResolveHTTPDeniesUnauthorizedDependency is the cross-project gate: a dependency the
// caller's role cannot reach is dropped, the rest still tunnel, and no capability target
// is minted for it.
func TestResolveHTTPDeniesUnauthorizedDependency(t *testing.T) {
	pdp := &recordingPDP{grants: []string{"ns/default/project/doclet"}}
	h, priv := newResolveHandler(t, pdp)

	resp := decodeResolve(t, postResolve(t, h, validResolveRequest()))

	if len(resp.Targets) != 1 || resp.Targets[0].Key != "ep/doclet/backend-api/http" {
		t.Fatalf("expected only the authorized dependency to resolve, got %+v", resp.Targets)
	}
	if got := unconnectableRefs(resp); len(got) != 1 || got[0] != "ep/finance/ledger-svc/http" {
		t.Fatalf("unconnectable = %v, want [ep/ledger-svc/http]", got)
	}

	// The capability must not carry a dial target for the denied dependency — it is the
	// only thing the dep-agent checks before opening a stream.
	claims, err := depconnect.VerifyCapability(resp.Capability, priv.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatalf("verify capability: %v", err)
	}
	if _, ok := claims.TargetByKey("ep/finance/ledger-svc/http"); ok {
		t.Fatal("capability signed a target the caller is not authorized for")
	}
}

// TestResolveHTTPHidesWhyADependencyIsUnavailable: unauthorized and non-existent must be
// indistinguishable, or the reason text becomes a way to enumerate other projects.
func TestResolveHTTPHidesWhyADependencyIsUnavailable(t *testing.T) {
	denied := &recordingPDP{grants: []string{"ns/default/project/doclet"}}
	hDenied, _ := newResolveHandler(t, denied)
	deniedResp := decodeResolve(t, postResolve(t, hDenied, validResolveRequest()))

	// Same request shape, but the dependency simply does not exist.
	missing := &recordingPDP{}
	hMissing, _ := newResolveHandler(t, missing)
	req := validResolveRequest()
	req.Endpoints[1] = endpointDep("finance", "no-such-component", "LEDGER_URL")
	missingResp := decodeResolve(t, postResolve(t, hMissing, req))

	if len(deniedResp.Unconnectable) != 1 || len(missingResp.Unconnectable) != 1 {
		t.Fatalf("expected one unconnectable each, got %d and %d",
			len(deniedResp.Unconnectable), len(missingResp.Unconnectable))
	}
	if deniedResp.Unconnectable[0].Reason != missingResp.Unconnectable[0].Reason {
		t.Errorf("reason leaks why the dependency is unavailable:\n denied  = %q\n missing = %q",
			deniedResp.Unconnectable[0].Reason, missingResp.Unconnectable[0].Reason)
	}
	for _, bad := range []string{"no-such-component", "ledger-svc", "release binding", "permission"} {
		if bytes.Contains([]byte(deniedResp.Unconnectable[0].Reason), []byte(bad)) {
			t.Errorf("reason %q should not mention %q", deniedResp.Unconnectable[0].Reason, bad)
		}
	}
}

// TestResolveHTTPTunnelsForUndeployedConsumer is the inner-loop requirement: a component
// that does not exist in OpenChoreo yet must still be able to tunnel its dependencies.
func TestResolveHTTPTunnelsForUndeployedConsumer(t *testing.T) {
	pdp := &recordingPDP{}
	h, _ := newResolveHandler(t, pdp)

	req := validResolveRequest()
	req.Project = "brand-new-project"
	req.Component = "never-deployed"

	resp := decodeResolve(t, postResolve(t, h, req))
	if len(resp.Targets) != 2 {
		t.Fatalf("undeployed consumer should still tunnel its dependencies, got %+v (unconnectable: %v)",
			resp.Targets, unconnectableRefs(resp))
	}
	if slices.Contains(pdp.checkedProviders(), "brand-new-project/never-deployed") {
		t.Error("authorized a component that does not exist; only dependencies should be checked")
	}
}

// TestResolveHTTPDeduplicatesProviderChecks: two dependencies on one component cost one check.
func TestResolveHTTPDeduplicatesProviderChecks(t *testing.T) {
	pdp := &recordingPDP{}
	h, _ := newResolveHandler(t, pdp)

	req := validResolveRequest()
	second := endpointDep("doclet", "backend-api", "BACKEND_API_URL_2")
	second.Name = "second"
	req.Endpoints = []depconnect.EndpointDep{req.Endpoints[0], second}

	postResolve(t, h, req)

	if got := pdp.checkedProviders(); len(got) != 1 {
		t.Fatalf("expected one check for one distinct provider, got %v", got)
	}
}

// TestResolveHTTPDefaultsDependencyProjectToCaller: an endpoint with no project is
// authorized against the caller's project, not skipped.
func TestResolveHTTPDefaultsDependencyProjectToCaller(t *testing.T) {
	pdp := &recordingPDP{}
	h, _ := newResolveHandler(t, pdp)

	req := validResolveRequest()
	req.Endpoints = []depconnect.EndpointDep{endpointDep("", "backend-api", "BACKEND_API_URL")}

	resp := decodeResolve(t, postResolve(t, h, req))
	if len(resp.Targets) != 1 {
		t.Fatalf("expected the dependency to resolve, got unconnectable: %v", unconnectableRefs(resp))
	}
	if got := pdp.checkedProviders(); len(got) != 1 || got[0] != "doclet/backend-api" {
		t.Fatalf("checked %v, want [doclet/backend-api]", got)
	}
}

// TestResolveHTTPRejectsMalformedRequests: validation runs before authorization, so a
// malformed body never reaches the PDP or the cluster.
func TestResolveHTTPRejectsMalformedRequests(t *testing.T) {
	tests := []struct {
		name string
		body any
	}{
		{"invalid json", "{not json"},
		{"missing namespace", func() depconnect.ResolveRequest {
			r := validResolveRequest()
			r.Namespace = ""
			return r
		}()},
		{"missing project", func() depconnect.ResolveRequest {
			r := validResolveRequest()
			r.Project = ""
			return r
		}()},
		{"missing component", func() depconnect.ResolveRequest {
			r := validResolveRequest()
			r.Component = ""
			return r
		}()},
		{"missing environment", func() depconnect.ResolveRequest {
			r := validResolveRequest()
			r.Environment = ""
			return r
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdp := &recordingPDP{}
			h, _ := newResolveHandler(t, pdp)

			rec := postResolve(t, h, tt.body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if len(pdp.requests) != 0 {
				t.Errorf("a malformed request must be rejected before authorization, got %d checks", len(pdp.requests))
			}
		})
	}
}

// TestResolveHTTPAuthzFailureIsNotAllow guards fail-open: a PDP error must 500, never
// fall through to a resolved capability.
func TestResolveHTTPAuthzFailureIsNotAllow(t *testing.T) {
	pdp := &recordingPDP{err: errors.New("pdp unreachable")}
	h, _ := newResolveHandler(t, pdp)

	rec := postResolve(t, h, validResolveRequest())

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("capability")) {
		t.Fatalf("an authorization failure must not return a capability: %s", rec.Body.String())
	}
}

// TestResolveMissingAuthzCheckerFailsClosed: a handler built without a checker must not
// resolve every dependency.
func TestResolveMissingAuthzCheckerFailsClosed(t *testing.T) {
	h, _ := newResolveHandler(t, &recordingPDP{})
	h.authzChecker = nil

	if _, err := h.resolve(context.Background(), validResolveRequest(), "user:alice"); err == nil {
		t.Fatal("expected resolve to fail closed with no authorization checker")
	}
}

// TestResolveHTTPHonoursScopedGrants walks the scope ladder a role binding can express.
// The component-scoped case is the regression: the PDP builds the request path from the
// hierarchy alone, so omitting Component made the request too coarse to ever match a
// component-scoped policy — project and wider scopes still matched, which is what made
// the gap easy to miss.
func TestResolveHTTPHonoursScopedGrants(t *testing.T) {
	tests := []struct {
		name       string
		grants     []string
		wantTunnel []string
	}{
		{
			name:       "cluster-wide",
			grants:     nil,
			wantTunnel: []string{"ep/doclet/backend-api/http", "ep/finance/ledger-svc/http"},
		},
		{
			name:       "namespace scope",
			grants:     []string{"ns/default"},
			wantTunnel: []string{"ep/doclet/backend-api/http", "ep/finance/ledger-svc/http"},
		},
		{
			name:       "project scope",
			grants:     []string{"ns/default/project/finance"},
			wantTunnel: []string{"ep/finance/ledger-svc/http"},
		},
		{
			name:       "component scope",
			grants:     []string{"ns/default/project/finance/component/ledger-svc"},
			wantTunnel: []string{"ep/finance/ledger-svc/http"},
		},
		{
			name:       "component scope on a component we do not depend on",
			grants:     []string{"ns/default/project/finance/component/something-else"},
			wantTunnel: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newResolveHandler(t, &recordingPDP{grants: tt.grants})

			resp := decodeResolve(t, postResolve(t, h, validResolveRequest()))

			got := make([]string, 0, len(resp.Targets))
			for _, target := range resp.Targets {
				got = append(got, target.Key)
			}
			slices.Sort(got)
			want := slices.Clone(tt.wantTunnel)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("tunneled %v, want %v (unconnectable: %v)", got, want, unconnectableRefs(resp))
			}
		})
	}
}

// TestResolveHTTPRequestCarriesFullHierarchy: the authorization request must name the
// component in its hierarchy, not only in ResourceID, or component-scoped bindings can
// never match.
func TestResolveHTTPRequestCarriesFullHierarchy(t *testing.T) {
	pdp := &recordingPDP{}
	h, _ := newResolveHandler(t, pdp)

	postResolve(t, h, validResolveRequest())

	wantPaths := []string{
		"ns/default/project/doclet/component/backend-api",
		"ns/default/project/finance/component/ledger-svc",
	}
	got := make([]string, 0, len(pdp.requests))
	for _, r := range pdp.requests {
		got = append(got, hierarchyPath(r.Resource.Hierarchy))
	}
	slices.Sort(got)
	slices.Sort(wantPaths)
	if !slices.Equal(got, wantPaths) {
		t.Fatalf("authorization paths = %v, want %v", got, wantPaths)
	}
}

// TestResolveHTTPDistinguishesSameNamedComponentsAcrossProjects is the collision
// regression: two projects may each own a component called "api". If the target key
// omits the project both collapse to one key, and the agent's capability lookup returns
// whichever host was signed first — silently tunneling one dependency to the other's
// service.
func TestResolveHTTPDistinguishesSameNamedComponentsAcrossProjects(t *testing.T) {
	env, dp := testEnvironmentAndDataPlane()
	cl := fake.NewClientBuilder().WithScheme(depConnectScheme(t)).
		WithObjects(env, dp, providerBinding("payments", "api"), providerBinding("billing", "api")).
		Build()
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &DepConnectHandler{
		k8sClient:    cl,
		authzChecker: svcpkg.NewAuthzChecker(&recordingPDP{}, logger),
		signer:       &capabilitySigner{privKey: priv, keyID: "k1", issuer: "cp", ttl: 30 * time.Minute},
		logger:       logger,
	}

	req := validResolveRequest()
	req.Endpoints = []depconnect.EndpointDep{
		endpointDep("payments", "api", "PAYMENTS_URL"),
		endpointDep("billing", "api", "BILLING_URL"),
	}

	resp := decodeResolve(t, postResolve(t, h, req))
	if len(resp.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %+v (unconnectable: %v)", resp.Targets, unconnectableRefs(resp))
	}

	keys := make([]string, 0, 2)
	for _, target := range resp.Targets {
		keys = append(keys, target.Key)
	}
	slices.Sort(keys)
	want := []string{"ep/billing/api/http", "ep/payments/api/http"}
	if !slices.Equal(keys, want) {
		t.Fatalf("target keys = %v, want %v", keys, want)
	}

	// Each key must resolve to its own provider's host in the signed capability.
	claims, err := depconnect.VerifyCapability(resp.Capability, priv.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatalf("verify capability: %v", err)
	}
	for _, tc := range []struct{ key, wantHost string }{
		{"ep/payments/api/http", "api.dp-payments.svc.cluster.local"},
		{"ep/billing/api/http", "api.dp-billing.svc.cluster.local"},
	} {
		target, ok := claims.TargetByKey(tc.key)
		if !ok {
			t.Fatalf("capability has no target for %s", tc.key)
		}
		if target.Host != tc.wantHost {
			t.Errorf("%s -> host %q, want %q", tc.key, target.Host, tc.wantHost)
		}
	}

	// The two targets must be served by different agents (one per provider namespace);
	// a shared key would have collapsed them onto one.
	if resp.Targets[0].AgentID == resp.Targets[1].AgentID {
		t.Errorf("both targets share agent %q; each provider project needs its own",
			resp.Targets[0].AgentID)
	}
}
