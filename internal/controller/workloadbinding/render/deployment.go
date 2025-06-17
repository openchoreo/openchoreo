// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// Deployment creates a complete Deployment resource for the new Resources array
func Deployment(rCtx *Context) *choreov1.Resource {
	var base appsv1.DeploymentSpec
	wlType := rCtx.WorkloadBinding.Spec.WorkloadSpec.Type
	switch wlType {
	case choreov1.WorkloadTypeService:
		base = rCtx.WorkloadClass.Spec.ServiceWorkload.DeploymentTemplate
	case choreov1.WorkloadTypeWebApplication:
		base = rCtx.WorkloadClass.Spec.WebApplicationWorkload.DeploymentTemplate
	default:
		rCtx.AddError(UnsupportedWorkloadTypeError(wlType))
		return nil
	}

	overlay := makeWorkloadDeploymentSpec(rCtx)
	mergedSpec, err := merge(&base, &overlay)
	if err != nil {
		rCtx.AddError(MergeError(err))
		return nil
	}

	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeDeploymentName(rCtx),
			Namespace: makeNamespaceName(rCtx),
			Labels:    makeWorkloadLabels(rCtx),
		},
		Spec: *mergedSpec,
	}

	rawExt := &runtime.RawExtension{}
	rawExt.Object = deployment

	return &choreov1.Resource{
		ID:     makeDeploymentResourceId(rCtx),
		Object: rawExt,
	}
}

func makeWorkloadDeploymentSpec(rCtx *Context) appsv1.DeploymentSpec {
	ds := appsv1.DeploymentSpec{}
	ds.Selector = &metav1.LabelSelector{
		MatchLabels: makeWorkloadLabels(rCtx),
	}
	ds.Template.Labels = makeWorkloadLabels(rCtx)
	ds.Template.Spec = *makeWorkloadPodSpec(rCtx)
	return ds
}

func makeDeploymentName(rCtx *Context) string {
	return dpkubernetes.GenerateK8sName(rCtx.WorkloadBinding.Name)
}

func makeNamespaceName(rCtx *Context) string {
	organizationName := rCtx.WorkloadBinding.Namespace // Namespace is the organization name
	projectName := rCtx.WorkloadBinding.Spec.WorkloadSpec.Owner.ProjectName
	environmentName := rCtx.WorkloadBinding.Spec.EnvironmentName
	// Limit the name to 63 characters to comply with the K8s name length limit for Namespaces
	return dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxNamespaceNameLength,
		"dp", organizationName, projectName, environmentName)
}

// TODO: Find a better way to generate resource IDs
func makeDeploymentResourceId(rCtx *Context) string {
	return rCtx.WorkloadBinding.Name + "-deployment"
}
