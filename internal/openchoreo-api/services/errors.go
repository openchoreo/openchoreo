// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import "errors"

// Common service errors
var (
	ErrProjectAlreadyExists       = errors.New("project already exists")
	ErrProjectNotFound            = errors.New("project not found")
	ErrComponentAlreadyExists     = errors.New("component already exists")
	ErrComponentNotFound          = errors.New("component not found")
	ErrComponentTypeAlreadyExists = errors.New("component type already exists")
	ErrComponentTypeNotFound      = errors.New("component type not found")
	ErrTraitAlreadyExists         = errors.New("trait already exists")
	ErrTraitNotFound              = errors.New("trait not found")
	ErrOrganizationNotFound       = errors.New("organization not found")
	ErrEnvironmentNotFound        = errors.New("environment not found")
	ErrEnvironmentAlreadyExists   = errors.New("environment already exists")
	ErrDataPlaneNotFound          = errors.New("dataplane not found")
	ErrDataPlaneAlreadyExists     = errors.New("dataplane already exists")
	ErrBindingNotFound            = errors.New("binding not found")
	ErrDeploymentPipelineNotFound = errors.New("deployment pipeline not found")
	ErrInvalidPromotionPath       = errors.New("invalid promotion path")
	ErrWorkflowNotFound           = errors.New("workflow not found")
	ErrWorkloadNotFound           = errors.New("workload not found")
	ErrComponentReleaseNotFound   = errors.New("component release not found")
	ErrReleaseBindingNotFound     = errors.New("release binding not found")
	ErrWorkflowSchemaInvalid      = errors.New("workflow schema is invalid")
	ErrReleaseNotFound            = errors.New("release not found")
)

// Error codes for API responses
const (
	CodeProjectExists              = "PROJECT_EXISTS"
	CodeProjectNotFound            = "PROJECT_NOT_FOUND"
	CodeComponentExists            = "COMPONENT_EXISTS"
	CodeComponentNotFound          = "COMPONENT_NOT_FOUND"
	CodeComponentTypeExists        = "COMPONENT_TYPE_EXISTS"
	CodeComponentTypeNotFound      = "COMPONENT_TYPE_NOT_FOUND"
	CodeTraitExists                = "TRAIT_EXISTS"
	CodeTraitNotFound              = "TRAIT_NOT_FOUND"
	CodeOrganizationNotFound       = "ORGANIZATION_NOT_FOUND"
	CodeEnvironmentNotFound        = "ENVIRONMENT_NOT_FOUND"
	CodeEnvironmentExists          = "ENVIRONMENT_EXISTS"
	CodeDataPlaneNotFound          = "DATAPLANE_NOT_FOUND"
	CodeDataPlaneExists            = "DATAPLANE_EXISTS"
	CodeBindingNotFound            = "BINDING_NOT_FOUND"
	CodeDeploymentPipelineNotFound = "DEPLOYMENT_PIPELINE_NOT_FOUND"
	CodeInvalidPromotionPath       = "INVALID_PROMOTION_PATH"
	CodeWorkflowNotFound           = "WORKFLOW_NOT_FOUND"
	CodeWorkloadNotFound           = "WORKLOAD_NOT_FOUND"
	CodeComponentReleaseNotFound   = "COMPONENT_RELEASE_NOT_FOUND"
	CodeReleaseBindingNotFound     = "RELEASE_BINDING_NOT_FOUND"
	CodeReleaseNotFound            = "RELEASE_NOT_FOUND"
	CodeInvalidInput               = "INVALID_INPUT"
	CodeInternalError              = "INTERNAL_ERROR"
	CodeWorkflowSchemaInvalid      = "WORKFLOW_SCHEMA_INVALID"
)
