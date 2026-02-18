// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"errors"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// Common service errors
var (
	ErrProjectAlreadyExists          = errors.New("project already exists")
	ErrProjectNotFound               = errors.New("project not found")
	ErrComponentAlreadyExists        = errors.New("component already exists")
	ErrComponentNotFound             = errors.New("component not found")
	ErrComponentTypeAlreadyExists    = errors.New("component type already exists")
	ErrComponentTypeNotFound         = errors.New("component type not found")
	ErrTraitAlreadyExists            = errors.New("trait already exists")
	ErrTraitNotFound                 = errors.New("trait not found")
	ErrNamespaceNotFound             = errors.New("namespace not found")
	ErrNamespaceAlreadyExists        = errors.New("namespace already exists")
	ErrEnvironmentNotFound           = errors.New("environment not found")
	ErrEnvironmentAlreadyExists      = errors.New("environment already exists")
	ErrDataPlaneNotFound             = errors.New("dataplane not found")
	ErrDataPlaneAlreadyExists        = errors.New("dataplane already exists")
	ErrBindingNotFound               = errors.New("binding not found")
	ErrDeploymentPipelineNotFound    = errors.New("deployment pipeline not found")
	ErrInvalidPromotionPath          = errors.New("invalid promotion path")
	ErrWorkflowNotFound              = errors.New("workflow not found")
	ErrWorkflowRunNotFound           = errors.New("workflow run not found")
	ErrWorkflowRunAlreadyExists      = errors.New("workflow run already exists")
	ErrWorkflowRunReferenceNotFound  = errors.New("workflow run reference not found")
	ErrComponentWorkflowNotFound     = errors.New("component workflow not found")
	ErrComponentWorkflowRunNotFound  = errors.New("component workflow run not found")
	ErrWorkloadNotFound              = errors.New("workload not found")
	ErrComponentReleaseNotFound      = errors.New("component release not found")
	ErrReleaseBindingNotFound        = errors.New("release binding not found")
	ErrWorkflowSchemaInvalid         = errors.New("workflow schema is invalid")
	ErrReleaseNotFound               = errors.New("release not found")
	ErrInvalidCommitSHA              = errors.New("invalid commit SHA format")
	ErrForbidden                     = errors.New("insufficient permissions to perform this action")
	ErrDuplicateTraitInstanceName    = errors.New("duplicate trait instance name")
	ErrInvalidTraitInstance          = errors.New("invalid trait instance")
	ErrBuildPlaneNotFound            = errors.New("build plane not found")
	ErrClusterDataPlaneNotFound      = errors.New("cluster dataplane not found")
	ErrClusterDataPlaneAlreadyExists = errors.New("cluster dataplane already exists")
	ErrGitSecretAlreadyExists        = errors.New("git secret already exists")
	ErrGitSecretNotFound             = errors.New("git secret not found")
	ErrSecretStoreNotConfigured      = errors.New("secret store not configured")
	ErrInvalidSecretType             = errors.New("secret type must be 'basic-auth' or 'ssh-auth'")
	ErrInvalidCredentials            = errors.New("for basic-auth type, provide 'token'; for ssh-auth type, provide 'sshKey'")
)

// Authorization errors (re-exported from authz core for service layer consistency)
var (
	ErrRoleAlreadyExists        = authzcore.ErrRoleAlreadyExists
	ErrRoleNotFound             = authzcore.ErrRoleNotFound
	ErrRoleInUse                = authzcore.ErrRoleInUse
	ErrRoleBindingAlreadyExists = authzcore.ErrRoleMappingAlreadyExists
	ErrRoleBindingNotFound      = authzcore.ErrRoleMappingNotFound
)

