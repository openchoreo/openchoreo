// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"time"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// APIResponse represents a standard API response wrapper
type APIResponse[T any] struct {
	Success bool   `json:"success"`
	Data    T      `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	Code    string `json:"code,omitempty"`
}

// ListResponse represents a paginated list response
type ListResponse[T any] struct {
	Items      []T `json:"items"`
	TotalCount int `json:"totalCount"`
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
}

// PaginationMetadata holds cursor-based pagination metadata.
type PaginationMetadata struct {
	NextCursor     string `json:"nextCursor,omitempty"`
	RemainingCount *int64 `json:"remainingCount,omitempty"`
}

// CursorListResponse represents a cursor-paginated list response.
type CursorListResponse[T any] struct {
	Items      []T                `json:"items"`
	Pagination PaginationMetadata `json:"pagination"`
}

// ProjectResponse represents a project in API responses
type ProjectResponse struct {
	UID                string         `json:"uid"`
	Name               string         `json:"name"`
	NamespaceName      string         `json:"namespaceName"`
	DisplayName        string         `json:"displayName,omitempty"`
	Description        string         `json:"description,omitempty"`
	DeploymentPipeline string         `json:"deploymentPipeline,omitempty"`
	BuildPlaneRef      *BuildPlaneRef `json:"buildPlaneRef,omitempty"`
	CreatedAt          time.Time      `json:"createdAt"`
	Status             string         `json:"status,omitempty"`
	DeletionTimestamp  *time.Time     `json:"deletionTimestamp,omitempty"`
}

// ComponentResponse represents a component in API responses
type ComponentResponse struct {
	UID               string                           `json:"uid"`
	Name              string                           `json:"name"`
	DisplayName       string                           `json:"displayName,omitempty"`
	Description       string                           `json:"description,omitempty"`
	Type              string                           `json:"type"`
	AutoDeploy        bool                             `json:"autoDeploy"`
	ProjectName       string                           `json:"projectName"`
	NamespaceName     string                           `json:"namespaceName"`
	CreatedAt         time.Time                        `json:"createdAt"`
	DeletionTimestamp *time.Time                       `json:"deletionTimestamp,omitempty"`
	Status            string                           `json:"status,omitempty"`
	Workload          *openchoreov1alpha1.WorkloadSpec `json:"workload,omitempty"`
	ComponentWorkflow *ComponentWorkflow               `json:"componentWorkflow,omitempty"`
}

type BindingResponse struct {
	Name          string        `json:"name"`
	Type          string        `json:"type"`
	ComponentName string        `json:"componentName"`
	ProjectName   string        `json:"projectName"`
	NamespaceName string        `json:"namespaceName"`
	Environment   string        `json:"environment"`
	BindingStatus BindingStatus `json:"status"`
	// Component-specific binding data
	ServiceBinding        *ServiceBinding        `json:"serviceBinding,omitempty"`
	WebApplicationBinding *WebApplicationBinding `json:"webApplicationBinding,omitempty"`
	ScheduledTaskBinding  *ScheduledTaskBinding  `json:"scheduledTaskBinding,omitempty"`
}

type BindingStatusType string

const (
	BindingStatusTypeInProgress BindingStatusType = "InProgress"
	BindingStatusTypeReady      BindingStatusType = "Active"
	BindingStatusTypeFailed     BindingStatusType = "Failed"
	BindingStatusTypeSuspended  BindingStatusType = "Suspended"
	BindingStatusTypeUndeployed BindingStatusType = "NotYetDeployed"
)

type BindingStatus struct {
	Reason           string            `json:"reason"`
	Message          string            `json:"message"`
	Status           BindingStatusType `json:"status"`
	LastTransitioned time.Time         `json:"lastTransitioned"`
}

type ServiceBinding struct {
	Endpoints    []EndpointStatus `json:"endpoints"`
	Image        string           `json:"image,omitempty"`
	ReleaseState string           `json:"releaseState,omitempty"`
}

type WebApplicationBinding struct {
	Endpoints    []EndpointStatus `json:"endpoints"`
	Image        string           `json:"image,omitempty"`
	ReleaseState string           `json:"releaseState,omitempty"`
}

type ScheduledTaskBinding struct {
	Image        string `json:"image,omitempty"`
	ReleaseState string `json:"releaseState,omitempty"`
}

type EndpointStatus struct {
	Name      string           `json:"name"`
	Type      string           `json:"type"`
	Project   *ExposedEndpoint `json:"project,omitempty"`
	Namespace *ExposedEndpoint `json:"namespace,omitempty"`
	Public    *ExposedEndpoint `json:"public,omitempty"`
}

type ExposedEndpoint struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Scheme   string `json:"scheme,omitempty"`   // gRPC, HTTP, etc.
	BasePath string `json:"basePath,omitempty"` // For HTTP-based endpoints
	URI      string `json:"uri,omitempty"`
}

// DeploymentPipelineResponse represents a deployment pipeline in API responses
type DeploymentPipelineResponse struct {
	Name           string          `json:"name"`
	DisplayName    string          `json:"displayName,omitempty"`
	Description    string          `json:"description,omitempty"`
	NamespaceName  string          `json:"namespaceName"`
	CreatedAt      time.Time       `json:"createdAt"`
	Status         string          `json:"status,omitempty"`
	PromotionPaths []PromotionPath `json:"promotionPaths,omitempty"`
}

// PromotionPath represents a promotion path in the deployment pipeline
type PromotionPath struct {
	SourceEnvironmentRef  string                 `json:"sourceEnvironmentRef"`
	TargetEnvironmentRefs []TargetEnvironmentRef `json:"targetEnvironmentRefs"`
}

// TargetEnvironmentRef represents a target environment reference with approval settings
type TargetEnvironmentRef struct {
	Name                     string `json:"name"`
	RequiresApproval         bool   `json:"requiresApproval,omitempty"`
	IsManualApprovalRequired bool   `json:"isManualApprovalRequired,omitempty"`
}

// NamespaceResponse represents a namespace in API responses
type NamespaceResponse struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName,omitempty"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	Status      string    `json:"status,omitempty"`
}

// CreateNamespaceRequest represents a request to create a new namespace
type CreateNamespaceRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
}

// EnvironmentResponse represents an environment in API responses
type EnvironmentResponse struct {
	UID          string        `json:"uid"`
	Name         string        `json:"name"`
	Namespace    string        `json:"namespace"`
	DisplayName  string        `json:"displayName,omitempty"`
	Description  string        `json:"description,omitempty"`
	DataPlaneRef *DataPlaneRef `json:"dataPlaneRef,omitempty"`
	IsProduction bool          `json:"isProduction"`
	DNSPrefix    string        `json:"dnsPrefix,omitempty"`
	CreatedAt    time.Time     `json:"createdAt"`
	Status       string        `json:"status,omitempty"`
}

// AgentConnectionStatusResponse represents the agent connection status in API responses
type AgentConnectionStatusResponse struct {
	Connected            bool       `json:"connected"`
	ConnectedAgents      int        `json:"connectedAgents"`
	LastConnectedTime    *time.Time `json:"lastConnectedTime,omitempty"`
	LastDisconnectedTime *time.Time `json:"lastDisconnectedTime,omitempty"`
	LastHeartbeatTime    *time.Time `json:"lastHeartbeatTime,omitempty"`
	Message              string     `json:"message,omitempty"`
}

// DataPlaneResponse represents a dataplane in API responses
type DataPlaneResponse struct {
	Name                    string                         `json:"name"`
	Namespace               string                         `json:"namespace"`
	DisplayName             string                         `json:"displayName,omitempty"`
	Description             string                         `json:"description,omitempty"`
	ImagePullSecretRefs     []string                       `json:"imagePullSecretRefs,omitempty"`
	SecretStoreRef          string                         `json:"secretStoreRef,omitempty"`
	PublicVirtualHost       string                         `json:"publicVirtualHost"`
	OrganizationVirtualHost string                         `json:"organizationVirtualHost"`
	PublicHTTPPort          int32                          `json:"publicHTTPPort"`
	PublicHTTPSPort         int32                          `json:"publicHTTPSPort"`
	OrganizationHTTPPort    int32                          `json:"organizationHTTPPort"`
	OrganizationHTTPSPort   int32                          `json:"organizationHTTPSPort"`
	ObservabilityPlaneRef   *ObservabilityPlaneRef         `json:"observabilityPlaneRef,omitempty"`
	AgentConnection         *AgentConnectionStatusResponse `json:"agentConnection,omitempty"`
	CreatedAt               time.Time                      `json:"createdAt"`
	Status                  string                         `json:"status,omitempty"`
}

// ObservabilityPlaneRef represents a reference to an ObservabilityPlane or ClusterObservabilityPlane in responses
type ObservabilityPlaneRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// BuildPlaneResponse represents a buildplane in API responses
type BuildPlaneResponse struct {
	Name                  string                         `json:"name"`
	Namespace             string                         `json:"namespace"`
	DisplayName           string                         `json:"displayName,omitempty"`
	Description           string                         `json:"description,omitempty"`
	ObservabilityPlaneRef *ObservabilityPlaneRef         `json:"observabilityPlaneRef,omitempty"`
	AgentConnection       *AgentConnectionStatusResponse `json:"agentConnection,omitempty"`
	CreatedAt             time.Time                      `json:"createdAt"`
	Status                string                         `json:"status,omitempty"`
}

// ComponentWorkflowResponse represents a component workflow run in API responses
type ComponentWorkflowResponse struct {
	Name          string                           `json:"name"`
	UUID          string                           `json:"uuid"`
	NamespaceName string                           `json:"namespaceName"`
	ProjectName   string                           `json:"projectName"`
	ComponentName string                           `json:"componentName"`
	Commit        string                           `json:"commit,omitempty"`
	Status        string                           `json:"status,omitempty"`
	Image         string                           `json:"image,omitempty"`
	Workflow      *ComponentWorkflowConfigResponse `json:"workflow,omitempty"`
	CreatedAt     time.Time                        `json:"createdAt"`
}

// ComponentWorkflowConfigResponse represents the workflow configuration in API responses
type ComponentWorkflowConfigResponse struct {
	Name             string                    `json:"name"`
	SystemParameters *SystemParametersResponse `json:"systemParameters,omitempty"`
	Parameters       map[string]any            `json:"parameters,omitempty"`
}

// SystemParametersResponse represents system parameters in API responses
type SystemParametersResponse struct {
	Repository *RepositoryResponse `json:"repository,omitempty"`
}

// RepositoryResponse represents repository configuration in API responses
type RepositoryResponse struct {
	URL      string                      `json:"url"`
	Revision *RepositoryRevisionResponse `json:"revision,omitempty"`
	AppPath  string                      `json:"appPath"`
}

// RepositoryRevisionResponse represents repository revision in API responses
type RepositoryRevisionResponse struct {
	Branch string `json:"branch"`
	Commit string `json:"commit,omitempty"`
}

func SuccessResponse[T any](data T) APIResponse[T] {
	return APIResponse[T]{
		Success: true,
		Data:    data,
	}
}

func ListSuccessResponse[T any](items []T, total, page, pageSize int) APIResponse[ListResponse[T]] {
	return APIResponse[ListResponse[T]]{
		Success: true,
		Data: ListResponse[T]{
			Items:      items,
			TotalCount: total,
			Page:       page,
			PageSize:   pageSize,
		},
	}
}

func CursorListSuccessResponse[T any](items []T, nextCursor string, remainingCount *int64) APIResponse[CursorListResponse[T]] {
	return APIResponse[CursorListResponse[T]]{
		Success: true,
		Data: CursorListResponse[T]{
			Items: items,
			Pagination: PaginationMetadata{
				NextCursor:     nextCursor,
				RemainingCount: remainingCount,
			},
		},
	}
}

func ErrorResponse(message, code string) APIResponse[any] {
	return APIResponse[any]{
		Success: false,
		Error:   message,
		Code:    code,
	}
}

// ComponentTypeResponse represents a ComponentType in API responses
type ComponentTypeResponse struct {
	Name             string    `json:"name"`
	DisplayName      string    `json:"displayName,omitempty"`
	Description      string    `json:"description,omitempty"`
	WorkloadType     string    `json:"workloadType"`
	AllowedWorkflows []string  `json:"allowedWorkflows,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
}

