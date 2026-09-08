// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/auditconfig"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	apiaudit "github.com/openchoreo/openchoreo/internal/openchoreo-api/audit"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	dataplanemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/dataplane/mocks"
	environmentmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/environment/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
	projectmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project/mocks"
	traitsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/trait"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// uidAssigningClient wraps the fake client so Create behaves like a real API server
// for the purposes of this test: it assigns a UID and persists it, since
// fake.NewClientBuilder's tracker does not generate one on its own.
func uidAssigningClient(t *testing.T) client.WithWatch {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if err := c.Create(ctx, obj, opts...); err != nil {
					return err
				}
				obj.SetUID(types.UID(obj.GetName() + "-uid"))
				return c.Update(ctx, obj)
			},
		}).
		Build()
}

// auditRecords extracts and decodes every AUDIT-LOG line from a JSON-lines log
// buffer, in emission order.
func auditRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	for line := range strings.SplitSeq(buf.String(), "\n") {
		if !strings.Contains(line, `"msg":"AUDIT-LOG"`) {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	return records
}

// TestAuditMiddlewareWired guards against a repeat of #2588 (audit silently
// dropped during a refactor with no test catching it). It drives a real
// request through newTestHTTPHandlerWithLogger, which builds its chain via
// the same OpenAPIMiddlewares constructor production uses — removing audit from
// that function fails this test.
func TestAuditMiddlewareWired(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	fc := uidAssigningClient(t)
	svc := projectsvc.NewServiceWithAuthz(fc, &allowAllPDP{}, logger)
	services := &handlerservices.Services{ProjectService: svc}
	handler := newTestHTTPHandlerWithLogger(t, services, logger)

	body, _ := json.Marshal(gen.Project{
		Metadata: gen.ObjectMeta{Name: "audit-test-proj"},
	})
	_, rec := doRequest(t, handler, http.MethodPost, "/api/v1/namespaces/"+testNS+"/projects", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	records := auditRecords(t, &buf)
	require.Len(t, records, 1, "expected exactly one AUDIT-LOG record, got:\n%s", buf.String())
	record := records[0]

	assert.Equal(t, "create_project", record["action"])
	assert.Equal(t, "management", record["category"])
	assert.Equal(t, "success", record["result"])
	assert.Equal(t, "api", record["origin"], "REST-originated events must carry origin=api")

	actor, ok := record["actor"].(map[string]any)
	require.True(t, ok, "actor must be a nested group")
	assert.Equal(t, "test-user", actor["id"])

	resource, ok := record["resource"].(map[string]any)
	require.True(t, ok, "resource must be populated")
	assert.Equal(t, "project", resource["type"])
	assert.NotEmpty(t, resource["id"], "resource.id must carry the K8s UID, not just the mutable name")
}

// TestAuditMiddlewareWired_ProjectCRUD locks in the update/delete coverage added
// alongside create for Project: it drives all three mutating operations on the same
// project through the production chain and checks each event's action/resource
// shape, including that resource.id (the K8s UID) survives a rename via update but
// is unavailable on delete because ProjectService.DeleteProject returns only an
// error, not the deleted object.
func TestAuditMiddlewareWired_ProjectCRUD(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	fc := uidAssigningClient(t)
	svc := projectsvc.NewServiceWithAuthz(fc, &allowAllPDP{}, logger)
	services := &handlerservices.Services{ProjectService: svc}
	handler := newTestHTTPHandlerWithLogger(t, services, logger)

	createBody, _ := json.Marshal(gen.Project{
		Metadata: gen.ObjectMeta{Name: "crud-test-proj"},
	})
	_, rec := doRequest(t, handler, http.MethodPost, "/api/v1/namespaces/"+testNS+"/projects", createBody)
	require.Equal(t, http.StatusCreated, rec.Code)

	updateBody, _ := json.Marshal(gen.Project{
		Metadata: gen.ObjectMeta{Name: "crud-test-proj", Labels: &map[string]string{"tier": "updated"}},
	})
	_, rec = doRequest(t, handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/projects/crud-test-proj", updateBody)
	require.Equal(t, http.StatusOK, rec.Code)

	_, rec = doRequest(t, handler, http.MethodDelete,
		"/api/v1/namespaces/"+testNS+"/projects/crud-test-proj", nil)
	require.Equal(t, http.StatusNoContent, rec.Code)

	records := auditRecords(t, &buf)
	require.Len(t, records, 3, "expected one AUDIT-LOG record per mutation, got:\n%s", buf.String())

	actions := make([]string, len(records))
	for i, r := range records {
		actions[i] = r["action"].(string)
	}
	assert.Equal(t, []string{"create_project", "update_project", "delete_project"}, actions)

	createResource := records[0]["resource"].(map[string]any)
	updateResource := records[1]["resource"].(map[string]any)
	deleteResource := records[2]["resource"].(map[string]any)

	assert.Equal(t, "crud-test-proj", createResource["name"])
	assert.NotEmpty(t, createResource["id"], "create must carry the K8s UID")
	assert.Equal(t, testNS, createResource["namespace"],
		"a create's SetResource call must not drop the namespace the pre-handler seed already had")

	assert.Equal(t, "crud-test-proj", updateResource["name"])
	assert.Equal(t, createResource["id"], updateResource["id"],
		"the UID must be stable across an update even though the name did not change")
	assert.Equal(t, testNS, updateResource["namespace"],
		"an update's SetResource call must not drop the namespace the pre-handler seed already had")

	assert.Equal(t, "crud-test-proj", deleteResource["name"])
	assert.NotContains(t, deleteResource, "id",
		"DeleteProject returns no object, so no UID is available to record")
	assert.Equal(t, testNS, deleteResource["namespace"],
		"namespace must be recorded even without a UID, so identically-named projects in different namespaces stay distinguishable")

	for _, r := range records {
		assert.Equal(t, "success", r["result"])
		assert.Equal(t, "management", r["category"])
	}
}

// TestAuditMiddlewareWired_TraitCRUD guards the same namespace-on-success
// contract as TestAuditMiddlewareWired_ProjectCRUD, but for one of the
// resources whose SetResource call previously omitted Namespace entirely:
// since SetResource replaces the whole Resource rather than merging into it,
// a create/update handler that set only ID and Name silently wiped out the
// namespace the pre-handler seed (middleware.go's r.PathValue("namespaceName"))
// had already populated — so a successful create/update lost resource.namespace
// while a denied or failed one for the same route kept it. Trait is
// representative of every other namespace-scoped resource fixed alongside it.
func TestAuditMiddlewareWired_TraitCRUD(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	fc := uidAssigningClient(t)
	svc := traitsvc.NewServiceWithAuthz(fc, &allowAllPDP{}, logger)
	services := &handlerservices.Services{TraitService: svc}
	handler := newTestHTTPHandlerWithLogger(t, services, logger)

	createBody, _ := json.Marshal(gen.Trait{
		Metadata: gen.ObjectMeta{Name: "crud-test-trait"},
	})
	_, rec := doRequest(t, handler, http.MethodPost, "/api/v1/namespaces/"+testNS+"/traits", createBody)
	require.Equal(t, http.StatusCreated, rec.Code)

	updateBody, _ := json.Marshal(gen.Trait{
		Metadata: gen.ObjectMeta{Name: "crud-test-trait", Labels: &map[string]string{"tier": "updated"}},
	})
	_, rec = doRequest(t, handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/traits/crud-test-trait", updateBody)
	require.Equal(t, http.StatusOK, rec.Code)

	records := auditRecords(t, &buf)
	require.Len(t, records, 2, "expected one AUDIT-LOG record per mutation, got:\n%s", buf.String())

	createResource := records[0]["resource"].(map[string]any)
	updateResource := records[1]["resource"].(map[string]any)

	assert.Equal(t, testNS, createResource["namespace"],
		"create's SetResource call must include Namespace, not just ID/Name")
	assert.Equal(t, testNS, updateResource["namespace"],
		"update's SetResource call must include Namespace, not just ID/Name")
}

// TestBuildPatternMap_AllOperationsResolve is the exhaustive-breadth
// counterpart to TestAuditMiddlewareWired_AllOperations' representative
// sample: it proves every spec-derived apiaudit.GetOperations() entry
// resolves against the live OpenAPI spec with a valid RESTResourceParam,
// using the same construction path production takes (OpenAPIMiddlewares ->
// audit.NewMiddleware -> BuildPatternMap). A renamed operationId, a pattern
// collision, or a RESTResourceParam typo fails here, not just at server
// startup. Operations with NotInOpenAPISpec set (Exec, Wirelogs) are
// deliberately excluded from the expected count — BuildPatternMap skips
// them by design, since they have no route in the spec to resolve at all.
func TestBuildPatternMap_AllOperationsResolve(t *testing.T) {
	swagger, err := gen.GetSwagger()
	require.NoError(t, err)

	ops := apiaudit.GetOperations()
	patternMap, err := audit.BuildPatternMap(ops, swagger)
	require.NoError(t, err)

	wantResolved := 0
	for _, op := range ops {
		if !op.NotInOpenAPISpec {
			wantResolved++
		}
	}
	assert.Len(t, patternMap, wantResolved, "every spec-derived operation must resolve to a distinct pattern")
}

// TestAuditMiddlewareWired_AllOperations drives one real HTTP request through
// the production chain per distinct operation *shape* — plain namespaced
// create/update/delete, a cluster-scoped route with no namespace ancestor,
// and the one non-uniform "trigger" route — and asserts exactly one
// AUDIT-LOG record with the expected action. It is a representative sample,
// not exhaustive: TestBuildPatternMap_AllOperationsResolve above already
// proves route resolution for every operation, and TestAuditCoverage proves
// every operation has a definition or exemption; hand-wiring a real HTTP
// request with a correct mocked service response for the rest would re-prove
// the same mechanism at the cost of one more mock service per operation.
//
// Business-logic success is irrelevant here — services are mocked, and
// Secret operations hit the disabled-by-default 501 path. The audit
// middleware fires once a route resolves, independent of status code.
func TestAuditMiddlewareWired_AllOperations(t *testing.T) {
	projectSvc := projectmocks.NewMockService(t)
	projectSvc.EXPECT().CreateProject(mock.Anything, mock.Anything, mock.Anything).
		Return(&openchoreov1alpha1.Project{ObjectMeta: metav1.ObjectMeta{Name: "tp-project", UID: "uid-1"}}, nil).Maybe()
	projectSvc.EXPECT().UpdateProject(mock.Anything, mock.Anything, mock.Anything).
		Return(&openchoreov1alpha1.Project{ObjectMeta: metav1.ObjectMeta{Name: "tp-project", UID: "uid-1"}}, nil).Maybe()
	projectSvc.EXPECT().DeleteProject(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	dataPlaneSvc := dataplanemocks.NewMockService(t)
	dataPlaneSvc.EXPECT().CreateDataPlane(mock.Anything, mock.Anything, mock.Anything).
		Return(&openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: "tp-dp", UID: "uid-2"}}, nil).Maybe()
	dataPlaneSvc.EXPECT().UpdateDataPlane(mock.Anything, mock.Anything, mock.Anything).
		Return(&openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: "tp-dp", UID: "uid-2"}}, nil).Maybe()
	dataPlaneSvc.EXPECT().DeleteDataPlane(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	envSvc := environmentmocks.NewMockService(t)
	envSvc.EXPECT().CreateEnvironment(mock.Anything, mock.Anything, mock.Anything).
		Return(&openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Name: "tp-env", UID: "uid-3"}}, nil).Maybe()
	envSvc.EXPECT().UpdateEnvironment(mock.Anything, mock.Anything, mock.Anything).
		Return(&openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Name: "tp-env", UID: "uid-3"}}, nil).Maybe()
	envSvc.EXPECT().DeleteEnvironment(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	services := &handlerservices.Services{
		ProjectService:     projectSvc,
		DataPlaneService:   dataPlaneSvc,
		EnvironmentService: envSvc,
	}
	handler := newTestHTTPHandlerWithLogger(t, services, logger)

	type opRequest struct {
		action string
		method string
		path   string
		body   string
	}
	requests := []opRequest{
		{"create_project", http.MethodPost, "/api/v1/namespaces/" + testNS + "/projects", `{"metadata":{"name":"tp-project"}}`},
		{"update_project", http.MethodPut, "/api/v1/namespaces/" + testNS + "/projects/tp-project", `{"metadata":{"name":"tp-project"}}`},
		{"delete_project", http.MethodDelete, "/api/v1/namespaces/" + testNS + "/projects/tp-project", ""},
		{"create_data_plane", http.MethodPost, "/api/v1/namespaces/" + testNS + "/dataplanes", `{"metadata":{"name":"tp-dp"}}`},
		{"update_data_plane", http.MethodPut, "/api/v1/namespaces/" + testNS + "/dataplanes/tp-dp", `{"metadata":{"name":"tp-dp"}}`},
		{"delete_data_plane", http.MethodDelete, "/api/v1/namespaces/" + testNS + "/dataplanes/tp-dp", ""},
		{"create_environment", http.MethodPost, "/api/v1/namespaces/" + testNS + "/environments", `{"metadata":{"name":"tp-env"}}`},
		{"update_environment", http.MethodPut, "/api/v1/namespaces/" + testNS + "/environments/tp-env", `{"metadata":{"name":"tp-env"}}`},
		{"delete_environment", http.MethodDelete, "/api/v1/namespaces/" + testNS + "/environments/tp-env", ""},
		{"create_secret", http.MethodPost, "/api/v1alpha1/namespaces/" + testNS + "/secrets", `{"secretName":"tp-secret"}`},
		{"update_secret", http.MethodPut, "/api/v1alpha1/namespaces/" + testNS + "/secrets/tp-secret", `{}`},
		{"delete_secret", http.MethodDelete, "/api/v1alpha1/namespaces/" + testNS + "/secrets/tp-secret", ""},
	}

	for _, r := range requests {
		t.Run(r.action, func(t *testing.T) {
			buf.Reset()
			var body []byte
			if r.body != "" {
				body = []byte(r.body)
			}
			doRequest(t, handler, r.method, r.path, body)

			records := auditRecords(t, &buf)
			require.Len(t, records, 1, "expected exactly one AUDIT-LOG record for %s, got:\n%s", r.action, buf.String())
			assert.Equal(t, r.action, records[0]["action"])
		})
	}
}

