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

package resources

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations"
	"github.com/choreo-idp/choreo/internal/labels"
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
							Port:     80,
						},
					},
				},
			},
		}
	}
}
