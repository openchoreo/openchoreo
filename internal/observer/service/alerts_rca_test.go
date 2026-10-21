// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreoapis "github.com/openchoreo/openchoreo/api/v1alpha1"
	legacytypes "github.com/openchoreo/openchoreo/internal/observer/types"
)

func TestTriggerRCAAnalysis_ReleaseLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		releaseLine string
		wantLine    any
	}{
		{name: "line alert carries the line", releaseLine: "canary", wantLine: "canary"},
		{name: "component alert omits the line", releaseLine: "", wantLine: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var body map[string]any
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v1alpha1/rca-agent/analyze", r.URL.Path)
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				w.WriteHeader(http.StatusAccepted)
			}))
			defer ts.Close()

			svc := newTestAlertService()
			svc.rcaServiceURL = ts.URL
			svc.triggerRCAAnalysis("alert-1", &legacytypes.AlertDetails{
				AlertName:   "high-errors",
				Namespace:   "default",
				Project:     "shop",
				Component:   "greeter",
				Environment: "development",
				ReleaseLine: tt.releaseLine,
			}, nil)

			require.NotNil(t, body)
			assert.Equal(t, "greeter", body["component"])
			assert.Equal(t, tt.wantLine, body["release_line"])
		})
	}
}

func TestTriggerFinOpsAnalysis_ReleaseLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		releaseLine string
		wantLine    any
	}{
		{name: "line alert carries the line", releaseLine: "canary", wantLine: "canary"},
		{name: "component alert omits the line", releaseLine: "", wantLine: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var body map[string]any
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v1alpha1/analyses", r.URL.Path)
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				w.WriteHeader(http.StatusAccepted)
			}))
			defer ts.Close()

			rule := &choreoapis.ObservabilityAlertRule{}
			rule.Spec.Condition.Window = metav1.Duration{Duration: 24 * time.Hour}
			rule.Spec.Condition.Threshold = 10

			svc := newTestAlertService()
			svc.finOpsAgentURL = ts.URL
			svc.triggerFinOpsAnalysis("alert-1", &legacytypes.AlertDetails{
				Namespace:   "default",
				Project:     "shop",
				Component:   "greeter",
				Environment: "development",
				AlertValue:  "12.5",
				ReleaseLine: tt.releaseLine,
			}, rule)

			require.NotNil(t, body)
			scope, ok := body["searchScope"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "greeter", scope["component"])
			assert.Equal(t, tt.wantLine, scope["releaseLine"])
		})
	}
}
