// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/build/integrations"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
)

func MakeDeploymentLabelName(environmentName string) string {
	return dpkubernetes.GenerateK8sNameWithLengthLimit(63, environmentName, "deployment")
}

func MakeDeploymentName(build *openchoreov1alpha1.Build, environmentName string) string {
	return dpkubernetes.GenerateK8sNameWithLengthLimit(
		dpkubernetes.MaxResourceNameLength,
		controller.GetOrganizationName(build),
		controller.GetProjectName(build),
		controller.GetComponentName(build),
		controller.GetDeploymentTrackName(build),
		environmentName,
	)
}

func MakeDeployment(buildCtx *integrations.BuildContext, environmentName string) *openchoreov1alpha1.Deployment {
	return &openchoreov1alpha1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "openchoreo.dev/v1alpha1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeDeploymentName(buildCtx.Build, environmentName),
			Namespace: buildCtx.Build.Namespace,
			Annotations: map[string]string{
				controller.AnnotationKeyDisplayName: MakeDeploymentLabelName(environmentName),
				controller.AnnotationKeyDescription: "Deployment was created by the build.",
			},
			Labels: map[string]string{
				labels.LabelKeyOrganizationName:    controller.GetOrganizationName(buildCtx.Build),
				labels.LabelKeyProjectName:         controller.GetProjectName(buildCtx.Build),
				labels.LabelKeyComponentName:       controller.GetComponentName(buildCtx.Build),
				labels.LabelKeyDeploymentTrackName: controller.GetDeploymentTrackName(buildCtx.Build),
				labels.LabelKeyEnvironmentName:     environmentName,
				labels.LabelKeyName:                MakeDeploymentLabelName(environmentName),
			},
		},
		Spec: openchoreov1alpha1.DeploymentSpec{
			DeploymentArtifactRef: buildCtx.Build.Name,
		},
	}
}