// TraitResponse represents an Trait in API responses
type TraitResponse struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName,omitempty"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ComponentTraitResponse represents a trait instance attached to a component in API responses
type ComponentTraitResponse struct {
	Name         string                 `json:"name"`
	InstanceName string                 `json:"instanceName"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
}

// WorkflowResponse represents a Workflow in API responses
type WorkflowResponse struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName,omitempty"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// WorkflowRunResponse represents a WorkflowRun in API responses
type WorkflowRunResponse struct {
	Name          string                 `json:"name"`
	UUID          string                 `json:"uuid,omitempty"`
	WorkflowName  string                 `json:"workflowName"`
	NamespaceName string                 `json:"namespaceName"`
	Status        string                 `json:"status"`
	Phase         string                 `json:"phase,omitempty"`
	Parameters    map[string]interface{} `json:"parameters,omitempty"`
	CreatedAt     time.Time              `json:"createdAt"`
	FinishedAt    *time.Time             `json:"finishedAt,omitempty"`
}

// ComponentReleaseResponse represents a ComponentRelease in API responses
type ComponentReleaseResponse struct {
	Name          string    `json:"name"`
	ComponentName string    `json:"componentName"`
	ProjectName   string    `json:"projectName"`
	NamespaceName string    `json:"namespaceName"`
	CreatedAt     time.Time `json:"createdAt"`
	Status        string    `json:"status,omitempty"`
}

// ReleaseBindingResponse represents a ReleaseBinding in API responses
type ReleaseBindingResponse struct {
	Name                      string                 `json:"name"`
	ComponentName             string                 `json:"componentName"`
	ProjectName               string                 `json:"projectName"`
	NamespaceName             string                 `json:"namespaceName"`
	Environment               string                 `json:"environment"`
	ReleaseName               string                 `json:"releaseName,omitempty"`
	ComponentTypeEnvOverrides map[string]interface{} `json:"componentTypeEnvOverrides,omitempty"`
	TraitOverrides            map[string]interface{} `json:"traitOverrides,omitempty"`
	WorkloadOverrides         *WorkloadOverrides     `json:"workloadOverrides,omitempty"`
	CreatedAt                 time.Time              `json:"createdAt"`
	Status                    string                 `json:"status,omitempty"`
}

// ReleaseResponse represents a Release in API responses
type ReleaseResponse struct {
	Spec   openchoreov1alpha1.ReleaseSpec   `json:"spec"`
	Status openchoreov1alpha1.ReleaseStatus `json:"status"`
}

// SecretReferenceResponse represents a SecretReference in API responses
type SecretReferenceResponse struct {
	Name            string                 `json:"name"`
	Namespace       string                 `json:"namespace"`
	DisplayName     string                 `json:"displayName,omitempty"`
	Description     string                 `json:"description,omitempty"`
	SecretStores    []SecretStoreReference `json:"secretStores,omitempty"`
	RefreshInterval string                 `json:"refreshInterval,omitempty"`
	Data            []SecretDataSourceInfo `json:"data,omitempty"`
	CreatedAt       time.Time              `json:"createdAt"`
	LastRefreshTime *time.Time             `json:"lastRefreshTime,omitempty"`
	Status          string                 `json:"status,omitempty"`
}

// SecretStoreReference represents where a SecretReference is being used
type SecretStoreReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
}

// SecretDataSourceInfo represents secret data source information for API responses
type SecretDataSourceInfo struct {
	SecretKey string              `json:"secretKey"`
	RemoteRef RemoteReferenceInfo `json:"remoteRef"`
}

// RemoteReferenceInfo represents remote reference information for API responses
type RemoteReferenceInfo struct {
	Key      string `json:"key"`
	Property string `json:"property,omitempty"`
	Version  string `json:"version,omitempty"`
}

// WebhookEventResponse represents the response after processing a webhook event
type WebhookEventResponse struct {
	Success            bool     `json:"success"`
	Message            string   `json:"message"`
	AffectedComponents []string `json:"affectedComponents,omitempty"`
	TriggeredBuilds    int      `json:"triggeredBuilds"`
}

// ObservabilityPlaneResponse represents an observability plane in API responses
type ObservabilityPlaneResponse struct {
	Name            string                         `json:"name"`
	Namespace       string                         `json:"namespace"`
	DisplayName     string                         `json:"displayName,omitempty"`
	Description     string                         `json:"description,omitempty"`
	ObserverURL     string                         `json:"observerURL,omitempty"`
	AgentConnection *AgentConnectionStatusResponse `json:"agentConnection,omitempty"`
	CreatedAt       time.Time                      `json:"createdAt"`
	Status          string                         `json:"status,omitempty"`
}

// ClusterDataPlaneResponse represents a cluster-scoped dataplane in API responses
type ClusterDataPlaneResponse struct {
	Name                    string                         `json:"name"`
	PlaneID                 string                         `json:"planeID"`
	DisplayName             string                         `json:"displayName,omitempty"`
	Description             string                         `json:"description,omitempty"`
	ImagePullSecretRefs     []string                       `json:"imagePullSecretRefs,omitempty"`
	SecretStoreRef          string                         `json:"secretStoreRef,omitempty"`
	PublicVirtualHost       string                         `json:"publicVirtualHost"`
	OrganizationVirtualHost string                         `json:"organizationVirtualHost"`
	PublicHTTPPort          int32                          `json:"publicHTTPPort"`
	PublicHTTPSPort         int32                          `json:"publicHTTPSPort"`
	OrganizationHTTPPort    int32                          `json:"organizationHTTPPort"`
	OrganizationHTTPSPort   int32                          `json:"organizationHTTPSPort"`
	ObservabilityPlaneRef   *ObservabilityPlaneRef         `json:"observabilityPlaneRef,omitempty"`
	AgentConnection         *AgentConnectionStatusResponse `json:"agentConnection,omitempty"`
	CreatedAt               time.Time                      `json:"createdAt"`
	Status                  string                         `json:"status,omitempty"`
}

// ClusterDataPlaneListResult holds a paginated list of cluster data planes.
type ClusterDataPlaneListResult struct {
	Items          []*ClusterDataPlaneResponse
	NextCursor     string
	RemainingCount *int64
}

// ClusterBuildPlaneResponse represents a cluster-scoped buildplane in API responses
type ClusterBuildPlaneResponse struct {
	Name                  string                         `json:"name"`
	PlaneID               string                         `json:"planeID"`
	DisplayName           string                         `json:"displayName,omitempty"`
	Description           string                         `json:"description,omitempty"`
	ObservabilityPlaneRef *ObservabilityPlaneRef         `json:"observabilityPlaneRef,omitempty"`
	AgentConnection       *AgentConnectionStatusResponse `json:"agentConnection,omitempty"`
	CreatedAt             time.Time                      `json:"createdAt"`
	Status                string                         `json:"status,omitempty"`
}

// ClusterObservabilityPlaneResponse represents a cluster-scoped observability plane in API responses
type ClusterObservabilityPlaneResponse struct {
	Name            string                         `json:"name"`
	PlaneID         string                         `json:"planeID"`
	DisplayName     string                         `json:"displayName,omitempty"`
	Description     string                         `json:"description,omitempty"`
	ObserverURL     string                         `json:"observerURL,omitempty"`
	RCAAgentURL     string                         `json:"rcaAgentURL,omitempty"`
	AgentConnection *AgentConnectionStatusResponse `json:"agentConnection,omitempty"`
	CreatedAt       time.Time                      `json:"createdAt"`
	Status          string                         `json:"status,omitempty"`
}

// VersionResponse represents the server version information in API responses.
type VersionResponse struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	GitRevision string `json:"gitRevision"`
	BuildTime   string `json:"buildTime"`
	GoOS        string `json:"goOS"`
	GoArch      string `json:"goArch"`
	GoVersion   string `json:"goVersion"`
}

// GitSecretResponse represents a git secret in API responses
type GitSecretResponse struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// ComponentWorkflowRunStatusResponse represents the status of a component workflow run
type ComponentWorkflowRunStatusResponse struct {
	Status               string               `json:"status"`               // Overall workflow status (Pending/Running/Completed/Failed)
	Steps                []WorkflowStepStatus `json:"steps"`                // Array of step-level statuses
	HasLiveObservability bool                 `json:"hasLiveObservability"` // Whether the workflow run has live observability (logs/events from build plane)
}

// WorkflowStepStatus represents the status of an individual workflow step
type WorkflowStepStatus struct {
	Name       string     `json:"name"`       // Step name/template name
	Phase      string     `json:"phase"`      // Step phase (Pending|Running|Succeeded|Failed|Skipped|Error)
	StartedAt  *time.Time `json:"startedAt"`  // When step started
	FinishedAt *time.Time `json:"finishedAt"` // When step finished
}

// ComponentWorkflowRunLogEntry represents a log entry from a component workflow run
type ComponentWorkflowRunLogEntry struct {
	Timestamp string `json:"timestamp"`
	Log       string `json:"log"`
}

// ComponentWorkflowRunEventEntry represents an event entry for component workflow run events
type ComponentWorkflowRunEventEntry struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
}
