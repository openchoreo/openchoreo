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

package kinds

//
// ERROR MESSAGES
//

// Common error messages used across resource implementations
const (
	// Common error messages
	ErrCreateKubeClient  = "failed to create Kubernetes client: %w"
	ErrFormatUnsupported = "unsupported output format: %s"
)

// Resource-specific error messages
const (
	ErrCreateProject = "failed to create project: %w"
	// Component related errors
	ErrCreateComponent = "failed to create component: %w"
	ErrCreateDepTrack  = "failed to create deployment track: %w"

	// Deployment related errors
	ErrCreateDeployment = "failed to create deployment: %w"

	// Build related errors
	ErrCreateBuild = "failed to create build: %w"

	// Environment related errors
	ErrCreateEnvironment = "failed to create environment: %w"

	// Endpoint related errors
	ErrCreateEndpoint = "failed to create endpoint: %w"

	// DataPlane related errors
	ErrCreateDataPlane = "failed to create dataplane: %w"

	// Organization related errors
	ErrCreateOrganization = "failed to create organization: %w"

	// DeployableArtifact related errors
	ErrCreateArtifact = "failed to create deployable artifact: %w"

	// DeploymentTrack related errors
	ErrCreateDeploymentTrack = "failed to create deployment track: %w"
)

//
// TABLE HEADERS
//

// Common header values used across resource types
const (
	HeaderName            = "NAME"
	HeaderStatus          = "STATUS"
	HeaderAge             = "AGE"
	HeaderType            = "TYPE"
	HeaderProject         = "PROJECT"
	HeaderOrganization    = "ORGANIZATION"
	HeaderComponent       = "COMPONENT"
	HeaderEnvironment     = "ENVIRONMENT"
	HeaderDeploymentTrack = "DEPLOYMENT TRACK"
	HeaderRevision        = "REVISION"
	HeaderDuration        = "DURATION"
	HeaderSource          = "SOURCE"
	HeaderArtifact        = "ARTIFACT"
	HeaderAPIVersion      = "API VERSION"
	HeaderAutoDeploy      = "AUTO DEPLOY"
	HeaderDataPlane       = "DATA PLANE"
	HeaderProduction      = "PRODUCTION"
	HeaderDNSPrefix       = "DNS PREFIX"
	HeaderCluster         = "CLUSTER"
	HeaderAddress         = "ADDRESS"
)

// Resource-specific table headers defined as variables (not constants)
var (
	// Organization table headers
	HeadersOrganization = []string{HeaderName, HeaderAge, HeaderStatus}

	// Project table headers
	HeadersProject = []string{HeaderName, HeaderStatus, HeaderAge, HeaderOrganization}

	// Component table headers
	HeadersComponent = []string{HeaderName, HeaderType, HeaderStatus, HeaderAge, HeaderProject, HeaderOrganization}

	// Build table headers
	HeadersBuild = []string{HeaderName, HeaderStatus, HeaderRevision, HeaderDuration, HeaderAge, HeaderComponent, HeaderProject, HeaderOrganization}

	// DeployableArtifact table headers
	HeadersDeployableArtifact = []string{HeaderName, HeaderSource, HeaderStatus, HeaderAge, HeaderComponent, HeaderProject, HeaderOrganization}

	// Deployment table headers
	HeadersDeployment = []string{HeaderName, HeaderArtifact, HeaderEnvironment, HeaderStatus, HeaderAge, HeaderComponent, HeaderProject, HeaderOrganization}

	// DeploymentTrack table headers
	HeadersDeploymentTrack = []string{HeaderName, HeaderAPIVersion, HeaderAutoDeploy, HeaderAge, HeaderComponent, HeaderProject, HeaderOrganization}

	// Environment table headers
	HeadersEnvironment = []string{HeaderName, HeaderDataPlane, HeaderProduction, HeaderDNSPrefix, HeaderAge, HeaderOrganization}

	// DataPlane table headers
	HeadersDataPlane = []string{HeaderName, HeaderCluster, HeaderStatus, HeaderAge, HeaderOrganization}

	// Endpoint table headers
	HeadersEndpoint = []string{HeaderName, HeaderType, HeaderAddress, HeaderStatus, HeaderAge, HeaderComponent, HeaderProject, HeaderOrganization, HeaderEnvironment}
)

//
// STATUS CONSTANTS
//

