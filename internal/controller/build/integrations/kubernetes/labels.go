// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"github.com/openchoreo/openchoreo/internal/controller/build/integrations"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

func MakeLabels(buildCtx *integrations.BuildContext) map[string]string {
	return map[string]string{
		dpkubernetes.LabelKeyManagedBy: dpkubernetes.LabelBuildControllerCreated,
	}
}

func ExtractManagedLabels(labels map[string]string) map[string]string {
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
