// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"fmt"
	"sort"
	"text/template"

	corev1 "k8s.io/api/core/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func makeServicePodSpec(rCtx Context) *corev1.PodSpec {
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

	return ps
}

func makeMainContainer(rCtx Context) *corev1.Container {
	wls := rCtx.ServiceBinding.Spec.WorkloadSpec

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

	// Add container ports from endpoints
	c.Ports = makeContainerPortsFromEndpoints(rCtx.ServiceBinding.Spec.WorkloadSpec.Endpoints)

	return c
}

func makeEnvironmentVariables(rCtx Context) []corev1.EnvVar {
	var k8sEnvVars []corev1.EnvVar

	// Get environment variables from the first container
	wls := rCtx.ServiceBinding.Spec.WorkloadSpec
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

	// Process connection environment variables
	connectionEnvVars := makeConnectionEnvironmentVariables(rCtx)
	k8sEnvVars = append(k8sEnvVars, connectionEnvVars...)

	return k8sEnvVars
}

func makeConnectionEnvironmentVariables(rCtx Context) []corev1.EnvVar {
	var k8sEnvVars []corev1.EnvVar

	wls := rCtx.ServiceBinding.Spec.WorkloadSpec
	
	// Get connection names and sort them for deterministic ordering
	var connectionNames []string
	for name := range wls.Connections {
		connectionNames = append(connectionNames, name)
	}
	sort.Strings(connectionNames)
	
	// Process connections in sorted order
	for _, connectionName := range connectionNames {
		connection := wls.Connections[connectionName]
		
		if connection.Type != openchoreov1alpha1.ConnectionTypeAPI {
			continue // TODO: Only handle API connections for POC
		}

		// Get pre-resolved endpoint access from controller
		resolvedEndpoint, exists := rCtx.ResolvedConnections[connectionName]
		if !exists {
			rCtx.AddError(fmt.Errorf("no resolved endpoint for connection %s", connectionName))
			continue
		}

		// Convert interface{} to *EndpointAccess
		endpointAccess, ok := resolvedEndpoint.(*openchoreov1alpha1.EndpointAccess)
		if !ok {
			rCtx.AddError(fmt.Errorf("invalid resolved endpoint type for connection %s", connectionName))
			continue
		}

		// Process each environment variable injection
		for _, envVar := range connection.Inject.Env {
			resolvedValue := processEndpointTemplate(envVar.Value, endpointAccess)
			k8sEnvVars = append(k8sEnvVars, corev1.EnvVar{
				Name:  envVar.Name,
				Value: resolvedValue,
			})
		}
	}

	return k8sEnvVars
}

func processEndpointTemplate(templateStr string, endpoint *openchoreov1alpha1.EndpointAccess) string {
	// Create a map with lowercase field names for user convenience
	data := map[string]string{
		"host":     endpoint.Host,
		"port":     fmt.Sprintf("%d", endpoint.Port),
		"scheme":   endpoint.Scheme,
		"basePath": endpoint.BasePath,
		"uri":      endpoint.URI,
		"url":      endpoint.URI, // Common alias for uri
	}
	
	// Parse and execute the template
	tmpl, err := template.New("connection").Parse(templateStr)
	if err != nil {
		// If template parsing fails, return the original string
		return templateStr
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		// If template execution fails, return the original string
		return templateStr
	}
	
	return buf.String()
}

