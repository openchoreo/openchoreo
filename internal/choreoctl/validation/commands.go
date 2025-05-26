// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"strings"
)

// CommandType represents the type of CLI command
type CommandType string

const (
	CmdCreate CommandType = "create"
	CmdGet    CommandType = "get"
	CmdLogs   CommandType = "logs"
	CmdApply  CommandType = "apply"
)

// ResourceType represents the resource being managed
type ResourceType string

const (
	ResourceProject            ResourceType = "project"
	ResourceComponent          ResourceType = "component"
	ResourceBuild              ResourceType = "build"
	ResourceDeployment         ResourceType = "deployment"
	ResourceDeploymentTrack    ResourceType = "deploymenttrack"
	ResourceEnvironment        ResourceType = "environment"
	ResourceDeployableArtifact ResourceType = "deployableartifact"
	ResourceEndpoint           ResourceType = "endpoint"
	ResourceOrganization       ResourceType = "organization"
	ResourceDataPlane          ResourceType = "dataplane"
	ResourceLogs               ResourceType = "logs"
	ResourceApply              ResourceType = "apply"
	ResourceDeploymentPipeline ResourceType = "deploymentpipeline"
	ResourceConfigurationGroup ResourceType = "configurationgroup"
)

// checkRequiredFields verifies if all required fields are populated
func checkRequiredFields(fields map[string]string) bool {
	for _, v := range fields {
		if v == "" {
			return false
		}
	}
	return true
}

// generateHelpError creates a help message for missing required fields
func generateHelpError(cmdType CommandType, resource ResourceType, fields map[string]string) error {
	var errMsg strings.Builder
	var missingFields []string

	// Identify which fields are missing
	for field, value := range fields {
		if value == "" {
			missingFields = append(missingFields, field)
		}
	}

	errMsg.WriteString(fmt.Sprintf("Missing required parameter%s: --%s\n\n",
		pluralS(len(missingFields)), strings.Join(missingFields, ", --")))

	errMsg.WriteString("To see usage details:\n")
	if resource == "" {
		errMsg.WriteString(fmt.Sprintf("  choreoctl %s -h", cmdType))
	} else {
		errMsg.WriteString(fmt.Sprintf("  choreoctl %s %s -h", cmdType, resource))
	}

	// Only show interactive mode for commands that typically support it
	if cmdType != CmdApply {
		errMsg.WriteString("\n\nTo use interactive mode:\n")
		if resource == "" {
			errMsg.WriteString(fmt.Sprintf("  choreoctl %s --interactive", cmdType))
		} else {
			errMsg.WriteString(fmt.Sprintf("  choreoctl %s %s --interactive", cmdType, resource))
		}
	}

	return fmt.Errorf("%s", errMsg.String())
}

// Helper function to handle plural forms
func pluralS(count int) string {
	if count > 1 {
		return "s"
	}
	return ""
}
