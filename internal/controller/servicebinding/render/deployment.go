// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// Deployment creates a complete Deployment resource for the new Resources array
func Deployment(rCtx Context) *openchoreov1alpha1.Resource {
	base := rCtx.ServiceClass.Spec.DeploymentTemplate

	overlay := makeServiceDeploymentSpec(rCtx)
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
			Labels:    makeServiceLabels(rCtx),
		},
		Spec: *mergedSpec,
	}

	rawExt := &runtime.RawExtension{}
	rawExt.Object = deployment

	return &openchoreov1alpha1.Resource{
		ID:     makeDeploymentResourceID(rCtx),
		Object: rawExt,
	}
}

func makeServiceDeploymentSpec(rCtx Context) appsv1.DeploymentSpec {
	ds := appsv1.DeploymentSpec{}
	ds.Selector = &metav1.LabelSelector{
		MatchLabels: makeServiceLabels(rCtx),
	}
	ds.Template.Labels = makeServiceLabels(rCtx)
	ds.Template.Spec = *makeServicePodSpec(rCtx)
	return ds
}

func makeDeploymentName(rCtx Context) string {
	return dpkubernetes.GenerateK8sName(rCtx.ServiceBinding.Name)
}

func makeNamespaceName(rCtx Context) string {
	organizationName := rCtx.ServiceBinding.Namespace // Namespace is the organization name
	projectName := rCtx.ServiceBinding.Spec.Owner.ProjectName
	environmentName := rCtx.ServiceBinding.Spec.Environment
	// Limit the name to 63 characters to comply with the K8s name length limit for Namespaces
	return dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxNamespaceNameLength,
		"dp", organizationName, projectName, environmentName)
}

// TODO: Find a better way to generate resource IDs
func makeDeploymentResourceID(rCtx Context) string {
	return rCtx.ServiceBinding.Name + "-deployment"
}
