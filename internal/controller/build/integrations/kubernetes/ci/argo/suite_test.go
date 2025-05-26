// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package argo

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
	RunSpecs(t, "Build Integration Kubernetes CI Argo Suite")
}

func imageName() string {
	return "test-organization-test-project-test-component-999d9b43:test-main-track-51849cfd"
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

func newDockerBasedBuildCtx(buildCtx *integrations.BuildContext) *integrations.BuildContext {
	buildCtx.DeploymentTrack.Spec.BuildTemplateSpec = &choreov1.BuildTemplateSpec{
		Branch: "main",
		Path:   "/test-service",
		BuildConfiguration: &choreov1.BuildConfiguration{
			Docker: &choreov1.DockerConfiguration{
				Context:        "/time-logger",
				DockerfilePath: "/time-logger/Dockerfile",
			},
		},
	}

	buildCtx.Build.Spec = choreov1.BuildSpec{
		Branch: "main",
		Path:   "/test-service",
		BuildConfiguration: choreov1.BuildConfiguration{
			Docker: &choreov1.DockerConfiguration{
				Context:        "/time-logger",
				DockerfilePath: "/time-logger/Dockerfile",
			},
		},
	}
	return buildCtx
}

func newBuildpackBasedBuildCtx(buildCtx *integrations.BuildContext) *integrations.BuildContext {
	buildCtx.DeploymentTrack.Spec.BuildTemplateSpec = &choreov1.BuildTemplateSpec{
		Branch: "main",
		Path:   "/test-service",
		BuildConfiguration: &choreov1.BuildConfiguration{
			Buildpack: &choreov1.BuildpackConfiguration{
				Name:    choreov1.BuildpackGo,
				Version: choreov1.SupportedVersions[choreov1.BuildpackGo][0],
			},
		},
	}

	buildCtx.Build.Spec = choreov1.BuildSpec{
		Branch: "main",
		Path:   "/test-service",
		BuildConfiguration: choreov1.BuildConfiguration{
			Buildpack: &choreov1.BuildpackConfiguration{
				Name:    choreov1.BuildpackGo,
				Version: choreov1.SupportedVersions[choreov1.BuildpackGo][0],
			},
		},
	}
	return buildCtx
}
