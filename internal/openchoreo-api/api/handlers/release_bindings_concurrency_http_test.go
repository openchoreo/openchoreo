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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	endpointreleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointreleasebinding"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	resourcereleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcereleasebinding"
)

// bindingConcurrencyCase is one binding kind whose update honors
// metadata.resourceVersion (#5370).
type bindingConcurrencyCase struct {
	kind   string
	path   string
	seed   client.Object
	stored client.Object
}

func bindingConcurrencyCases() []bindingConcurrencyCase {
	return []bindingConcurrencyCase{
		{
			kind: "ReleaseBinding",
			path: "/api/v1/namespaces/" + testNS + "/releasebindings/web-dev",
			seed: &openchoreov1alpha1.ReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "web-dev", Namespace: testNS},
				Spec: openchoreov1alpha1.ReleaseBindingSpec{
					Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: "test-proj", ComponentName: "web"},
					Environment: "dev",
				},
			},
			stored: &openchoreov1alpha1.ReleaseBinding{},
		},
		{
			kind: "ResourceReleaseBinding",
			path: "/api/v1/namespaces/" + testNS + "/resourcereleasebindings/db-dev",
			seed: &openchoreov1alpha1.ResourceReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "db-dev", Namespace: testNS},
				Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
					Owner:       openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "test-proj", ResourceName: "db"},
					Environment: "dev",
				},
			},
			stored: &openchoreov1alpha1.ResourceReleaseBinding{},
		},
		{
			kind: "EndpointReleaseBinding",
			path: "/api/v1/namespaces/" + testNS + "/endpointreleasebindings/api-dev",
			seed: &openchoreov1alpha1.EndpointReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "api-dev", Namespace: testNS},
				Spec: openchoreov1alpha1.EndpointReleaseBindingSpec{
					Owner:       openchoreov1alpha1.EndpointReleaseBindingOwner{ProjectName: "test-proj", EndpointName: "api"},
					Environment: "dev",
				},
			},
			stored: &openchoreov1alpha1.EndpointReleaseBinding{},
		},
	}
}

func newBindingConcurrencyHandler(t *testing.T, seed client.Object) (http.Handler, client.Client) {
	t.Helper()
	fc := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(seed).Build()
	services := &handlerservices.Services{
		ReleaseBindingService:         releasebindingsvc.NewServiceWithAuthz(fc, &allowAllPDP{}, slog.Default()),
		ResourceReleaseBindingService: resourcereleasebindingsvc.NewServiceWithAuthz(fc, &allowAllPDP{}, slog.Default()),
		EndpointReleaseBindingService: endpointreleasebindingsvc.NewServiceWithAuthz(fc, &allowAllPDP{}, slog.Default()),
	}
	return newTestHTTPHandler(t, services), fc
}

// annotateBinding stands in for another writer (the portal, kubectl) changing the
// binding between a client's GET and its PUT.
func annotateBinding(t *testing.T, fc client.Client, tc bindingConcurrencyCase) {
	t.Helper()
	obj := tc.stored.DeepCopyObject().(client.Object)
	require.NoError(t, fc.Get(context.Background(), client.ObjectKeyFromObject(tc.seed), obj))
	obj.SetAnnotations(map[string]string{"example/marker": "x"})
	require.NoError(t, fc.Update(context.Background(), obj))
}

// getBindingJSON reads the binding through the API and returns its body as a map.
func getBindingJSON(t *testing.T, h http.Handler, path string) map[string]any {
	t.Helper()
	_, rec := doRequest(t, h, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

func TestBindingHTTPUpdateStaleResourceVersion(t *testing.T) {
	for _, tc := range bindingConcurrencyCases() {
		t.Run(tc.kind, func(t *testing.T) {
			h, fc := newBindingConcurrencyHandler(t, tc.seed)

			// 1. Read the binding, as `occ ... promote` does.
			_, getRec := doRequest(t, h, http.MethodGet, tc.path, nil)
			require.Equal(t, http.StatusOK, getRec.Code)
			read := getRec.Body.Bytes()
			var meta struct {
				Metadata gen.ObjectMeta `json:"metadata"`
			}
			require.NoError(t, json.Unmarshal(read, &meta))
			require.NotNil(t, meta.Metadata.ResourceVersion, "GET must expose metadata.resourceVersion")
			require.NotEmpty(t, *meta.Metadata.ResourceVersion)

			// 2. Someone else changes it.
			annotateBinding(t, fc, tc)

			// 3. Writing the copy from step 1 back is rejected...
			req, rec := doRequest(t, h, http.MethodPut, tc.path, read)
			require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
			var errResp gen.ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
			assert.Equal(t, gen.CONFLICT, errResp.Code)
			assert.Contains(t, errResp.Error, tc.kind+" was modified since it was read")
			assertConformsToSpec(t, req, rec.Code, rec.Result().Header, rec.Body.Bytes())

			// 4. ...and the other writer's change is kept.
			stored := tc.stored.DeepCopyObject().(client.Object)
			require.NoError(t, fc.Get(context.Background(), client.ObjectKeyFromObject(tc.seed), stored))
			assert.Equal(t, "x", stored.GetAnnotations()["example/marker"])
		})
	}
}

func TestBindingHTTPUpdateCurrentResourceVersion(t *testing.T) {
	for _, tc := range bindingConcurrencyCases() {
		t.Run(tc.kind, func(t *testing.T) {
			h, _ := newBindingConcurrencyHandler(t, tc.seed)

			body := getBindingJSON(t, h, tc.path)
			metadata := body["metadata"].(map[string]any)
			readVersion := metadata["resourceVersion"]
			metadata["labels"] = map[string]string{"tier": "updated"}
			put, err := json.Marshal(body)
			require.NoError(t, err)

			req, rec := doRequest(t, h, http.MethodPut, tc.path, put)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			var resp struct {
				Metadata gen.ObjectMeta `json:"metadata"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.NotNil(t, resp.Metadata.ResourceVersion)
			assert.NotEqual(t, readVersion, *resp.Metadata.ResourceVersion, "the update returns the new resourceVersion")
			assertConformsToSpec(t, req, rec.Code, rec.Result().Header, rec.Body.Bytes())
		})
	}
}

func TestBindingHTTPUpdateWithoutResourceVersion(t *testing.T) {
	for _, tc := range bindingConcurrencyCases() {
		t.Run(tc.kind, func(t *testing.T) {
			h, fc := newBindingConcurrencyHandler(t, tc.seed)

			body := getBindingJSON(t, h, tc.path)
			delete(body["metadata"].(map[string]any), "resourceVersion")
			put, err := json.Marshal(body)
			require.NoError(t, err)

			annotateBinding(t, fc, tc)

			// Clients that don't send a version keep the unconditional update.
			_, rec := doRequest(t, h, http.MethodPut, tc.path, put)
			assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		})
	}
}

func TestBindingHTTPUpdateMalformedResourceVersion(t *testing.T) {
	for _, tc := range bindingConcurrencyCases() {
		t.Run(tc.kind, func(t *testing.T) {
			h, _ := newBindingConcurrencyHandler(t, tc.seed)

			body := getBindingJSON(t, h, tc.path)
			body["metadata"].(map[string]any)["resourceVersion"] = "abc"
			put, err := json.Marshal(body)
			require.NoError(t, err)

			_, rec := doRequest(t, h, http.MethodPut, tc.path, put)
			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), "metadata.resourceVersion")
		})
	}
}
