// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

const (
	fileContentConfigMapKey = "content"
)

func makePodSpec(deployCtx *dataplane.DeploymentContext) *corev1.PodSpec {
	ps := &corev1.PodSpec{}
	ps.RestartPolicy = getRestartPolicy(deployCtx)

	// Create the main container
	mainContainer := makeMainContainer(deployCtx)

	// Add file volumes and mounts
	fileVolumes, fileMounts := makeFileVolumes(deployCtx)
	mainContainer.VolumeMounts = append(mainContainer.VolumeMounts, fileMounts...)
	ps.Volumes = append(ps.Volumes, fileVolumes...)

	// Add the secret volumes and mounts for the secret storage CSI driver
	secretCSIVolumes, secretCSIMounts := makeSecretCSIVolumes(deployCtx)
	mainContainer.VolumeMounts = append(mainContainer.VolumeMounts, secretCSIMounts...)
	ps.Volumes = append(ps.Volumes, secretCSIVolumes...)

	ps.Containers = []corev1.Container{*mainContainer}

	return ps
}

func makeMainContainer(deployCtx *dataplane.DeploymentContext) *corev1.Container {
	c := &corev1.Container{
		Name:  "main",
		Image: deployCtx.ContainerImage,
	}

	c.Env = makeEnvironmentVariables(deployCtx)

	artifactConfig := deployCtx.DeployableArtifact.Spec.Configuration
	if artifactConfig != nil {
		c.Ports = makeContainerPortsFromEndpointTemplates(artifactConfig.EndpointTemplates)
	}

	return c
}

func makeEnvironmentVariables(deployCtx *dataplane.DeploymentContext) []corev1.EnvVar {
	if deployCtx.DeployableArtifact.Spec.Configuration == nil ||
		deployCtx.DeployableArtifact.Spec.Configuration.Application == nil {
		return nil
	}

	var k8sEnvVars []corev1.EnvVar

	// Build the container environment variables from the direct values.
	// Example Direct values:
	// env:
	//   - key: REDIS_HOST
	//	   value: redis.example.com
	envVars := deployCtx.DeployableArtifact.Spec.Configuration.Application.Env
	for _, envVar := range envVars {
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

	// Build the container environment variables from the configuration groups.
	for _, cg := range deployCtx.ConfigurationGroups {
		mappedCfg := newMappedEnvVarConfig(deployCtx, cg)

		// Add plain configuration values to the environment variables
		configMapName := makeConfigMapName(deployCtx, cg)
		for _, pc := range mappedCfg.PlainConfigs {
			k8sEnvVars = append(k8sEnvVars, corev1.EnvVar{
				Name: pc.EnvVarKey,
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: configMapName,
						},
						Key: pc.ConfigGroupKey,
					},
				},
			})
		}

		// Add secret configuration values to the environment variables
		secretName := makeSecretProviderClassName(deployCtx, cg)
		for _, sc := range mappedCfg.SecretConfigs {
			k8sEnvVars = append(k8sEnvVars, corev1.EnvVar{
				Name: sc.EnvVarKey,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secretName,
						},
						Key: sc.ConfigGroupKey,
					},
				},
			})
		}
	}

	return k8sEnvVars
}

