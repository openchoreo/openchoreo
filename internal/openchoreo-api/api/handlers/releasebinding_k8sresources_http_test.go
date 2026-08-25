// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	k8sresourcesmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/k8sresources/mocks"
)

func TestTriggerReleaseBindingCronJobHTTP(t *testing.T) {
	triggerPath := "/api/v1alpha1/namespaces/test-ns/releasebindings/rb-1/trigger"
	requireRouteInSpec(t, http.MethodPost, triggerPath)

	t.Run("body-less POST forwards nil args and conforms to spec", func(t *testing.T) {
		svc := k8sresourcesmocks.NewMockService(t)
		svc.EXPECT().TriggerCronJob(mock.Anything, "test-ns", "rb-1", (*[]string)(nil)).
			Return(&models.CronJobTriggerResponse{
				JobName:     "job-inherited-1",
				Namespace:   "dp-test-ns",
				CronJobName: "cronjob-1",
			}, nil)

		services := &handlerservices.Services{K8sResourcesService: svc}
		handler := newTestHTTPHandler(t, services)

		req, rec := doRequest(t, handler, http.MethodPost, triggerPath, nil)
		assert.Equal(t, http.StatusOK, rec.Code)

		bodyBytes := rec.Body.Bytes()
		var resp gen.CronJobTriggerResponse
		require.NoError(t, json.Unmarshal(bodyBytes, &resp))
		assert.Equal(t, "job-inherited-1", resp.JobName)
		assert.Equal(t, "dp-test-ns", resp.Namespace)
		assert.Equal(t, "cronjob-1", resp.CronJobName)

		assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
	})

	t.Run("POST with empty JSON object forwards nil args and conforms to spec", func(t *testing.T) {
		svc := k8sresourcesmocks.NewMockService(t)
		svc.EXPECT().TriggerCronJob(mock.Anything, "test-ns", "rb-1", (*[]string)(nil)).
			Return(&models.CronJobTriggerResponse{
				JobName:     "job-inherited-2",
				Namespace:   "dp-test-ns",
				CronJobName: "cronjob-1",
			}, nil)

		services := &handlerservices.Services{K8sResourcesService: svc}
		handler := newTestHTTPHandler(t, services)

		req, rec := doRequest(t, handler, http.MethodPost, triggerPath, []byte("{}"))
		assert.Equal(t, http.StatusOK, rec.Code)

		bodyBytes := rec.Body.Bytes()
		var resp gen.CronJobTriggerResponse
		require.NoError(t, json.Unmarshal(bodyBytes, &resp))
		assert.Equal(t, "job-inherited-2", resp.JobName)

		assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
	})

	t.Run("POST with args array forwards exact arguments and conforms to spec", func(t *testing.T) {
		wantArgs := []string{"--flag", "value", "--mode", "rebuild"}
		svc := k8sresourcesmocks.NewMockService(t)
		svc.EXPECT().TriggerCronJob(mock.Anything, "test-ns", "rb-1", &wantArgs).
			Return(&models.CronJobTriggerResponse{
				JobName:     "job-override-1",
				Namespace:   "dp-test-ns",
				CronJobName: "cronjob-1",
			}, nil)

		services := &handlerservices.Services{K8sResourcesService: svc}
		handler := newTestHTTPHandler(t, services)

		reqBody, err := json.Marshal(map[string]any{"args": wantArgs})
		require.NoError(t, err)

		req, rec := doRequest(t, handler, http.MethodPost, triggerPath, reqBody)
		assert.Equal(t, http.StatusOK, rec.Code)

		bodyBytes := rec.Body.Bytes()
		var resp gen.CronJobTriggerResponse
		require.NoError(t, json.Unmarshal(bodyBytes, &resp))
		assert.Equal(t, "job-override-1", resp.JobName)

		assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
	})

	t.Run("POST with explicit empty args forwards empty slice and conforms to spec", func(t *testing.T) {
		emptyArgs := []string{}
		svc := k8sresourcesmocks.NewMockService(t)
		svc.EXPECT().TriggerCronJob(mock.Anything, "test-ns", "rb-1", &emptyArgs).
			Return(&models.CronJobTriggerResponse{
				JobName:     "job-empty-args-1",
				Namespace:   "dp-test-ns",
				CronJobName: "cronjob-1",
			}, nil)

		services := &handlerservices.Services{K8sResourcesService: svc}
		handler := newTestHTTPHandler(t, services)

		req, rec := doRequest(t, handler, http.MethodPost, triggerPath, []byte(`{"args":[]}`))
		assert.Equal(t, http.StatusOK, rec.Code)

		bodyBytes := rec.Body.Bytes()
		var resp gen.CronJobTriggerResponse
		require.NoError(t, json.Unmarshal(bodyBytes, &resp))
		assert.Equal(t, "job-empty-args-1", resp.JobName)

		assertConformsToSpec(t, req, rec.Code, rec.Result().Header, bodyBytes)
	})

	t.Run("POST with malformed JSON body returns 400 Bad Request", func(t *testing.T) {
		svc := k8sresourcesmocks.NewMockService(t)
		services := &handlerservices.Services{K8sResourcesService: svc}
		handler := newTestHTTPHandler(t, services)

		_, rec := doRequest(t, handler, http.MethodPost, triggerPath, []byte(`{malformed-json`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}
