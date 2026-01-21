// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

type PlaneAPI struct {
	connMgr *ConnectionManager
	logger  *slog.Logger
}

type PlaneNotification struct {
	PlaneType string `json:"planeType"` // "dataplane", "buildplane", "observabilityplane"
	PlaneID   string `json:"planeID"`
	Event     string `json:"event"` // "created", "updated", "deleted"
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

func NewPlaneAPI(connMgr *ConnectionManager, logger *slog.Logger) *PlaneAPI {
	return &PlaneAPI{
		connMgr: connMgr,
		logger:  logger.With("component", "plane-api"),
	}
}

func (api *PlaneAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/planes/notify", api.handlePlaneNotification)
	mux.HandleFunc("POST /api/v1/planes/{type}/{id}/reconnect", api.handleReconnect)
	mux.HandleFunc("GET /api/v1/planes/{type}/{id}/status", api.handleGetPlaneStatus)
	mux.HandleFunc("GET /api/v1/planes/status", api.handleGetAllPlaneStatus)
}

func (api *PlaneAPI) handlePlaneNotification(w http.ResponseWriter, r *http.Request) {
	var notification PlaneNotification
	if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
		api.logger.Error("invalid notification payload", "error", err)
		http.Error(w, fmt.Sprintf("invalid payload: %v", err), http.StatusBadRequest)
		return
	}

	if notification.PlaneType == "" || notification.PlaneID == "" || notification.Event == "" {
		api.logger.Error("missing required fields in notification",
			"planeType", notification.PlaneType,
			"planeID", notification.PlaneID,
			"event", notification.Event,
		)
		http.Error(w, "missing required fields: planeType, planeID, event", http.StatusBadRequest)
		return
	}

	api.logger.Info("received plane notification",
		"planeType", notification.PlaneType,
		"planeID", notification.PlaneID,
		"event", notification.Event,
		"cr", fmt.Sprintf("%s/%s", notification.Namespace, notification.Name),
	)

	var disconnectedCount int

	switch notification.Event {
	case "created", "updated", "deleted":
		disconnectedCount = api.connMgr.DisconnectAllForPlane(notification.PlaneType, notification.PlaneID)
		api.logger.Info("disconnected agents for reconnection",
			"planeType", notification.PlaneType,
			"planeID", notification.PlaneID,
			"event", notification.Event,
			"disconnectedAgents", disconnectedCount,
		)

	default:
		api.logger.Warn("unknown event type", "event", notification.Event)
		http.Error(w, fmt.Sprintf("unknown event type: %s", notification.Event), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"disconnectedAgents": disconnectedCount,
		"success":            true,
	}); err != nil {
		api.logger.Error("failed to encode response", "error", err)
	}
}

func (api *PlaneAPI) handleReconnect(w http.ResponseWriter, r *http.Request) {
	planeType := r.PathValue("type")
	planeID := r.PathValue("id")

	if planeType == "" || planeID == "" {
		http.Error(w, "missing planeType or planeID in path", http.StatusBadRequest)
		return
	}

	api.logger.Info("manual reconnection requested",
		"planeType", planeType,
		"planeID", planeID,
	)

	disconnectedCount := api.connMgr.DisconnectAllForPlane(planeType, planeID)

	api.logger.Info("manual reconnection processed",
		"planeType", planeType,
		"planeID", planeID,
		"disconnectedAgents", disconnectedCount,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"disconnectedAgents": disconnectedCount,
		"success":            true,
	}); err != nil {
		api.logger.Error("failed to encode response", "error", err)
	}
}

// handleGetPlaneStatus returns connection status for a specific plane
// This endpoint is called by the controller to update DataPlane CR status
func (api *PlaneAPI) handleGetPlaneStatus(w http.ResponseWriter, r *http.Request) {
	planeType := r.PathValue("type")
	planeID := r.PathValue("id")

	if planeType == "" || planeID == "" {
		http.Error(w, "missing planeType or planeID in path", http.StatusBadRequest)
		return
	}

	status := api.connMgr.GetPlaneStatus(planeType, planeID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(status); err != nil {
		api.logger.Error("failed to encode response", "error", err)
	}
}

// handleGetAllPlaneStatus returns connection status for all planes
// This endpoint is called by the controller to get status of all planes at once
func (api *PlaneAPI) handleGetAllPlaneStatus(w http.ResponseWriter, r *http.Request) {
	statuses := api.connMgr.GetAllPlaneStatuses()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"planes": statuses,
		"total":  len(statuses),
	}); err != nil {
		api.logger.Error("failed to encode response", "error", err)
	}
}
