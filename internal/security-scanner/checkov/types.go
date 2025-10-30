// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Copyright 2024 Choreo LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
