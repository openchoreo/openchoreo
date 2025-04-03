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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

func TestDeploymentIntegrationKubernetes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Build Integration Kubernetes CI Suite")
}

func newBuildpackBasedBuild() *choreov1.Build {
	return &choreov1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-build",
			Labels: map[string]string{
				"core.choreo.dev/organization":     "test-organization",
				"core.choreo.dev/project":          "test-project",
				"core.choreo.dev/component":        "test-component",
				"core.choreo.dev/deployment-track": "test-main",
				"core.choreo.dev/name":             "test-build",
			},
		},
		Spec: choreov1.BuildSpec{
			Branch: "main",
			Path:   "/test-service",
			BuildConfiguration: choreov1.BuildConfiguration{
				Buildpack: &choreov1.BuildpackConfiguration{
					Name:    "Go",
					Version: "1.x",
				},
			},
		},
	}
}