// TestAuditMiddlewareWired_DeniedRequestCarriesResource guards against a PDP
// denial shipping with resource: null. UpdateProject's ErrForbidden branch
// returns before the handler ever calls audit.SetResource, so resource.type
// and resource.name must come from the pre-handler seed the audit middleware
// builds from the Operation and the request's path value (see
// middleware.go's Handler) — not from the handler.
func TestAuditMiddlewareWired_DeniedRequestCarriesResource(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	projectSvc := projectmocks.NewMockService(t)
	projectSvc.EXPECT().UpdateProject(mock.Anything, mock.Anything, mock.Anything).
		Return(nil, services.ErrForbidden)

	services := &handlerservices.Services{ProjectService: projectSvc}
	handler := newTestHTTPHandlerWithLogger(t, services, logger)

	body, _ := json.Marshal(gen.Project{Metadata: gen.ObjectMeta{Name: "denied-proj"}})
	_, rec := doRequest(t, handler, http.MethodPut,
		"/api/v1/namespaces/"+testNS+"/projects/denied-proj", body)
	require.Equal(t, http.StatusForbidden, rec.Code)

	records := auditRecords(t, &buf)
	require.Len(t, records, 1, "expected exactly one AUDIT-LOG record, got:\n%s", buf.String())
	record := records[0]

	assert.Equal(t, "denied", record["result"])

	resource, ok := record["resource"].(map[string]any)
	require.True(t, ok, "resource must be populated even though the handler never called SetResource")
	assert.Equal(t, "project", resource["type"])
	assert.Equal(t, "denied-proj", resource["name"], "name must come from the path value, not the handler")
	assert.Equal(t, testNS, resource["namespace"], "namespace must come from the path value, not the handler")
}

