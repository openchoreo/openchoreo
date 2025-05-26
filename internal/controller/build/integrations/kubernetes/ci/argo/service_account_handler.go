// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package argo

import (
	"context"
	"errors"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/controller/build/integrations"
	"github.com/openchoreo/openchoreo/internal/controller/build/integrations/kubernetes"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

type serviceAccountHandler struct {
	kubernetesClient client.Client
}

var _ dataplane.ResourceHandler[integrations.BuildContext] = (*serviceAccountHandler)(nil)

func NewServiceAccountHandler(kubernetesClient client.Client) dataplane.ResourceHandler[integrations.BuildContext] {
	return &serviceAccountHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *serviceAccountHandler) Name() string {
	return "ArgoWorkflowServiceAccount"
}

func (h *serviceAccountHandler) GetCurrentState(ctx context.Context, builtCtx *integrations.BuildContext) (interface{}, error) {
	name := makeServiceAccountName()
	sa := corev1.ServiceAccount{}
	err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name, Namespace: kubernetes.MakeNamespaceName(builtCtx)}, &sa)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return sa, nil
}

func (h *serviceAccountHandler) Create(ctx context.Context, builtCtx *integrations.BuildContext) error {
	sa := makeServiceAccount(builtCtx)
	return h.kubernetesClient.Create(ctx, sa)
}

func (h *serviceAccountHandler) Update(ctx context.Context, builtCtx *integrations.BuildContext, currentState interface{}) error {
	currentSA, ok := currentState.(*corev1.ServiceAccount)
	if !ok {
		return errors.New("failed to cast current state to ServiceAccount")
	}
	newSA := makeServiceAccount(builtCtx)

	if h.shouldUpdate(currentSA, newSA) {
		newSA.ResourceVersion = currentSA.ResourceVersion
		return h.kubernetesClient.Update(ctx, newSA)
	}

	return nil
}

func (h *serviceAccountHandler) Delete(ctx context.Context, builtCtx *integrations.BuildContext) error {
	return nil
}

func (h *serviceAccountHandler) IsRequired(builtCtx *integrations.BuildContext) bool {
	return true
}

func makeServiceAccountName() string {
	return "workflow-sa"
}

func makeServiceAccount(builtCtx *integrations.BuildContext) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeServiceAccountName(),
			Namespace: kubernetes.MakeNamespaceName(builtCtx),
			Labels:    kubernetes.MakeLabels(builtCtx),
		},
	}
}

func (h *serviceAccountHandler) shouldUpdate(current, new *corev1.ServiceAccount) bool {
	return !cmp.Equal(kubernetes.ExtractManagedLabels(current.Labels), kubernetes.ExtractManagedLabels(new.Labels))
}
