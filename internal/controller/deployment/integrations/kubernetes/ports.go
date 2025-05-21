/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package kubernetes

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

func makeServicePortsFromEndpointTemplates(endpointTemplates []choreov1.EndpointTemplate) []corev1.ServicePort {
	return makeUniquePorts(endpointTemplates, func(name string, port int32, protocol corev1.Protocol) corev1.ServicePort {
		return corev1.ServicePort{
			Name:     name,
			Protocol: protocol,
			Port:     port,
		}
	})
}

func makeContainerPortsFromEndpointTemplates(endpointTemplates []choreov1.EndpointTemplate) []corev1.ContainerPort {
	return makeUniquePorts(endpointTemplates, func(name string, port int32, protocol corev1.Protocol) corev1.ContainerPort {
		return corev1.ContainerPort{
			Name:          name,
			ContainerPort: port,
			Protocol:      protocol,
		}
	})
}

// makeUniquePorts generates a list of unique ports based on the endpoint templates.
// This will ensure that the k8s port list does not have duplicates.
func makeUniquePorts[T any](
	endpointTemplates []choreov1.EndpointTemplate,
	createPort func(name string, port int32, protocol corev1.Protocol) T,
) []T {
	uniquePorts := make(map[string]struct{})

	// Generator fn for make a unique key to avoid duplicate mappings
	generatePortKey := func(port int32, t choreov1.EndpointType) string {
		return fmt.Sprintf("%d-%s", port, toK8SProtocol(t))
	}

	var result []T

	// Track the unique ports to avoid duplicates for the same port.
	// Example: Two REST endpoints with the same port but different base path.
	// Note the same port can be used for different protocols like TCP and UDP.
	for _, endpointTemplate := range endpointTemplates {
		key := generatePortKey(endpointTemplate.Spec.Service.Port, endpointTemplate.Spec.Type)
		if _, ok := uniquePorts[key]; !ok {
			uniquePorts[key] = struct{}{}
			protocol := toK8SProtocol(endpointTemplate.Spec.Type)
			port := endpointTemplate.Spec.Service.Port
			name := makePortNameFromEndpointTemplate(port, protocol)
			result = append(result, createPort(name, port, protocol))
		}
	}
	return result
}

// makePortNameFromEndpointTemplate generates a unique name for the k8s service port based on the
// port number and protocol.
// Example: ep-8080-tcp, ep-8080-udp
func makePortNameFromEndpointTemplate(port int32, protocol corev1.Protocol) string {
	return fmt.Sprintf("ep-%d-%s", port, strings.ToLower(string(protocol)))
}

func toK8SProtocol(endpointType choreov1.EndpointType) corev1.Protocol {
	if endpointType == choreov1.EndpointTypeUDP {
		return corev1.ProtocolUDP
	}
	return corev1.ProtocolTCP
}
