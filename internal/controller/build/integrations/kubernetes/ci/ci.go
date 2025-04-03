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

package ci

import (
	"fmt"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// ConstructImageNameWithTag constructs an image name with the tag.
// The git revision is added from the workflow.
func ConstructImageNameWithTag(build *choreov1.Build) string {
	orgName := build.ObjectMeta.Labels[labels.LabelKeyOrganizationName]
	projName := build.ObjectMeta.Labels[labels.LabelKeyProjectName]
	componentName := build.ObjectMeta.Labels[labels.LabelKeyComponentName]
	dtName := build.ObjectMeta.Labels[labels.LabelKeyDeploymentTrackName]

	// To prevent excessively long image names, we limit them to 128 characters for the name and 128 characters for the tag.
	imageName := dpkubernetes.GenerateK8sNameWithLengthLimit(128, orgName, projName, componentName)
	// Reserve 8 chars for commit SHA.
	tagName := dpkubernetes.GenerateK8sNameWithLengthLimit(119, dtName)

	return fmt.Sprintf("%s:%s", imageName, tagName)
}
