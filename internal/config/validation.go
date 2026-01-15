// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"slices"
	"strings"

	"golang.org/x/exp/constraints"
)

// Path represents a path to a config field for error reporting.
// It builds paths like "server.middlewares.jwt.jwks.url" for clear error messages.
type Path struct {
	segments []string
}

// NewPath creates a new path with a root segment.
func NewPath(root string) *Path {
	return &Path{segments: []string{root}}
}

// Child returns a new path with the child segment appended.
func (p *Path) Child(name string) *Path {
	newSegments := make([]string, len(p.segments)+1)
	copy(newSegments, p.segments)
	newSegments[len(p.segments)] = name
	return &Path{segments: newSegments}
}

// Index returns a new path with an array index appended to the last segment.
// Example: path.Child("user_types").Index(0) produces "parent.user_types[0]"
func (p *Path) Index(i int) *Path {
	if len(p.segments) == 0 {
		return &Path{segments: []string{fmt.Sprintf("[%d]", i)}}
	}
	newSegments := make([]string, len(p.segments))
	copy(newSegments, p.segments)
	newSegments[len(newSegments)-1] = fmt.Sprintf("%s[%d]", newSegments[len(newSegments)-1], i)
	return &Path{segments: newSegments}
}

// String returns the dot-separated path string.
func (p *Path) String() string {
	return strings.Join(p.segments, ".")
}

// FieldError represents a validation error for a specific config field.
type FieldError struct {
	Field   string
	Message string
}

// Error implements the error interface.
func (e *FieldError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors collects multiple validation errors.
type ValidationErrors []*FieldError

// Error implements the error interface, formatting all errors.
func (ve ValidationErrors) Error() string {
	var b strings.Builder
	for i, e := range ve {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("- ")
		b.WriteString(e.Error())
	}
	return b.String()
}

// OrNil returns nil if there are no errors, otherwise returns the ValidationErrors.
func (ve ValidationErrors) OrNil() error {
	if len(ve) == 0 {
		return nil
	}
	return ve
}

// Required returns an error indicating a field is required.
func Required(path *Path) *FieldError {
	return &FieldError{Field: path.String(), Message: "is required"}
}

// Invalid returns a generic validation error with a custom message.
func Invalid(path *Path, msg string) *FieldError {
	return &FieldError{Field: path.String(), Message: msg}
}

// MustBeInRange returns an error if value is not within [min, max].
func MustBeInRange[T constraints.Ordered](path *Path, value, min, max T) *FieldError {
	if value < min || value > max {
		return Invalid(path, fmt.Sprintf("must be between %v and %v", min, max))
	}
	return nil
}

// MustBeNonNegative returns an error if value is negative.
func MustBeNonNegative[T constraints.Ordered](path *Path, value T) *FieldError {
	var zero T
	if value < zero {
		return Invalid(path, "must be non-negative")
	}
	return nil
}

// MustBeGreaterThan returns an error if value is not greater than min.
func MustBeGreaterThan[T constraints.Ordered](path *Path, value, min T) *FieldError {
	if value <= min {
		return Invalid(path, fmt.Sprintf("must be greater than %v", min))
	}
	return nil
}

// MustBeLessThanOrEqual returns an error if value is greater than max.
func MustBeLessThanOrEqual[T constraints.Ordered](path *Path, value, max T) *FieldError {
	if value > max {
		return Invalid(path, fmt.Sprintf("must be <= %v", max))
	}
	return nil
}

// MustBeOneOf returns an error if value is not in the allowed list.
func MustBeOneOf(path *Path, value string, allowed []string) *FieldError {
	if slices.Contains(allowed, value) {
		return nil
	}
	return Invalid(path, fmt.Sprintf("must be one of: %s", strings.Join(allowed, ", ")))
}

// MustNotBeEmpty returns an error if the string value is empty.
func MustNotBeEmpty(path *Path, value string) *FieldError {
	if value == "" {
		return Invalid(path, "must not be empty")
	}
	return nil
}
