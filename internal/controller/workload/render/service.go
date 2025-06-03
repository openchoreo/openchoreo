// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// Service creates a complete Service resource for the new Resources array
func Service(rCtx *Context) *choreov1.Resource {
	var base corev1.ServiceSpec
	wlType := rCtx.Workload.Spec.Type
	switch wlType {
	case choreov1.WorkloadTypeService:
		base = rCtx.WorkloadClass.Spec.ServiceWorkload.ServiceTemplate
	case choreov1.WorkloadTypeWebApplication:
		base = rCtx.WorkloadClass.Spec.WebApplicationWorkload.ServiceTemplate
	default:
		rCtx.AddError(UnsupportedWorkloadTypeError(wlType))
		return nil
	}

	overlay := makeWorkloadServiceSpec(rCtx)
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
			Labels:    makeWorkloadLabels(rCtx),
		},
		Spec: *mergedSpec,
	}

	rawExt := &runtime.RawExtension{}
	rawExt.Object = service

	return &choreov1.Resource{
		ID:     makeServiceResourceId(rCtx),
		Object: rawExt,
	}
}

func makeWorkloadServiceSpec(rCtx *Context) corev1.ServiceSpec {
	ports := makeServicePortsFromEndpoints(rCtx.Endpoints)
	return corev1.ServiceSpec{
		Selector: makeWorkloadLabels(rCtx),
		Ports:    ports,
		Type:     corev1.ServiceTypeClusterIP,
	}
}

func makeServiceName(rCtx *Context) string {
	return dpkubernetes.GenerateK8sName(rCtx.Workload.Name)
}

func makeServiceResourceId(rCtx *Context) string {
	return rCtx.Workload.Name + "-service"
}
