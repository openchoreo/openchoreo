// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"errors"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
)

// CreateProjectRequest represents the request to create a new project
type CreateProjectRequest struct {
	Name               string `json:"name"`
	DisplayName        string `json:"displayName,omitempty"`
	Description        string `json:"description,omitempty"`
	DeploymentPipeline string `json:"deploymentPipeline,omitempty"`
}

// BuildConfig represents the build configuration for a component

type TemplateParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Workflow struct {
	Name   string                `json:"name"`
	Schema *runtime.RawExtension `json:"schema,omitempty"`
}

// CreateComponentRequest represents the request to create a new component
type CreateComponentRequest struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName,omitempty"`
	Description string    `json:"description,omitempty"`
	Type        string    `json:"type"`
	Workflow    *Workflow `json:"workflow,omitempty"`
}

// PromoteComponentRequest Promote from one environment to another
type PromoteComponentRequest struct {
	SourceEnvironment string `json:"sourceEnv"`
	TargetEnvironment string `json:"targetEnv"`
	// TODO Support overrides for the target environment
}

type CreateComponentReleaseRequest struct {
	ReleaseName string `json:"releaseName,omitempty"`
}

// Sanitize sanitizes the CreateComponentReleaseRequest by trimming whitespace
func (req *CreateComponentReleaseRequest) Sanitize() {
	req.ReleaseName = strings.TrimSpace(req.ReleaseName)
}

// DeployReleaseRequest represents the request to deploy a release to the lowest environment
type DeployReleaseRequest struct {
	ReleaseName string `json:"releaseName"`
}

// Sanitize sanitizes the DeployReleaseRequest by trimming whitespace
func (req *DeployReleaseRequest) Sanitize() {
	req.ReleaseName = strings.TrimSpace(req.ReleaseName)
}

// Validate validates the DeployReleaseRequest
func (req *DeployReleaseRequest) Validate() error {
	if req.ReleaseName == "" {
		return errors.New("releaseName is required")
	}
	return nil
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
	Name                    string `json:"name"`
	DisplayName             string `json:"displayName,omitempty"`
	Description             string `json:"description,omitempty"`
	KubernetesClusterName   string `json:"kubernetesClusterName"`
	APIServerURL            string `json:"apiServerURL"`
	CACert                  string `json:"caCert"`
	ClientCert              string `json:"clientCert"`
	ClientKey               string `json:"clientKey"`
	PublicVirtualHost       string `json:"publicVirtualHost"`
	OrganizationVirtualHost string `json:"organizationVirtualHost"`
	ObserverURL             string `json:"observerURL,omitempty"`
	ObserverUsername        string `json:"observerUsername,omitempty"`
	ObserverPassword        string `json:"observerPassword,omitempty"`
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

// Validate validates the PromoteComponentRequest
func (req *PromoteComponentRequest) Validate() error {
	// TODO: Implement custom validation using Go stdlib
	return nil
}

// Sanitize sanitizes the CreateProjectRequest by trimming whitespace
func (req *CreateProjectRequest) Sanitize() {
	req.Name = strings.TrimSpace(req.Name)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Description = strings.TrimSpace(req.Description)
	req.DeploymentPipeline = strings.TrimSpace(req.DeploymentPipeline)
}

// Sanitize sanitizes the CreateComponentRequest by trimming whitespace
func (req *CreateComponentRequest) Sanitize() {
	req.Name = strings.TrimSpace(req.Name)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Description = strings.TrimSpace(req.Description)
	req.Type = strings.TrimSpace(req.Type)
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

// Sanitize sanitizes the PromoteComponentRequest by trimming whitespace
func (req *PromoteComponentRequest) Sanitize() {
	req.SourceEnvironment = strings.TrimSpace(req.SourceEnvironment)
	req.TargetEnvironment = strings.TrimSpace(req.TargetEnvironment)
}

type BindingReleaseState string

const (
	ReleaseStateActive   BindingReleaseState = "Active"
	ReleaseStateSuspend  BindingReleaseState = "Suspend"
	ReleaseStateUndeploy BindingReleaseState = "Undeploy"
)

// UpdateBindingRequest represents the request to update a component binding
// Only includes fields that can be updated via PATCH
type UpdateBindingRequest struct {
	// ReleaseState controls the state of the Release created by this binding.
	// Valid values: Active, Suspend, Undeploy
	ReleaseState BindingReleaseState `json:"releaseState"`
}

// Validate validates the UpdateBindingRequest
func (req *UpdateBindingRequest) Validate() error {
	// Validate releaseState values
	switch req.ReleaseState {
	case "Active", "Suspend", "Undeploy":
		// Valid values
	case "":
		// Empty is not allowed for PATCH
		return errors.New("releaseState is required")
	default:
		return errors.New("releaseState must be one of: Active, Suspend, Undeploy")
	}
	return nil
}

// PatchReleaseBindingRequest represents the request to patch a ReleaseBinding
type PatchReleaseBindingRequest struct {
	// ReleaseName is the name of the release to bind (required when creating a new binding)
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`

	// Environment is the target environment (required when creating a new binding)
	// +optional
	Environment string `json:"environment,omitempty"`

	// ComponentTypeEnvOverrides for ComponentType envOverrides parameters
	// These values override the defaults defined in the Component for this specific environment
	// +optional
	ComponentTypeEnvOverrides map[string]interface{} `json:"componentTypeEnvOverrides,omitempty"`

	// TraitOverrides provides environment-specific overrides for trait configurations
	// Keyed by instanceName (which must be unique across all traits in the component)
	// Structure: map[instanceName]overrideValues
	// +optional
	TraitOverrides map[string]map[string]interface{} `json:"traitOverrides,omitempty"`

	// ConfigurationOverrides provides environment-specific overrides for workload configurations
	// +optional
	ConfigurationOverrides *ConfigurationOverrides `json:"configurationOverrides,omitempty"`
}

// ConfigurationOverrides represents environment-specific configuration overrides
type ConfigurationOverrides struct {
	// Environment variable overrides
	// +optional
	Env []EnvVar `json:"env,omitempty"`

	// File configuration overrides
	// +optional
	Files []FileVar `json:"files,omitempty"`
}

// EnvVar represents an environment variable
type EnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// FileVar represents a file configuration
type FileVar struct {
	Key       string `json:"key"`
	MountPath string `json:"mountPath"`
	Value     string `json:"value,omitempty"`
}
