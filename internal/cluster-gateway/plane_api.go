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
	server  *Server // For accessing k8sClient to fetch CR CAs
	logger  *slog.Logger
}

type PlaneNotification struct {
	PlaneType string `json:"planeType"` // "dataplane", "buildplane", "observabilityplane"
	PlaneID   string `json:"planeID"`
	Event     string `json:"event"` // "created", "updated", "deleted"
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// PlaneNotificationResponse represents the response to a plane notification
type PlaneNotificationResponse struct {
	Success               bool   `json:"success"`
	Action                string `json:"action"` // "disconnect", "disconnect_fallback", "revalidate"
	DisconnectedAgents    *int   `json:"disconnectedAgents,omitempty"`
	AuthorizationsGranted *int   `json:"authorizationsGranted,omitempty"`
	AuthorizationsRevoked *int   `json:"authorizationsRevoked,omitempty"`
	Error                 string `json:"error,omitempty"`
}

// PlaneReconnectResponse represents the response to a manual reconnect request
type PlaneReconnectResponse struct {
	Success            bool `json:"success"`
	DisconnectedAgents int  `json:"disconnectedAgents"`
}

// AllPlaneStatusResponse represents the response for all plane statuses
type AllPlaneStatusResponse struct {
	Planes []PlaneConnectionStatus `json:"planes"`
	Total  int                     `json:"total"`
}

func NewPlaneAPI(connMgr *ConnectionManager, server *Server, logger *slog.Logger) *PlaneAPI {
	return &PlaneAPI{
		connMgr: connMgr,
		server:  server,
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

	var result PlaneNotificationResponse
	result.Success = true

	switch notification.Event {
	case "created":
		// New CR: Disconnect agents to pick up new CA
		disconnectedCount := api.connMgr.DisconnectAllForPlane(notification.PlaneType, notification.PlaneID)
		api.logger.Info("disconnected agents for new CR",
			"planeType", notification.PlaneType,
			"planeID", notification.PlaneID,
			"disconnectedAgents", disconnectedCount,
		)
		result.Action = "disconnect"
		result.DisconnectedAgents = &disconnectedCount

	case "updated":
		caData, err := api.fetchCRClientCA(notification)
		if err != nil {
			api.logger.Error("failed to fetch CR CA for re-validation", "error", err)
			// Fall back to disconnect on error
			disconnectedCount := api.connMgr.DisconnectAllForPlane(notification.PlaneType, notification.PlaneID)
			api.logger.Warn("falling back to disconnect due to CA fetch error",
				"planeType", notification.PlaneType,
				"planeID", notification.PlaneID,
				"disconnectedAgents", disconnectedCount,
			)
			result.Action = "disconnect_fallback"
			result.DisconnectedAgents = &disconnectedCount
			result.Error = err.Error()
		} else {
			updated, removed, err := api.connMgr.RevalidateCR(
				notification.PlaneType,
				notification.PlaneID,
				notification.Namespace,
				notification.Name,
				caData,
			)
			if err != nil {
				api.logger.Error("CR re-validation failed", "error", err)
				http.Error(w, fmt.Sprintf("re-validation failed: %v", err), http.StatusInternalServerError)
				return
			}
			api.logger.Info("CR re-validation completed",
				"planeType", notification.PlaneType,
				"planeID", notification.PlaneID,
				"cr", fmt.Sprintf("%s/%s", notification.Namespace, notification.Name),
				"authorizationsGranted", updated,
				"authorizationsRevoked", removed,
			)
			result.Action = "revalidate"
			result.AuthorizationsGranted = &updated
			result.AuthorizationsRevoked = &removed
		}

	case "deleted":
		disconnectedCount := api.connMgr.DisconnectAllForPlane(notification.PlaneType, notification.PlaneID)
		api.logger.Info("disconnected agents for CR deletion",
			"planeType", notification.PlaneType,
			"planeID", notification.PlaneID,
			"disconnectedAgents", disconnectedCount,
		)
		result.Action = "disconnect"
		result.DisconnectedAgents = &disconnectedCount

	default:
		api.logger.Warn("unknown event type", "event", notification.Event)
		http.Error(w, fmt.Sprintf("unknown event type: %s", notification.Event), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
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

	response := PlaneReconnectResponse{
		Success:            true,
		DisconnectedAgents: disconnectedCount,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		api.logger.Error("failed to encode response", "error", err)
	}
}

// handleGetPlaneStatus returns connection status for a specific plane
// This endpoint is called by the controller to update DataPlane CR status
// If name query parameter is provided, it returns CR-specific authorization status
// GET /api/v1/planes/{type}/{id}/status -> plane-level status (all agents connected to plane)
// GET /api/v1/planes/{type}/{id}/status?namespace=X&name=Y -> CR-specific authorization status (namespace-scoped CR)
// GET /api/v1/planes/{type}/{id}/status?name=Y -> CR-specific authorization status (cluster-scoped CR, empty namespace)
func (api *PlaneAPI) handleGetPlaneStatus(w http.ResponseWriter, r *http.Request) {
	planeType := r.PathValue("type")
	planeID := r.PathValue("id")

	if planeType == "" || planeID == "" {
		http.Error(w, "missing planeType or planeID in path", http.StatusBadRequest)
		return
	}

	// Check for CR-specific query parameters
	// For cluster-scoped CRs, namespace will be empty but name will be provided
	// For namespace-scoped CRs, both namespace and name will be provided
	namespace := r.URL.Query().Get("namespace")
	name := r.URL.Query().Get("name")

	var status *PlaneConnectionStatus
	if name != "" {
		// CR-specific status (works for both namespace-scoped and cluster-scoped CRs)
		// For cluster-scoped CRs, namespace is empty and CR key becomes "/name"
		status = api.connMgr.GetCRAuthorizationStatus(planeType, planeID, namespace, name)
	} else {
		// Plane-level status (all agents connected to the plane)
		status = api.connMgr.GetPlaneStatus(planeType, planeID)
	}

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

	response := AllPlaneStatusResponse{
		Planes: statuses,
		Total:  len(statuses),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		api.logger.Error("failed to encode response", "error", err)
	}
}

// fetchCRClientCA fetches the client CA certificate for a specific CR
// Uses the server's getAllPlaneClientCAs() method to query Kubernetes API
func (api *PlaneAPI) fetchCRClientCA(notification PlaneNotification) ([]byte, error) {
	// Use server's method to get CA data for all CRs with matching planeType/planeID
	allCRs, err := api.server.getAllPlaneClientCAs(notification.PlaneType, notification.PlaneID)
	if err != nil {
		return nil, fmt.Errorf("failed to get CRs: %w", err)
	}

	crKey := fmt.Sprintf("%s/%s", notification.Namespace, notification.Name)
	caData, exists := allCRs[crKey]
	if !exists {
		return nil, fmt.Errorf("CR %s not found", crKey)
	}

	if caData == nil {
		return nil, fmt.Errorf("CR %s has no CA configured", crKey)
	}

	api.logger.Debug("fetched CA for CR",
		"cr", crKey,
		"caSize", len(caData),
	)

	return caData, nil
}
