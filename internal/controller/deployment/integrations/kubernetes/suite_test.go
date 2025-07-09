// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	"github.com/openchoreo/openchoreo/internal/labels"
)

func TestDeploymentIntegrationKubernetes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Deployment Integration Kubernetes Suite")
}

// Create a new DeploymentContext for testing. Each test should create a new context
// and set the required fields for the test.
func newTestDeploymentContext() *dataplane.DeploymentContext {
	deployCtx := &dataplane.DeploymentContext{}

	deployCtx.Project = &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-project",
			Namespace: "test-organization",
			Labels: map[string]string{
				labels.LabelKeyOrganizationName: "test-organization",
				labels.LabelKeyName:             "my-project",
			},
		},
	}
	deployCtx.Environment = &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-environment",
			Namespace: "test-organization",
			Labels: map[string]string{
				labels.LabelKeyOrganizationName: "test-organization",
				labels.LabelKeyName:             "test-environment",
			},
		},
	}
	deployCtx.Component = &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-component",
			Namespace: "test-organization",
			Labels: map[string]string{
				labels.LabelKeyOrganizationName: "test-organization",
				labels.LabelKeyProjectName:      "my-project",
				labels.LabelKeyName:             "my-component",
			},
		},
	}
	deployCtx.DeploymentTrack = &openchoreov1alpha1.DeploymentTrack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-main-track",
			Namespace: "test-organization",
			Labels: map[string]string{
				labels.LabelKeyOrganizationName: "test-organization",
				labels.LabelKeyProjectName:      "my-project",
				labels.LabelKeyComponentName:    "my-component",
				labels.LabelKeyName:             "my-main-track",
			},
		},
	}
	deployCtx.DeployableArtifact = &openchoreov1alpha1.DeployableArtifact{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-artifact",
			Namespace: "test-organization",
			Labels: map[string]string{
				labels.LabelKeyOrganizationName:    "test-organization",
				labels.LabelKeyProjectName:         "my-project",
				labels.LabelKeyComponentName:       "my-component",
				labels.LabelKeyDeploymentTrackName: "my-main-track",
				labels.LabelKeyName:                "my-artifact",
			},
		},
	}

	deployCtx.Deployment = &openchoreov1alpha1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deployment",
			Namespace: "test-organization",
			Labels: map[string]string{
				labels.LabelKeyOrganizationName:    "test-organization",
				labels.LabelKeyProjectName:         "my-project",
				labels.LabelKeyEnvironmentName:     "test-environment",
				labels.LabelKeyComponentName:       "my-component",
				labels.LabelKeyDeploymentTrackName: "my-main-track",
				labels.LabelKeyName:                "my-deployment",
			},
		},
	}

	deployCtx.ContainerImage = "my-image:latest"

	return deployCtx
}

func newTestConfigurationGroup(name string, spec openchoreov1alpha1.ConfigurationGroupSpec) *openchoreov1alpha1.ConfigurationGroup {
	return &openchoreov1alpha1.ConfigurationGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-organization",
			Labels: map[string]string{
				labels.LabelKeyOrganizationName: "test-organization",
				labels.LabelKeyName:             name,
			},
		},
		Spec: spec,
	}
}

func newTestRedisConfigurationGroup() *openchoreov1alpha1.ConfigurationGroup {
	return newTestConfigurationGroup(
		"redis-config-group",
		openchoreov1alpha1.ConfigurationGroupSpec{
			Configurations: []openchoreov1alpha1.ConfigurationGroupConfiguration{
				{
					Key: "host",
					Values: []openchoreov1alpha1.ConfigurationValue{
						{
							Environment: "test-environment",
							Value:       "redis-dev.test.com",
						},
						{
							Environment: "production",
							Value:       "redis.test.com",
						},
					},
				},
				{
					Key: "port",
					Values: []openchoreov1alpha1.ConfigurationValue{
						{
							Environment: "test-environment",
							Value:       "6379",
						},
						{
							Environment: "production",
							Value:       "6380",
						},
					},
				},
				{
					Key: "password",
					Values: []openchoreov1alpha1.ConfigurationValue{
						{
							Environment: "test-environment",
							VaultKey:    "secret/test/redis/password",
						},
						{
							Environment: "production",
							VaultKey:    "secret/prod/redis/password",
						},
					},
				},
			},
		})
}

func newTestMysqlConfigurationGroup() *openchoreov1alpha1.ConfigurationGroup {
	return newTestConfigurationGroup(
		"mysql-config-group",
		openchoreov1alpha1.ConfigurationGroupSpec{
			Configurations: []openchoreov1alpha1.ConfigurationGroupConfiguration{
				{
					Key: "host",
					Values: []openchoreov1alpha1.ConfigurationValue{
						{
							Environment: "test-environment",
							Value:       "mysql-dev.test.com",
						},
						{
							Environment: "production",
							Value:       "mysql.test.com",
						},
					},
				},
				{
					Key: "port",
					Values: []openchoreov1alpha1.ConfigurationValue{
						{
							Environment: "test-environment",
							Value:       "3306",
						},
						{
							Environment: "production",
							Value:       "3306",
						},
					},
				},
				{
					Key: "password",
					Values: []openchoreov1alpha1.ConfigurationValue{
						{
							Environment: "test-environment",
							VaultKey:    "secret/test/mysql/password",
						},
						{
							Environment: "production",
							VaultKey:    "secret/prod/mysql/password",
						},
					},
				},
			},
		})
}
