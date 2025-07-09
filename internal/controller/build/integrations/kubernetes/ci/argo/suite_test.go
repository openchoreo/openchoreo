// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package argo

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
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

	buildCtx.Registry = openchoreov1alpha1.Registry{
		Unauthenticated: []string{
			"registry.choreo-system:5000",
		},
	}

	buildCtx.Component = &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-component",
			Namespace: "test-organization",
			Labels: map[string]string{
				labels.LabelKeyOrganizationName: "test-organization",
				labels.LabelKeyProjectName:      "test-project",
				labels.LabelKeyName:             "test-component",
			},
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Type: openchoreov1alpha1.ComponentTypeService,
			Source: openchoreov1alpha1.ComponentSource{
				GitRepository: &openchoreov1alpha1.GitRepository{
					URL: "https://github.com/openchoreo/test",
				},
			},
		},
	}
	buildCtx.DeploymentTrack = &openchoreov1alpha1.DeploymentTrack{
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
	buildCtx.Build = &openchoreov1alpha1.Build{
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
	buildCtx.DeploymentTrack.Spec.BuildTemplateSpec = &openchoreov1alpha1.BuildTemplateSpec{
		Branch: "main",
		Path:   "/test-service",
		BuildConfiguration: &openchoreov1alpha1.BuildConfiguration{
			Docker: &openchoreov1alpha1.DockerConfiguration{
				Context:        "/time-logger",
				DockerfilePath: "/time-logger/Dockerfile",
			},
		},
	}

	buildCtx.Build.Spec = openchoreov1alpha1.BuildSpec{
		Branch: "main",
		Path:   "/test-service",
		BuildConfiguration: openchoreov1alpha1.BuildConfiguration{
			Docker: &openchoreov1alpha1.DockerConfiguration{
				Context:        "/time-logger",
				DockerfilePath: "/time-logger/Dockerfile",
			},
		},
	}
	return buildCtx
}

func newBuildpackBasedBuildCtx(buildCtx *integrations.BuildContext) *integrations.BuildContext {
	buildCtx.DeploymentTrack.Spec.BuildTemplateSpec = &openchoreov1alpha1.BuildTemplateSpec{
		Branch: "main",
		Path:   "/test-service",
		BuildConfiguration: &openchoreov1alpha1.BuildConfiguration{
			Buildpack: &openchoreov1alpha1.BuildpackConfiguration{
				Name:    openchoreov1alpha1.BuildpackGo,
				Version: openchoreov1alpha1.SupportedVersions[openchoreov1alpha1.BuildpackGo][0],
			},
		},
	}

	buildCtx.Build.Spec = openchoreov1alpha1.BuildSpec{
		Branch: "main",
		Path:   "/test-service",
		BuildConfiguration: openchoreov1alpha1.BuildConfiguration{
			Buildpack: &openchoreov1alpha1.BuildpackConfiguration{
				Name:    openchoreov1alpha1.BuildpackGo,
				Version: openchoreov1alpha1.SupportedVersions[openchoreov1alpha1.BuildpackGo][0],
			},
		},
	}
	return buildCtx
}

func newBuildContextWithRegistries(buildCtx *integrations.BuildContext) *integrations.BuildContext {
	buildCtx.Registry = openchoreov1alpha1.Registry{
		Unauthenticated: []string{
			"registry.choreo-system:5000",
		},
		ImagePushSecrets: []openchoreov1alpha1.ImagePushSecret{
			{
				Name:   "dev-dockerhub-push-secret",
				Prefix: "docker.io/test-org",
			},
			{
				Name:   "prod-ghcr-push-secret",
				Prefix: "ghcr.io/test-org",
			},
		},
	}

	return buildCtx
}
