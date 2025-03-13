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
	corev1 "k8s.io/api/core/v1"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/dataplane"
)

func makePodSpec(deployCtx *dataplane.DeploymentContext) *corev1.PodSpec {
	ps := &corev1.PodSpec{}
	ps.Containers = []corev1.Container{*makeMainContainer(deployCtx)}
	ps.RestartPolicy = getRestartPolicy(deployCtx)
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

	// Build the container environment variables from the direct values and configuration groups mapping.
	// Example Direct values:
	// env:
	//   - key: REDIS_HOST
	//	   value: redis.example.com
	// Example Configuration group mapping:
	// env:
	//   - key: REDIS_HOST
	//	   valueFrom:
	//	     configurationGroupRef:
	//		   name: redis-config
	//		   key: redis-host
	envVars := deployCtx.DeployableArtifact.Spec.Configuration.Application.Env
	for _, envVar := range envVars {
		if envVar.Key == "" {
			continue
		}
		// Direct values set in the deployable artifact configuration
		if envVar.Value != "" {
			k8sEnvVars = append(k8sEnvVars, corev1.EnvVar{
				Name:  envVar.Key,
				Value: envVar.Value,
			})
			continue
		}
		// Values set from a configuration group
		if envVar.ValueFrom != nil && envVar.ValueFrom.ConfigurationGroupRef != nil {
			cgRef := envVar.ValueFrom.ConfigurationGroupRef
			targetCg := findConfigGroupByName(deployCtx.ConfigurationGroups, cgRef.Name)
			if targetCg == nil {
				continue // Ignore if the configuration group is not found.
			}
			configMapName := makeConfigMapName(deployCtx, targetCg)
			k8sEnvVars = append(k8sEnvVars, corev1.EnvVar{
				Name: envVar.Key,
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: configMapName,
						},
						Key: cgRef.Key,
					},
				},
			})
		}
	}

	// Build the container environment variables from the bulk configuration group injection.
	// Example Configuration group injection:
	// envFrom:
	//   - configurationGroupRef:
	//       name: redis-config
	envFromSources := deployCtx.DeployableArtifact.Spec.Configuration.Application.EnvFrom
	for _, envFrom := range envFromSources {
		if envFrom.ConfigurationGroupRef == nil {
			continue
		}
		cgRef := envFrom.ConfigurationGroupRef
		targetCg := findConfigGroupByName(deployCtx.ConfigurationGroups, cgRef.Name)
		if targetCg == nil {
			continue // Ignore if the configuration group is not found.
		}
		configMapName := makeConfigMapName(deployCtx, targetCg)

		for _, cfConfig := range targetCg.Spec.Configurations {
			envKey := sanitizeEnvVarKey(cfConfig.Key)
			if envKey == "" {
				continue
			}
			k8sEnvVars = append(k8sEnvVars, corev1.EnvVar{
				Name: envKey,
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: configMapName,
						},
						Key: cfConfig.Key,
					},
				},
			})
		}
	}

	return k8sEnvVars
}

func getRestartPolicy(deployCtx *dataplane.DeploymentContext) corev1.RestartPolicy {
	if deployCtx.Component.Spec.Type == choreov1.ComponentTypeScheduledTask ||
		deployCtx.Component.Spec.Type == choreov1.ComponentTypeManualTask {
		return corev1.RestartPolicyNever
	}
	return corev1.RestartPolicyAlways
}
