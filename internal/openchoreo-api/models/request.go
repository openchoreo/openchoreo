package models

import (
	"strings"
)

// CreateProjectRequest represents the request to create a new project
type CreateProjectRequest struct {
	Name               string `json:"name"`
	RepositoryURL      string `json:"repositoryUrl,omitempty"`
	RepositoryBranch   string `json:"repositoryBranch,omitempty"`
	DeploymentPipeline string `json:"deploymentPipeline,omitempty"`
}

// CreateComponentRequest represents the request to create a new component
type CreateComponentRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Type          string `json:"type"`
	RepositoryURL string `json:"repositoryUrl"`
	Branch        string `json:"branch,omitempty"`
}

// Validate validates the CreateProjectRequest
func (req *CreateProjectRequest) Validate() error {
	// TODO: Implement custom validation using Go stdlib
	return nil
}

// Validate validates the CreateComponentRequest
func (req *CreateComponentRequest) Validate() error {
	// TODO: Implement custom validation using Go stdlib
	return nil
}

// Sanitize sanitizes the CreateProjectRequest by trimming whitespace
func (req *CreateProjectRequest) Sanitize() {
	req.Name = strings.TrimSpace(req.Name)
	req.RepositoryURL = strings.TrimSpace(req.RepositoryURL)
	req.RepositoryBranch = strings.TrimSpace(req.RepositoryBranch)
	req.DeploymentPipeline = strings.TrimSpace(req.DeploymentPipeline)
}

// Sanitize sanitizes the CreateComponentRequest by trimming whitespace
func (req *CreateComponentRequest) Sanitize() {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.Type = strings.TrimSpace(req.Type)
	req.RepositoryURL = strings.TrimSpace(req.RepositoryURL)
	req.Branch = strings.TrimSpace(req.Branch)
}
