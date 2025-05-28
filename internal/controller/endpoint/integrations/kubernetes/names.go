// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/endpoint/integrations/kubernetes/visibility"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// makeNamespaceName has the format dp-<organization-name>-<project-name>-<environment-name>-<hash>
func makeNamespaceName(epCtx *dataplane.EndpointContext) string {
	organizationName := controller.GetOrganizationName(epCtx.Project)
	projectName := controller.GetName(epCtx.Project)
	environmentName := controller.GetName(epCtx.Environment)
	return dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxNamespaceNameLength, "dp", organizationName, projectName, environmentName)
}

// makeServiceName has the format dp-<component-name>-<deployment-track-name>-<hash>
func makeServiceName(epCtx *dataplane.EndpointContext) string {
	componentName := epCtx.Component.Name
	deploymentTrackName := epCtx.DeploymentTrack.Name
	return dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxServiceNameLength, componentName, deploymentTrackName)
}

// makeHTTPRouteName has the format dp-<gateway-name>-<endpoint-name>-<method>-<operation>-<hash>
func makeHTTPRouteName(epCtx *dataplane.EndpointContext, gwType visibility.GatewayType, method, operation string) string {
	endpointName := epCtx.Endpoint.Name
	return dpkubernetes.GenerateK8sName(string(gwType), endpointName, method, operation)
}
