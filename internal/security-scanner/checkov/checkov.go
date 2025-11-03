// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package checkov

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Scanner interface {
	Scan(ctx context.Context, manifest []byte) ([]Finding, error)
}

type checkovScanner struct{}

var defaultScanner Scanner = &checkovScanner{}

func RunCheckov(ctx context.Context, manifest []byte) ([]Finding, error) {
	return defaultScanner.Scan(ctx, manifest)
}

func (s *checkovScanner) Scan(ctx context.Context, manifest []byte) ([]Finding, error) {
	tmpFile, err := os.CreateTemp("", "checkov-manifest-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(manifest); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("failed to write manifest to temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close temp file: %w", err)
	}

	tmpFilePath := tmpFile.Name()
	if !isValidFilePath(tmpFilePath) {
		return nil, fmt.Errorf("invalid temp file path: %s", tmpFilePath)
	}

	cmd := exec.CommandContext(ctx, "checkov", "-f", tmpFilePath, "--framework", "kubernetes", "--output", "json", "--quiet")
	output, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() == 127 {
			return nil, fmt.Errorf("checkov command failed: %w (output: %s)", err, string(output))
		}
	}

	var result checkovOutput
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse checkov output: %w (output: %s)", err, string(output))
	}

	findings := make([]Finding, 0, len(result.Results.FailedChecks))
	for _, check := range result.Results.FailedChecks {
		checkName := check.CheckName
		if check.ShortDescription != nil && *check.ShortDescription != "" {
			checkName = *check.ShortDescription
		}

		description := ""
		if check.Description != nil {
			description = *check.Description
		}

		severityStr := ""
		if check.Severity != nil {
			severityStr = *check.Severity
		}

		category := ""
		if check.BcCategory != nil {
			category = *check.BcCategory
		} else {
			category = categorizeCheck(check.CheckClass)
		}

		findings = append(findings, Finding{
			CheckID:     check.CheckID,
			CheckName:   checkName,
			Severity:    mapSeverity(severityStr),
			Category:    category,
			Description: description,
			Remediation: check.Guideline,
		})
	}

	return findings, nil
}

func mapSeverity(checkovSeverity string) Severity {
	if checkovSeverity == "" {
		return SeverityMedium
	}
	switch strings.ToUpper(checkovSeverity) {
	case "CRITICAL":
		return SeverityCritical
	case "HIGH":
		return SeverityHigh
	case "MEDIUM":
		return SeverityMedium
	case "LOW":
		return SeverityLow
	case "INFO", "INFORMATIONAL":
		return SeverityInfo
	default:
		return SeverityUnknown
	}
}

func categorizeCheck(checkClass string) string {
	if checkClass == "" {
		return "security"
	}
	parts := strings.Split(checkClass, ".")
	if len(parts) > 0 {
		category := parts[len(parts)-1]
		category = strings.ToLower(category)
		category = strings.TrimSuffix(category, "check")
		return category
	}
	return "security"
}

func isValidFilePath(path string) bool {
	// Basic validation to prevent command injection
	// Only allow alphanumeric characters, dots, hyphens, underscores, and forward slashes
	for _, r := range path {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '-' || r == '_' || r == '/') {
			return false
		}
	}
	return true
}
