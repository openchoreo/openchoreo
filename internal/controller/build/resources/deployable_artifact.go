// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/build/integrations"
	"github.com/openchoreo/openchoreo/internal/labels"
)

func MakeDeployableArtifactName(build *choreov1.Build) string {
	return build.Name
}

func MakeDeployableArtifact(build *choreov1.Build) *choreov1.DeployableArtifact {
	return &choreov1.DeployableArtifact{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DeployableArtifact",
			APIVersion: "core.choreo.dev/v1",
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
		Spec: choreov1.DeployableArtifactSpec{
			TargetArtifact: choreov1.TargetArtifact{
				FromBuildRef: &choreov1.FromBuildRef{
					Name: build.Name,
				},
			},
		},
	}
}

func AddComponentSpecificConfigs(buildCtx *integrations.BuildContext, deployableArtifact *choreov1.DeployableArtifact, endpoints *[]choreov1.EndpointTemplate) {
	componentType := buildCtx.Component.Spec.Type
	if componentType == choreov1.ComponentTypeService {
		deployableArtifact.Spec.Configuration = &choreov1.Configuration{
			EndpointTemplates: *endpoints,
		}
	} else if componentType == choreov1.ComponentTypeScheduledTask {
		deployableArtifact.Spec.Configuration = &choreov1.Configuration{
			Application: &choreov1.Application{
				Task: &choreov1.TaskConfig{
					Disabled: false,
					Schedule: &choreov1.TaskSchedule{
						Cron:     "*/5 * * * *",
						Timezone: "Asia/Colombo",
					},
				},
			},
		}
	} else if componentType == choreov1.ComponentTypeWebApplication {
		// Set default port for the web app
		webAppPort := int32(80)
		// TODO: Currently, there is no straightforward way to configure the Nginx port in Google Buildpacks.
		//       The default port used by the buildpack is 8080.
		//       We need to find a way to change this configuration.
		if buildPackConfig := buildCtx.Build.Spec.BuildConfiguration.Buildpack; buildPackConfig != nil &&
			buildPackConfig.Name == choreov1.BuildpackPHP {
			webAppPort = 8080
		}
		deployableArtifact.Spec.Configuration = &choreov1.Configuration{
			EndpointTemplates: []choreov1.EndpointTemplate{
				{
					// TODO: This should come from the component descriptor in source code.
					ObjectMeta: metav1.ObjectMeta{
						Name: "webapp",
					},
					Spec: choreov1.EndpointSpec{
						Type: "HTTP",
						Service: choreov1.EndpointServiceSpec{
							BasePath: "/",
							Port:     webAppPort,
						},
					},
				},
			},
		}
	}
}