// rejectingAuth is a MiddlewareFunc that mimics a real auth failure: it
// writes 401 and returns without calling next, and — unlike
// injectTestSubject — never sets a SubjectContext. This is what a real JWT
// rejection (missing/invalid/expired token) looks like to everything
// downstream, and is the only way to reach NewUnauthenticatedMiddleware's
// emitting branch in a test, since the inner Middleware instance runs
// strictly inside auth and never sees a request auth itself rejects.
var rejectingAuth gen.MiddlewareFunc = func(_ http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
}

// TestAuditMiddlewareWired_UnauthenticatedRejection proves the outer
// NewUnauthenticatedMiddleware instance is really wired into the production
// chain (OpenAPIMiddlewares) and fires exactly once on a real auth rejection —
// a gap the inner instance alone can't close, since auth short-circuits
// before ever calling next.
func TestAuditMiddlewareWired_UnauthenticatedRejection(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	auditCfg := config.AuditDefaults()
	policies, err := auditCfg.BuildPolicySet(auditconfig.Vocabulary{}, nil)
	require.NoError(t, err)
	emitter, err := audit.NewEmitter("openchoreo-api", policies, audit.NewLogger(logger))
	require.NoError(t, err)

	h := &Handler{services: &handlerservices.Services{}, logger: logger}
	strictHandler := gen.NewStrictHandler(h, nil)
	mux := http.NewServeMux()
	middlewares, err := OpenAPIMiddlewares(OpenAPIMiddlewareOptions{
		Logger:         logger,
		AuthMiddleware: rejectingAuth,
		AuditEmitter:   emitter,
		AuditEnabled:   auditCfg.Enabled,
	})
	require.NoError(t, err)
	gen.HandlerWithOptions(strictHandler, gen.StdHTTPServerOptions{BaseRouter: mux, Middlewares: middlewares})

	_, rec := doRequest(t, mux, http.MethodPost, "/api/v1/namespaces/"+testNS+"/projects", []byte(`{"metadata":{"name":"x"}}`))
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	records := auditRecords(t, &buf)
	require.Len(t, records, 1, "expected exactly one AUDIT-LOG record for a rejected request, got:\n%s", buf.String())

	record := records[0]
	assert.Equal(t, "unauthenticated", record["result"])
	assert.Equal(t, "", record["action"], "a rejection has no resolved operation")
	assert.Equal(t, "", record["category"], "a rejection has no resolved operation")
	assert.NotContains(t, record, "resource", "a rejection has no resolved operation, so no resource to seed")
}

