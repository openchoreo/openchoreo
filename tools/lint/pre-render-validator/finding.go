// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemavalidator

// Severity indicates how severe a schema validation finding is.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
	SeverityHint    Severity = "hint"
)

// Finding represents a parameter schema validation issue.
type Finding struct {
	Path     string
	Severity Severity
	Code     string
	Message  string
	Line     int
	Column   int
}
