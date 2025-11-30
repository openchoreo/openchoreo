// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package messaging

// ClusterAgentRequest represents a request sent from control plane to data plane agent
// Follows CQRS pattern: Commands modify state, Queries read state
type ClusterAgentRequest struct {
	// Type indicates whether this is a 'command' or 'query'
	Type RequestType `json:"type"`

	// Identifier specifies the operation (e.g., "apply-resource", "list-pods")
	Identifier string `json:"identifier"`

	// RequestID is a unique identifier for this request (UUID)
	RequestID string `json:"requestID"`

	// ClusterID identifies the target cluster/plane
	ClusterID string `json:"clusterId"`

	// Payload contains operation-specific data (manifests, parameters, etc.)
	Payload map[string]interface{} `json:"payload,omitempty"`

	// OverrideRequestTimeouts allows custom retry timeouts (optional)
	OverrideRequestTimeouts []int `json:"overrideRequestTimeouts,omitempty"`
}

// ClusterAgentResponse represents a response sent from data plane agent to control plane
type ClusterAgentResponse struct {
	// Type indicates whether this is a 'command' or 'query' response
	Type RequestType `json:"type"`

	// Identifier specifies the operation this is a response to
	Identifier string `json:"identifier"`

	// RequestID matches the ID from the original request
	RequestID string `json:"requestID"`

	// ClusterID identifies the responding cluster/plane
	ClusterID string `json:"clusterId"`

	// Status indicates success or failure
	Status ResponseStatus `json:"status"`

	// Payload contains the operation result data
	Payload map[string]interface{} `json:"payload,omitempty"`

	// Error contains error details if status is 'fail'
	Error *ErrorDetails `json:"error,omitempty"`
}

// ErrorDetails provides structured error information
type ErrorDetails struct {
	// Code is the error code (e.g., 404, 500)
	Code int `json:"code,omitempty"`

	// Message is a human-readable error message
	Message string `json:"message"`

	// Details contains additional error context
	Details map[string]interface{} `json:"details,omitempty"`
}

// RequestType represents the CQRS type
type RequestType string

const (
	TypeCommand RequestType = "command"

	TypeQuery RequestType = "query"
)

// ResponseStatus represents the outcome of a request
type ResponseStatus string

const (
	StatusSuccess ResponseStatus = "success"

	StatusFail ResponseStatus = "fail"
)

type Message struct {
	// ID is a unique identifier for the message (UUID)
	ID string `json:"id"`

	// Type indicates the message type (request, response, broadcast, heartbeat)
	Type MessageType `json:"type"`

	// Action specifies the operation to perform (for request messages)
	Action Action `json:"action,omitempty"`

	// Payload contains the message data (resource manifests, query parameters, etc.)
	Payload map[string]interface{} `json:"payload,omitempty"`

	// ReplyTo contains the ID of the message being replied to (for response messages)
	ReplyTo string `json:"replyTo,omitempty"`

	// From identifies the sender (data plane name or control plane)
	From string `json:"from,omitempty"`

	// Success indicates whether the operation succeeded (for response messages)
	Success bool `json:"success,omitempty"`

	// Error contains error information if the operation failed
	Error string `json:"error,omitempty"`

	// Timestamp is when the message was created (RFC3339 format)
	Timestamp string `json:"timestamp,omitempty"`
}

// MessageType represents the type of message (legacy)
type MessageType string

const (
	// TypeRequest is a request message from control plane to data plane
	TypeRequest MessageType = "request"

	// TypeResponse is a response message from data plane to control plane
	TypeResponse MessageType = "response"

	// TypeBroadcast is a broadcast message to all data planes
	TypeBroadcast MessageType = "broadcast"

	// TypeHeartbeat is a periodic heartbeat message
	TypeHeartbeat MessageType = "heartbeat"
)

// Action represents the operation to perform
type Action string

const (
	// ActionApplyResource applies a resource using server-side apply
	ActionApplyResource Action = "apply-resource"

	// ActionListResources lists resources by GVK and labels
	ActionListResources Action = "list-resources"

	// ActionGetResource gets a specific resource
	ActionGetResource Action = "get-resource"

	// ActionDeleteResource deletes a resource
	ActionDeleteResource Action = "delete-resource"

	// ActionPatchResource patches a resource
	ActionPatchResource Action = "patch-resource"

	// ActionCreateNamespace creates a namespace if it doesn't exist
	ActionCreateNamespace Action = "create-namespace"

	// ActionWatchResources watches resources for changes
	ActionWatchResources Action = "watch-resources"
)

func (mt MessageType) IsValid() bool {
	switch mt {
	case TypeRequest, TypeResponse, TypeBroadcast, TypeHeartbeat:
		return true
	default:
		return false
	}
}

func (a Action) IsValid() bool {
	switch a {
	case ActionApplyResource, ActionListResources, ActionGetResource,
		ActionDeleteResource, ActionPatchResource, ActionCreateNamespace,
		ActionWatchResources:
		return true
	default:
		return false
	}
}

func (mt MessageType) String() string {
	return string(mt)
}

func (a Action) String() string {
	return string(a)
}

func NewCommand(identifier, requestID, clusterID string, payload map[string]interface{}) *ClusterAgentRequest {
	return &ClusterAgentRequest{
		Type:       TypeCommand,
		Identifier: identifier,
		RequestID:  requestID,
		ClusterID:  clusterID,
		Payload:    payload,
	}
}

func NewQuery(identifier, requestID, clusterID string, payload map[string]interface{}) *ClusterAgentRequest {
	return &ClusterAgentRequest{
		Type:       TypeQuery,
		Identifier: identifier,
		RequestID:  requestID,
		ClusterID:  clusterID,
		Payload:    payload,
	}
}

func NewClusterAgentSuccessResponse(req *ClusterAgentRequest, payload map[string]interface{}) *ClusterAgentResponse {
	return &ClusterAgentResponse{
		Type:       req.Type,
		Identifier: req.Identifier,
		RequestID:  req.RequestID,
		ClusterID:  req.ClusterID,
		Status:     StatusSuccess,
		Payload:    payload,
	}
}

func NewClusterAgentFailResponse(req *ClusterAgentRequest, errCode int, errMsg string, errDetails map[string]interface{}) *ClusterAgentResponse {
	return &ClusterAgentResponse{
		Type:       req.Type,
		Identifier: req.Identifier,
		RequestID:  req.RequestID,
		ClusterID:  req.ClusterID,
		Status:     StatusFail,
		Error: &ErrorDetails{
			Code:    errCode,
			Message: errMsg,
			Details: errDetails,
		},
	}
}

func (r *ClusterAgentRequest) IsCommand() bool {
	return r.Type == TypeCommand
}

func (r *ClusterAgentRequest) IsQuery() bool {
	return r.Type == TypeQuery
}

func (r *ClusterAgentResponse) IsSuccess() bool {
	return r.Status == StatusSuccess
}

func (r *ClusterAgentResponse) IsFail() bool {
	return r.Status == StatusFail
}
