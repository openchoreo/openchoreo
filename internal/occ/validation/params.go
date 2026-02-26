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
	case ResourceObservabilityAlertsNotificationChannel:
		return validateObservabilityAlertsNotificationChannelParams(cmdType, params)
	case ResourceAuthzClusterRole:
		return validateAuthzClusterRoleParams(cmdType, params)
	case ResourceAuthzClusterRoleBinding:
		return validateAuthzClusterRoleBindingParams(cmdType, params)
	case ResourceAuthzRole:
		return validateAuthzRoleParams(cmdType, params)
	case ResourceAuthzRoleBinding:
		return validateAuthzRoleBindingParams(cmdType, params)
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
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdGet, ResourceEnvironment, p.GetNamespace())
		}
	case CmdDelete:
		return validateDeleteEnvironmentParams(params)
	case CmdList:
		return validateEnvironmentListParams(params)
	}
	return nil
}

// deleteEnvironmentParams is an interface for delete environment parameter validation
type deleteEnvironmentParams interface {
	GetNamespace() string
	GetEnvironmentName() string
}

// validateDeleteEnvironmentParams validates parameters for delete environment operations
func validateDeleteEnvironmentParams(params interface{}) error {
	if p, ok := params.(deleteEnvironmentParams); ok {
		fields := map[string]string{
			"namespace": p.GetNamespace(),
			"name":      p.GetEnvironmentName(),
		}
		if !checkRequiredFields(fields) {
			return generateHelpError(CmdDelete, ResourceEnvironment, fields)
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
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdGet, ResourceDataPlane, p.GetNamespace())
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
	case CmdDelete:
		return validateDeleteDataPlaneParams(params)
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

// validateDeploymentPipelineParams validates parameters for deployment pipeline operations
func validateDeploymentPipelineParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdGet:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdGet, ResourceDeploymentPipeline, p.GetNamespace())
		}
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
	case CmdDelete:
		return validateDeleteDeploymentPipelineParams(params)
	case CmdList:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdList, ResourceDeploymentPipeline, p.GetNamespace())
		}
	}
	return nil
}

// deleteDeploymentPipelineParams is an interface for delete deployment pipeline parameter validation
type deleteDeploymentPipelineParams interface {
	GetNamespace() string
	GetDeploymentPipelineName() string
}

// validateDeleteDeploymentPipelineParams validates parameters for delete deployment pipeline operations
func validateDeleteDeploymentPipelineParams(params interface{}) error {
	if p, ok := params.(deleteDeploymentPipelineParams); ok {
		fields := map[string]string{
			"namespace": p.GetNamespace(),
			"name":      p.GetDeploymentPipelineName(),
		}
		if !checkRequiredFields(fields) {
			return generateHelpError(CmdDelete, ResourceDeploymentPipeline, fields)
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
	switch cmdType {
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
	case CmdList:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdList, ResourceWorkload, p.GetNamespace())
		}
	case CmdGet:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdGet, ResourceWorkload, p.GetNamespace())
		}
	case CmdDelete:
		return validateDeleteWorkloadParams(params)
	}
	return nil
}

// deleteWorkloadParams is an interface for delete workload parameter validation
type deleteWorkloadParams interface {
	GetNamespace() string
	GetWorkloadName() string
}

// validateDeleteWorkloadParams validates parameters for delete workload operations
func validateDeleteWorkloadParams(params interface{}) error {
	if p, ok := params.(deleteWorkloadParams); ok {
		fields := map[string]string{
			"namespace": p.GetNamespace(),
			"name":      p.GetWorkloadName(),
		}
		if !checkRequiredFields(fields) {
			return generateHelpError(CmdDelete, ResourceWorkload, fields)
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

// deleteDataPlaneParams is an interface for delete data plane parameter validation
type deleteDataPlaneParams interface {
	GetNamespace() string
	GetDataPlaneName() string
}

// validateDeleteDataPlaneParams validates parameters for delete data plane operations
func validateDeleteDataPlaneParams(params interface{}) error {
	if p, ok := params.(deleteDataPlaneParams); ok {
		fields := map[string]string{
			"namespace": p.GetNamespace(),
			"name":      p.GetDataPlaneName(),
		}
		if !checkRequiredFields(fields) {
			return generateHelpError(CmdDelete, ResourceDataPlane, fields)
		}
	}
	return nil
}

// validateBuildPlaneParams validates parameters for build plane operations
func validateBuildPlaneParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdGet:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdGet, ResourceBuildPlane, p.GetNamespace())
		}
	case CmdDelete:
		return validateDeleteBuildPlaneParams(params)
	case CmdList:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdList, ResourceBuildPlane, p.GetNamespace())
		}
	}
	return nil
}