// Error codes for API responses
const (
	CodeProjectExists                = "PROJECT_EXISTS"
	CodeProjectNotFound              = "PROJECT_NOT_FOUND"
	CodeComponentExists              = "COMPONENT_EXISTS"
	CodeComponentNotFound            = "COMPONENT_NOT_FOUND"
	CodeComponentTypeExists          = "COMPONENT_TYPE_EXISTS"
	CodeComponentTypeNotFound        = "COMPONENT_TYPE_NOT_FOUND"
	CodeTraitExists                  = "TRAIT_EXISTS"
	CodeTraitNotFound                = "TRAIT_NOT_FOUND"
	CodeNamespaceNotFound            = "NAMESPACE_NOT_FOUND"
	CodeNamespaceExists              = "NAMESPACE_EXISTS"
	CodeEnvironmentNotFound          = "ENVIRONMENT_NOT_FOUND"
	CodeEnvironmentExists            = "ENVIRONMENT_EXISTS"
	CodeDataPlaneNotFound            = "DATAPLANE_NOT_FOUND"
	CodeDataPlaneExists              = "DATAPLANE_EXISTS"
	CodeBindingNotFound              = "BINDING_NOT_FOUND"
	CodeDeploymentPipelineNotFound   = "DEPLOYMENT_PIPELINE_NOT_FOUND"
	CodeInvalidPromotionPath         = "INVALID_PROMOTION_PATH"
	CodeWorkflowNotFound             = "WORKFLOW_NOT_FOUND"
	CodeWorkflowRunNotFound          = "WORKFLOW_RUN_NOT_FOUND"
	CodeWorkflowRunAlreadyExists     = "WORKFLOW_RUN_ALREADY_EXISTS"
	CodeWorkflowRunReferenceNotFound = "WORKFLOW_RUN_REFERENCE_NOT_FOUND"
	CodeComponentWorkflowNotFound    = "COMPONENT_WORKFLOW_NOT_FOUND"
	CodeComponentWorkflowRunNotFound = "COMPONENT_WORKFLOW_RUN_NOT_FOUND"
	CodeWorkloadNotFound             = "WORKLOAD_NOT_FOUND"
	CodeComponentReleaseNotFound     = "COMPONENT_RELEASE_NOT_FOUND"
	CodeReleaseBindingNotFound       = "RELEASE_BINDING_NOT_FOUND"
	CodeReleaseNotFound              = "RELEASE_NOT_FOUND"
	CodeInvalidInput                 = "INVALID_INPUT"
	CodeConflict                     = "CONFLICT"
	CodeInternalError                = "INTERNAL_ERROR"
	CodeForbidden                    = "FORBIDDEN"
	CodeNotFound                     = "NOT_FOUND"
	CodeWorkflowSchemaInvalid        = "WORKFLOW_SCHEMA_INVALID"
	CodeInvalidCommitSHA             = "INVALID_COMMIT_SHA"
	CodeInvalidParams                = "INVALID_PARAMS"
	CodeDuplicateTraitInstanceName   = "DUPLICATE_TRAIT_INSTANCE_NAME"
	CodeInvalidTraitInstance         = "INVALID_TRAIT_INSTANCE"
	CodeBuildPlaneNotFound           = "BUILDPLANE_NOT_FOUND"
	CodeClusterDataPlaneNotFound     = "CLUSTER_DATAPLANE_NOT_FOUND"
	CodeClusterDataPlaneExists       = "CLUSTER_DATAPLANE_EXISTS"
	CodeGitSecretExists              = "GIT_SECRET_EXISTS"
	CodeGitSecretNotFound            = "GIT_SECRET_NOT_FOUND"
	CodeSecretStoreNotConfigured     = "SECRET_STORE_NOT_CONFIGURED"
	CodeInvalidSecretType            = "INVALID_SECRET_TYPE"
	CodeInvalidCredentials           = "INVALID_CREDENTIALS" //nolint:gosec // False positive: this is an error code, not credentials
	CodeRoleExists                   = "ROLE_EXISTS"
	CodeRoleNotFound                 = "ROLE_NOT_FOUND"
	CodeRoleInUse                    = "ROLE_IN_USE"
	CodeRoleBindingExists            = "ROLE_BINDING_EXISTS"
	CodeRoleBindingNotFound          = "ROLE_BINDING_NOT_FOUND"
)
