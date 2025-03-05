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
	"github.com/choreo-idp/choreo/internal/controller"
	"github.com/choreo-idp/choreo/internal/dataplane"
	dpkubernetes "github.com/choreo-idp/choreo/internal/dataplane/kubernetes"
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
