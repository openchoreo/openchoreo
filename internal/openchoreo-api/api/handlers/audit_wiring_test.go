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
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	apiaudit "github.com/openchoreo/openchoreo/internal/openchoreo-api/audit"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	dataplanemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/dataplane/mocks"
	environmentmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/environment/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
	projectmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project/mocks"
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
// the same APIMiddlewares constructor production uses — removing audit from
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

	assert.Equal(t, "crud-test-proj", updateResource["name"])
	assert.Equal(t, createResource["id"], updateResource["id"],
		"the UID must be stable across an update even though the name did not change")

	assert.Equal(t, "crud-test-proj", deleteResource["name"])
	assert.NotContains(t, deleteResource, "id",
		"DeleteProject returns no object, so no UID is available to record")

	for _, r := range records {
		assert.Equal(t, "success", r["result"])
		assert.Equal(t, "management", r["category"])
	}
}

// TestAuditMiddlewareWired_AllOperations drives one real HTTP request per
// apiaudit.GetOperations() entry through the production chain and asserts
// exactly one AUDIT-LOG record with the expected action — the only test that
// exercises BuildPatternMap's computed pattern against what r.Pattern
// actually reports at runtime.
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
		{"create_dataplane", http.MethodPost, "/api/v1/namespaces/" + testNS + "/dataplanes", `{"metadata":{"name":"tp-dp"}}`},
		{"update_dataplane", http.MethodPut, "/api/v1/namespaces/" + testNS + "/dataplanes/tp-dp", `{"metadata":{"name":"tp-dp"}}`},
		{"delete_dataplane", http.MethodDelete, "/api/v1/namespaces/" + testNS + "/dataplanes/tp-dp", ""},
		{"create_environment", http.MethodPost, "/api/v1/namespaces/" + testNS + "/environments", `{"metadata":{"name":"tp-env"}}`},
		{"update_environment", http.MethodPut, "/api/v1/namespaces/" + testNS + "/environments/tp-env", `{"metadata":{"name":"tp-env"}}`},
		{"delete_environment", http.MethodDelete, "/api/v1/namespaces/" + testNS + "/environments/tp-env", ""},
		{"create_secret", http.MethodPost, "/api/v1alpha1/namespaces/" + testNS + "/secrets", `{"secretName":"tp-secret"}`},
		{"update_secret", http.MethodPut, "/api/v1alpha1/namespaces/" + testNS + "/secrets/tp-secret", `{}`},
		{"delete_secret", http.MethodDelete, "/api/v1alpha1/namespaces/" + testNS + "/secrets/tp-secret", ""},
	}
	require.Len(t, requests, len(apiaudit.GetOperations()), "this table must cover every operation definitions.go declares")

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
}

// TestAPIMiddlewares_ErrorsOnNilEmitter guards the loud-failure guard itself:
// a nil AuditEmitter must fail at wiring time — as an error main.go turns
// into a clean startup-failure log line, not a panic's stack trace — rather
// than let audit silently disappear from the chain.
func TestAPIMiddlewares_ErrorsOnNilEmitter(t *testing.T) {
	middlewares, err := APIMiddlewares(APIMiddlewareOptions{
		Logger:         slog.Default(),
		AuthMiddleware: injectTestSubject,
		AuditEmitter:   nil,
		AuditEnabled:   true,
	})
	require.Error(t, err, "expected APIMiddlewares to error when AuditEmitter is nil")
	assert.Nil(t, middlewares)
}
