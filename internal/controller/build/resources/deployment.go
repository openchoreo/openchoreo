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
	dpkubernetes "github.com/choreo-idp/choreo/internal/dataplane/kubernetes"
	"github.com/choreo-idp/choreo/internal/labels"
)

func MakeDeploymentLabelName(environmentName string) string {
	return dpkubernetes.GenerateK8sNameWithLengthLimit(63, environmentName, "deployment")
}

func MakeDeploymentName(build *choreov1.Build, environmentName string) string {
	return dpkubernetes.GenerateK8sNameWithLengthLimit(
		dpkubernetes.MaxResourceNameLength,
		controller.GetOrganizationName(build),
		controller.GetProjectName(build),
		controller.GetComponentName(build),
		controller.GetDeploymentTrackName(build),
		environmentName,
	)
}

func MakeDeployment(buildCtx *integrations.BuildContext, environmentName string) *choreov1.Deployment {
	return &choreov1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "core.choreo.dev/v1",
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
		Spec: choreov1.DeploymentSpec{
			DeploymentArtifactRef: buildCtx.Build.Name,
		},
	}
}
