/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package kubernetes

import (
	"context"
	"errors"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

type serviceHandler struct {
	kubernetesClient client.Client
}

var _ dataplane.ResourceHandler[dataplane.DeploymentContext] = (*serviceHandler)(nil)

func NewServiceHandler(kubernetesClient client.Client) dataplane.ResourceHandler[dataplane.DeploymentContext] {
	return &serviceHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *serviceHandler) Name() string {
	return "KubernetesServiceHandler"
}

func (h *serviceHandler) IsRequired(deployCtx *dataplane.DeploymentContext) bool {
	// Services are required for Web Applications
	return deployCtx.Component.Spec.Type == choreov1.ComponentTypeWebApplication ||
		deployCtx.Component.Spec.Type == choreov1.ComponentTypeService
}

func (h *serviceHandler) GetCurrentState(ctx context.Context, deployCtx *dataplane.DeploymentContext) (interface{}, error) {
	namespace := makeNamespaceName(deployCtx)
	name := makeServiceName(deployCtx)
	out := &corev1.Service{}
	err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, out)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return out, nil
}

func (h *serviceHandler) Create(ctx context.Context, deployCtx *dataplane.DeploymentContext) error {
	service := makeService(deployCtx)
	return h.kubernetesClient.Create(ctx, service)
}

func (h *serviceHandler) Update(ctx context.Context, deployCtx *dataplane.DeploymentContext, currentState interface{}) error {
	currentService, ok := currentState.(*corev1.Service)
	if !ok {
		return errors.New("failed to cast current state to Service")
	}

	newService := makeService(deployCtx)

	if h.shouldUpdate(currentService, newService) {
		newService.ResourceVersion = currentService.ResourceVersion
		// Preserve the cluster IP which is immutable
		newService.Spec.ClusterIP = currentService.Spec.ClusterIP
		return h.kubernetesClient.Update(ctx, newService)
	}

	return nil
}

func (h *serviceHandler) Delete(ctx context.Context, deployCtx *dataplane.DeploymentContext) error {
	service := makeService(deployCtx)
	err := h.kubernetesClient.Delete(ctx, service)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func makeServiceName(deployCtx *dataplane.DeploymentContext) string {
	componentName := deployCtx.Component.Name
	deploymentTrackName := deployCtx.DeploymentTrack.Name
	// Limit the name to 63 characters to comply with the K8s name length limit for Services
	return dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxServiceNameLength, componentName, deploymentTrackName)
}

func makeService(deployCtx *dataplane.DeploymentContext) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeServiceName(deployCtx),
			Namespace: makeNamespaceName(deployCtx),
			Labels:    makeWorkloadLabels(deployCtx),
		},
		Spec: makeServiceSpec(deployCtx),
	}
}

func makeServiceSpec(deployCtx *dataplane.DeploymentContext) corev1.ServiceSpec {
	return corev1.ServiceSpec{
		Selector: makeWorkloadLabels(deployCtx),
		Ports:    makeServicePortsFromEndpointTemplates(deployCtx.DeployableArtifact.Spec.Configuration.EndpointTemplates),
		Type:     corev1.ServiceTypeClusterIP,
	}
}

func (h *serviceHandler) shouldUpdate(current, new *corev1.Service) bool {
	// Compare the labels
	if !cmp.Equal(extractManagedLabels(current.Labels), extractManagedLabels(new.Labels)) {
		return true
	}

	// Compare spec excluding the ClusterIP field which is immutable
	currentSpec := current.Spec.DeepCopy()
	currentSpec.ClusterIP = ""
	newSpec := new.Spec.DeepCopy()
	newSpec.ClusterIP = ""

	return !cmp.Equal(currentSpec, newSpec, cmpopts.EquateEmpty())
}
