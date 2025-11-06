// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"errors"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// Common service errors
var (
	ErrProjectAlreadyExists       = errors.New("project already exists")
	ErrProjectNotFound            = errors.New("project not found")
	ErrComponentAlreadyExists     = errors.New("component already exists")
	ErrComponentNotFound          = errors.New("component not found")
	ErrComponentResourceNotFound  = errors.New("component resource not found")
	ErrOrganizationNotFound       = errors.New("organization not found")
	ErrEnvironmentNotFound        = errors.New("environment not found")
	ErrEnvironmentAlreadyExists   = errors.New("environment already exists")
	ErrDataPlaneNotFound          = errors.New("dataplane not found")
	ErrDataPlaneAlreadyExists     = errors.New("dataplane already exists")
	ErrBindingNotFound            = errors.New("binding not found")
	ErrDeploymentPipelineNotFound = errors.New("deployment pipeline not found")
	ErrInvalidPromotionPath       = errors.New("invalid promotion path")
	ErrContinueTokenExpired       = errors.New("continue token has expired")
	ErrInvalidCursorFormat        = errors.New("invalid cursor format")
	ErrInvalidPaginationMode      = errors.New("invalid pagination mode")
)

// Error codes for API responses
const (
	CodeProjectExists              = "PROJECT_EXISTS"
	CodeProjectNotFound            = "PROJECT_NOT_FOUND"
	CodeComponentExists            = "COMPONENT_EXISTS"
	CodeComponentNotFound          = "COMPONENT_NOT_FOUND"
	CodeOrganizationNotFound       = "ORGANIZATION_NOT_FOUND"
	CodeEnvironmentNotFound        = "ENVIRONMENT_NOT_FOUND"
	CodeEnvironmentExists          = "ENVIRONMENT_EXISTS"
	CodeDataPlaneNotFound          = "DATAPLANE_NOT_FOUND"
	CodeDataPlaneExists            = "DATAPLANE_EXISTS"
	CodeBindingNotFound            = "BINDING_NOT_FOUND"
	CodeDeploymentPipelineNotFound = "DEPLOYMENT_PIPELINE_NOT_FOUND"
	CodeInvalidPromotionPath       = "INVALID_PROMOTION_PATH"
	CodeInvalidInput               = "INVALID_INPUT"
	CodeInternalError              = "INTERNAL_ERROR"
	CodeContinueTokenExpired       = "CONTINUE_TOKEN_EXPIRED"
	CodeInvalidCursorFormat        = "INVALID_CURSOR_FORMAT"
	CodeMissingLimit               = "MISSING_LIMIT"
	CodeInvalidPaginationMode      = "INVALID_PAGINATION_MODE"
)

// isExpiredTokenError checks if an error indicates an expired continue token
func isExpiredTokenError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's a K8s Gone error (410 status) which indicates an expired token
	if apierrors.IsGone(err) {
		return true
	}

	// Check for specific Kubernetes continue token error messages
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "continue token") && strings.Contains(errMsg, "expired")
}

// isInvalidCursorError checks if an error indicates an invalid cursor format
func isInvalidCursorError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's a K8s BadRequest error (400 status)
	if apierrors.IsBadRequest(err) {
		return true
	}

	// Check for specific cursor/token validation error messages
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "invalid") && (strings.Contains(errMsg, "cursor") || strings.Contains(errMsg, "token"))
}

// isServiceUnavailableError checks if an error indicates service unavailability
func isServiceUnavailableError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's a K8s ServiceUnavailable error (503 status)
	return apierrors.IsServiceUnavailable(err)
}

// GetPaginationErrorCode returns the appropriate error code for pagination errors
func GetPaginationErrorCode(err error) string {
	if err == nil {
		return CodeInvalidInput
	}

	// Check by error message content for pagination errors
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "invalid pagination mode") {
		return CodeInvalidPaginationMode
	}

	// Default to generic invalid input
	return CodeInvalidInput
}
