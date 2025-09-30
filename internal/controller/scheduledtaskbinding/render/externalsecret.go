// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	esov1 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/externalsecrets/v1"
)

// ExternalSecrets generates ExternalSecret resources for image pull secrets
func ExternalSecrets(rCtx Context) []*openchoreov1alpha1.Resource {
	var resources []*openchoreov1alpha1.Resource

	// Skip if no DataPlane or no secret store configured
	if rCtx.DataPlane == nil || rCtx.DataPlane.Spec.SecretStoreRef == nil {
		return resources
	}

	// Skip if no image pull secrets configured
	if len(rCtx.DataPlane.Spec.ImagePullSecretRefs) == 0 {
		return resources
	}

	namespace := makeNamespaceName(rCtx)

	for _, secretRefName := range rCtx.DataPlane.Spec.ImagePullSecretRefs {
		// Get the SecretReference from context
		secretRef, exists := rCtx.ImagePullSecretReferences[secretRefName]
		if !exists {
			rCtx.AddError(fmt.Errorf("image pull SecretReference %q not found", secretRefName))
			continue
		}

		externalSecret := makeExternalSecret(rCtx, secretRef, namespace)
		if externalSecret != nil {
			rawExt := &runtime.RawExtension{}
			rawExt.Object = externalSecret

			resource := &openchoreov1alpha1.Resource{
				ID:     makeExternalSecretResourceID(rCtx, secretRef.Name),
				Object: rawExt,
			}
			resources = append(resources, resource)
		}
	}

	return resources
}

func makeExternalSecret(rCtx Context, secretRef *openchoreov1alpha1.SecretReference,
	namespace string) *esov1.ExternalSecret {
	secretName := makeImagePullSecretName(rCtx, secretRef.Name)

	// Use refresh interval from SecretReference if specified, otherwise let ESO use its default
	var refreshInterval *metav1.Duration
	if secretRef.Spec.RefreshInterval != nil {
		refreshInterval = &metav1.Duration{
			Duration: secretRef.Spec.RefreshInterval.Duration,
		}
	}

	externalSecret := &esov1.ExternalSecret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "external-secrets.io/v1",
			Kind:       "ExternalSecret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels:    makeScheduledTaskLabels(rCtx),
		},
		Spec: esov1.ExternalSecretSpec{
			SecretStoreRef: esov1.SecretStoreRef{
				Name: rCtx.DataPlane.Spec.SecretStoreRef.Name,
				Kind: "ClusterSecretStore", // Always use ClusterSecretStore
			},
			Target: esov1.ExternalSecretTarget{
				Name: secretName,
				Template: &esov1.ExternalSecretTemplate{
					Type: secretRef.Spec.Template.Type,
					Metadata: esov1.ExternalSecretTemplateMetadata{
						Labels:      secretRef.Spec.Template.Metadata.Labels,
						Annotations: secretRef.Spec.Template.Metadata.Annotations,
					},
				},
				CreationPolicy: esov1.CreatePolicyOwner,
				DeletionPolicy: esov1.DeletionPolicyDelete,
			},
			RefreshInterval: refreshInterval,
		},
	}

	// Map data sources from SecretReference to ExternalSecret
	for _, dataSource := range secretRef.Spec.Data {
		externalSecretData := esov1.ExternalSecretData{
			SecretKey: dataSource.SecretKey,
			RemoteRef: esov1.ExternalSecretDataRemoteRef{
				Key:      dataSource.RemoteRef.Key,
				Property: dataSource.RemoteRef.Property,
				Version:  dataSource.RemoteRef.Version,
			},
		}
		externalSecret.Spec.Data = append(externalSecret.Spec.Data, externalSecretData)
	}

	return externalSecret
}

func makeExternalSecretResourceID(rCtx Context, secretRefName string) string {
	return fmt.Sprintf("%s-externalsecret-%s", rCtx.ScheduledTaskBinding.Name, secretRefName)
}

// makeImagePullSecretName generates a K8s-compliant name for image pull secrets
// Includes ScheduledTaskBinding name to prevent collisions when multiple components
// in the same namespace reference the same SecretReference
func makeImagePullSecretName(rCtx Context, secretRefName string) string {
	return dpkubernetes.GenerateK8sName(rCtx.ScheduledTaskBinding.Name, secretRefName)
}
