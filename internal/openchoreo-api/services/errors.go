package services

import "errors"

// Common service errors
var (
	ErrProjectAlreadyExists   = errors.New("project already exists")
	ErrProjectNotFound        = errors.New("project not found")
	ErrComponentAlreadyExists = errors.New("component already exists")
	ErrComponentNotFound      = errors.New("component not found")
)

// Error codes for API responses
const (
	CodeProjectExists     = "PROJECT_EXISTS"
	CodeProjectNotFound   = "PROJECT_NOT_FOUND"
	CodeComponentExists   = "COMPONENT_EXISTS"
	CodeComponentNotFound = "COMPONENT_NOT_FOUND"
	CodeInvalidInput      = "INVALID_INPUT"
	CodeInternalError     = "INTERNAL_ERROR"
)
