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
	"fmt"
	"path"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
)

func makeHostname(componentName, environmentName string, componentType choreov1.ComponentType) gatewayv1.Hostname {
	if componentType == choreov1.ComponentTypeWebApplication {
		return gatewayv1.Hostname(fmt.Sprintf("%s-%s.choreo.local", componentName, environmentName))
	}
	return gatewayv1.Hostname(fmt.Sprintf("%s.apis.choreo.local", environmentName))
}

func makePathPrefix(projectName, componentName string, componentType choreov1.ComponentType) string {
	if componentType == choreov1.ComponentTypeWebApplication {
		return "/"
	}
	return path.Clean(path.Join("/", projectName, componentName))
}

func MakeAddress(projectName, componentName, environmentName string, componentType choreov1.ComponentType, basePath string) string {
	host := makeHostname(componentName, environmentName, componentType)
	pathPrefix := makePathPrefix(projectName, componentName, componentType)

	return fmt.Sprintf("https://%s", path.Join(string(host), pathPrefix, basePath))
}
