// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"strings"

	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// ValidateParams validates command parameters based on command and resource types
func ValidateParams(cmdType CommandType, resource ResourceType, params interface{}) error {
	switch resource {
	case ResourceProject:
		return validateProjectParams(cmdType, params)
	case ResourceComponent:
		return validateComponentParams(cmdType, params)
	case ResourceBuild:
		return validateBuildParams(cmdType, params)
	case ResourceDeployment:
		return validateDeploymentParams(cmdType, params)
	case ResourceDeploymentTrack:
		return validateDeploymentTrackParams(cmdType, params)
	case ResourceEnvironment:
		return validateEnvironmentParams(cmdType, params)
	case ResourceDeployableArtifact:
		return validateDeployableArtifactParams(cmdType, params)
	case ResourceDataPlane:
		return validateDataPlaneParams(cmdType, params)
	case ResourceOrganization:
		return validateOrganizationParams(cmdType, params)
	case ResourceEndpoint:
		return validateEndpointParams(cmdType, params)
	case ResourceLogs:
		return validateLogParams(cmdType, params)
	case ResourceApply:
		return validateApplyParams(cmdType, params)
	case ResourceDeploymentPipeline:
		return validateDeploymentPipelineParams(cmdType, params)
	case ResourceConfigurationGroup:
		return validateConfigurationGroupParams(cmdType, params)
	default:
		return fmt.Errorf("unknown resource type: %s", resource)
	}
}

// validateProjectParams validates parameters for project operations
func validateProjectParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdCreate:
		if p, ok := params.(api.CreateProjectParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
				"name":         p.Name,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceProject, fields)
			}
		}
	case CmdGet:
		if p, ok := params.(api.GetProjectParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceProject, fields)
			}
		}
	}
	return nil
}

// validateComponentParams validates parameters for component operations
func validateComponentParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdCreate:
		if p, ok := params.(api.CreateComponentParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
				"project":      p.Project,
				"name":         p.Name,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceComponent, fields)
			}
			return ValidateGitHubURL(p.GitRepositoryURL)
		}
	case CmdGet:
		if p, ok := params.(api.GetComponentParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
				"project":      p.Project,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceComponent, fields)
			}
		}
	}
	return nil
}

// validateBuildParams validates parameters for build operations
func validateBuildParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdCreate:
		if p, ok := params.(api.CreateBuildParams); ok {
			// All required fields
			requiredFields := map[string]string{
				"organization": p.Organization,
				"project":      p.Project,
				"component":    p.Component,
				"name":         p.Name,
			}

			if !checkRequiredFields(requiredFields) {
				return generateHelpError(cmdType, ResourceBuild, requiredFields)
			}
		}

	case CmdGet:
		if p, ok := params.(api.GetBuildParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
				"project":      p.Project,
				"component":    p.Component,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceBuild, fields)
			}
		}
	}
	return nil
}

// validateDeploymentParams validates parameters for deployment operations
func validateDeploymentParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdCreate:
		if p, ok := params.(api.CreateDeploymentParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
				"project":      p.Project,
				"component":    p.Component,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceDeployment, fields)
			}
		}
	case CmdGet:
		if p, ok := params.(api.GetDeploymentParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
				"project":      p.Project,
				"component":    p.Component,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceDeployment, fields)
			}
		}
	}
	return nil
}

// validateDeploymentTrackParams validates parameters for deployment track operations
func validateDeploymentTrackParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdCreate:
		if p, ok := params.(api.CreateDeploymentTrackParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
				"project":      p.Project,
				"component":    p.Component,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceDeploymentTrack, fields)
			}
		}
	case CmdGet:
		if p, ok := params.(api.GetDeploymentTrackParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
				"project":      p.Project,
				"component":    p.Component,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceDeploymentTrack, fields)
			}
		}
	}
	return nil
}

