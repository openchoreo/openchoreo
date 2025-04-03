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
