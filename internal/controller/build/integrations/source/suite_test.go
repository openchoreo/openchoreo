// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package source

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
	RunSpecs(t, "Build Source Suite")
}

// Create a new BuildContext for testing. Each test should create a new context
// and set the required fields for the test.
func newTestBuildContext() *integrations.BuildContext {
	buildCtx := &integrations.BuildContext{}

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
				labels.LabelKeyName:             "test-main",
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
				labels.LabelKeyDeploymentTrackName: "test-main",
				labels.LabelKeyName:                "test-build",
			},
		},
	}

	return buildCtx
}

func newTestBuildpackBasedBuild() *openchoreov1alpha1.Build {
	return &openchoreov1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-build",
			Labels: map[string]string{
				"openchoreo.dev/organization":     "test-organization",
				"openchoreo.dev/project":          "test-project",
				"openchoreo.dev/component":        "test-component",
				"openchoreo.dev/deployment-track": "test-main",
				"openchoreo.dev/name":             "test-build",
			},
			Namespace: "test-organization",
		},
		Spec: openchoreov1alpha1.BuildSpec{
			Branch: "main",
			Path:   "/test-service",
			BuildConfiguration: openchoreov1alpha1.BuildConfiguration{
				Buildpack: &openchoreov1alpha1.BuildpackConfiguration{
					Name:    "Go",
					Version: "1.x",
				},
			},
		},
	}
}
