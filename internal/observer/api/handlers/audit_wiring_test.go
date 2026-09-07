// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	authzmocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/api/internalgen"
	observeraudit "github.com/openchoreo/openchoreo/internal/observer/audit"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/server/middleware"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

const testIncidentID = "incident-42"

// authAs returns a middleware standing in for the JWT layer: it puts a
// SubjectContext on the request context the way jwt.Middleware does, so
// audit.ExtractActor has a real subject to read.
func authAs(subjectID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := auth.SetSubjectContext(r.Context(), &auth.SubjectContext{
				ID:   subjectID,
				Type: "user",
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// incidentHandler builds a Handler whose UpdateIncident returns resp/err,
// with no authorization wrapper. For the tests that only care how a service
// error maps onto an audit result.
func incidentHandler(t *testing.T, resp *gen.IncidentPutResponse, err error) *Handler {
	t.Helper()

	svc := servicemocks.NewMockAlertIncidentService(t)
	svc.On("UpdateIncident", mock.Anything, mock.Anything, mock.Anything).Return(resp, err).Maybe()

	return &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: svc,
	}
}

// incidentHandlerWithAuthz builds a Handler over the *real* authorization
// wrapper, with the incident's scope stubbed and a PDP returning allow.
//
// The hierarchy on an audit event comes from
// observerAuthz.CheckAuthorization, so a test that mocks
// AlertIncidentService directly never exercises it. Driving the wrapper is
// what proves the authz-path fix actually populates the event.
func incidentHandlerWithAuthz(
	t *testing.T, resp *gen.IncidentPutResponse, namespace, project, component string, allow bool,
) *Handler {
	t.Helper()

	svc := servicemocks.NewMockAlertIncidentService(t)
	svc.On("IncidentScope", mock.Anything, mock.Anything).
		Return(namespace, project, component, nil).Maybe()
	svc.On("UpdateIncident", mock.Anything, mock.Anything, mock.Anything).Return(resp, nil).Maybe()

	pdp := authzmocks.NewMockPDP(t)
	pdp.On("Evaluate", mock.Anything, mock.Anything).
		Return(&authzcore.Decision{Decision: allow}, nil).Maybe()

	return &Handler{
		baseHandler: baseHandler{logger: noopLogger()},
		alertIncidentService: service.NewAlertIncidentServiceWithAuthz(
			svc, pdp, noopLogger()),
	}
}

// incidentResponse builds the service response for a successful update.
//
// It carries no labels: the audit event's hierarchy comes from the authz
// check, not from the response, so nothing here needs to supply it.
func incidentResponse(id string) *gen.IncidentPutResponse {
	return &gen.IncidentPutResponse{IncidentId: &id}
}

func updateIncidentRequest() *http.Request {
	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/incidents/"+testIncidentID,
		strings.NewReader(`{"status":"resolved"}`))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// TestUpdateIncidentAuditEvent drives the one audited operation through the
// real composed chain. Two assertions carry the weight:
//
//   - actor.id must be the subject's ID, not "anonymous" — proving audit sits
//     inside auth, since outside it every event would emit as anonymous with
//     nothing failing.
//   - resource.id must be the incident ID — proving the strict handler's ctx
//     descends from the one the audit middleware seeded.
func TestUpdateIncidentAuditEvent(t *testing.T) {
	t.Parallel()

	id := testIncidentID
	h := incidentHandlerWithAuthz(t, incidentResponse(id), "ns-a", "proj-b", "comp-c", true)

	emitter, sink := newAuditSink(t)
	srv := newPublicServerWithAudit(t, h, authAs("user-7"), emitter)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, updateIncidentRequest())
	require.Equal(t, http.StatusOK, rr.Code)

	events := auditEvents(t, sink)
	require.Len(t, events, 1, "a successful UpdateIncident must emit exactly one audit event")
	event := events[0]

	assert.Equal(t, "UpdateIncident", event["operation_id"])
	assert.Equal(t, "success", event["result"])

	actor, ok := event["actor"].(map[string]any)
	require.True(t, ok, "event must carry an actor group")
	assert.Equal(t, "user-7", actor["id"],
		"actor must come from the SubjectContext — audit sitting outside auth would make this "+
			"\"anonymous\" with nothing else failing")

	resource, ok := event["resource"].(map[string]any)
	require.True(t, ok, "event must carry a resource group")
	assert.Equal(t, testIncidentID, resource["id"],
		"SetResource must reach the audit middleware's context via the strict handler's ctx")

	// Hierarchy is rendered into the same resource group as namespace/project/
	// component. Without it the event cannot be filtered by tenant, and no
	// policy selector keyed on hierarchy can match it.
	assert.Equal(t, "ns-a", resource["namespace"])
	assert.Equal(t, "proj-b", resource["project"])
	assert.Equal(t, "comp-c", resource["component"])
}

// TestUpdateIncidentAuditEventOnFailure covers a service error becoming a 500,
// which determineResult maps to "failure" — not "denied", which is what a 403
// maps to (see TestUpdateIncidentAuditEventOnDenial).
//
// Resource identity still arrives, seeded from the path by the middleware
// before the handler runs — as resource.name, not resource.id: the pre-call
// seed sets only Namespace and Name, and the handler's SetResource (which
// fills id) never runs when the update fails.
func TestUpdateIncidentAuditEventOnFailure(t *testing.T) {
	t.Parallel()

	h := incidentHandler(t, nil, assert.AnError)

	emitter, sink := newAuditSink(t)
	srv := newPublicServerWithAudit(t, h, authAs("user-7"), emitter)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, updateIncidentRequest())
	require.Equal(t, http.StatusInternalServerError, rr.Code)

	events := auditEvents(t, sink)
	require.Len(t, events, 1, "a failed UpdateIncident must still emit exactly one audit event")
	event := events[0]

	assert.Equal(t, "UpdateIncident", event["operation_id"])
	assert.Equal(t, "failure", event["result"])

	actor, ok := event["actor"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "user-7", actor["id"])

	resource, ok := event["resource"].(map[string]any)
	require.True(t, ok, "a failed request must still carry the path-derived resource identity")
	assert.Equal(t, testIncidentID, resource["name"],
		"the middleware's pre-call seed fills name from RESTResourceParam, so a failed update "+
			"still identifies which incident was targeted")
}

