// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var projectlog = logf.Log.WithName("project-resource")

// SetupProjectWebhookWithManager registers the webhook for Project in the manager.
func SetupProjectWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&openchoreov1alpha1.Project{}).
		WithValidator(&Validator{client: mgr.GetClient()}).
		WithDefaulter(&Defaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-openchoreo-dev-v1alpha1-project,mutating=true,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=projects,verbs=create;update,versions=v1alpha1,name=mproject-v1alpha1.kb.io,admissionReviewVersions=v1

// Defaulter struct is responsible for setting default values on the custom resource of the
// Kind Project when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type Defaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &Defaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Project.
func (d *Defaulter) Default(ctx context.Context, obj runtime.Object) error {
	project, ok := obj.(*openchoreov1alpha1.Project)
	if !ok {
		return fmt.Errorf("expected an Project object but got %T", obj)
	}
	projectlog.Info("Defaulting for Project", "name", project.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-project,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=projects,verbs=create;update,versions=v1alpha1,name=vproject-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator struct is responsible for validating the Project resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type Validator struct {
	client client.Client
}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Project.
func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	project, ok := obj.(*openchoreov1alpha1.Project)
	if !ok {
		return nil, fmt.Errorf("expected a Project object but got %T", obj)
	}

	if err := v.validateProjectCommon(ctx, project); err != nil {
		return nil, err
	}
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Project.
func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	project, ok := newObj.(*openchoreov1alpha1.Project)
	if !ok {
		return nil, fmt.Errorf("expected a Project object for the newObj but got %T", newObj)
	}
	projectlog.Info("Validation for Project upon update", "name", project.GetName())
	if err := v.validateProjectCommon(ctx, project); err != nil {
		return nil, err
	}
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Project.
func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	project, ok := obj.(*openchoreov1alpha1.Project)
	if !ok {
		return nil, fmt.Errorf("expected a Project object but got %T", obj)
	}
	projectlog.Info("Validation for Project upon deletion", "name", project.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}

func (v *Validator) validateProjectCommon(ctx context.Context, project *openchoreov1alpha1.Project) error {
	if err := v.ensureDeploymentPipelineExists(ctx, project.Spec.DeploymentPipelineRef, project); err != nil {
		return err
	}
	return nil
}

// ensureDeploymentPipelineExists checks whether the deployment pipeline specified in the project exists in the namespace.
func (v *Validator) ensureDeploymentPipelineExists(ctx context.Context, pipelineName string, project *openchoreov1alpha1.Project) error {
	pipeline := &openchoreov1alpha1.DeploymentPipeline{}
	pipelineKey := client.ObjectKey{
		Name:      pipelineName,
		Namespace: project.Namespace,
	}
	if err := v.client.Get(ctx, pipelineKey, pipeline); err != nil {
		return fmt.Errorf("deployment pipeline '%s' specified in project '%s' not found: %w", pipelineName, project.Name, err)
	}
	return nil
}
