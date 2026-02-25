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
	case ResourceNamespace:
		return validateNamespaceParams(cmdType, params)
	case ResourceEndpoint:
		return validateEndpointParams(cmdType, params)
	case ResourceLogs:
		return validateLogParams(cmdType, params)
	case ResourceApply:
		return validateApplyParams(cmdType, params)
	case ResourceDelete:
		return validateDeleteParams(cmdType, params)
	case ResourceDeploymentPipeline:
		return validateDeploymentPipelineParams(cmdType, params)
	case ResourceConfigurationGroup:
		return validateConfigurationGroupParams(cmdType, params)
	case ResourceWorkload:
		return validateWorkloadParams(cmdType, params)
	case ResourceBuildPlane:
		return validateBuildPlaneParams(cmdType, params)
	case ResourceObservabilityPlane:
		return validateObservabilityPlaneParams(cmdType, params)
	case ResourceComponentType:
		return validateComponentTypeParams(cmdType, params)
	case ResourceTrait:
		return validateTraitParams(cmdType, params)
	case ResourceWorkflow:
		return validateWorkflowParams(cmdType, params)
	case ResourceSecretReference:
		return validateSecretReferenceParams(cmdType, params)
	case ResourceComponentRelease:
		return validateComponentReleaseParams(cmdType, params)
	case ResourceReleaseBinding:
		return validateReleaseBindingParams(cmdType, params)
	case ResourceWorkflowRun:
		return validateWorkflowRunParams(cmdType, params)
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
				"namespace": p.Namespace,
				"name":      p.Name,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceProject, fields)
			}
		}
	case CmdGet:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdGet, ResourceProject, p.GetNamespace())
		}
	case CmdDelete:
		return validateDeleteProjectParams(params)
	case CmdList:
		return validateProjectListParams(params)
	}
	return nil
}

// deleteProjectParams is an interface for delete project parameter validation
type deleteProjectParams interface {
	GetNamespace() string
	GetProjectName() string
}

// validateDeleteProjectParams validates parameters for delete project operations
func validateDeleteProjectParams(params interface{}) error {
	if p, ok := params.(deleteProjectParams); ok {
		fields := map[string]string{
			"namespace": p.GetNamespace(),
			"name":      p.GetProjectName(),
		}
		if !checkRequiredFields(fields) {
			return generateHelpError(CmdDelete, ResourceProject, fields)
		}
	}
	return nil
}

// validateComponentParams validates parameters for component operations
func validateComponentParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdGet:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdGet, ResourceComponent, p.GetNamespace())
		}
	case CmdDelete:
		return validateDeleteComponentParams(params)
	case CmdList:
		return validateComponentListParams(params)
	case CmdDeploy:
		return validateDeployComponentParams(params)
	}
	return nil
}

// deleteComponentParams is an interface for delete component parameter validation
type deleteComponentParams interface {
	GetNamespace() string
	GetComponentName() string
}

// validateDeleteComponentParams validates parameters for delete component operations
func validateDeleteComponentParams(params interface{}) error {
	if p, ok := params.(deleteComponentParams); ok {
		fields := map[string]string{
			"namespace": p.GetNamespace(),
			"name":      p.GetComponentName(),
		}
		if !checkRequiredFields(fields) {
			return generateHelpError(CmdDelete, ResourceComponent, fields)
		}
	}
	return nil
}

// deployComponentParams is an interface for deploy component parameter validation
type deployComponentParams interface {
	GetNamespace() string
	GetProject() string
	GetComponentName() string
}

