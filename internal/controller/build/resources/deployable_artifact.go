// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/build/integrations"
	"github.com/openchoreo/openchoreo/internal/labels"
)

func MakeDeployableArtifactName(build *openchoreov1alpha1.Build) string {
	return build.Name
}

func MakeDeployableArtifact(build *openchoreov1alpha1.Build) *openchoreov1alpha1.DeployableArtifact {
	return &openchoreov1alpha1.DeployableArtifact{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DeployableArtifact",
			APIVersion: "openchoreo.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeDeployableArtifactName(build),
			Namespace: build.Namespace,
			Annotations: map[string]string{
				controller.AnnotationKeyDisplayName: MakeDeployableArtifactName(build),
				controller.AnnotationKeyDescription: "Deployable Artifact was created by the build.",
			},
			Labels: map[string]string{
				labels.LabelKeyOrganizationName:    controller.GetOrganizationName(build),
				labels.LabelKeyProjectName:         controller.GetProjectName(build),
				labels.LabelKeyComponentName:       controller.GetComponentName(build),
				labels.LabelKeyDeploymentTrackName: controller.GetDeploymentTrackName(build),
				labels.LabelKeyName:                MakeDeployableArtifactName(build),
			},
		},
		Spec: openchoreov1alpha1.DeployableArtifactSpec{
			TargetArtifact: openchoreov1alpha1.TargetArtifact{
				FromBuildRef: &openchoreov1alpha1.FromBuildRef{
					Name: build.Name,
				},
			},
		},
	}
}

func AddComponentSpecificConfigs(buildCtx *integrations.BuildContext, deployableArtifact *openchoreov1alpha1.DeployableArtifact, endpoints *[]openchoreov1alpha1.EndpointTemplate) {
	componentType := buildCtx.Component.Spec.Type
	if componentType == openchoreov1alpha1.ComponentTypeService {
		deployableArtifact.Spec.Configuration = &openchoreov1alpha1.Configuration{
			EndpointTemplates: *endpoints,
		}
	} else if componentType == openchoreov1alpha1.ComponentTypeScheduledTask {
		deployableArtifact.Spec.Configuration = &openchoreov1alpha1.Configuration{
			Application: &openchoreov1alpha1.Application{
				Task: &openchoreov1alpha1.TaskConfig{
					Disabled: false,
					Schedule: &openchoreov1alpha1.TaskSchedule{
						Cron:     "*/5 * * * *",
						Timezone: "Asia/Colombo",
					},
				},
			},
		}
	} else if componentType == openchoreov1alpha1.ComponentTypeWebApplication {
		// Set default port for the web app
		webAppPort := int32(80)
		// TODO: Currently, there is no straightforward way to configure the Nginx port in Google Buildpacks.
		//       The default port used by the buildpack is 8080.
		//       We need to find a way to change this configuration.
		if buildPackConfig := buildCtx.Build.Spec.BuildConfiguration.Buildpack; buildPackConfig != nil &&
			buildPackConfig.Name == openchoreov1alpha1.BuildpackPHP {
			webAppPort = 8080
		}
		deployableArtifact.Spec.Configuration = &openchoreov1alpha1.Configuration{
			EndpointTemplates: []openchoreov1alpha1.EndpointTemplate{
				{
					// TODO: This should come from the component descriptor in source code.
					ObjectMeta: metav1.ObjectMeta{
						Name: "webapp",
					},
					Spec: openchoreov1alpha1.EndpointSpec{
						Type: "HTTP",
						BackendRef: openchoreov1alpha1.BackendRef{
							BasePath: "/",
							Type:     openchoreov1alpha1.BackendRefTypeComponentRef,
							ComponentRef: &openchoreov1alpha1.ComponentRef{
								Port: webAppPort,
							},
						},
					},
				},
			},
		}
	}
}
