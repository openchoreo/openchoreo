// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	workflowRoleNameSuffix        = "role"
	workflowRoleBindingNameSuffix = "role-binding"
)

// ensurePrerequisites creates prerequisite resources in the build plane
// before creating the workflow run: create namespace, service account, role, and role binding.
func (r *Reconciler) ensurePrerequisites(ctx context.Context, namespace, serviceAccountName string, bpClient client.Client) error {
	logger := log.FromContext(ctx).WithValues("namespace", namespace, "serviceAccount", serviceAccountName)

	roleName := fmt.Sprintf("%s-%s", serviceAccountName, workflowRoleNameSuffix)
	roleBindingName := fmt.Sprintf("%s-%s", serviceAccountName, workflowRoleBindingNameSuffix)

	resources := []struct {
		obj  client.Object
		name string
	}{
		{makeNamespace(namespace), "Namespace"},
		{makeServiceAccount(namespace, serviceAccountName), "ServiceAccount"},
		{makeRole(namespace, roleName), "Role"},
		{makeRoleBinding(namespace, serviceAccountName, roleName, roleBindingName), "RoleBinding"},
	}

	for _, res := range resources {
		if err := ensureResource(ctx, bpClient, res.obj, res.name, logger); err != nil {
			return fmt.Errorf("failed to ensure %s: %w", res.name, err)
		}
	}

	return nil
}

func ensureResource(ctx context.Context, client client.Client, obj client.Object, resourceType string, logger logr.Logger) error {
	err := client.Create(ctx, obj)
	if err == nil {
		return nil
	}

	if apierrors.IsAlreadyExists(err) {
		return nil
	}

	logger.Error(err, "Failed to create resource", "type", resourceType, "name", obj.GetName(), "namespace", obj.GetNamespace())
	return err
}

func makeNamespace(namespace string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
}

func makeServiceAccount(namespace, serviceAccountName string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: namespace,
		},
	}
}

func makeRole(namespace, roleName string) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
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

func makeRoleBinding(namespace, serviceAccountName, roleName, roleBindingName string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccountName,
				Namespace: namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     roleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
}
