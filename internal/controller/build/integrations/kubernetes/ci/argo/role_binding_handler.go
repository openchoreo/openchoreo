// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package argo

import (
	"context"
	"errors"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/controller/build/integrations"
	"github.com/openchoreo/openchoreo/internal/controller/build/integrations/kubernetes"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

type roleBindingHandler struct {
	kubernetesClient client.Client
}

var _ dataplane.ResourceHandler[integrations.BuildContext] = (*roleBindingHandler)(nil)

func NewRoleBindingHandler(kubernetesClient client.Client) dataplane.ResourceHandler[integrations.BuildContext] {
	return &roleBindingHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *roleBindingHandler) Name() string {
	return "ArgoWorkflowRoleBinding"
}

func (h *roleBindingHandler) GetCurrentState(ctx context.Context, builtCtx *integrations.BuildContext) (interface{}, error) {
	name := makeRoleBindingName()
	role := rbacv1.Role{}
	err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name, Namespace: kubernetes.MakeNamespaceName(builtCtx)}, &role)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return role, nil
}

func (h *roleBindingHandler) Create(ctx context.Context, builtCtx *integrations.BuildContext) error {
	roleBinding := makeRoleBinding(builtCtx)
	return h.kubernetesClient.Create(ctx, roleBinding)
}

func (h *roleBindingHandler) Update(ctx context.Context, builtCtx *integrations.BuildContext, currentState interface{}) error {
	currentRoleBinding, ok := currentState.(*rbacv1.RoleBinding)
	if !ok {
		return errors.New("failed to cast current state to Role Binding")
	}
	newRoleBinding := makeRoleBinding(builtCtx)

	if h.shouldUpdate(currentRoleBinding, newRoleBinding) {
		newRoleBinding.ResourceVersion = currentRoleBinding.ResourceVersion
		return h.kubernetesClient.Update(ctx, newRoleBinding)
	}

	return nil
}

func (h *roleBindingHandler) Delete(ctx context.Context, builtCtx *integrations.BuildContext) error {
	return nil
}

func (h *roleBindingHandler) IsRequired(builtCtx *integrations.BuildContext) bool {
	return true
}

func (h *roleBindingHandler) shouldUpdate(current, new *rbacv1.RoleBinding) bool {
	// Compare the labels
	if !cmp.Equal(kubernetes.ExtractManagedLabels(current.Labels), kubernetes.ExtractManagedLabels(new.Labels)) {
		return true
	}
	if !cmp.Equal(current.Subjects, new.Subjects, cmpopts.EquateEmpty()) {
		return true
	}

	if !cmp.Equal(current.RoleRef, new.RoleRef, cmpopts.EquateEmpty()) {
		return true
	}

	return false
}

func makeRoleBindingName() string {
	return "workflow-role-binding"
}

func makeRoleBinding(builtCtx *integrations.BuildContext) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeRoleBindingName(),
			Namespace: kubernetes.MakeNamespaceName(builtCtx),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      makeServiceAccountName(),
				Namespace: kubernetes.MakeNamespaceName(builtCtx),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     makeRoleName(),
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
}