// validateDeployComponentParams validates parameters for deploy component operations
func validateDeployComponentParams(params interface{}) error {
	if p, ok := params.(deployComponentParams); ok {
		fields := map[string]string{
			"namespace": p.GetNamespace(),
			"project":   p.GetProject(),
		}
		if !checkRequiredFields(fields) {
			return generateHelpError(CmdDeploy, ResourceComponent, fields)
		}
		if p.GetComponentName() == "" {
			return fmt.Errorf("component name is required")
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
				"namespace": p.Namespace,
				"project":   p.Project,
				"component": p.Component,
				"name":      p.Name,
			}

			if !checkRequiredFields(requiredFields) {
				return generateHelpError(cmdType, ResourceBuild, requiredFields)
			}
		}

	case CmdGet:
		if p, ok := params.(api.GetBuildParams); ok {
			fields := map[string]string{
				"namespace": p.Namespace,
				"project":   p.Project,
				"component": p.Component,
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
				"namespace": p.Namespace,
				"project":   p.Project,
				"component": p.Component,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceDeployment, fields)
			}
		}
	case CmdGet:
		if p, ok := params.(api.GetDeploymentParams); ok {
			fields := map[string]string{
				"namespace": p.Namespace,
				"project":   p.Project,
				"component": p.Component,
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
				"namespace": p.Namespace,
				"project":   p.Project,
				"component": p.Component,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceDeploymentTrack, fields)
			}
		}
	case CmdGet:
		if p, ok := params.(api.GetDeploymentTrackParams); ok {
			fields := map[string]string{
				"namespace": p.Namespace,
				"project":   p.Project,
				"component": p.Component,
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
				"namespace": p.Namespace,
				"name":      p.Name,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceEnvironment, fields)
			}
		}
	case CmdGet:
		if p, ok := params.(api.GetEnvironmentParams); ok {
			fields := map[string]string{
				"namespace": p.Namespace,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceEnvironment, fields)
			}
		}
	case CmdList:
		return validateEnvironmentListParams(params)
	}
	return nil
}

// validateDeployableArtifactParams validates parameters for deployable artifact operations
func validateDeployableArtifactParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdCreate:
		if p, ok := params.(api.CreateDeployableArtifactParams); ok {
			fields := map[string]string{
				"namespace": p.Namespace,
				"project":   p.Project,
				"component": p.Component,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceDeployableArtifact, fields)
			}
		}
	case CmdGet:
		if p, ok := params.(api.GetDeployableArtifactParams); ok {
			fields := map[string]string{
				"namespace": p.Namespace,
				"project":   p.Project,
				"component": p.Component,
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
			// Validate required fields
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
					"namespace": p.Namespace,
					"build":     p.Build,
				}
				if !checkRequiredFields(buildFields) {
					return generateHelpError(cmdType, ResourceLogs, buildFields)
				}
			case "deployment":
				deployFields := map[string]string{
					"namespace":   p.Namespace,
					"project":     p.Project,
					"component":   p.Component,
					"environment": p.Environment,
					"deployment":  p.Deployment,
				}
				if !checkRequiredFields(deployFields) {
					return generateHelpError(cmdType, ResourceLogs, deployFields)
				}
			default:
				return fmt.Errorf("log type '%s' not supported. Valid types are: build, deployment", p.Type)
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
				"namespace": p.Namespace,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceDataPlane, fields)
			}
		}
	case CmdCreate:
		if p, ok := params.(api.CreateDataPlaneParams); ok {
			fields := map[string]string{
				"namespace": p.Namespace,
				"name":      p.Name,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceDataPlane, fields)
			}
		}
	case CmdList:
		return validateDataPlaneListParams(params)
	}
	return nil
}

// validateNamespaceParams validates parameters for namespace operations
func validateNamespaceParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdCreate {
		if p, ok := params.(api.CreateNamespaceParams); ok {
			fields := map[string]string{
				"name": p.Name,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceNamespace, fields)
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
				"namespace": p.Namespace,
				"project":   p.Project,
				"component": p.Component,
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

// validateDeleteParams validates parameters for delete operations
func validateDeleteParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdDelete {
		if p, ok := params.(api.DeleteParams); ok {
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
				"namespace": p.Namespace,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceDeploymentPipeline, fields)
			}
		}
	case CmdCreate:
		if p, ok := params.(api.CreateDeploymentPipelineParams); ok {
			fields := map[string]string{
				"namespace":         p.Namespace,
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
				"namespace": p.Namespace,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceConfigurationGroup, fields)
			}
		}
	}
	return nil
}

