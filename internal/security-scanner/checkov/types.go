// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package checkov

type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityHigh     Severity = "HIGH"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
	SeverityInfo     Severity = "INFO"
	SeverityUnknown  Severity = "UNKNOWN"
)

type Finding struct {
	CheckID     string
	CheckName   string
	Severity    Severity
	Category    string
	Description string
	Remediation string
}

type checkovOutput struct {
	Results checkovResults `json:"results"`
}

type checkovResults struct {
	FailedChecks []checkovCheck `json:"failed_checks"`
}

type checkovCheck struct {
	CheckID          string             `json:"check_id"`
	BcCheckID        string             `json:"bc_check_id"`
	CheckName        string             `json:"check_name"`
	CheckClass       string             `json:"check_class"`
	CheckResult      checkovCheckResult `json:"check_result"`
	Resource         string             `json:"resource"`
	Guideline        string             `json:"guideline"`
	Description      *string            `json:"description"`
	ShortDescription *string            `json:"short_description"`
	Severity         *string            `json:"severity"`
	BcCategory       *string            `json:"bc_category"`
}

type checkovCheckResult struct {
	Result string `json:"result"`
}
