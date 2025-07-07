// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

func makeServicePortsFromEndpoints(endpoints map[string]choreov1.WorkloadEndpoint) []corev1.ServicePort {
	return makeUniquePorts(endpoints, func(name string, port int32, protocol corev1.Protocol) corev1.ServicePort {
		return corev1.ServicePort{
			Name:       name,
			Protocol:   protocol,
			Port:       port,
			TargetPort: intstr.FromInt32(port),
		}
	})
}

func makeContainerPortsFromEndpoints(endpoints map[string]choreov1.WorkloadEndpoint) []corev1.ContainerPort {
	return makeUniquePorts(endpoints, func(name string, port int32, protocol corev1.Protocol) corev1.ContainerPort {
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
	endpoints map[string]choreov1.WorkloadEndpoint,
	createPort func(name string, port int32, protocol corev1.Protocol) T,
) []T {
	// uniquePorts := make(map[string]struct{})

	// Generator fn for make a unique key to avoid duplicate mappings
	// generatePortKey := func(port int32, t choreov1.EndpointType) string {
	//	return fmt.Sprintf("%d-%s", port, toK8SProtocol(t))
	//}

	var result []T

	// TODO: The following code block is commented out because the spec for endpoints is not yet fully defined to be operate at L4 or L7.

	// Track the unique ports to avoid duplicates for the same port.
	// Example: Two REST endpoints with the same port but different base path.
	// Note the same port can be used for different protocols like TCP and UDP.
	// for _, endpoint := range endpoints {
	//	epType := endpoint.Spec.Type
	//	var epPort int32
	//
	//	// Extract port based on the endpoint type
	//	switch epType {
	//	case choreov1.EndpointTypeREST:
	//		if endpoint.Spec.RESTEndpoint != nil {
	//			epPort = endpoint.Spec.RESTEndpoint.Backend.Port
	//		}
	//	// TODO: Add support for other endpoint types (gRPC, TCP, etc.)
	//	default:
	//		continue // Skip unsupported endpoint types
	//	}
	//
	//	if epPort <= 0 {
	//		continue // Skip invalid ports
	//	}
	//
	//	key := generatePortKey(epPort, epType)
	//	if _, ok := uniquePorts[key]; !ok {
	//		uniquePorts[key] = struct{}{}
	//		protocol := toK8SProtocol(epType)
	//		port := epPort
	//		name := makePortNameFromEndpointTemplate(port, protocol)
	//		result = append(result, createPort(name, port, protocol))
	//	}
	//}

	for epName, endpoint := range endpoints {
		result = append(result, createPort(epName, endpoint.Port, endpoint.Protocol))
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
