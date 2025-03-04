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

package v1

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/labels"
)

// nolint:unused
// log is for logging in this package.
var projectlog = logf.Log.WithName("project-resource")

// SetupProjectWebhookWithManager registers the webhook for Project in the manager.
func SetupProjectWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&corev1.Project{}).
		WithValidator(&ProjectCustomValidator{client: mgr.GetClient()}).
		WithDefaulter(&ProjectCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-core-choreo-dev-v1-project,mutating=true,failurePolicy=fail,sideEffects=None,groups=core.choreo.dev,resources=projects,verbs=create;update,versions=v1,name=mproject-v1.kb.io,admissionReviewVersions=v1

// ProjectCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Project when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type ProjectCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &ProjectCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Project.
func (d *ProjectCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	project, ok := obj.(*corev1.Project)

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
// +kubebuilder:webhook:path=/validate-core-choreo-dev-v1-project,mutating=false,failurePolicy=fail,sideEffects=None,groups=core.choreo.dev,resources=projects,verbs=create;update,versions=v1,name=vproject-v1.kb.io,admissionReviewVersions=v1

// ProjectCustomValidator struct is responsible for validating the Project resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ProjectCustomValidator struct {
	client client.Client
}

var _ webhook.CustomValidator = &ProjectCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Project.
func (v *ProjectCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	project, ok := obj.(*corev1.Project)
	if !ok {
		return nil, fmt.Errorf("expected a Project object but got %T", obj)
	}

	if err := v.validateProjectCommon(ctx, project); err != nil {
		return nil, err
	}

	// Check whether project already exists using the lable name
	if err := v.ensureNoDuplicateProjectInOrganization(ctx, project); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Project.
func (v *ProjectCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	project, ok := newObj.(*corev1.Project)
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
func (v *ProjectCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	project, ok := obj.(*corev1.Project)
	if !ok {
		return nil, fmt.Errorf("expected a Project object but got %T", obj)
	}
	projectlog.Info("Validation for Project upon deletion", "name", project.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}

func (v *ProjectCustomValidator) validateProjectCommon(ctx context.Context, project *corev1.Project) error {
	// First validate the required labels for the Project resource.
	if err := validateProjectLabels(project); err != nil {
		return err
	}

	// Validate whether the project's namespace matches with the namespace created for the organization.
	// First get the organization object from the labelKeyOrganizationName label.
	orgName := project.Labels[labels.LabelKeyOrganizationName]
	org, err := v.findOrganizationByNameLabel(ctx, orgName)
	if err != nil {
		return err
	}

	// Then check whether the organization's namespace matches with the project's namespace.
	if org.Status.Namespace != project.Namespace {
		return fmt.Errorf("project namespace '%s' does not match with the namespace '%s' of the organization '%s'",
			project.Namespace, org.Status.Namespace, orgName)
	}

	// Check whether the deploymentPipelineRef: <name> exists in the namespace
	if err := v.ensureDeploymentPipelineExists(ctx, project.Spec.DeploymentPipelineRef, project); err != nil {
		return err
	}

	return nil
}

// validateProjectLabels validates the required labels for the Project resource.
func validateProjectLabels(project *corev1.Project) error {
	requiredLabels := []string{
		labels.LabelKeyOrganizationName,
		labels.LabelKeyName,
	}

	var missingLabels []string
	for _, label := range requiredLabels {
		if _, exists := project.Labels[label]; !exists {
			missingLabels = append(missingLabels, label)
		}
	}

	if len(missingLabels) > 0 {
		return fmt.Errorf("required labels missing for the project '%s': %s", project.Name, strings.Join(missingLabels, ", "))
	}

	return nil
}

// ensureDeploymentPipelineExists checks whether the deployment pipeline specified in the project exists in the namespace.
func (v *ProjectCustomValidator) ensureDeploymentPipelineExists(ctx context.Context, pipelineName string, project *corev1.Project) error {
	pipelineList := &corev1.DeploymentPipelineList{}

	// Define label selector
	listOpts := []client.ListOption{
		client.InNamespace(project.Namespace),
		client.MatchingLabels{
			labels.LabelKeyName: pipelineName,
		},
	}

	// Get the deployment pipeline object from the namespace
	if err := v.client.List(ctx, pipelineList, listOpts...); err != nil {
		return fmt.Errorf("failed to get deployment pipeline '%s' specified in project '%s': %w", pipelineName, project.Labels[labels.LabelKeyName], err)
	}

	// Check whether the deployment pipeline exists
	if len(pipelineList.Items) == 0 {
		return fmt.Errorf("deployment pipeline '%s' specified in project '%s' not found", pipelineName, project.Labels[labels.LabelKeyName])
	}

	return nil
}

func (v *ProjectCustomValidator) ensureNoDuplicateProjectInOrganization(ctx context.Context, project *corev1.Project) error {
	// Create a list to hold the projects
	projectList := &corev1.ProjectList{}

	// Define label selector
	listOpts := []client.ListOption{
		client.InNamespace(project.Namespace),
		client.MatchingLabels{
			labels.LabelKeyName:             project.Labels[labels.LabelKeyName],
			labels.LabelKeyOrganizationName: project.Labels[labels.LabelKeyOrganizationName],
		},
	}

	// List all projects with the specified label
	if err := v.client.List(ctx, projectList, listOpts...); err != nil {
		return fmt.Errorf("failed to get project '%s' specified in label '%s': %w", project.Labels[labels.LabelKeyName], labels.LabelKeyName, err)
	}

	// Check whether the project exists
	if len(projectList.Items) > 0 {
		return fmt.Errorf("project '%s' specified in label '%s' already exists in organization '%s'", project.Labels[labels.LabelKeyName], labels.LabelKeyName, project.Labels[labels.LabelKeyOrganizationName])
	}

	return nil
}

func (v *ProjectCustomValidator) findOrganizationByNameLabel(ctx context.Context, orgName string) (*corev1.Organization, error) {
	// Create a list to hold the organizations
	orgList := &corev1.OrganizationList{}

	// Define label selector
	listOpts := []client.ListOption{
		client.MatchingLabels{
			labels.LabelKeyName: orgName,
		},
	}

	// List all organizations with the specified label
	if err := v.client.List(ctx, orgList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to get organization '%s' specified in label '%s': %w", orgName, labels.LabelKeyOrganizationName, err)
	}

	// Check whether the organization exists
	if len(orgList.Items) == 0 {
		return nil, fmt.Errorf("organization '%s' specified in label '%s' not found", orgName, labels.LabelKeyOrganizationName)
	}

	// Check whether multiple organizations found
	if len(orgList.Items) > 1 {
		// This should not happen as the organization name is unique and we validate it during the creation
		return nil, fmt.Errorf("multiple organizations found with name '%s', specified in label '%s'", orgName, labels.LabelKeyOrganizationName)
	}

	return &orgList.Items[0], nil
}
