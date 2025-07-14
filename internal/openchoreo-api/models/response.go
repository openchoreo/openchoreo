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
	RepositoryURL      string    `json:"repositoryUrl,omitempty"`
	RepositoryBranch   string    `json:"repositoryBranch,omitempty"`
	DeploymentPipeline string    `json:"deploymentPipeline,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	Status             string    `json:"status,omitempty"`
}

// ComponentResponse represents a component in API responses
type ComponentResponse struct {
	Name           string                                 `json:"name"`
	Description    string                                 `json:"description,omitempty"`
	Type           string                                 `json:"type"`
	ProjectName    string                                 `json:"projectName"`
	OrgName        string                                 `json:"orgName"`
	RepositoryURL  string                                 `json:"repositoryUrl"`
	Branch         string                                 `json:"branch,omitempty"`
	CreatedAt      time.Time                              `json:"createdAt"`
	Status         string                                 `json:"status,omitempty"`
	Service        *openchoreov1alpha1.ServiceSpec        `json:"service,omitempty"`
	WebApplication *openchoreov1alpha1.WebApplicationSpec `json:"webApplication,omitempty"`
	ScheduledTask  *openchoreov1alpha1.ScheduledTaskSpec  `json:"scheduledTask,omitempty"`
	API            *openchoreov1alpha1.APISpec            `json:"api,omitempty"`
	Workload       *openchoreov1alpha1.WorkloadSpec       `json:"workload,omitempty"`
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

// Response helper functions
func SuccessResponse[T any](data T) APIResponse[T] {
	return APIResponse[T]{
		Success: true,
		Data:    data,
	}
}

func ErrorResponse(message, code string) APIResponse[any] {
	return APIResponse[any]{
		Success: false,
		Error:   message,
		Code:    code,
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
