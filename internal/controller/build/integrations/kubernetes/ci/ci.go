/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
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
