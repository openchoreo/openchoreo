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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
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
		func(endpointTemplates []choreov1.EndpointTemplate, expectedPorts []fakePort) {
			ports := makeUniquePorts(endpointTemplates, createFakePort)
			Expect(ports).To(Equal(expectedPorts))
		},
		Entry("for a single endpoint",
			[]choreov1.EndpointTemplate{
				{
					Spec: choreov1.EndpointSpec{
						Service: choreov1.EndpointServiceSpec{
							BasePath: "/customer",
							Port:     8080,
						},
						Type: choreov1.EndpointTypeREST,
					},
				},
			},
			[]fakePort{
				{Name: "ep-8080-tcp", Port: 8080, Protocol: corev1.ProtocolTCP},
			},
		),
		Entry("for two endpoints with same port and type",
			[]choreov1.EndpointTemplate{
				{
					Spec: choreov1.EndpointSpec{
						Service: choreov1.EndpointServiceSpec{
							BasePath: "/customer",
							Port:     8080,
						},
						Type: choreov1.EndpointTypeREST,
					},
				},
				{
					Spec: choreov1.EndpointSpec{
						Service: choreov1.EndpointServiceSpec{
							BasePath: "/order",
							Port:     8080,
						},
						Type: choreov1.EndpointTypeREST,
					},
				},
			},
			[]fakePort{
				{Name: "ep-8080-tcp", Port: 8080, Protocol: corev1.ProtocolTCP},
			},
		),
		Entry("for two endpoints with same port but tcp and udp",
			[]choreov1.EndpointTemplate{
				{
					Spec: choreov1.EndpointSpec{
						Service: choreov1.EndpointServiceSpec{
							Port: 8080,
						},
						Type: choreov1.EndpointTypeTCP,
					},
				},
				{
					Spec: choreov1.EndpointSpec{
						Service: choreov1.EndpointServiceSpec{
							Port: 8080,
						},
						Type: choreov1.EndpointTypeUDP,
					},
				},
			},
			[]fakePort{
				{Name: "ep-8080-tcp", Port: 8080, Protocol: corev1.ProtocolTCP},
				{Name: "ep-8080-udp", Port: 8080, Protocol: corev1.ProtocolUDP},
			},
		),
		Entry("for three endpoints with different ports",
			[]choreov1.EndpointTemplate{
				{
					Spec: choreov1.EndpointSpec{
						Service: choreov1.EndpointServiceSpec{
							BasePath: "/customer",
							Port:     8080,
						},
						Type: choreov1.EndpointTypeREST,
					},
				},
				{
					Spec: choreov1.EndpointSpec{
						Service: choreov1.EndpointServiceSpec{
							BasePath: "/graphql",
							Port:     8081,
						},
						Type: choreov1.EndpointTypeGraphQL,
					},
				},
				{
					Spec: choreov1.EndpointSpec{
						Service: choreov1.EndpointServiceSpec{
							Port: 8082,
						},
						Type: choreov1.EndpointTypeGRPC,
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