// TestUpdateIncidentAuditEventOnDenial covers the security-interesting case
// of an authenticated subject refused by policy.
//
// ErrAuthzForbidden becomes a 403, which determineResult maps to "denied" —
// distinct from the 500/"failure" case above. The actor must still be the real
// subject, since a denial with no attribution is the one event shape an audit
// trail cannot afford.
func TestUpdateIncidentAuditEventOnDenial(t *testing.T) {
	t.Parallel()

	// A real PDP denial through the real authz wrapper, not a service error:
	// that is the only way to prove hierarchy is recorded *before* the
	// decision, and so survives a refusal.
	h := incidentHandlerWithAuthz(t, nil, "ns-a", "proj-b", "comp-c", false)

	emitter, sink := newAuditSink(t)
	srv := newPublicServerWithAudit(t, h, authAs("user-7"), emitter)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, updateIncidentRequest())
	require.Equal(t, http.StatusForbidden, rr.Code)

	events := auditEvents(t, sink)
	require.Len(t, events, 1, "a denied UpdateIncident must emit exactly one audit event")
	event := events[0]

	assert.Equal(t, "UpdateIncident", event["operation_id"])
	assert.Equal(t, "denied", event["result"])

	actor, ok := event["actor"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "user-7", actor["id"],
		"a policy denial must record who was refused")

	resource, ok := event["resource"].(map[string]any)
	require.True(t, ok, "a denied request must still carry the path-derived resource identity")
	assert.Equal(t, testIncidentID, resource["name"])

	// The point of recording hierarchy in CheckAuthorization rather than in
	// the handler: it is set before the PDP evaluates, so a refusal still
	// says which tenant was targeted.
	assert.Equal(t, "ns-a", resource["namespace"])
	assert.Equal(t, "proj-b", resource["project"])
	assert.Equal(t, "comp-c", resource["component"])
}

// TestAuthRejectionEmitsUnauthenticatedEvent covers a 401 on a protected
// route being recorded, stamped OriginAPI. Only works with the
// unauthenticated-audit middleware outside auth — auth never calls next, so
// the inner audit middleware cannot see the rejection.
func TestAuthRejectionEmitsUnauthenticatedEvent(t *testing.T) {
	t.Parallel()

	rejectAll := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
	}

	h := incidentHandler(t, nil, nil)

	emitter, sink := newAuditSink(t)
	srv := newPublicServerWithAudit(t, h, rejectAll, emitter)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, updateIncidentRequest())
	require.Equal(t, http.StatusUnauthorized, rr.Code)

	events := auditEvents(t, sink)
	require.Len(t, events, 1,
		"a 401 must emit exactly one event — two would mean the inner and outer audit "+
			"middlewares both fired for the same request")
	event := events[0]

	assert.Equal(t, "api", event["origin"], "a REST rejection must not be stamped as MCP")
	assert.Equal(t, "unauthenticated", event["result"])

	actor, ok := event["actor"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "anonymous", actor["id"],
		"a rejected request has no subject, so the actor is genuinely anonymous here")

	// Emitted with a nil Operation, so such an event is selectable only by
	// origins/results/actor_types/actors — see the coverage matrix.
	assert.NotContains(t, event, "operation_id")
}

