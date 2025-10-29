// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	corev1 "k8s.io/api/core/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func makeScheduledTaskPodSpec(rCtx Context) *corev1.PodSpec {
	ps := &corev1.PodSpec{}

	// Create the main container
	mainContainer := makeMainContainer(rCtx)

	// Add file volumes and mounts
	// fileVolumes, fileMounts := makeFileVolumes(deployCtx)
	// mainContainer.VolumeMounts = append(mainContainer.VolumeMounts, fileMounts...)
	// ps.Volumes = append(ps.Volumes, fileVolumes...)

	// Add the secret volumes and mounts for the secret storage CSI driver
	// secretCSIVolumes, secretCSIMounts := makeSecretCSIVolumes(deployCtx)
	// mainContainer.VolumeMounts = append(mainContainer.VolumeMounts, secretCSIMounts...)
	// ps.Volumes = append(ps.Volumes, secretCSIVolumes...)

	ps.Containers = []corev1.Container{*mainContainer}

	// Add imagePullSecrets from DataPlane configuration
	ps.ImagePullSecrets = makeImagePullSecrets(rCtx)

	// Scheduled tasks should not restart on failure - they should be retried by CronJob
	ps.RestartPolicy = corev1.RestartPolicyOnFailure

	return ps
}

func makeMainContainer(rCtx Context) *corev1.Container {
	wls := rCtx.ScheduledTaskBinding.Spec.WorkloadSpec

	// Use the first container as the main container
	// TODO: Fix me later to support multiple containers
	var mainContainerSpec openchoreov1alpha1.Container
	var containerName string
	for name, container := range wls.Containers {
		mainContainerSpec = container
		containerName = name
		break
	}

	c := &corev1.Container{
		Name:    containerName,
		Image:   mainContainerSpec.Image,
		Command: mainContainerSpec.Command,
		Args:    mainContainerSpec.Args,
	}

	c.Env = makeEnvironmentVariables(rCtx)

	// Scheduled tasks typically don't expose ports, but we'll include them if defined
	// No container ports needed for scheduled tasks typically

	return c
}

func makeEnvironmentVariables(rCtx Context) []corev1.EnvVar {
	var k8sEnvVars []corev1.EnvVar

	// Get environment variables from the first container
	wls := rCtx.ScheduledTaskBinding.Spec.WorkloadSpec
	for _, container := range wls.Containers {
		// Build the container environment variables from the container's env values
		for _, envVar := range container.Env {
			if envVar.Key == "" {
				continue
			}
			if envVar.Value != "" {
				k8sEnvVars = append(k8sEnvVars, corev1.EnvVar{
					Name:  envVar.Key,
					Value: envVar.Value,
				})
			}
		}
		break // Use only the first container's env vars as this is for the main container
	}

	return k8sEnvVars
}

// makeImagePullSecrets creates imagePullSecret references for the pod spec
func makeImagePullSecrets(rCtx Context) []corev1.LocalObjectReference {
	if rCtx.DataPlane == nil || len(rCtx.DataPlane.Spec.ImagePullSecretRefs) == 0 {
		return nil
	}

	imagePullSecrets := make([]corev1.LocalObjectReference, 0, len(rCtx.DataPlane.Spec.ImagePullSecretRefs))

	// Add a reference for each secret that will be created by ExternalSecret
	for _, secretRefName := range rCtx.DataPlane.Spec.ImagePullSecretRefs {
		secretRef, exists := rCtx.ImagePullSecretReferences[secretRefName]
		if !exists {
			// Error already added in ExternalSecrets function
			continue
		}

		secretName := makeImagePullSecretName(rCtx, secretRef.Name)
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{
			Name: secretName,
		})
	}

	return imagePullSecrets
}
