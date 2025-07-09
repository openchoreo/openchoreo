// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

type fakePort struct {
	Name     string
	Port     int32
	Protocol corev1.Protocol
}

func createFakePort(name string, port int32, protocol corev1.Protocol) fakePort {
	return fakePort{
		Name:     name,
		Port:     port,
		Protocol: protocol,
	}
}

var _ = Describe("makeUniquePorts", func() {
	DescribeTable("should produce unique ports",
		func(endpointTemplates []openchoreov1alpha1.EndpointTemplate, expectedPorts []fakePort) {
			ports := makeUniquePorts(endpointTemplates, createFakePort)
			Expect(ports).To(Equal(expectedPorts))
		},
		Entry("for a single endpoint",
			[]openchoreov1alpha1.EndpointTemplate{
				{
					Spec: openchoreov1alpha1.EndpointSpec{
						BackendRef: openchoreov1alpha1.BackendRef{
							BasePath: "/customer",
							Type:     openchoreov1alpha1.BackendRefTypeComponentRef,
							ComponentRef: &openchoreov1alpha1.ComponentRef{
								Port: 8080,
							},
						},
						Type: openchoreov1alpha1.EndpointTypeREST,
					},
				},
			},
			[]fakePort{
				{Name: "ep-8080-tcp", Port: 8080, Protocol: corev1.ProtocolTCP},
			},
		),
		Entry("for two endpoints with same port and type",
			[]openchoreov1alpha1.EndpointTemplate{
				{
					Spec: openchoreov1alpha1.EndpointSpec{
						BackendRef: openchoreov1alpha1.BackendRef{
							BasePath: "/customer",
							Type:     openchoreov1alpha1.BackendRefTypeComponentRef,
							ComponentRef: &openchoreov1alpha1.ComponentRef{
								Port: 8080,
							},
						},
						Type: openchoreov1alpha1.EndpointTypeREST,
					},
				},
				{
					Spec: openchoreov1alpha1.EndpointSpec{
						BackendRef: openchoreov1alpha1.BackendRef{
							BasePath: "/order",
							Type:     openchoreov1alpha1.BackendRefTypeComponentRef,
							ComponentRef: &openchoreov1alpha1.ComponentRef{
								Port: 8080,
							},
						},
						Type: openchoreov1alpha1.EndpointTypeREST,
					},
				},
			},
			[]fakePort{
				{Name: "ep-8080-tcp", Port: 8080, Protocol: corev1.ProtocolTCP},
			},
		),
		Entry("for two endpoints with same port but tcp and udp",
			[]openchoreov1alpha1.EndpointTemplate{
				{
					Spec: openchoreov1alpha1.EndpointSpec{
						BackendRef: openchoreov1alpha1.BackendRef{
							Type: openchoreov1alpha1.BackendRefTypeComponentRef,
							ComponentRef: &openchoreov1alpha1.ComponentRef{
								Port: 8080,
							},
						},
						Type: openchoreov1alpha1.EndpointTypeTCP,
					},
				},
				{
					Spec: openchoreov1alpha1.EndpointSpec{
						BackendRef: openchoreov1alpha1.BackendRef{
							Type: openchoreov1alpha1.BackendRefTypeComponentRef,
							ComponentRef: &openchoreov1alpha1.ComponentRef{
								Port: 8080,
							},
						},
						Type: openchoreov1alpha1.EndpointTypeUDP,
					},
				},
			},
			[]fakePort{
				{Name: "ep-8080-tcp", Port: 8080, Protocol: corev1.ProtocolTCP},
				{Name: "ep-8080-udp", Port: 8080, Protocol: corev1.ProtocolUDP},
			},
		),
		Entry("for three endpoints with different ports",
			[]openchoreov1alpha1.EndpointTemplate{
				{
					Spec: openchoreov1alpha1.EndpointSpec{
						BackendRef: openchoreov1alpha1.BackendRef{
							BasePath: "/customer",
							Type:     openchoreov1alpha1.BackendRefTypeComponentRef,
							ComponentRef: &openchoreov1alpha1.ComponentRef{
								Port: 8080,
							},
						},
						Type: openchoreov1alpha1.EndpointTypeREST,
					},
				},
				{
					Spec: openchoreov1alpha1.EndpointSpec{
						BackendRef: openchoreov1alpha1.BackendRef{
							BasePath: "/customer",
							Type:     openchoreov1alpha1.BackendRefTypeComponentRef,
							ComponentRef: &openchoreov1alpha1.ComponentRef{
								Port: 8081,
							},
						},
						Type: openchoreov1alpha1.EndpointTypeGraphQL,
					},
				},
				{
					Spec: openchoreov1alpha1.EndpointSpec{
						BackendRef: openchoreov1alpha1.BackendRef{
							BasePath: "/customer",
							Type:     openchoreov1alpha1.BackendRefTypeComponentRef,
							ComponentRef: &openchoreov1alpha1.ComponentRef{
								Port: 8082,
							},
						},
						Type: openchoreov1alpha1.EndpointTypeGRPC,
					},
				},
			},
			[]fakePort{
				{Name: "ep-8080-tcp", Port: 8080, Protocol: corev1.ProtocolTCP},
				{Name: "ep-8081-tcp", Port: 8081, Protocol: corev1.ProtocolTCP},
				{Name: "ep-8082-tcp", Port: 8082, Protocol: corev1.ProtocolTCP},
			},
		),
	)
})