func makeFileVolumes(deployCtx *dataplane.DeploymentContext) ([]corev1.Volume, []corev1.VolumeMount) {
	volumes := make([]corev1.Volume, 0)
	mounts := make([]corev1.VolumeMount, 0)

	if deployCtx.DeployableArtifact.Spec.Configuration == nil ||
		deployCtx.DeployableArtifact.Spec.Configuration.Application == nil {
		return volumes, mounts
	}

	// Build the volumes and mounts from the direct values.
	// Example file mounts with direct values:
	// fileMounts:
	//   - mountPath: /etc/config/test.properties
	//     value: |
	//        key1=value1
	//        key2=value2
	fileMounts := deployCtx.DeployableArtifact.Spec.Configuration.Application.FileMounts
	for _, fileMount := range fileMounts {
		if fileMount.MountPath == "" {
			continue
		}
		if fileMount.Value != "" {
			volumeName := makeDirectFileMountVolumeName(&fileMount)
			mounts = append(mounts, corev1.VolumeMount{
				Name:      volumeName,
				MountPath: fileMount.MountPath,
				SubPath:   fileContentConfigMapKey,
			})
			volumes = append(volumes, corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: makeDirectFileMountConfigMapName(deployCtx, &fileMount),
						},
					},
				},
			})
		}
	}

	// Build the container file mounts from the configuration groups.
	for _, cg := range deployCtx.ConfigurationGroups {
		mappedCfg := newMappedFileMountConfig(deployCtx, cg)

		if len(mappedCfg.PlainConfigs) > 0 {
			// Add plain configuration values to the file mounts
			cgName := controller.GetName(cg)
			configMapName := makeConfigMapName(deployCtx, cg)
			volumeName := dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxVolumeNameLength, cgName, "cm")

			volumes = append(volumes, corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: configMapName,
						},
					},
				},
			})

			for _, pc := range mappedCfg.PlainConfigs {
				mounts = append(mounts, corev1.VolumeMount{
					Name:      volumeName,
					MountPath: pc.MountPath,
					SubPath:   pc.ConfigGroupKey,
				})
			}
		}

		// Add secret configuration values to the file mounts
		if len(mappedCfg.SecretConfigs) > 0 {
			cgName := controller.GetName(cg)
			secretName := makeSecretProviderClassName(deployCtx, cg)
			volumeName := dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxVolumeNameLength, cgName, "csi")

			volumes = append(volumes, corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					CSI: &corev1.CSIVolumeSource{
						Driver:   "secrets-store.csi.k8s.io",
						ReadOnly: ptr.To(true),
						VolumeAttributes: map[string]string{
							"secretProviderClass": secretName,
						},
					},
				},
			})

			for _, sc := range mappedCfg.SecretConfigs {
				mounts = append(mounts, corev1.VolumeMount{
					Name:      volumeName,
					MountPath: sc.MountPath,
					SubPath:   sc.ConfigGroupKey,
				})
			}
		}
	}
	return volumes, mounts
}

// makeSecretCSIVolumes creates the secret volumes and mounts for the secret storage CSI driver.
func makeSecretCSIVolumes(deployCtx *dataplane.DeploymentContext) ([]corev1.Volume, []corev1.VolumeMount) {
	volumes := make([]corev1.Volume, 0)
	mounts := make([]corev1.VolumeMount, 0)

	for _, cg := range deployCtx.ConfigurationGroups {
		mappedCfg := newMappedEnvVarConfig(deployCtx, cg)
		// If there are no secrets in the mapped configuration group, skip creating the secret volumes and mounts
		if len(mappedCfg.SecretConfigs) == 0 {
			continue
		}

		cgName := controller.GetName(cg)
		secretName := makeSecretProviderClassName(deployCtx, cg)
		volumeName := dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxVolumeNameLength, cgName, "csi-env")

		volumes = append(volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				CSI: &corev1.CSIVolumeSource{
					Driver:   "secrets-store.csi.k8s.io",
					ReadOnly: ptr.To(true),
					VolumeAttributes: map[string]string{
						"secretProviderClass": secretName,
					},
				},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: fmt.Sprintf("/mnt/secrets-store/%s", cgName),
		})
	}

	return volumes, mounts
}

func getRestartPolicy(deployCtx *dataplane.DeploymentContext) corev1.RestartPolicy {
	if deployCtx.Component.Spec.Type == openchoreov1alpha1.ComponentTypeScheduledTask ||
		deployCtx.Component.Spec.Type == openchoreov1alpha1.ComponentTypeManualTask {
		return corev1.RestartPolicyNever
	}
	return corev1.RestartPolicyAlways
}

// makeDirectFileMountVolumeName generates a unique name for the file mount volume for a given FileMount spec
// The name will be in the format: filemount-<hash-of-the-mount-path>
func makeDirectFileMountVolumeName(fileMount *openchoreov1alpha1.FileMount) string {
	hashLength := 8
	hashBytes := sha256.Sum256([]byte(fileMount.MountPath))
	hash := hex.EncodeToString(hashBytes[:])[:hashLength]
	return fmt.Sprintf("filemount-%s", hash)
}