// deleteBuildPlaneParams is an interface for delete build plane parameter validation
type deleteBuildPlaneParams interface {
	GetNamespace() string
	GetBuildPlaneName() string
}

// validateDeleteBuildPlaneParams validates parameters for delete build plane operations
func validateDeleteBuildPlaneParams(params interface{}) error {
	if p, ok := params.(deleteBuildPlaneParams); ok {
		fields := map[string]string{
			"namespace": p.GetNamespace(),
			"name":      p.GetBuildPlaneName(),
		}
		if !checkRequiredFields(fields) {
			return generateHelpError(CmdDelete, ResourceBuildPlane, fields)
		}
	}
	return nil
}

// validateObservabilityPlaneParams validates parameters for observability plane operations
func validateObservabilityPlaneParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdGet:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdGet, ResourceObservabilityPlane, p.GetNamespace())
		}
	case CmdDelete:
		return validateDeleteObservabilityPlaneParams(params)
	case CmdList:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdList, ResourceObservabilityPlane, p.GetNamespace())
		}
	}
	return nil
}

// deleteObservabilityPlaneParams is an interface for delete observability plane parameter validation
type deleteObservabilityPlaneParams interface {
	GetNamespace() string
	GetObservabilityPlaneName() string
}

// validateDeleteObservabilityPlaneParams validates parameters for delete observability plane operations
func validateDeleteObservabilityPlaneParams(params interface{}) error {
	if p, ok := params.(deleteObservabilityPlaneParams); ok {
		fields := map[string]string{
			"namespace": p.GetNamespace(),
			"name":      p.GetObservabilityPlaneName(),
		}
		if !checkRequiredFields(fields) {
			return generateHelpError(CmdDelete, ResourceObservabilityPlane, fields)
		}
	}
	return nil
}

// validateComponentTypeParams validates parameters for component type operations
func validateComponentTypeParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdList:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdList, ResourceComponentType, p.GetNamespace())
		}
	case CmdGet:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdGet, ResourceComponentType, p.GetNamespace())
		}
	case CmdDelete:
		return validateDeleteComponentTypeParams(params)
	}
	return nil
}

// deleteComponentTypeParams is an interface for delete component type parameter validation
type deleteComponentTypeParams interface {
	GetNamespace() string
	GetComponentTypeName() string
}

// validateDeleteComponentTypeParams validates parameters for delete component type operations
func validateDeleteComponentTypeParams(params interface{}) error {
	if p, ok := params.(deleteComponentTypeParams); ok {
		fields := map[string]string{
			"namespace": p.GetNamespace(),
			"name":      p.GetComponentTypeName(),
		}
		if !checkRequiredFields(fields) {
			return generateHelpError(CmdDelete, ResourceComponentType, fields)
		}
	}
	return nil
}

// validateTraitParams validates parameters for trait operations
func validateTraitParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdList:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdList, ResourceTrait, p.GetNamespace())
		}
	case CmdGet:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdGet, ResourceTrait, p.GetNamespace())
		}
	case CmdDelete:
		return validateDeleteTraitParams(params)
	}
	return nil
}

// deleteTraitParams is an interface for delete trait parameter validation
type deleteTraitParams interface {
	GetNamespace() string
	GetTraitName() string
}

// validateDeleteTraitParams validates parameters for delete trait operations
func validateDeleteTraitParams(params interface{}) error {
	if p, ok := params.(deleteTraitParams); ok {
		fields := map[string]string{
			"namespace": p.GetNamespace(),
			"name":      p.GetTraitName(),
		}
		if !checkRequiredFields(fields) {
			return generateHelpError(CmdDelete, ResourceTrait, fields)
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
	switch cmdType {
	case CmdList:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdList, ResourceSecretReference, p.GetNamespace())
		}
	case CmdGet:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdGet, ResourceSecretReference, p.GetNamespace())
		}
	case CmdDelete:
		return validateDeleteSecretReferenceParams(params)
	}
	return nil
}

// deleteSecretReferenceParams is an interface for delete secret reference parameter validation
type deleteSecretReferenceParams interface {
	GetNamespace() string
	GetSecretReferenceName() string
}

// validateDeleteSecretReferenceParams validates parameters for delete secret reference operations
func validateDeleteSecretReferenceParams(params interface{}) error {
	if p, ok := params.(deleteSecretReferenceParams); ok {
		fields := map[string]string{
			"namespace": p.GetNamespace(),
			"name":      p.GetSecretReferenceName(),
		}
		if !checkRequiredFields(fields) {
			return generateHelpError(CmdDelete, ResourceSecretReference, fields)
		}
	}
	return nil
}

// componentReleaseListParams is an interface for component release list parameter validation
type componentReleaseListParams interface {
	GetNamespace() string
	GetProject() string
	GetComponent() string
}

