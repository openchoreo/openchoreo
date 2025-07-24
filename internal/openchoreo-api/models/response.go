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

// ProjectResponse represents a project in API responses
type ProjectResponse struct {
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
	Name           string                                 `json:"name"`
	DisplayName    string                                 `json:"displayName,omitempty"`
	Description    string                                 `json:"description,omitempty"`
	Type           string                                 `json:"type"`
	ProjectName    string                                 `json:"projectName"`
	OrgName        string                                 `json:"orgName"`
	CreatedAt      time.Time                              `json:"createdAt"`
	Status         string                                 `json:"status,omitempty"`
	Service        *openchoreov1alpha1.ServiceSpec        `json:"service,omitempty"`
	WebApplication *openchoreov1alpha1.WebApplicationSpec `json:"webApplication,omitempty"`
	ScheduledTask  *openchoreov1alpha1.ScheduledTaskSpec  `json:"scheduledTask,omitempty"`
	API            *openchoreov1alpha1.APISpec            `json:"api,omitempty"`
	Workload       *openchoreov1alpha1.WorkloadSpec       `json:"workload,omitempty"`
	BuildConfig    *BuildConfig                           `json:"buildConfig,omitempty"`
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
	BindingStatusTypePending    BindingStatusType = "InProgress"
	BindingStatusTypeReady      BindingStatusType = "Active"
	BindingStatusTypeFailed     BindingStatusType = "Failed"
	BindingStatusTypeSuspended  BindingStatusType = "Suspended"
	BindingStatusTypeInProgress BindingStatusType = "NotYetDeployed"
)

type BindingStatus struct {
	Reason           string            `json:"reason"`
	Message          string            `json:"message"`
	Status           BindingStatusType `json:"status"`
	LastTransitioned time.Time         `json:"lastTransitioned"`
}

type ServiceBinding struct {
	Endpoints []EndpointStatus `json:"endpoints"`
	Image     string           `json:"image,omitempty"`
}

type WebApplicationBinding struct {
	Endpoints []EndpointStatus `json:"endpoints"`
	Image     string           `json:"image,omitempty"`
}

type ScheduledTaskBinding struct {
	Image string `json:"image,omitempty"`
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
	RegistryPrefix          string    `json:"registryPrefix"`
	RegistrySecretRef       string    `json:"registrySecretRef,omitempty"`
	KubernetesClusterName   string    `json:"kubernetesClusterName"`
	APIServerURL            string    `json:"apiServerURL"`
	PublicVirtualHost       string    `json:"publicVirtualHost"`
	OrganizationVirtualHost string    `json:"organizationVirtualHost"`
	ObserverURL             string    `json:"observerURL,omitempty"`
	ObserverUsername        string    `json:"observerUsername,omitempty"`
	CreatedAt               time.Time `json:"createdAt"`
	Status                  string    `json:"status,omitempty"`
}

// BuildPlaneResponse represents a buildplane in API responses
type BuildPlaneResponse struct {
	Name                  string    `json:"name"`
	Namespace             string    `json:"namespace"`
	DisplayName           string    `json:"displayName,omitempty"`
	Description           string    `json:"description,omitempty"`
	KubernetesClusterName string    `json:"kubernetesClusterName"`
	APIServerURL          string    `json:"apiServerURL"`
	ObserverURL           string    `json:"observerURL,omitempty"`
	ObserverUsername      string    `json:"observerUsername,omitempty"`
	CreatedAt             time.Time `json:"createdAt"`
	Status                string    `json:"status,omitempty"`
}

// BuildResponse represents a build in API responses
type BuildResponse struct {
	Name          string    `json:"name"`
	UUID          string    `json:"uuid"`
	ComponentName string    `json:"componentName"`
	ProjectName   string    `json:"projectName"`
	OrgName       string    `json:"orgName"`
	Commit        string    `json:"commit,omitempty"`
	Status        string    `json:"status,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}

// BuildTemplateResponse represents a build template (ClusterWorkflowTemplate) in API responses
type BuildTemplateResponse struct {
	Name       string                   `json:"name"`
	Parameters []BuildTemplateParameter `json:"parameters,omitempty"`
	CreatedAt  time.Time                `json:"createdAt"`
}

// BuildTemplateParameter represents a parameter of a build template
type BuildTemplateParameter struct {
	Name    string `json:"name"`
	Default string `json:"default,omitempty"`
}

// Response helper functions
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

func ErrorResponse(message, code string) APIResponse[any] {
	return APIResponse[any]{
		Success: false,
		Error:   message,
		Code:    code,
	}
}