//
// func makeFileVolumes(deployCtx *dataplane.DeploymentContext) ([]corev1.Volume, []corev1.VolumeMount) {
//	volumes := make([]corev1.Volume, 0)
//	mounts := make([]corev1.VolumeMount, 0)
//
//	if deployCtx.DeployableArtifact.Spec.Configuration == nil ||
//		deployCtx.DeployableArtifact.Spec.Configuration.Application == nil {
//		return volumes, mounts
//	}
//
//	// Build the volumes and mounts from the direct values.
//	// Example file mounts with direct values:
//	// fileMounts:
//	//   - mountPath: /etc/config/test.properties
//	//     value: |
//	//        key1=value1
//	//        key2=value2
//	fileMounts := deployCtx.DeployableArtifact.Spec.Configuration.Application.FileMounts
//	for _, fileMount := range fileMounts {
//		if fileMount.MountPath == "" {
//			continue
//		}
//		if fileMount.Value != "" {
//			volumeName := makeDirectFileMountVolumeName(&fileMount)
//			mounts = append(mounts, corev1.VolumeMount{
//				Name:      volumeName,
//				MountPath: fileMount.MountPath,
//				SubPath:   fileContentConfigMapKey,
//			})
//			volumes = append(volumes, corev1.Volume{
//				Name: volumeName,
//				VolumeSource: corev1.VolumeSource{
//					ConfigMap: &corev1.ConfigMapVolumeSource{
//						LocalObjectReference: corev1.LocalObjectReference{
//							Name: makeDirectFileMountConfigMapName(deployCtx, &fileMount),
//						},
//					},
//				},
//			})
//		}
//	}
//
//	// Build the container file mounts from the configuration groups.
//	for _, cg := range deployCtx.ConfigurationGroups {
//		mappedCfg := newMappedFileMountConfig(deployCtx, cg)
//
//		if len(mappedCfg.PlainConfigs) > 0 {
//			// Add plain configuration values to the file mounts
//			cgName := controller.GetName(cg)
//			configMapName := makeConfigMapName(deployCtx, cg)
//			volumeName := dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxVolumeNameLength, cgName, "cm")
//
//			volumes = append(volumes, corev1.Volume{
//				Name: volumeName,
//				VolumeSource: corev1.VolumeSource{
//					ConfigMap: &corev1.ConfigMapVolumeSource{
//						LocalObjectReference: corev1.LocalObjectReference{
//							Name: configMapName,
//						},
//					},
//				},
//			})
//
//			for _, pc := range mappedCfg.PlainConfigs {
//				mounts = append(mounts, corev1.VolumeMount{
//					Name:      volumeName,
//					MountPath: pc.MountPath,
//					SubPath:   pc.ConfigGroupKey,
//				})
//			}
//		}
//
//		// Add secret configuration values to the file mounts
//		if len(mappedCfg.SecretConfigs) > 0 {
//			cgName := controller.GetName(cg)
//			secretName := makeSecretProviderClassName(deployCtx, cg)
//			volumeName := dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxVolumeNameLength, cgName, "csi")
//
//			volumes = append(volumes, corev1.Volume{
//				Name: volumeName,
//				VolumeSource: corev1.VolumeSource{
//					CSI: &corev1.CSIVolumeSource{
//						Driver:   "secrets-store.csi.k8s.io",
//						ReadOnly: ptr.Bool(true),
//						VolumeAttributes: map[string]string{
//							"secretProviderClass": secretName,
//						},
//					},
//				},
//			})
//
//			for _, sc := range mappedCfg.SecretConfigs {
//				mounts = append(mounts, corev1.VolumeMount{
//					Name:      volumeName,
//					MountPath: sc.MountPath,
//					SubPath:   sc.ConfigGroupKey,
//				})
//			}
//		}
//	}
//	return volumes, mounts
// }
//
// // makeSecretCSIVolumes creates the secret volumes and mounts for the secret storage CSI driver.
// func makeSecretCSIVolumes(deployCtx *dataplane.DeploymentContext) ([]corev1.Volume, []corev1.VolumeMount) {
//	volumes := make([]corev1.Volume, 0)
//	mounts := make([]corev1.VolumeMount, 0)
//
//	for _, cg := range deployCtx.ConfigurationGroups {
//		mappedCfg := newMappedEnvVarConfig(deployCtx, cg)
//		// If there are no secrets in the mapped configuration group, skip creating the secret volumes and mounts
//		if len(mappedCfg.SecretConfigs) == 0 {
//			continue
//		}
//
//		cgName := controller.GetName(cg)
//		secretName := makeSecretProviderClassName(deployCtx, cg)
//		volumeName := dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxVolumeNameLength, cgName, "csi-env")
//
//		volumes = append(volumes, corev1.Volume{
//			Name: volumeName,
//			VolumeSource: corev1.VolumeSource{
//				CSI: &corev1.CSIVolumeSource{
//					Driver:   "secrets-store.csi.k8s.io",
//					ReadOnly: ptr.Bool(true),
//					VolumeAttributes: map[string]string{
//						"secretProviderClass": secretName,
//					},
//				},
//			},
//		})
//		mounts = append(mounts, corev1.VolumeMount{
//			Name:      volumeName,
//			MountPath: fmt.Sprintf("/mnt/secrets-store/%s", cgName),
//		})
//	}
//
//	return volumes, mounts
// }
//
// func getRestartPolicy(deployCtx *dataplane.DeploymentContext) corev1.RestartPolicy {
//	if deployCtx.Component.Spec.Type == openchoreov1alpha1.ComponentTypeScheduledTask ||
//		deployCtx.Component.Spec.Type == openchoreov1alpha1.ComponentTypeManualTask {
//		return corev1.RestartPolicyNever
//	}
//	return corev1.RestartPolicyAlways
// }
//
// // makeDirectFileMountVolumeName generates a unique name for the file mount volume for a given FileMount spec
// // The name will be in the format: filemount-<hash-of-the-mount-path>
// func makeDirectFileMountVolumeName(fileMount *openchoreov1alpha1.FileMount) string {
//	hashLength := 8
//	hashBytes := sha256.Sum256([]byte(fileMount.MountPath))
//	hash := hex.EncodeToString(hashBytes[:])[:hashLength]
//	return fmt.Sprintf("filemount-%s", hash)
// }
