// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"regexp"
	"time"
)

var granularityPattern = regexp.MustCompile(`^[1-9][0-9]*[hdw]$`)

// strPtr returns a pointer to the string, or nil if the string is empty.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// parseRFC3339Time parses a time string in RFC3339 format.
func parseRFC3339Time(timeStr string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return time.Time{}, invalidArgument(fmt.Errorf("invalid time format (expected RFC3339): %w", err))
	}
	return t, nil
}

// setDefaults applies default values for common query parameters.
func setDefaults(limit int, sortOrder string, logLevels []string) (int, string, []string) {
	if limit == 0 {
		limit = 100
	}
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if logLevels == nil {
		logLevels = []string{}
	}
	return limit, sortOrder, logLevels
}

// validateComponentScope validates that the required scope fields are present.
func validateComponentScope(namespace, project, component string) error {
	if namespace == "" {
		return invalidArgument(fmt.Errorf("namespace is required"))
	}
	if component != "" && project == "" {
		return invalidArgument(fmt.Errorf("project is required when component is provided"))
	}
	return nil
}

// validateComponentLineScope validates a component scope that may name a release line, which
// requires the component and the environment.
func validateComponentLineScope(namespace, project, component, environment, releaseLine string) error {
	if err := validateComponentScope(namespace, project, component); err != nil {
		return err
	}
	if releaseLine != "" && (component == "" || environment == "") {
		return invalidArgument(fmt.Errorf("component and environment are required when release_line is provided"))
	}
	return nil
}

// validateComponentLineAnyEnvScope validates a component scope that may name a release line on a
// query that spans environments, which requires only the component.
func validateComponentLineAnyEnvScope(namespace, project, component, releaseLine string) error {
	if err := validateComponentScope(namespace, project, component); err != nil {
		return err
	}
	if releaseLine != "" && component == "" {
		return invalidArgument(fmt.Errorf("component is required when release_line is provided"))
	}
	return nil
}

// releaseLineProperty is the release_line argument of component-scoped query tools.
func releaseLineProperty() map[string]any {
	return stringProperty("Release line of the component, for components with release lines enabled. " +
		"Restricts the result to that line. Requires component and environment")
}

// releaseLineAnyEnvProperty is the release_line argument of query tools that span environments.
func releaseLineAnyEnvProperty() map[string]any {
	return stringProperty("Release line of the component, for components with release lines enabled. " +
		"Restricts the result to that line. Requires component")
}

// validateFinOpsScope validates the scope fields for FinOps queries, which
// additionally require an environment.
func validateFinOpsScope(namespace, environment, project, component string) error {
	if namespace == "" {
		return invalidArgument(fmt.Errorf("namespace is required"))
	}
	if environment == "" {
		return invalidArgument(fmt.Errorf("environment is required"))
	}
	if component != "" && project == "" {
		return invalidArgument(fmt.Errorf("project is required when component is provided"))
	}
	return nil
}

func validateGranularity(granularity string) error {
	if granularity != "" && !granularityPattern.MatchString(granularity) {
		return invalidArgument(fmt.Errorf("granularity must match <count><unit> notation (e.g. 1h, 2d, 3w)"))
	}
	return nil
}

// validateTimeRange checks that both bounds are RFC3339, for the tools whose
// service would otherwise report a malformed time without marking it as the
// caller's mistake.
func validateTimeRange(startTime, endTime string) error {
	if _, err := parseRFC3339Time(startTime); err != nil {
		return fmt.Errorf("invalid start_time: %w", err)
	}
	if _, err := parseRFC3339Time(endTime); err != nil {
		return fmt.Errorf("invalid end_time: %w", err)
	}
	return nil
}