// Status constants used for resource status reporting
const (
	StatusPending      = "Pending"
	StatusReady        = "Ready"
	StatusNotReady     = "Not Ready"
	StatusInitializing = "Initializing"
)

//
// CONDITION TYPES
//

// Common condition types used across resources
const (
	ConditionTypeReady       = "Ready"
	ConditionTypeCreated     = "Created"
	ConditionTypeInitialized = "Initialized"
)

// Deployment specific condition types
const (
	ConditionTypeDeployed    = "Deployed"
	ConditionTypeProgressing = "Progressing"
	ConditionTypeAvailable   = "Available"
)

// Build specific condition types
const (
	ConditionTypeBuildStarted   = "BuildStarted"
	ConditionTypeBuildComplete  = "BuildComplete"
	ConditionTypeBuildFailed    = "BuildFailed"
	ConditionTypeBuildSucceeded = "BuildSucceeded"
	ConditionTypePushSucceeded  = "PushSucceeded"
	ConditionTypePushFailed     = "PushFailed"
)

// Environment specific condition types
const (
	ConditionTypeConfigured = "Configured"
)

//
// CONDITION STATUS VALUES
//

// Condition status constants
const (
	ConditionStatusTrue  = "True"
	ConditionStatusFalse = "False"
)

//
// FORMAT STRINGS
//

// Format strings for status reporting
const (
	// Status formatting
	FmtStatusWithReason     = "%s (%s)"
	FmtStatusWithMessage    = "%s: %s - %s"
	FmtStatusWithType       = "%s: %s"
	FmtStatusWithTypeReason = "%s: %s"
)

// Duration formatting strings
const (
	FmtDurationSeconds = "%ds"
	FmtDurationMinSec  = "%dm%ds"
	FmtDurationHourMin = "%dh%dm"
)

//
// SUCCESS MESSAGES
//

// Success messages for resource creation
const (
	FmtProjectSuccess = "Project '%s' created successfully in organization '%s'\n"
	// Component success messages
	FmtComponentSuccess = "Component '%s' created successfully in project '%s' of organization '%s'\n"

	// Deployment success messages
	FmtDeploymentSuccess = "Deployment '%s' created successfully in environment '%s' for component '%s' of project '%s' in organization '%s'\n"
	FmtDeploySuccessMsg  = "Deployment '%s' created successfully in environment '%s' for component '%s' of project '%s' in organization '%s'\n"

	// Build success messages
	FmtBuildSuccess       = "Build '%s' created successfully for component '%s' in project '%s' of organization '%s'\n"
	FmtBuildCreateSuccess = "Build '%s' created successfully for component '%s' in project '%s' of organization '%s'\n"

	// Environment success messages
	FmtEnvironmentSuccess = "Environment '%s' created successfully in organization '%s'\n"

	// DataPlane success messages
	FmtDataPlaneCreateSuccess = "DataPlane '%s' created successfully in organization '%s'\n"

	// Organization success messages
	FmtOrganizationSuccess = "Organization '%s' created\n"

	// DeploymentTrack success messages
	FmtDeploymentTrackSuccess = "Deployment track '%s' created successfully in component '%s' of project '%s' in organization '%s'\n"

	// DeployableArtifact success messages
	FmtDeployableArtifactSuccess = "Deployable artifact '%s' created successfully in component '%s' of project '%s' in organization '%s'\n"

	// Endpoint success messages
	FmtEndpointSuccess = "Endpoint '%s' created successfully in component '%s' of project '%s' in organization '%s' for environment '%s'\n"
)

//
// DEFAULT VALUES
//

// Default values used across resources
const (
	// Path and repository defaults
	DefaultBranch     = "main"
	DefaultPath       = "/"
	DefaultContext    = "/"
	DefaultDockerfile = "Dockerfile"

	// Track names
	DefaultTrackName = "default"
)

//
// ANNOTATIONS
//

// Annotations used across resources
const (
	// DeploymentTrack annotations
	AutoDeployAnnotation = "core.choreo.dev/auto-deploy"
)

//
// PLACEHOLDERS
//

// Placeholder values used in output formatting
const (
	PlaceholderDuration = "-"
	PlaceholderAddress  = "-"
)

//
// DEPLOYMENT PIPELINES
//

// Deployment pipelines
const (
	DefaultDeploymentPipeline = "default"
)
