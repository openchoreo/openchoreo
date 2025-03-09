/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

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

	errMsg.WriteString(fmt.Sprintf("Missing required fields for %s %s: %s\n\n",
		cmdType, resource, strings.Join(missingFields, ", ")))

	errMsg.WriteString("To see how to use this command, run:\n")
	errMsg.WriteString(fmt.Sprintf("  choreoctl %s %s -h\n\n", cmdType, resource))
	errMsg.WriteString("To use interactive mode, run:\n")
	errMsg.WriteString(fmt.Sprintf("  choreoctl %s %s --interactive", cmdType, resource))

	return fmt.Errorf("%s", errMsg.String())
}
