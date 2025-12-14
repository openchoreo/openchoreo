// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	ErrComponentWorkflowNotFound  = errors.New("component workflow not found")
	ErrWorkloadNotFound           = errors.New("workload not found")
	ErrComponentReleaseNotFound   = errors.New("component release not found")
	ErrReleaseBindingNotFound     = errors.New("release binding not found")
	ErrWorkflowSchemaInvalid      = errors.New("workflow schema is invalid")
	ErrReleaseNotFound            = errors.New("release not found")
	ErrInvalidCommitSHA           = errors.New("invalid commit SHA format")
	ErrForbidden                  = errors.New("insufficient permissions to perform this action")
	ErrDuplicateTraitInstanceName = errors.New("duplicate trait instance name")
	ErrInvalidTraitInstance       = errors.New("invalid trait instance")

	// Continue token errors
	ErrContinueTokenExpired = errors.New("continue token has expired - please restart the list operation from the beginning")
	ErrInvalidContinueToken = errors.New("invalid continue token - please check if the token is malformed or from a different resource")
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
	CodeComponentWorkflowNotFound  = "COMPONENT_WORKFLOW_NOT_FOUND"
	CodeWorkloadNotFound           = "WORKLOAD_NOT_FOUND"
	CodeComponentReleaseNotFound   = "COMPONENT_RELEASE_NOT_FOUND"
	CodeReleaseBindingNotFound     = "RELEASE_BINDING_NOT_FOUND"
	CodeReleaseNotFound            = "RELEASE_NOT_FOUND"
	CodeInvalidInput               = "INVALID_INPUT"
	CodeConflict                   = "CONFLICT"
	CodeInternalError              = "INTERNAL_ERROR"
	CodeForbidden                  = "FORBIDDEN"
	CodeNotFound                   = "NOT_FOUND"
	CodeWorkflowSchemaInvalid      = "WORKFLOW_SCHEMA_INVALID"
	CodeInvalidCommitSHA           = "INVALID_COMMIT_SHA"
	CodeInvalidParams              = "INVALID_PARAMS"
	CodeDuplicateTraitInstanceName = "DUPLICATE_TRAIT_INSTANCE_NAME"
	CodeInvalidTraitInstance       = "INVALID_TRAIT_INSTANCE"

	// Continue token error codes
	CodeContinueTokenExpired = "CONTINUE_TOKEN_EXPIRED" // HTTP 410
	CodeInvalidContinueToken = "INVALID_CONTINUE_TOKEN" // HTTP 400
)

// HandleListError handles common errors from Kubernetes list operations,
// specifically handling pagination-related errors (expired/invalid continue tokens).
// This function centralizes error handling to reduce duplication across service methods.
//
// Parameters:
//   - err: the error returned from the k8sClient.List call
//   - logger: the service logger for logging warnings/errors
//   - continueToken: the continue token that was used in the request (for logging)
//   - resourceType: a human-readable name of the resource being listed (e.g., "projects", "components")
//
// Returns:
//   - A standardized error (ErrContinueTokenExpired, ErrInvalidContinueToken, or wrapped error)
func HandleListError(err error, logger *slog.Logger, continueToken, resourceType string) error {
	// Truncate token for logging to avoid polluting logs with large tokens
	logToken := continueToken
	if len(logToken) > 20 {
		logToken = logToken[:10] + "..." + logToken[len(logToken)-5:]
	}

	// Handle expired continue token (410 Gone)
	if apierrors.IsResourceExpired(err) {
		logger.Warn("Continue token expired", "continue", logToken)
		return ErrContinueTokenExpired
	}
	// Handle invalid continue token. Prefer structured inspection of an APIStatus
	// (Status.Details.Causes) if available. This handles cases where the apiserver
	// explicitly marks the continue field as invalid.
	if statusErr, ok := err.(apierrors.APIStatus); ok {
		status := statusErr.Status()
		if status.Details != nil {
			for _, cause := range status.Details.Causes {
				if strings.EqualFold(cause.Field, "continue") || strings.Contains(strings.ToLower(cause.Message), "continue") {
					logger.Warn("Invalid continue token", "continue", logToken)
					return ErrInvalidContinueToken
				}
			}
		}
		if status.Reason == metav1.StatusReasonInvalid && strings.Contains(strings.ToLower(status.Message), "continue") {
			logger.Warn("Invalid continue token", "continue", logToken)
			return ErrInvalidContinueToken
		}
	}

	// As a conservative fallback, inspect the error message for an explicit
	// mention of the continue token when the error is a BadRequest.
	if apierrors.IsBadRequest(err) {
		if strings.Contains(strings.ToLower(err.Error()), "invalid value for continue") || strings.Contains(strings.ToLower(err.Error()), "invalid continue") || strings.Contains(strings.ToLower(err.Error()), "continue token") {
			logger.Warn("Invalid continue token", "continue", logToken)
			return ErrInvalidContinueToken
		}
	}
	logger.Error("Failed to list "+resourceType, "error", err)
	return fmt.Errorf("failed to list %s: %w", resourceType, err)
}
