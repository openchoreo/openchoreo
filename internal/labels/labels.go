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

package labels

// This file contains the all the labels that are used to store Choreo specific the metadata in the Kubernetes objects.

const (
	LabelKeyOrganizationName       = "core.choreo.dev/organization"
	LabelKeyProjectName            = "core.choreo.dev/project"
	LabelKeyComponentName          = "core.choreo.dev/component"
	LabelKeyDeploymentTrackName    = "core.choreo.dev/deployment-track"
	LabelKeyBuildName              = "core.choreo.dev/build"
	LabelKeyEnvironmentName        = "core.choreo.dev/environment"
	LabelKeyName                   = "core.choreo.dev/name"
	LabelKeyDeployableArtifactName = "core.choreo.dev/deployable-artifact"
	LabelKeyDeploymentName         = "core.choreo.dev/deployment"

	LabelKeyManagedBy = "managed-by"

	LabelValueManagedBy = "choreo-control-plane"
)
