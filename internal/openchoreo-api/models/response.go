package models

import "time"

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
	RepositoryURL      string    `json:"repositoryUrl,omitempty"`
	RepositoryBranch   string    `json:"repositoryBranch,omitempty"`
	DeploymentPipeline string    `json:"deploymentOipeline,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	Status             string    `json:"status,omitempty"`
}

// ComponentResponse represents a component in API responses
type ComponentResponse struct {
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	Type          string    `json:"type"`
	ProjectName   string    `json:"projectName"`
	OrgName       string    `json:"orgName"`
	RepositoryURL string    `json:"repositoryUrl"`
	Branch        string    `json:"branch,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	Status        string    `json:"status,omitempty"`
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