// validateComponentReleaseParams validates parameters for component release operations
func validateComponentReleaseParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdList:
		if p, ok := params.(componentReleaseListParams); ok {
			fields := map[string]string{
				"namespace": p.GetNamespace(),
				"project":   p.GetProject(),
				"component": p.GetComponent(),
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(CmdList, ResourceComponentRelease, fields)
			}
		}
	case CmdGet:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdGet, ResourceComponentRelease, p.GetNamespace())
		}
	}
	return nil
}

// releaseBindingListParams is an interface for release binding list parameter validation
type releaseBindingListParams interface {
	GetNamespace() string
	GetProject() string
	GetComponent() string
}

// validateReleaseBindingParams validates parameters for release binding operations
func validateReleaseBindingParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdList:
		if p, ok := params.(releaseBindingListParams); ok {
			fields := map[string]string{
				"namespace": p.GetNamespace(),
				"project":   p.GetProject(),
				"component": p.GetComponent(),
			}
			if !checkRequiredFields(fields) {
				return generateHelpError(CmdList, ResourceReleaseBinding, fields)
			}
		}
	case CmdGet:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdGet, ResourceReleaseBinding, p.GetNamespace())
		}
	case CmdDelete:
		return validateDeleteReleaseBindingParams(params)
	}
	return nil
}

// deleteReleaseBindingParams is an interface for delete release binding parameter validation
type deleteReleaseBindingParams interface {
	GetNamespace() string
	GetReleaseBindingName() string
}

// validateDeleteReleaseBindingParams validates parameters for delete release binding operations
func validateDeleteReleaseBindingParams(params interface{}) error {
	if p, ok := params.(deleteReleaseBindingParams); ok {
		fields := map[string]string{
			"namespace": p.GetNamespace(),
			"name":      p.GetReleaseBindingName(),
		}
		if !checkRequiredFields(fields) {
			return generateHelpError(CmdDelete, ResourceReleaseBinding, fields)
		}
	}
	return nil
}

// validateWorkflowRunParams validates parameters for workflow run operations
func validateWorkflowRunParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdList:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdList, ResourceWorkflowRun, p.GetNamespace())
		}
	case CmdGet:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdGet, ResourceWorkflowRun, p.GetNamespace())
		}
	}
	return nil
}

// validateObservabilityAlertsNotificationChannelParams validates parameters for observability alerts notification channel operations
func validateObservabilityAlertsNotificationChannelParams(cmdType CommandType, params interface{}) error {
	switch cmdType {
	case CmdList:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdList, ResourceObservabilityAlertsNotificationChannel, p.GetNamespace())
		}
	case CmdGet:
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(CmdGet, ResourceObservabilityAlertsNotificationChannel, p.GetNamespace())
		}
	case CmdDelete:
		return validateDeleteObservabilityAlertsNotificationChannelParams(params)
	}
	return nil
}

// deleteObservabilityAlertsNotificationChannelParams is an interface for delete observability alerts notification channel parameter validation
type deleteObservabilityAlertsNotificationChannelParams interface {
	GetNamespace() string
	GetChannelName() string
}

// validateDeleteObservabilityAlertsNotificationChannelParams validates parameters for delete observability alerts notification channel operations
func validateDeleteObservabilityAlertsNotificationChannelParams(params interface{}) error {
	if p, ok := params.(deleteObservabilityAlertsNotificationChannelParams); ok {
		fields := map[string]string{
			"namespace": p.GetNamespace(),
			"name":      p.GetChannelName(),
		}
		if !checkRequiredFields(fields) {
			return generateHelpError(CmdDelete, ResourceObservabilityAlertsNotificationChannel, fields)
		}
	}
	return nil
}

// validateAuthzClusterRoleParams validates parameters for authz cluster role operations
func validateAuthzClusterRoleParams(_ CommandType, _ interface{}) error {
	return nil
}

// validateAuthzClusterRoleBindingParams validates parameters for authz cluster role binding operations
func validateAuthzClusterRoleBindingParams(_ CommandType, _ interface{}) error {
	return nil
}

// validateAuthzRoleParams validates parameters for authz role operations
func validateAuthzRoleParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdList || cmdType == CmdGet || cmdType == CmdDelete {
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(cmdType, ResourceAuthzRole, p.GetNamespace())
		}
	}
	return nil
}

// validateAuthzRoleBindingParams validates parameters for authz role binding operations
func validateAuthzRoleBindingParams(cmdType CommandType, params interface{}) error {
	if cmdType == CmdList || cmdType == CmdGet || cmdType == CmdDelete {
		if p, ok := params.(namespaceParams); ok {
			return validateNamespace(cmdType, ResourceAuthzRoleBinding, p.GetNamespace())
		}
	}
	return nil
}
