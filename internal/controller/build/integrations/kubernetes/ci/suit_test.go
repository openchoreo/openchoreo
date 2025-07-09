// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package ci

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestDeploymentIntegrationKubernetes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Build Integration Kubernetes CI Suite")
}

func newBuildpackBasedBuild() *openchoreov1alpha1.Build {
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