// TestAuditMiddlewareWired_PanicOnAuthenticatedRouteEmitsExactlyOnce is the
// full-stack double-emission regression test: a real derived-request auth
// success (injectTestSubject, shaped exactly like the production JWT
// middleware) reaching a handler that panics on a resolved operation must
// produce exactly one AUDIT-LOG record, from the inner instance, with the
// outer NewUnauthenticatedMiddleware staying silent.
func TestAuditMiddlewareWired_PanicOnAuthenticatedRouteEmitsExactlyOnce(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	projectSvc := projectmocks.NewMockService(t)
	projectSvc.EXPECT().CreateProject(mock.Anything, mock.Anything, mock.Anything).
		Run(func(context.Context, string, *openchoreov1alpha1.Project) { panic("handler blew up") })

	services := &handlerservices.Services{ProjectService: projectSvc}
	handler := newTestHTTPHandlerWithLogger(t, services, logger)

	body, _ := json.Marshal(gen.Project{Metadata: gen.ObjectMeta{Name: "panic-proj"}})

	require.Panics(t, func() {
		doRequest(t, handler, http.MethodPost, "/api/v1/namespaces/"+testNS+"/projects", body)
	})

	records := auditRecords(t, &buf)
	require.Len(t, records, 1, "expected exactly one AUDIT-LOG record despite the panic, got:\n%s", buf.String())
	assert.Equal(t, "create_project", records[0]["action"])
	assert.Equal(t, "failure", records[0]["result"])
}

// TestOpenAPIMiddlewares_ErrorsOnNilEmitter guards the loud-failure guard itself:
// a nil AuditEmitter must fail at wiring time — as an error main.go turns
// into a clean startup-failure log line, not a panic's stack trace — rather
// than let audit silently disappear from the chain.
func TestOpenAPIMiddlewares_ErrorsOnNilEmitter(t *testing.T) {
	middlewares, err := OpenAPIMiddlewares(OpenAPIMiddlewareOptions{
		Logger:         slog.Default(),
		AuthMiddleware: injectTestSubject,
		AuditEmitter:   nil,
		AuditEnabled:   true,
	})
	require.Error(t, err, "expected OpenAPIMiddlewares to error when AuditEmitter is nil")
	assert.Nil(t, middlewares)
}
