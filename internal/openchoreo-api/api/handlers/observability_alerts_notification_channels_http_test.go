// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	observabilityalertsnotificationchannelsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/observabilityalertsnotificationchannel"
)

const ncBasePath = "/api/v1/namespaces/" + testNS + "/observabilityalertsnotificationchannels"

// ncBundle holds the real HTTP handler wired to a fake K8s client so tests can
// both drive the handler through HTTP and inspect the resulting K8s state.
type ncBundle struct {
	handler    http.Handler
	fakeClient client.Client
}

// newNCBundle builds an ncBundle seeded with the given objects and using the
// supplied PDP for authorization decisions.
func newNCBundle(t *testing.T, objects []client.Object, pdp authzcore.PDP) ncBundle {
	t.Helper()
	fc := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithObjects(objects...).
		Build()
	svc := observabilityalertsnotificationchannelsvc.NewServiceWithAuthz(fc, pdp, slog.Default())
	services := &handlerservices.Services{ObservabilityAlertsNotificationChannelService: svc}
	return ncBundle{
		handler:    newTestHTTPHandler(t, services),
		fakeClient: fc,
	}
}

// seedNC is a convenience constructor for an ObservabilityAlertsNotificationChannel object.
// spec.type and spec.environment are required by the OpenAPI schema.
func seedNC(name string) *openchoreov1alpha1.ObservabilityAlertsNotificationChannel {
	return &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
		Spec: openchoreov1alpha1.ObservabilityAlertsNotificationChannelSpec{
			Type:        openchoreov1alpha1.NotificationChannelTypeWebhook,
			Environment: "dev",
			WebhookConfig: &openchoreov1alpha1.WebhookConfig{
				URL: "https://example.com/hook",
			},
		},
	}
}

// newNCBody returns a JSON body for creating/updating a notification channel.
// spec.type and spec.webhookConfig.url are required by the OpenAPI schema.
func newNCBody(name string) []byte {
	ncType := gen.Webhook
	b, _ := json.Marshal(gen.ObservabilityAlertsNotificationChannel{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.ObservabilityAlertsNotificationChannelSpec{
			Type:        ncType,
			Environment: "dev",
			WebhookConfig: &gen.NotificationWebhookConfig{
				Url: "https://example.com/hook",
			},
		},
	})
	return b
}

// --- List ---

func TestObservabilityAlertsNotificationChannelHTTPList(t *testing.T) {
	bundle := newNCBundle(t, []client.Object{
		seedNC("nc-a"),
		seedNC("nc-b"),
	}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet, ncBasePath, nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ObservabilityAlertsNotificationChannelList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp), "response body must be valid JSON")
	assert.Len(t, resp.Items, 2, "list must return both seeded notification channels")

	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Metadata.Name
	}
	assert.ElementsMatch(t, []string{"nc-a", "nc-b"}, names)

	// Concern 2: response must conform to the OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestObservabilityAlertsNotificationChannelHTTPListEmpty(t *testing.T) {
	bundle := newNCBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet, ncBasePath, nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ObservabilityAlertsNotificationChannelList
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Empty(t, resp.Items, "empty store must return an empty items array")

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

// --- Get ---