// TestMCPMiddlewaresAuditUnauthenticated covers the MCP counterpart of
// TestAuthRejectionEmitsUnauthenticatedEvent, and the ordering hazard behind
// it: MCPMiddlewares is Chain-ordered (first
// outermost) while the generated slices are the reverse. Get it backwards and
// JWTAuth short-circuits before the audit middleware runs, so an MCP token
// rejection emits nothing.
//
// Driven through middleware.Chain, as routes.Group(...).Handle does in
// production, rather than restating the slice's contents.
func TestMCPMiddlewaresAuditUnauthenticated(t *testing.T) {
	t.Parallel()

	rejectAll := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
	}
	passThrough := func(next http.Handler) http.Handler { return next }

	emitter, sink := newAuditSink(t)
	mws, err := MCPMiddlewares(MCPMiddlewareOptions{
		Auth401:      passThrough,
		JWTAuth:      rejectAll,
		AuditEmitter: emitter,
		AuditEnabled: true,
	})
	require.NoError(t, err)

	reached := false
	handler := middleware.Chain(mws...)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`)))

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	require.False(t, reached, "auth must short-circuit before the MCP server")

	events := auditEvents(t, sink)
	require.Len(t, events, 1,
		"an MCP 401 must emit exactly one event — none means the audit middleware sits inside "+
			"auth and never runs; two means it is nested with another instance")

	event := events[0]
	assert.Equal(t, "mcp", event["origin"],
		"an MCP rejection stamped as api would misattribute it to the REST surface")
	assert.Equal(t, "unauthenticated", event["result"])
}

// TestMCPMiddlewaresRequireDependencies pins that every dependency is checked
// at startup rather than nil-panicking on the first /mcp request.
func TestMCPMiddlewaresRequireDependencies(t *testing.T) {
	t.Parallel()

	passThrough := func(next http.Handler) http.Handler { return next }
	full := MCPMiddlewareOptions{
		Auth401:      passThrough,
		JWTAuth:      passThrough,
		AuditEmitter: noopAuditEmitter(t),
	}

	missingAuth401 := full
	missingAuth401.Auth401 = nil
	_, err := MCPMiddlewares(missingAuth401)
	require.Error(t, err, "a nil Auth401 must be rejected")

	missingJWT := full
	missingJWT.JWTAuth = nil
	_, err = MCPMiddlewares(missingJWT)
	require.Error(t, err, "a nil JWTAuth must be rejected")

	missingEmitter := full
	missingEmitter.AuditEmitter = nil
	_, err = MCPMiddlewares(missingEmitter)
	require.Error(t, err, "a nil AuditEmitter must be rejected")
}

// TestInternalSpecHasNoAuditedOperationsToday pins that the internal port's
// audit middleware is currently a pass-through, and that it is empty *because
// of the exemptions* — naming each one, so lifting any of them fails here.
// Without that, an empty set caused by an OperationsIn bug would look
// identical to the intended state.
func TestInternalSpecHasNoAuditedOperationsToday(t *testing.T) {
	t.Parallel()

	swagger, err := internalgen.GetSwagger()
	require.NoError(t, err)

	assert.Empty(t, observeraudit.OperationsIn(swagger),
		"no internal operation is audited yet; if this fails, an exemption lifted — confirm the "+
			"event's actor is meaningful now that port 8081 records one")

	for _, id := range []string{"CreateAlertRule", "UpdateAlertRule", "DeleteAlertRule", "HandleAlertWebhook"} {
		assert.Contains(t, observeraudit.RESTExemptions, id,
			"%s must be exempted for the emptiness above to be intentional", id)
	}
}

// TestHealthEmitsNoAuditEvent pins the other side of spec-driven auth: /health
// is public and is a GET, so neither audit middleware has anything to record.
// A liveness probe generating an audit event every few seconds would drown the
// trail.
func TestHealthEmitsNoAuditEvent(t *testing.T) {
	t.Parallel()

	healthSvc := servicemocks.NewMockHealthChecker(t)
	healthSvc.On("Check", mock.Anything).Return(nil).Maybe()
	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		healthService: healthSvc,
	}

	emitter, sink := newAuditSink(t)
	srv := newPublicServerWithAudit(t, h, passThroughAuth, emitter)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	require.Equal(t, http.StatusOK, rr.Code)

	assert.Empty(t, auditEvents(t, sink))
}
