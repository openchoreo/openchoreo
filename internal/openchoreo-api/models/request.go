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

// CreateEnvironmentRequest represents the request to create a new environment
type CreateEnvironmentRequest struct {
	Name         string `json:"name"`
	DisplayName  string `json:"displayName,omitempty"`
	Description  string `json:"description,omitempty"`
	DataPlaneRef string `json:"dataPlaneRef,omitempty"`
	IsProduction bool   `json:"isProduction"`
	DNSPrefix    string `json:"dnsPrefix,omitempty"`
}

// CreateDataPlaneRequest represents the request to create a new dataplane
type CreateDataPlaneRequest struct {
	Name                        string                       `json:"name"`
	DisplayName                 string                       `json:"displayName,omitempty"`
	Description                 string                       `json:"description,omitempty"`
	RegistryPrefix              string                       `json:"registryPrefix"`
	RegistrySecretRef           string                       `json:"registrySecretRef,omitempty"`
	KubernetesClusterName       string                       `json:"kubernetesClusterName"`
	APIServerURL                string                       `json:"apiServerURL"`
	CACert                      string                       `json:"caCert"`
	ClientCert                  string                       `json:"clientCert"`
	ClientKey                   string                       `json:"clientKey"`
	PublicVirtualHost           string                       `json:"publicVirtualHost"`
	OrganizationVirtualHost     string                       `json:"organizationVirtualHost"`
	ObserverURL                 string                       `json:"observerURL,omitempty"`
	ObserverUsername            string                       `json:"observerUsername,omitempty"`
	ObserverPassword            string                       `json:"observerPassword,omitempty"`
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

// Validate validates the CreateEnvironmentRequest
func (req *CreateEnvironmentRequest) Validate() error {
	// TODO: Implement custom validation using Go stdlib
	return nil
}

// Validate validates the CreateDataPlaneRequest
func (req *CreateDataPlaneRequest) Validate() error {
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

// Sanitize sanitizes the CreateEnvironmentRequest by trimming whitespace
func (req *CreateEnvironmentRequest) Sanitize() {
	req.Name = strings.TrimSpace(req.Name)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Description = strings.TrimSpace(req.Description)
	req.DataPlaneRef = strings.TrimSpace(req.DataPlaneRef)
	req.DNSPrefix = strings.TrimSpace(req.DNSPrefix)
}

// Sanitize sanitizes the CreateDataPlaneRequest by trimming whitespace
func (req *CreateDataPlaneRequest) Sanitize() {
	req.Name = strings.TrimSpace(req.Name)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Description = strings.TrimSpace(req.Description)
	req.RegistryPrefix = strings.TrimSpace(req.RegistryPrefix)
	req.RegistrySecretRef = strings.TrimSpace(req.RegistrySecretRef)
	req.KubernetesClusterName = strings.TrimSpace(req.KubernetesClusterName)
	req.APIServerURL = strings.TrimSpace(req.APIServerURL)
	req.CACert = strings.TrimSpace(req.CACert)
	req.ClientCert = strings.TrimSpace(req.ClientCert)
	req.ClientKey = strings.TrimSpace(req.ClientKey)
	req.PublicVirtualHost = strings.TrimSpace(req.PublicVirtualHost)
	req.OrganizationVirtualHost = strings.TrimSpace(req.OrganizationVirtualHost)
	
	req.ObserverURL = strings.TrimSpace(req.ObserverURL)
	req.ObserverUsername = strings.TrimSpace(req.ObserverUsername)
	req.ObserverPassword = strings.TrimSpace(req.ObserverPassword)
}
