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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller/build/integrations"
	"github.com/openchoreo/openchoreo/internal/labels"
)

func TestDeploymentIntegrationKubernetes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Build Integration Kubernetes Suite")
}

// Create a new BuildContext for testing. Each test should create a new context
// and set the required fields for the test.
func newTestBuildContext() *integrations.BuildContext {
	buildCtx := &integrations.BuildContext{}

	buildCtx.Component = &choreov1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-component",
			Namespace: "test-organization",
			Labels: map[string]string{
				labels.LabelKeyOrganizationName: "test-organization",
				labels.LabelKeyProjectName:      "test-project",
				labels.LabelKeyName:             "test-component",
			},
		},
		Spec: choreov1.ComponentSpec{
			Type: choreov1.ComponentTypeService,
			Source: choreov1.ComponentSource{
				GitRepository: &choreov1.GitRepository{
					URL: "https://github.com/openchoreo/test",
				},
			},
		},
	}
	buildCtx.DeploymentTrack = &choreov1.DeploymentTrack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-main-track",
			Namespace: "test-organization",
			Labels: map[string]string{
				labels.LabelKeyOrganizationName: "test-organization",
				labels.LabelKeyProjectName:      "test-project",
				labels.LabelKeyComponentName:    "test-component",
				labels.LabelKeyName:             "test-main-track",
			},
		},
	}
	buildCtx.Build = &choreov1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-build",
			Namespace: "test-organization",
			Labels: map[string]string{
				labels.LabelKeyOrganizationName:    "test-organization",
				labels.LabelKeyProjectName:         "test-project",
				labels.LabelKeyComponentName:       "test-component",
				labels.LabelKeyDeploymentTrackName: "test-main-track",
				labels.LabelKeyName:                "test-build",
			},
		},
	}

	return buildCtx
}
