// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"errors"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

type deploymentHandler struct {
	kubernetesClient client.Client
}

var _ dataplane.ResourceHandler[dataplane.DeploymentContext] = (*deploymentHandler)(nil)

func NewDeploymentHandler(kubernetesClient client.Client) dataplane.ResourceHandler[dataplane.DeploymentContext] {
	return &deploymentHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *deploymentHandler) Name() string {
	return "KubernetesDeploymentHandler"
}

// IsRequired indicates whether the external resource needs to be configured or not based on the deployment context.
// If this returns false, the controller will attempt to delete the resource.
func (h *deploymentHandler) IsRequired(deployCtx *dataplane.DeploymentContext) bool {
	// Kubernetes Deployments are required for Web Applications and Services
	return deployCtx.Component.Spec.Type == openchoreov1alpha1.ComponentTypeWebApplication ||
		deployCtx.Component.Spec.Type == openchoreov1alpha1.ComponentTypeService
}

// GetCurrentState returns the current state of the external resource.
// If the resource does not exist, the implementation should return nil.
func (h *deploymentHandler) GetCurrentState(ctx context.Context, deployCtx *dataplane.DeploymentContext) (interface{}, error) {
	namespace := makeNamespaceName(deployCtx)
	name := makeDeploymentName(deployCtx)
	out := &appsv1.Deployment{}
	err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, out)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return out, nil
}

// Create creates the external resource.
func (h *deploymentHandler) Create(ctx context.Context, deployCtx *dataplane.DeploymentContext) error {
	deployment := makeDeployment(deployCtx)
	return h.kubernetesClient.Create(ctx, deployment)
}

// Update updates the external resource.
// The currentState parameter will provide the current state of the resource that is returned by GetCurrentState
// Implementation should compare the current state with the new derived state and update the resource accordingly.
func (h *deploymentHandler) Update(ctx context.Context, deployCtx *dataplane.DeploymentContext, currentState interface{}) error {
	currentDeployment, ok := currentState.(*appsv1.Deployment)
	if !ok {
		return errors.New("failed to cast current state to CronJob")
	}
	newDeployment := makeDeployment(deployCtx)

	if h.shouldUpdate(currentDeployment, newDeployment) {
		newDeployment.ResourceVersion = currentDeployment.ResourceVersion
		return h.kubernetesClient.Update(ctx, newDeployment)
	}

	return nil
}

// Delete deletes the external resource.
// The implementation should handle the case where the resource does not exist and return nil.
func (h *deploymentHandler) Delete(ctx context.Context, deployCtx *dataplane.DeploymentContext) error {
	deployment := makeDeployment(deployCtx)
	err := h.kubernetesClient.Delete(ctx, deployment)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (h *deploymentHandler) shouldUpdate(current, new *appsv1.Deployment) bool {
	// Compare the labels
	if !cmp.Equal(extractManagedLabels(current.Labels), extractManagedLabels(new.Labels)) {
		return true
	}

	if !cmp.Equal(current.Spec, new.Spec, cmpopts.EquateEmpty()) {
		return true
	}
	return false
}

func makeDeploymentName(deployCtx *dataplane.DeploymentContext) string {
	componentName := deployCtx.Component.Name
	deploymentTrackName := deployCtx.DeploymentTrack.Name
	// Limit the name to 253 characters to comply with the K8s name length limit for Deployments
	return dpkubernetes.GenerateK8sName(componentName, deploymentTrackName)
}

func makeDeployment(deployCtx *dataplane.DeploymentContext) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeDeploymentName(deployCtx),
			Namespace: makeNamespaceName(deployCtx),
			Labels:    makeWorkloadLabels(deployCtx),
		},
		Spec: makeDeploymentSpec(deployCtx),
	}
}

func makeDeploymentSpec(deployCtx *dataplane.DeploymentContext) appsv1.DeploymentSpec {
	deploymentSpec := appsv1.DeploymentSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: makeWorkloadLabels(deployCtx),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: makeWorkloadLabels(deployCtx),
			},
			Spec: *makePodSpec(deployCtx),
		},
	}

	return deploymentSpec
}
