// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// Service creates a complete Service resource for the new Resources array
func Service(rCtx Context) *openchoreov1alpha1.Resource {
	base := rCtx.ServiceClass.Spec.ServiceTemplate

	overlay := makeServiceServiceSpec(rCtx)
	mergedSpec, err := merge(&base, &overlay)
	if err != nil {
		rCtx.AddError(MergeError(err))
		return nil
	}

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeServiceName(rCtx),
			Namespace: makeNamespaceName(rCtx),
			Labels:    makeServiceLabels(rCtx),
		},
		Spec: *mergedSpec,
	}

	rawExt := &runtime.RawExtension{}
	rawExt.Object = service

	return &openchoreov1alpha1.Resource{
		ID:     makeServiceResourceID(rCtx),
		Object: rawExt,
	}
}

// The ServiceServiceSpec is not a typo
func makeServiceServiceSpec(rCtx Context) corev1.ServiceSpec {
	ports := makeServicePortsFromEndpoints(rCtx.ServiceBinding.Spec.WorkloadSpec.Endpoints)
	return corev1.ServiceSpec{
		Selector: makeServiceLabels(rCtx),
		Ports:    ports,
		Type:     corev1.ServiceTypeClusterIP,
	}
}

func makeServiceName(rCtx Context) string {
	return dpkubernetes.GenerateK8sName(rCtx.ServiceBinding.Name)
}

func makeServiceResourceID(rCtx Context) string {
	return rCtx.ServiceBinding.Name + "-service"
}
