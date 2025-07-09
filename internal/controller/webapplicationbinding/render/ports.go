// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func makeServicePortsFromEndpoints(endpoints map[string]openchoreov1alpha1.WorkloadEndpoint) []corev1.ServicePort {
	return makeUniquePorts(endpoints, func(name string, port int32, protocol corev1.Protocol) corev1.ServicePort {
		return corev1.ServicePort{
			Name:       name,
			Protocol:   protocol,
			Port:       port,
			TargetPort: intstr.FromInt32(port),
		}
	})
}

func makeContainerPortsFromEndpoints(endpoints map[string]openchoreov1alpha1.WorkloadEndpoint) []corev1.ContainerPort {
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
	endpoints map[string]openchoreov1alpha1.WorkloadEndpoint,
	createPort func(name string, port int32, protocol corev1.Protocol) T,
) []T {
	var result []T

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

func toK8SProtocol(endpointType openchoreov1alpha1.EndpointType) corev1.Protocol {
	if endpointType == openchoreov1alpha1.EndpointTypeUDP {
		return corev1.ProtocolUDP
	}
	return corev1.ProtocolTCP
}