// validateWorkloadParams validates parameters for workload operations
func validateWorkloadParams(cmdType CommandType, params interface{}) error {
	switch cmdType { //nolint:gocritic // switch is needed for future extensibility
	case CmdCreate:
		if p, ok := params.(api.CreateWorkloadParams); ok {
			fields := map[string]string{
				"namespace": p.NamespaceName,
				"project":   p.ProjectName,
				"component": p.ComponentName,
				"image":     p.ImageURL,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(cmdType, ResourceWorkload, fields)
			}
		}
	}
	return nil
}

// namespaceParams is an interface for parameter validation requiring a namespace
type namespaceParams interface {
	GetNamespace() string
}

// validateProjectListParams validates parameters for project list operations
func validateProjectListParams(params interface{}) error {
	if p, ok := params.(namespaceParams); ok {
		return validateNamespace(CmdList, ResourceProject, p.GetNamespace())
	}
	return nil
}

// validateComponentListParams validates parameters for component list operations
func validateComponentListParams(params interface{}) error {
	if p, ok := params.(namespaceParams); ok {
		fields := map[string]string{
			"namespace": p.GetNamespace(),
		}
		if !checkRequiredFields(fields) {
			return generateHelpError(CmdList, ResourceComponent, fields)
		}
	}
	return nil
}

// NamespaceProvider is an interface for params that have a Namespace field
type NamespaceProvider interface {
	GetNamespace() string
}

// validateNamespace validates params that only require namespace
func validateNamespace(cmdType CommandType, resource ResourceType, namespace string) error {
	if namespace == "" {
		fields := map[string]string{
			"namespace": namespace,
		}
		return generateHelpError(cmdType, resource, fields)
	}
	return nil
}

// validateEnvironmentListParams validates parameters for environment list operations
func validateEnvironmentListParams(params interface{}) error {
	if p, ok := params.(namespaceParams); ok {
		return validateNamespace(CmdList, ResourceEnvironment, p.GetNamespace())
	}
	return nil
}

// validateDataPlaneListParams validates parameters for data plane list operations
func validateDataPlaneListParams(params interface{}) error {
	if p, ok := params.(namespaceParams); ok {
		return validateNamespace(CmdList, ResourceDataPlane, p.GetNamespace())
	}
	return nil
}

// validateBuildPlaneParams validates parameters for build plane operations
func validateBuildPlaneParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdList {
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdList, ResourceBuildPlane, p.GetNamespace())
		}
	}
	return nil
}

// validateObservabilityPlaneParams validates parameters for observability plane operations
func validateObservabilityPlaneParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdList {
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdList, ResourceObservabilityPlane, p.GetNamespace())
		}
	}
	return nil
}

// validateComponentTypeParams validates parameters for component type operations
func validateComponentTypeParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdList {
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdList, ResourceComponentType, p.GetNamespace())
		}
	}
	return nil
}

// validateTraitParams validates parameters for trait operations
func validateTraitParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdList {
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdList, ResourceTrait, p.GetNamespace())
		}
	}
	return nil
}

// validateWorkflowParams validates parameters for workflow operations
func validateWorkflowParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdList {
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdList, ResourceWorkflow, p.GetNamespace())
		}
	}
	return nil
}

// validateSecretReferenceParams validates parameters for secret reference operations
func validateSecretReferenceParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdList {
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdList, ResourceSecretReference, p.GetNamespace())
		}
	}
	return nil
}

// validateComponentReleaseParams validates parameters for component release operations
func validateComponentReleaseParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdList {
		if p, ok := params.(api.ListComponentReleasesParams); ok {
			fields := map[string]string{
				"namespace": p.Namespace,
				"project":   p.Project,
				"component": p.Component,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(CmdList, ResourceComponentRelease, fields)
			}
		}
	}
	return nil
}

// validateReleaseBindingParams validates parameters for release binding operations
func validateReleaseBindingParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdList {
		if p, ok := params.(api.ListReleaseBindingsParams); ok {
			fields := map[string]string{
				"namespace": p.Namespace,
				"project":   p.Project,
				"component": p.Component,
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(CmdList, ResourceReleaseBinding, fields)
			}
		}
	}
	return nil
}

// validateWorkflowRunParams validates parameters for workflow run operations
func validateWorkflowRunParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdList {
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdList, ResourceWorkflowRun, p.GetNamespace())
		}
	}
	return nil
}
