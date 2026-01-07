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

// ListResponse represents a list response
type ListResponse[T any] struct {
	Items    []T              `json:"items"`
	Metadata ResponseMetadata `json:"metadata"`
}

// ResponseMetadata contains metadata following Kubernetes API conventions
type ResponseMetadata struct {
	ResourceVersion string `json:"resourceVersion"`
	Continue        string `json:"continue,omitempty"` // Empty when no more results
	HasMore         bool   `json:"hasMore"`            // True if more results available
}

// ProjectResponse represents a project in API responses
type ProjectResponse struct {
	UID                string    `json:"uid"`
	Name               string    `json:"name"`
	OrgName            string    `json:"orgName"`
	DisplayName        string    `json:"displayName,omitempty"`
	Description        string    `json:"description,omitempty"`
	DeploymentPipeline string    `json:"deploymentPipeline,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	Status             string    `json:"status,omitempty"`
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
	OrgName           string                           `json:"orgName"`
	CreatedAt         time.Time                        `json:"createdAt"`
	Status            string                           `json:"status,omitempty"`
	Workload          *openchoreov1alpha1.WorkloadSpec `json:"workload,omitempty"`
	ComponentWorkflow *ComponentWorkflow               `json:"componentWorkflow,omitempty"`
}

type BindingResponse struct {
	Name          string        `json:"name"`
	Type          string        `json:"type"`
	ComponentName string        `json:"componentName"`
	ProjectName   string        `json:"projectName"`
	OrgName       string        `json:"orgName"`
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
	Name         string           `json:"name"`
	Type         string           `json:"type"`
	Project      *ExposedEndpoint `json:"project,omitempty"`
	Organization *ExposedEndpoint `json:"organization,omitempty"`
	Public       *ExposedEndpoint `json:"public,omitempty"`
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
	OrgName        string          `json:"orgName"`
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

// OrganizationResponse represents an organization in API responses
type OrganizationResponse struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName,omitempty"`
	Description string    `json:"description,omitempty"`
	Namespace   string    `json:"namespace,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	Status      string    `json:"status,omitempty"`
}

// EnvironmentResponse represents an environment in API responses
type EnvironmentResponse struct {
	UID          string    `json:"uid"`
	Name         string    `json:"name"`
	Namespace    string    `json:"namespace"`
	DisplayName  string    `json:"displayName,omitempty"`
	Description  string    `json:"description,omitempty"`
	DataPlaneRef string    `json:"dataPlaneRef,omitempty"`
	IsProduction bool      `json:"isProduction"`
	DNSPrefix    string    `json:"dnsPrefix,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	Status       string    `json:"status,omitempty"`
}

// DataPlaneResponse represents a dataplane in API responses
type DataPlaneResponse struct {
	Name                    string    `json:"name"`
	Namespace               string    `json:"namespace"`
	DisplayName             string    `json:"displayName,omitempty"`
	Description             string    `json:"description,omitempty"`
	ImagePullSecretRefs     []string  `json:"imagePullSecretRefs,omitempty"`
	SecretStoreRef          string    `json:"secretStoreRef,omitempty"`
	PublicVirtualHost       string    `json:"publicVirtualHost"`
	OrganizationVirtualHost string    `json:"organizationVirtualHost"`
	PublicHTTPPort          int32     `json:"publicHTTPPort"`
	PublicHTTPSPort         int32     `json:"publicHTTPSPort"`
	OrganizationHTTPPort    int32     `json:"organizationHTTPPort"`
	OrganizationHTTPSPort   int32     `json:"organizationHTTPSPort"`
	ObservabilityPlaneRef   string    `json:"observabilityPlaneRef,omitempty"`
	CreatedAt               time.Time `json:"createdAt"`
	Status                  string    `json:"status,omitempty"`
}

// BuildPlaneResponse represents a buildplane in API responses
type BuildPlaneResponse struct {
	Name                  string    `json:"name"`
	Namespace             string    `json:"namespace"`
	DisplayName           string    `json:"displayName,omitempty"`
	Description           string    `json:"description,omitempty"`
	ObservabilityPlaneRef string    `json:"observabilityPlaneRef,omitempty"`
	CreatedAt             time.Time `json:"createdAt"`
	Status                string    `json:"status,omitempty"`
}

// ComponentWorkflowResponse represents a component workflow run in API responses
type ComponentWorkflowResponse struct {
	Name          string                           `json:"name"`
	UUID          string                           `json:"uuid"`
	OrgName       string                           `json:"orgName"`
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

// ListSuccessResponse creates a successful list API response
func ListSuccessResponse[T any](items []T, resourceVersion, continueToken string) APIResponse[ListResponse[T]] {
	return APIResponse[ListResponse[T]]{
		Success: true,
		Data: ListResponse[T]{
			Items: items,
			Metadata: ResponseMetadata{
				ResourceVersion: resourceVersion,
				Continue:        continueToken,
				HasMore:         continueToken != "",
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

// ComponentReleaseResponse represents a ComponentRelease in API responses
type ComponentReleaseResponse struct {
	Name          string    `json:"name"`
	ComponentName string    `json:"componentName"`
	ProjectName   string    `json:"projectName"`
	OrgName       string    `json:"orgName"`
	CreatedAt     time.Time `json:"createdAt"`
	Status        string    `json:"status,omitempty"`
}

// ReleaseBindingResponse represents a ReleaseBinding in API responses
type ReleaseBindingResponse struct {
	Name                      string                 `json:"name"`
	ComponentName             string                 `json:"componentName"`
	ProjectName               string                 `json:"projectName"`
	OrgName                   string                 `json:"orgName"`
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
	Name        string    `json:"name"`
	Namespace   string    `json:"namespace"`
	DisplayName string    `json:"displayName,omitempty"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	Status      string    `json:"status,omitempty"`
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
