// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package argo

import (
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller/build/names"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
)

const (
	WorkflowServiceAccountName = "workflow-sa"
	WorkflowRoleName           = "workflow-role"
	WorkflowRoleBindingName    = "workflow-role-binding"
)

// makeArgoWorkflow creates an Argo Workflow from a Build resource
func (e *Engine) makeArgoWorkflow(build *openchoreov1alpha1.Build) *argoproj.Workflow {
	workflow := &argoproj.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.MakeWorkflowName(build),
			Namespace: names.MakeNamespaceName(build),
			Labels:    names.MakeWorkflowLabels(build),
		},
		Spec: e.makeWorkflowSpec(build),
	}
	return workflow
}

// makeWorkflowSpec creates the workflow specification from a Build resource
func (e *Engine) makeWorkflowSpec(build *openchoreov1alpha1.Build) argoproj.WorkflowSpec {
	parameters := e.buildWorkflowParameters(build)

	return argoproj.WorkflowSpec{
		PodMetadata: &argoproj.Metadata{
			Labels: names.MakeWorkflowLabels(build),
		},
		ServiceAccountName: WorkflowServiceAccountName,
		WorkflowTemplateRef: &argoproj.WorkflowTemplateRef{
			Name:         build.Spec.TemplateRef.Name,
			ClusterScope: true,
		},
		Arguments: argoproj.Arguments{
			Parameters: parameters,
		},
	}
}

// buildWorkflowParameters constructs the parameters for the workflow
func (e *Engine) buildWorkflowParameters(build *openchoreov1alpha1.Build) []argoproj.Parameter {
	parameters := []argoproj.Parameter{
		e.createParameter("project-name", build.Spec.Owner.ProjectName),
		e.createParameter("component-name", build.Spec.Owner.ComponentName),
		e.createParameter("git-repo", build.Spec.Repository.URL),
		e.createParameter("app-path", build.Spec.Repository.AppPath),
		e.createParameter("image-name", names.MakeImageName(build)),
		e.createParameter("image-tag", names.MakeImageTag(build)),
	}

	commit := build.Spec.Repository.Revision.Commit
	branch := build.Spec.Repository.Revision.Branch

	if commit != "" {
		branch = "" // ignore branch when commit is provided
	} else {
		if branch == "" {
			branch = "main"
		}
		commit = "" // ensure commit is empty when using branch
	}

	parameters = append(
		parameters,
		e.createParameter("commit", commit),
		e.createParameter("branch", branch),
	)

	for _, param := range build.Spec.TemplateRef.Parameters {
		parameters = append(parameters, e.createParameter(param.Name, param.Value))
	}

	return parameters
}

// createParameter creates a workflow parameter with proper type conversion
func (e *Engine) createParameter(name, value string) argoproj.Parameter {
	paramValue := argoproj.AnyString(value)
	return argoproj.Parameter{
		Name:  name,
		Value: &paramValue,
	}
}

// makeServiceAccount creates a service account for the workflow
func (e *Engine) makeServiceAccount(build *openchoreov1alpha1.Build) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      WorkflowServiceAccountName,
			Namespace: names.MakeNamespaceName(build),
			Labels:    names.MakeWorkflowLabels(build),
		},
	}
}

// makeRole creates a role for the workflow
func (e *Engine) makeRole(build *openchoreov1alpha1.Build) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      WorkflowRoleName,
			Namespace: names.MakeNamespaceName(build),
			Labels:    names.MakeWorkflowLabels(build),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"argoproj.io"},
				Resources: []string{"workflowtaskresults"},
				Verbs:     []string{"create", "get", "list", "watch", "update", "patch"},
			},
		},
	}
}

// makeRoleBinding creates a role binding for the workflow
func (e *Engine) makeRoleBinding(build *openchoreov1alpha1.Build) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      WorkflowRoleBindingName,
			Namespace: names.MakeNamespaceName(build),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      WorkflowServiceAccountName,
				Namespace: names.MakeNamespaceName(build),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     WorkflowRoleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
}
