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

package kubernetes

import (
	"github.com/choreo-idp/choreo/internal/controller"
	"github.com/choreo-idp/choreo/internal/dataplane"
	dpkubernetes "github.com/choreo-idp/choreo/internal/dataplane/kubernetes"
)

// MakeNamespaceName has the format dp-<organization-name>-<project-name>-<environment-name>-<hash>
func MakeNamespaceName(epCtx *dataplane.EndpointContext) string {
	organizationName := controller.GetOrganizationName(epCtx.Project)
	projectName := controller.GetName(epCtx.Project)
	environmentName := controller.GetName(epCtx.Environment)
	return dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxNamespaceNameLength, "dp", organizationName, projectName, environmentName)
}

// MakeServiceName has the format dp-<component-name>-<deployment-track-name>-<hash>
func MakeServiceName(deployCtx *dataplane.EndpointContext) string {
	componentName := deployCtx.Component.Name
	deploymentTrackName := deployCtx.DeploymentTrack.Name
	return dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxServiceNameLength, componentName, deploymentTrackName)
}

// MakeHTTPRouteName has the format dp-<gateway-name>-<component-name>-<endpoint-name>-<hash>
func MakeHTTPRouteName(epCtx *dataplane.EndpointContext, gwName string) string {
	componentName := epCtx.Component.Name
	endpointName := epCtx.Endpoint.Name
	return dpkubernetes.GenerateK8sName(gwName, componentName, endpointName)
}
