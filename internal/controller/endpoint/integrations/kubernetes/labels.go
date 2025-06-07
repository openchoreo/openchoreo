// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/endpoint/integrations/kubernetes/visibility"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

func makeLabels(epCtx *dataplane.EndpointContext) map[string]string {
	return map[string]string{
		dpkubernetes.LabelKeyOrganizationName:    controller.GetOrganizationName(epCtx.Project),
		dpkubernetes.LabelKeyProjectName:         controller.GetName(epCtx.Project),
		dpkubernetes.LabelKeyComponentName:       controller.GetName(epCtx.Component),
		dpkubernetes.LabelKeyDeploymentTrackName: controller.GetName(epCtx.DeploymentTrack),
		dpkubernetes.LabelKeyEnvironmentName:     controller.GetName(epCtx.Environment),
		dpkubernetes.LabelKeyDeploymentName:      controller.GetName(epCtx.Deployment),
		dpkubernetes.LabelKeyManagedBy:           dpkubernetes.LabelValueManagedBy,
		dpkubernetes.LabelKeyBelongTo:            dpkubernetes.LabelValueBelongTo,
	}
}

func makeWorkloadLabels(epCtx *dataplane.EndpointContext, gwType visibility.GatewayType) map[string]string {
	labels := makeLabels(epCtx)
	labels[dpkubernetes.LabelKeyComponentType] = string(epCtx.Component.Spec.Type)
	labels[dpkubernetes.LabelKeyVisibility] = string(gwType)
	return labels
}

func extractManagedLabels(labels map[string]string) map[string]string {
	return map[string]string{
		dpkubernetes.LabelKeyOrganizationName:    labels[dpkubernetes.LabelKeyOrganizationName],
		dpkubernetes.LabelKeyProjectName:         labels[dpkubernetes.LabelKeyProjectName],
		dpkubernetes.LabelKeyComponentName:       labels[dpkubernetes.LabelKeyComponentName],
		dpkubernetes.LabelKeyDeploymentTrackName: labels[dpkubernetes.LabelKeyDeploymentTrackName],
		dpkubernetes.LabelKeyEnvironmentName:     labels[dpkubernetes.LabelKeyEnvironmentName],
		dpkubernetes.LabelKeyDeploymentName:      labels[dpkubernetes.LabelKeyDeploymentName],
		dpkubernetes.LabelKeyManagedBy:           labels[dpkubernetes.LabelKeyManagedBy],
		dpkubernetes.LabelKeyBelongTo:            labels[dpkubernetes.LabelKeyBelongTo],
		dpkubernetes.LabelKeyComponentType:       labels[dpkubernetes.LabelKeyComponentType],
	}
}
