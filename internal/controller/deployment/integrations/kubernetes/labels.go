/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package kubernetes

import (
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

func makeNamespaceLabels(deployCtx *dataplane.DeploymentContext) map[string]string {
	return map[string]string{
		dpkubernetes.LabelKeyOrganizationName: controller.GetOrganizationName(deployCtx.Project),
		dpkubernetes.LabelKeyProjectName:      controller.GetName(deployCtx.Project),
		dpkubernetes.LabelKeyEnvironmentName:  controller.GetName(deployCtx.Environment),
		dpkubernetes.LabelKeyManagedBy:        dpkubernetes.LabelValueManagedBy,
		dpkubernetes.LabelKeyBelongTo:         dpkubernetes.LabelValueBelongTo,
	}
}

func makeWorkloadLabels(deployCtx *dataplane.DeploymentContext) map[string]string {
	labels := makeNamespaceLabels(deployCtx)
	labels[dpkubernetes.LabelKeyComponentName] = controller.GetName(deployCtx.Component)
	labels[dpkubernetes.LabelKeyComponentType] = string(deployCtx.Component.Spec.Type)
	labels[dpkubernetes.LabelKeyDeploymentTrackName] = controller.GetName(deployCtx.DeploymentTrack)
	labels[dpkubernetes.LabelKeyDeploymentName] = controller.GetName(deployCtx.Deployment)
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
