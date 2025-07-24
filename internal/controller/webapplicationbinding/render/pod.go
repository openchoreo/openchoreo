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

func makeWebApplicationPodSpec(rCtx Context) *corev1.PodSpec {
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
	wls := rCtx.WebApplicationBinding.Spec.WorkloadSpec

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
	c.Ports = makeContainerPortsFromEndpoints(rCtx.WebApplicationBinding.Spec.WorkloadSpec.Endpoints)

	return c
}

func makeEnvironmentVariables(rCtx Context) []corev1.EnvVar {
	var k8sEnvVars []corev1.EnvVar

	// Get environment variables from the first container
	wls := rCtx.WebApplicationBinding.Spec.WorkloadSpec
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

	wls := rCtx.WebApplicationBinding.Spec.WorkloadSpec
	
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
