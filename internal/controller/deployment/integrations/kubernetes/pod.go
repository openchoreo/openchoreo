/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package kubernetes

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
	"github.com/choreo-idp/choreo/internal/dataplane"
	dpkubernetes "github.com/choreo-idp/choreo/internal/dataplane/kubernetes"
	"github.com/choreo-idp/choreo/internal/ptr"
)

func makePodSpec(deployCtx *dataplane.DeploymentContext) *corev1.PodSpec {
	ps := &corev1.PodSpec{}
	ps.Containers = []corev1.Container{*makeMainContainer(deployCtx)}
	ps.RestartPolicy = getRestartPolicy(deployCtx)

	// Add the secret volumes for the secret storage CSI driver
	secretCSIVolumes, _ := makeSecretCSIVolumes(deployCtx)
	ps.Volumes = append(ps.Volumes, secretCSIVolumes...)
	return ps
}

func makeMainContainer(deployCtx *dataplane.DeploymentContext) *corev1.Container {
	c := &corev1.Container{
		Name:  "main",
		Image: deployCtx.ContainerImage,
	}

	c.Env = makeEnvironmentVariables(deployCtx)

	// Add the secret volumes mounts for the secret storage CSI driver
	_, secretCSIMounts := makeSecretCSIVolumes(deployCtx)
	c.VolumeMounts = append(c.VolumeMounts, secretCSIMounts...)

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
		mappedCfg := newMappedConfig(deployCtx, cg)

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

// makeSecretCSIVolumes creates the secret volumes and mounts for the secret storage CSI driver.
func makeSecretCSIVolumes(deployCtx *dataplane.DeploymentContext) ([]corev1.Volume, []corev1.VolumeMount) {
	volumes := make([]corev1.Volume, 0)
	mounts := make([]corev1.VolumeMount, 0)

	for _, cg := range deployCtx.ConfigurationGroups {
		mappedCfg := newMappedConfig(deployCtx, cg)
		// If there are no secrets in the mapped configuration group, skip creating the secret volumes and mounts
		if len(mappedCfg.SecretConfigs) == 0 {
			continue
		}

		cgName := controller.GetName(cg)
		secretName := makeSecretProviderClassName(deployCtx, cg)
		volumeName := dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxVolumeNameLength, cgName)

		volumes = append(volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				CSI: &corev1.CSIVolumeSource{
					Driver:   "secrets-store.csi.k8s.io",
					ReadOnly: ptr.Bool(true),
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
	if deployCtx.Component.Spec.Type == choreov1.ComponentTypeScheduledTask ||
		deployCtx.Component.Spec.Type == choreov1.ComponentTypeManualTask {
		return corev1.RestartPolicyNever
	}
	return corev1.RestartPolicyAlways
}
