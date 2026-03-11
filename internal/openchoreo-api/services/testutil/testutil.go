// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"fmt"
	"io"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// NewScheme returns a runtime.Scheme with the OpenChoreo and core Kubernetes API types registered.
func NewScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("failed to register core v1 scheme: %v", err))
	}
	if err := openchoreov1alpha1.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("failed to register openchoreo v1alpha1 scheme: %v", err))
	}
	return scheme
}

// NewFakeClient creates an in-memory Kubernetes client pre-loaded with the given objects.
func NewFakeClient(objects ...client.Object) client.Client {
	builder := fake.NewClientBuilder().
		WithScheme(NewScheme()).
		WithStatusSubresource(statusSubresourceObjects()...)

	if len(objects) > 0 {
		builder = builder.WithObjects(objects...)
	}

	return builder.Build()
}

func statusSubresourceObjects() []client.Object {
	return []client.Object{
		&corev1.Namespace{},
		&openchoreov1alpha1.WorkflowPlane{},
		&openchoreov1alpha1.ClusterWorkflowPlane{},
		&openchoreov1alpha1.ClusterComponentType{},
		&openchoreov1alpha1.ClusterDataPlane{},
		&openchoreov1alpha1.ClusterObservabilityPlane{},
		&openchoreov1alpha1.ClusterTrait{},
		&openchoreov1alpha1.ClusterWorkflow{},
		&openchoreov1alpha1.Component{},
		&openchoreov1alpha1.ComponentRelease{},
		&openchoreov1alpha1.ComponentType{},
		&openchoreov1alpha1.DataPlane{},
		&openchoreov1alpha1.DeploymentPipeline{},
		&openchoreov1alpha1.Environment{},
		&openchoreov1alpha1.ObservabilityAlertsNotificationChannel{},
		&openchoreov1alpha1.ObservabilityPlane{},
		&openchoreov1alpha1.Project{},
		&openchoreov1alpha1.ReleaseBinding{},
		&openchoreov1alpha1.SecretReference{},
		&openchoreov1alpha1.Trait{},
		&openchoreov1alpha1.Workflow{},
		&openchoreov1alpha1.WorkflowRun{},
		&openchoreov1alpha1.Workload{},
	}
}

// TestLogger returns a logger that suppresses normal test output noise.
func TestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// NewProject creates a Project test fixture.
func NewProject(namespace, name string) *openchoreov1alpha1.Project {
	return &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"},
		},
	}
}

// NewNamespace creates a Namespace test fixture.
func NewNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

// NewComponent creates a Component test fixture.
func NewComponent(namespace, projectName, name string) *openchoreov1alpha1.Component {
	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{
				ProjectName: projectName,
			},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{
				Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType,
				Name: "deployment/web-app",
			},
		},
	}
}

// NewWorkflowPlane creates a WorkflowPlane test fixture.
func NewWorkflowPlane(namespace, name string) *openchoreov1alpha1.WorkflowPlane {
	return &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: testClusterAgentConfig(),
		},
	}
}

// NewClusterWorkflowPlane creates a ClusterWorkflowPlane test fixture.
func NewClusterWorkflowPlane(name string) *openchoreov1alpha1.ClusterWorkflowPlane {
	return &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
			PlaneID:      name,
			ClusterAgent: testClusterAgentConfig(),
		},
	}
}

// NewClusterComponentType creates a ClusterComponentType test fixture.
func NewClusterComponentType(name string) *openchoreov1alpha1.ClusterComponentType {
	return &openchoreov1alpha1.ClusterComponentType{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: defaultClusterComponentTypeSpec(),
	}
}

// NewClusterDataPlane creates a ClusterDataPlane test fixture.
func NewClusterDataPlane(name string) *openchoreov1alpha1.ClusterDataPlane {
	return &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlaneID:      name,
			ClusterAgent: testClusterAgentConfig(),
		},
	}
}

// NewClusterObservabilityPlane creates a ClusterObservabilityPlane test fixture.
func NewClusterObservabilityPlane(name string) *openchoreov1alpha1.ClusterObservabilityPlane {
	return &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
			PlaneID:      name,
			ClusterAgent: testClusterAgentConfig(),
			ObserverURL:  "https://observer.test",
		},
	}
}

// NewClusterTrait creates a ClusterTrait test fixture.
func NewClusterTrait(name string) *openchoreov1alpha1.ClusterTrait {
	return &openchoreov1alpha1.ClusterTrait{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

// NewClusterWorkflow creates a ClusterWorkflow test fixture.
func NewClusterWorkflow(name string) *openchoreov1alpha1.ClusterWorkflow {
	return &openchoreov1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: openchoreov1alpha1.ClusterWorkflowSpec{
			RunTemplate: testRunTemplate(),
		},
	}
}

// NewEnvironment creates an Environment test fixture.
func NewEnvironment(namespace, name string) *openchoreov1alpha1.Environment {
	return &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// NewDataPlane creates a DataPlane test fixture.
func NewDataPlane(namespace, name string) *openchoreov1alpha1.DataPlane {
	return &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			ClusterAgent: testClusterAgentConfig(),
		},
	}
}

// NewComponentRelease creates a ComponentRelease test fixture.
func NewComponentRelease(namespace, projectName, componentName, name string) *openchoreov1alpha1.ComponentRelease {
	return &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   projectName,
				ComponentName: componentName,
			},
			ComponentType: defaultComponentTypeSpec(),
			Workload:      defaultWorkloadTemplateSpec(),
		},
	}
}

// NewComponentType creates a ComponentType test fixture.
func NewComponentType(namespace, name string) *openchoreov1alpha1.ComponentType {
	return &openchoreov1alpha1.ComponentType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: defaultComponentTypeSpec(),
	}
}

