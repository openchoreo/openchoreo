// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package testgateway provides a fake cluster-gateway HTTP server for controller tests.
// It is only intended to be imported from _test.go files.
//
// This is a separate package from testutils because StartFakeGateway returns *gateway.Client,
// a concrete internal type that would force internal/clients/gateway into testutils' import
// graph. testutils stays generic (controller-runtime interfaces only, zero internal imports);
// gateway-specific test infrastructure lives here instead.
package testgateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	gw "github.com/openchoreo/openchoreo/internal/clients/gateway"
)

// StartFakeGateway starts a minimal HTTP server mimicking the cluster-gateway API.
// POST requests (notify) return notifyCode; GET requests (status) return statusResp (nil → 500).
// Returns the configured client, a pointer to the notify call count, and a shutdown function.
func StartFakeGateway(notifyCode int, statusResp *gw.PlaneConnectionStatus) (*gw.Client, *int, func()) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			n++
			w.WriteHeader(notifyCode)
			if notifyCode == http.StatusOK {
				_ = json.NewEncoder(w).Encode(gw.NotificationResponse{Success: true})
			}
			return
		}
		if statusResp == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(statusResp)
	}))
	c, err := gw.NewClientWithConfig(&gw.Config{BaseURL: srv.URL})
	if err != nil {
		srv.Close()
		panic("StartFakeGateway: failed to build gateway client: " + err.Error())
	}
	return c, &n, srv.Close
}