func TestObservabilityAlertsNotificationChannelHTTPGet(t *testing.T) {
	bundle := newNCBundle(t, []client.Object{seedNC("nc-x")}, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodGet, ncBasePath+"/nc-x", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ObservabilityAlertsNotificationChannel
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "nc-x", resp.Metadata.Name)

	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestObservabilityAlertsNotificationChannelHTTPGetNotFound(t *testing.T) {
	bundle := newNCBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet, ncBasePath+"/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestObservabilityAlertsNotificationChannelHTTPGetForbidden(t *testing.T) {
	bundle := newNCBundle(t, []client.Object{seedNC("nc-x")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodGet, ncBasePath+"/nc-x", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Create ---

func TestObservabilityAlertsNotificationChannelHTTPCreate(t *testing.T) {
	bundle := newNCBundle(t, nil, &allowAllPDP{})

	req, rec := doRequest(t, bundle.handler, http.MethodPost, ncBasePath, newNCBody("new-nc"))

	assert.Equal(t, http.StatusCreated, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ObservabilityAlertsNotificationChannel
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "new-nc", resp.Metadata.Name)

	// Concern 3: verify the object was actually persisted to the fake K8s store.
	var k8sObj openchoreov1alpha1.ObservabilityAlertsNotificationChannel
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "new-nc", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "notification channel must be persisted to K8s after creation")
	assert.Equal(t, "new-nc", k8sObj.Name)

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestObservabilityAlertsNotificationChannelHTTPCreateAlreadyExists(t *testing.T) {
	bundle := newNCBundle(t, []client.Object{seedNC("new-nc")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodPost, ncBasePath, newNCBody("new-nc"))

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestObservabilityAlertsNotificationChannelHTTPCreateForbidden(t *testing.T) {
	bundle := newNCBundle(t, nil, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodPost, ncBasePath, newNCBody("new-nc"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Update ---

func TestObservabilityAlertsNotificationChannelHTTPUpdate(t *testing.T) {
	bundle := newNCBundle(t, []client.Object{seedNC("nc-y")}, &allowAllPDP{})

	// Include a label so we can assert the updated value is persisted.
	ncType := gen.Webhook
	body, _ := json.Marshal(gen.ObservabilityAlertsNotificationChannel{
		Metadata: gen.ObjectMeta{
			Name:   "nc-y",
			Labels: &map[string]string{"tier": "updated"},
		},
		Spec: &gen.ObservabilityAlertsNotificationChannelSpec{
			Type:          ncType,
			Environment:   "dev",
			WebhookConfig: &gen.NotificationWebhookConfig{Url: "https://example.com/hook"},
		},
	})

	req, rec := doRequest(t, bundle.handler, http.MethodPut, ncBasePath+"/nc-y", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	bodyBytes := rec.Body.Bytes()
	var resp gen.ObservabilityAlertsNotificationChannel
	require.NoError(t, json.Unmarshal(bodyBytes, &resp))
	assert.Equal(t, "nc-y", resp.Metadata.Name)

	// Concern 3: verify the label mutation is reflected in the fake K8s store.
	var k8sObj openchoreov1alpha1.ObservabilityAlertsNotificationChannel
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "nc-y", Namespace: testNS}, &k8sObj)
	require.NoError(t, err, "notification channel must still exist in K8s after update")
	assert.Equal(t, "updated", k8sObj.Labels["tier"],
		"updated label must be persisted to K8s")

	// Concern 2: validate against OpenAPI contract.
	assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
}

func TestObservabilityAlertsNotificationChannelHTTPUpdateNotFound(t *testing.T) {
	bundle := newNCBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodPut, ncBasePath+"/nonexistent", newNCBody("nonexistent"))

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestObservabilityAlertsNotificationChannelHTTPUpdateForbidden(t *testing.T) {
	bundle := newNCBundle(t, []client.Object{seedNC("nc-y")}, &denyAllPDP{})
	_, rec := doRequest(t, bundle.handler, http.MethodPut, ncBasePath+"/nc-y", newNCBody("nc-y"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Delete ---

func TestObservabilityAlertsNotificationChannelHTTPDelete(t *testing.T) {
	bundle := newNCBundle(t, []client.Object{seedNC("nc-z")}, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete, ncBasePath+"/nc-z", nil)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Concern 3: confirm the object is gone from the fake K8s store.
	var gone openchoreov1alpha1.ObservabilityAlertsNotificationChannel
	err := bundle.fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "nc-z", Namespace: testNS}, &gone)
	require.True(t, apierrors.IsNotFound(err),
		"notification channel must be removed from K8s after deletion, got err: %v", err)
}

func TestObservabilityAlertsNotificationChannelHTTPDeleteNotFound(t *testing.T) {
	bundle := newNCBundle(t, nil, &allowAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete, ncBasePath+"/missing", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestObservabilityAlertsNotificationChannelHTTPDeleteForbidden(t *testing.T) {
	bundle := newNCBundle(t, []client.Object{seedNC("nc-z")}, &denyAllPDP{})

	_, rec := doRequest(t, bundle.handler, http.MethodDelete, ncBasePath+"/nc-z", nil)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