// NewDeploymentPipeline creates a DeploymentPipeline test fixture.
func NewDeploymentPipeline(namespace, name string) *openchoreov1alpha1.DeploymentPipeline {
	return &openchoreov1alpha1.DeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{
				{
					SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: "dev"},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
						{Name: "prod"},
					},
				},
			},
		},
	}
}

// NewObservabilityAlertsNotificationChannel creates an ObservabilityAlertsNotificationChannel fixture.
func NewObservabilityAlertsNotificationChannel(namespace, environmentName, name string) *openchoreov1alpha1.ObservabilityAlertsNotificationChannel {
	return &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.ObservabilityAlertsNotificationChannelSpec{
			Environment: environmentName,
			Type:        openchoreov1alpha1.NotificationChannelTypeEmail,
			EmailConfig: &openchoreov1alpha1.EmailConfig{
				From: "alerts@example.com",
				To:   []string{"platform@example.com"},
				SMTP: openchoreov1alpha1.SMTPConfig{
					Host: "smtp.example.com",
					Port: 587,
					Auth: &openchoreov1alpha1.SMTPAuth{
						Username: &openchoreov1alpha1.SecretValueFrom{
							SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{
								Name: "smtp-auth",
								Key:  "username",
							},
						},
						Password: &openchoreov1alpha1.SecretValueFrom{
							SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{
								Name: "smtp-auth",
								Key:  "password",
							},
						},
					},
					TLS: &openchoreov1alpha1.SMTPTLSConfig{},
				},
				Template: &openchoreov1alpha1.EmailTemplate{
					Subject: "Test Alert",
					Body:    "Test alert body",
				},
			},
		},
	}
}

// NewObservabilityPlane creates an ObservabilityPlane test fixture.
func NewObservabilityPlane(namespace, name string) *openchoreov1alpha1.ObservabilityPlane {
	return &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.ObservabilityPlaneSpec{
			ClusterAgent: testClusterAgentConfig(),
			ObserverURL:  "https://observer.test",
		},
	}
}

// NewReleaseBinding creates a ReleaseBinding test fixture.
func NewReleaseBinding(namespace, projectName, componentName, environmentName, name string) *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner: openchoreov1alpha1.ReleaseBindingOwner{
				ProjectName:   projectName,
				ComponentName: componentName,
			},
			Environment: environmentName,
		},
	}
}

// NewSecretReference creates a SecretReference test fixture.
func NewSecretReference(namespace, name string) *openchoreov1alpha1.SecretReference {
	return &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			Template: openchoreov1alpha1.SecretTemplate{
				Type: corev1.SecretTypeOpaque,
			},
			Data: []openchoreov1alpha1.SecretDataSource{
				{
					SecretKey: "token",
					RemoteRef: openchoreov1alpha1.RemoteReference{
						Key:      "secret/test",
						Property: "token",
					},
				},
			},
		},
	}
}

// NewTrait creates a Trait test fixture.
func NewTrait(namespace, name string) *openchoreov1alpha1.Trait {
	return &openchoreov1alpha1.Trait{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// NewWorkflow creates a Workflow test fixture.
func NewWorkflow(namespace, name string) *openchoreov1alpha1.Workflow {
	return &openchoreov1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.WorkflowSpec{
			RunTemplate: testRunTemplate(),
		},
	}
}

// NewWorkflowRun creates a WorkflowRun test fixture.
func NewWorkflowRun(namespace, workflowName, name string) *openchoreov1alpha1.WorkflowRun {
	return &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.WorkflowRunSpec{
			Workflow: openchoreov1alpha1.WorkflowRunConfig{
				Kind: openchoreov1alpha1.WorkflowRefKindWorkflow,
				Name: workflowName,
			},
		},
	}
}

// NewWorkload creates a Workload test fixture.
func NewWorkload(namespace, projectName, componentName, name string) *openchoreov1alpha1.Workload {
	return &openchoreov1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   projectName,
				ComponentName: componentName,
			},
			WorkloadTemplateSpec: defaultWorkloadTemplateSpec(),
		},
	}
}

func testClusterAgentConfig() openchoreov1alpha1.ClusterAgentConfig {
	return openchoreov1alpha1.ClusterAgentConfig{
		ClientCA: openchoreov1alpha1.ValueFrom{
			Value: "test-ca",
		},
	}
}

func testResourceTemplate(resourceID string) openchoreov1alpha1.ResourceTemplate {
	return openchoreov1alpha1.ResourceTemplate{
		ID:       resourceID,
		Template: testRunTemplate(),
	}
}

func defaultComponentTypeSpec() openchoreov1alpha1.ComponentTypeSpec {
	return openchoreov1alpha1.ComponentTypeSpec{
		WorkloadType: "deployment",
		Resources: []openchoreov1alpha1.ResourceTemplate{
			testResourceTemplate("deployment"),
		},
	}
}

func defaultClusterComponentTypeSpec() openchoreov1alpha1.ClusterComponentTypeSpec {
	return openchoreov1alpha1.ClusterComponentTypeSpec{
		WorkloadType: "deployment",
		Resources: []openchoreov1alpha1.ResourceTemplate{
			testResourceTemplate("deployment"),
		},
	}
}

func defaultWorkloadTemplateSpec() openchoreov1alpha1.WorkloadTemplateSpec {
	return openchoreov1alpha1.WorkloadTemplateSpec{
		Container: openchoreov1alpha1.Container{
			Image: "ghcr.io/openchoreo/test:latest",
		},
	}
}

func testRunTemplate() *runtime.RawExtension {
	return &runtime.RawExtension{
		Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"template"}}`),
	}
}