// validateEnvironmentParams validates parameters for environment operations
func validateEnvironmentParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdCreate:
		if p, ok := params.(api.CreateEnvironmentParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
				"name":         p.Name,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceEnvironment, fields)
			}
		}
	case CmdGet:
		if p, ok := params.(api.GetEnvironmentParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceEnvironment, fields)
			}
		}
	}
	return nil
}

// validateDeployableArtifactParams validates parameters for deployable artifact operations
func validateDeployableArtifactParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdCreate:
		if p, ok := params.(api.CreateDeployableArtifactParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
				"project":      p.Project,
				"component":    p.Component,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceDeployableArtifact, fields)
			}
		}
	case CmdGet:
		if p, ok := params.(api.GetDeployableArtifactParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
				"project":      p.Project,
				"component":    p.Component,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceDeployableArtifact, fields)
			}
		}
	}
	return nil
}

// validateLogParams validates parameters for log operations
func validateLogParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdLogs {
		if p, ok := params.(api.LogParams); ok {
			// For non-interactive mode, validate required fields
			if !p.Interactive {
				// Check type parameter first
				if p.Type == "" {
					fields := map[string]string{
						"type": "",
					}
					// Use empty resource string since this is a top-level parameter
					return generateHelpError(cmdType, "", fields)
				}

				// Validate type-specific required fields based on the type
				switch p.Type {
				case "build":
					buildFields := map[string]string{
						"organization": p.Organization,
						"build":        p.Build,
					}
					if !checkRequiredFields(buildFields) {
						return generateHelpError(cmdType, ResourceLogs, buildFields)
					}
				case "deployment":
					deployFields := map[string]string{
						"organization": p.Organization,
						"project":      p.Project,
						"component":    p.Component,
						"environment":  p.Environment,
						"deployment":   p.Deployment,
					}
					if !checkRequiredFields(deployFields) {
						return generateHelpError(cmdType, ResourceLogs, deployFields)
					}
				default:
					return fmt.Errorf("log type '%s' not supported. Valid types are: build, deployment", p.Type)
				}
			}
		}
	}
	return nil
}

// validateDataPlaneParams validates parameters for data plane operations
func validateDataPlaneParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdGet:
		if p, ok := params.(api.GetDataPlaneParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceDataPlane, fields)
			}
		}
	case CmdCreate:
		if p, ok := params.(api.CreateDataPlaneParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
				"name":         p.Name,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceDataPlane, fields)
			}
		}
	}
	return nil
}

// validateOrganizationParams validates parameters for organization operations
func validateOrganizationParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdCreate {
		if p, ok := params.(api.CreateOrganizationParams); ok {
			fields := map[string]string{
				"name": p.Name,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceOrganization, fields)
			}
		}
	}
	return nil
}

// validateEndpointParams validates parameters for endpoint operations
func validateEndpointParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdGet {
		if p, ok := params.(api.GetEndpointParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
				"project":      p.Project,
				"component":    p.Component,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceEndpoint, fields)
			}
		}
	}
	return nil
}

// validateApplyParams validates parameters for apply operations
func validateApplyParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdApply {
		if p, ok := params.(api.ApplyParams); ok {
			fields := map[string]string{
				"file": p.FilePath,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, "", fields)
			}
		}
	}
	return nil
}

// Add validation function:
func validateDeploymentPipelineParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdGet:
		if p, ok := params.(api.GetDeploymentPipelineParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceDeploymentPipeline, fields)
			}
		}
	case CmdCreate:
		if p, ok := params.(api.CreateDeploymentPipelineParams); ok {
			fields := map[string]string{
				"organization":      p.Organization,
				"name":              p.Name,
				"environment-order": strings.Join(p.EnvironmentOrder, ","),
			}

			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceDeploymentPipeline, fields)
			}
		}
	}
	return nil
}

func validateConfigurationGroupParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdGet {
		if p, ok := params.(api.GetConfigurationGroupParams); ok {
			fields := map[string]string{
				"organization": p.Organization,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceConfigurationGroup, fields)
			}
		}
	}
	return nil
}
