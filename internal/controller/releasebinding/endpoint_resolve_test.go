// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// makeHTTPRouteJSON builds an unstructured HTTPRoute JSON blob for testing.
func makeHTTPRouteJSON(opts httpRouteOpts) []byte {
	route := map[string]interface{}{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "HTTPRoute",
		"metadata": map[string]interface{}{
			"name":      opts.name,
			"namespace": "default",
		},
	}

	spec := map[string]interface{}{}

	if len(opts.hostnames) > 0 {
		spec["hostnames"] = opts.hostnames
	}

	if len(opts.parentRefs) > 0 {
		spec["parentRefs"] = opts.parentRefs
	}

	rule := map[string]interface{}{}

	if opts.backendRefPort != 0 {
		rule["backendRefs"] = []interface{}{
			map[string]interface{}{
				"name": "svc",
				"port": opts.backendRefPort,
			},
		}
	}

	if opts.pathValue != "" {
		rule["matches"] = []interface{}{
			map[string]interface{}{
				"path": map[string]interface{}{
					"type":  "PathPrefix",
					"value": opts.pathValue,
				},
			},
		}
	}

	if len(rule) > 0 {
		spec["rules"] = []interface{}{rule}
	}

	route["spec"] = spec

	b, _ := json.Marshal(route)
	return b
}

type httpRouteOpts struct {
	name           string
	hostnames      []interface{}
	parentRefs     []interface{}
	backendRefPort int64
	pathValue      string
}

func parentRef(name, namespace string) map[string]interface{} {
	ref := map[string]interface{}{"name": name}
	if namespace != "" {
		ref["namespace"] = namespace
	}
	return ref
}

func makeResource(raw []byte) openchoreov1alpha1.Resource {
	return openchoreov1alpha1.Resource{
		ID:     "test-resource",
		Object: &runtime.RawExtension{Raw: raw},
	}
}

func makeEndpoints(entries ...endpointEntry) map[string]openchoreov1alpha1.WorkloadEndpoint {
	m := make(map[string]openchoreov1alpha1.WorkloadEndpoint, len(entries))
	for _, e := range entries {
		m[e.name] = openchoreov1alpha1.WorkloadEndpoint{
			Port: e.port,
			Type: "REST",
		}
	}
	return m
}

type endpointEntry struct {
	name string
	port int32
}

func makeDataPlane(gw openchoreov1alpha1.GatewaySpec) *openchoreov1alpha1.DataPlane {
	return &openchoreov1alpha1.DataPlane{
		Spec: openchoreov1alpha1.DataPlaneSpec{
			Gateway: gw,
		},
	}
}

func makeEnvironment(gw openchoreov1alpha1.GatewaySpec) *openchoreov1alpha1.Environment {
	return &openchoreov1alpha1.Environment{
		Spec: openchoreov1alpha1.EnvironmentSpec{
			Gateway: gw,
		},
	}
}

var _ = Describe("resolveEndpointURLStatuses", func() {
	Context("when there are no endpoints", func() {
		It("should return nil", func() {
			result := resolveEndpointURLStatuses(
				ctx,
				[]openchoreov1alpha1.Resource{},
				nil,
				nil,
				makeDataPlane(openchoreov1alpha1.GatewaySpec{}),
			)
			Expect(result).To(BeNil())
		})
	})

	Context("when there are no resources", func() {
		It("should return empty slice", func() {
			endpoints := makeEndpoints(endpointEntry{name: "greeter", port: 8080})
			result := resolveEndpointURLStatuses(
				ctx,
				nil,
				endpoints,
				nil,
				makeDataPlane(openchoreov1alpha1.GatewaySpec{}),
			)
			Expect(result).To(BeEmpty())
		})
	})

	Context("when resource is not an HTTPRoute", func() {
		It("should skip non-HTTPRoute resources", func() {
			svcJSON, _ := json.Marshal(map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata":   map[string]interface{}{"name": "svc"},
			})
			endpoints := makeEndpoints(endpointEntry{name: "greeter", port: 8080})
			result := resolveEndpointURLStatuses(
				ctx,
				[]openchoreov1alpha1.Resource{makeResource(svcJSON)},
				endpoints,
				nil,
				makeDataPlane(openchoreov1alpha1.GatewaySpec{}),
			)
			Expect(result).To(BeEmpty())
		})
	})

	Context("when backendRef port does not match any endpoint", func() {
		It("should skip the HTTPRoute", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:           "route",
				hostnames:      []interface{}{"app.example.com"},
				backendRefPort: 9999,
			})
			endpoints := makeEndpoints(endpointEntry{name: "greeter", port: 8080})
			result := resolveEndpointURLStatuses(
				ctx,
				[]openchoreov1alpha1.Resource{makeResource(raw)},
				endpoints,
				nil,
				makeDataPlane(openchoreov1alpha1.GatewaySpec{}),
			)
			Expect(result).To(BeEmpty())
		})
	})

	Context("when backendRef port is missing", func() {
		It("should skip the HTTPRoute", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:      "route",
				hostnames: []interface{}{"app.example.com"},
				// backendRefPort is 0, so no backendRefs are created
			})
			endpoints := makeEndpoints(endpointEntry{name: "greeter", port: 8080})
			result := resolveEndpointURLStatuses(
				ctx,
				[]openchoreov1alpha1.Resource{makeResource(raw)},
				endpoints,
				nil,
				makeDataPlane(openchoreov1alpha1.GatewaySpec{}),
			)
			Expect(result).To(BeEmpty())
		})
	})

	Context("when hostname is absent", func() {
		It("should skip the HTTPRoute", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:           "route",
				backendRefPort: 8080,
				// no hostnames
			})
			endpoints := makeEndpoints(endpointEntry{name: "greeter", port: 8080})
			result := resolveEndpointURLStatuses(
				ctx,
				[]openchoreov1alpha1.Resource{makeResource(raw)},
				endpoints,
				nil,
				makeDataPlane(openchoreov1alpha1.GatewaySpec{}),
			)
			Expect(result).To(BeEmpty())
		})
	})

	Context("with standard HTTPS port (no port in URL)", func() {
		It("should produce URL without port suffix", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:           "route",
				hostnames:      []interface{}{"app.example.com"},
				backendRefPort: 8080,
				parentRefs:     []interface{}{parentRef("gateway-default", "openchoreo-data-plane")},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				PublicVirtualHost:      "example.com",
				PublicGatewayName:      "gateway-default",
				PublicHTTPSPort:        standardHTTPSPort,
				PublicGatewayNamespace: "openchoreo-data-plane",
			})
			endpoints := makeEndpoints(endpointEntry{name: "greeter", port: 8080})

			result := resolveEndpointURLStatuses(
				ctx,
				[]openchoreov1alpha1.Resource{makeResource(raw)},
				endpoints,
				nil,
				dp,
			)
			Expect(result).To(HaveLen(1))
			Expect(result[0].Name).To(Equal("greeter"))
			Expect(result[0].InvokeURL).To(Equal("https://app.example.com:19443"))
		})
	})

	Context("with non-standard gateway port (port in URL)", func() {
		It("should include port in the URL", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:           "route",
				hostnames:      []interface{}{"app.example.com"},
				backendRefPort: 8080,
				parentRefs:     []interface{}{parentRef("gateway-default", "openchoreo-data-plane")},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				PublicVirtualHost:      "example.com",
				PublicGatewayName:      "gateway-default",
				PublicHTTPSPort:        30443,
				PublicGatewayNamespace: "openchoreo-data-plane",
			})
			endpoints := makeEndpoints(endpointEntry{name: "greeter", port: 8080})

			result := resolveEndpointURLStatuses(
				ctx,
				[]openchoreov1alpha1.Resource{makeResource(raw)},
				endpoints,
				nil,
				dp,
			)
			Expect(result).To(HaveLen(1))
			Expect(result[0].Name).To(Equal("greeter"))
			Expect(result[0].InvokeURL).To(Equal("https://app.example.com:30443"))
		})
	})

	Context("with path in route", func() {
		It("should include path in the URL", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:           "route",
				hostnames:      []interface{}{"app.example.com"},
				backendRefPort: 8080,
				pathValue:      "/api/v1",
				parentRefs:     []interface{}{parentRef("gateway-default", "openchoreo-data-plane")},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				PublicVirtualHost:      "example.com",
				PublicGatewayName:      "gateway-default",
				PublicHTTPSPort:        standardHTTPSPort,
				PublicGatewayNamespace: "openchoreo-data-plane",
			})
			endpoints := makeEndpoints(endpointEntry{name: "greeter", port: 8080})

			result := resolveEndpointURLStatuses(
				ctx,
				[]openchoreov1alpha1.Resource{makeResource(raw)},
				endpoints,
				nil,
				dp,
			)
			Expect(result).To(HaveLen(1))
			Expect(result[0].Name).To(Equal("greeter"))
			Expect(result[0].InvokeURL).To(Equal("https://app.example.com:19443/api/v1"))
		})
	})

	Context("with non-standard port and path", func() {
		It("should include both port and path in the URL", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:           "route",
				hostnames:      []interface{}{"app.example.com"},
				backendRefPort: 8080,
				pathValue:      "/greeter",
				parentRefs:     []interface{}{parentRef("gateway-default", "openchoreo-data-plane")},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				PublicVirtualHost:      "example.com",
				PublicGatewayName:      "gateway-default",
				PublicHTTPSPort:        30443,
				PublicGatewayNamespace: "openchoreo-data-plane",
			})
			endpoints := makeEndpoints(endpointEntry{name: "greeter", port: 8080})

			result := resolveEndpointURLStatuses(
				ctx,
				[]openchoreov1alpha1.Resource{makeResource(raw)},
				endpoints,
				nil,
				dp,
			)
			Expect(result).To(HaveLen(1))
			Expect(result[0].Name).To(Equal("greeter"))
			Expect(result[0].InvokeURL).To(Equal("https://app.example.com:30443/greeter"))
		})
	})

	Context("with environment gateway override", func() {
		It("should use environment port instead of dataplane port", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:           "route",
				hostnames:      []interface{}{"app.env.example.com"},
				backendRefPort: 8080,
				parentRefs:     []interface{}{parentRef("gateway-default", "openchoreo-data-plane")},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				PublicVirtualHost:      "example.com",
				PublicGatewayName:      "gateway-default",
				PublicHTTPSPort:        30443,
				PublicGatewayNamespace: "openchoreo-data-plane",
			})
			env := makeEnvironment(openchoreov1alpha1.GatewaySpec{
				PublicVirtualHost:      "env.example.com",
				PublicGatewayName:      "gateway-default",
				PublicHTTPSPort:        443,
				PublicGatewayNamespace: "openchoreo-data-plane",
			})
			endpoints := makeEndpoints(endpointEntry{name: "greeter", port: 8080})

			result := resolveEndpointURLStatuses(
				ctx,
				[]openchoreov1alpha1.Resource{makeResource(raw)},
				endpoints,
				env,
				dp,
			)
			Expect(result).To(HaveLen(1))
			Expect(result[0].Name).To(Equal("greeter"))
			Expect(result[0].InvokeURL).To(Equal("https://app.env.example.com:443"))
		})
	})

	Context("with gateway port 0 (no parentRef match)", func() {
		It("should produce URL without port", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:           "route",
				hostnames:      []interface{}{"app.example.com"},
				backendRefPort: 8080,
				parentRefs:     []interface{}{parentRef("unknown-gw", "some-ns")},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				PublicVirtualHost:      "example.com",
				PublicGatewayName:      "gateway-default",
				PublicGatewayNamespace: "openchoreo-data-plane",
			})
			endpoints := makeEndpoints(endpointEntry{name: "greeter", port: 8080})

			result := resolveEndpointURLStatuses(
				ctx,
				[]openchoreov1alpha1.Resource{makeResource(raw)},
				endpoints,
				nil,
				dp,
			)
			Expect(result).To(HaveLen(1))
			Expect(result[0].Name).To(Equal("greeter"))
			Expect(result[0].InvokeURL).To(Equal("https://app.example.com"))
		})
	})

	Context("with multiple endpoints and HTTPRoutes", func() {
		It("should resolve each HTTPRoute to the correct endpoint", func() {
			route1 := makeHTTPRouteJSON(httpRouteOpts{
				name:           "route-greeter",
				hostnames:      []interface{}{"greeter.example.com"},
				backendRefPort: 8080,
				pathValue:      "/greet",
				parentRefs:     []interface{}{parentRef("gateway-default", "openchoreo-data-plane")},
			})
			route2 := makeHTTPRouteJSON(httpRouteOpts{
				name:           "route-health",
				hostnames:      []interface{}{"health.example.com"},
				backendRefPort: 9090,
				parentRefs:     []interface{}{parentRef("gateway-default", "openchoreo-data-plane")},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				PublicVirtualHost:      "example.com",
				PublicGatewayName:      "gateway-default",
				PublicHTTPSPort:        standardHTTPSPort,
				PublicGatewayNamespace: "openchoreo-data-plane",
			})
			endpoints := makeEndpoints(
				endpointEntry{name: "greeter", port: 8080},
				endpointEntry{name: "health", port: 9090},
			)

			result := resolveEndpointURLStatuses(
				ctx,
				[]openchoreov1alpha1.Resource{
					makeResource(route1),
					makeResource(route2),
				},
				endpoints,
				nil,
				dp,
			)
			Expect(result).To(HaveLen(2))
			Expect(result[0].Name).To(Equal("greeter"))
			Expect(result[0].InvokeURL).To(Equal("https://greeter.example.com:19443/greet"))
			Expect(result[1].Name).To(Equal("health"))
			Expect(result[1].InvokeURL).To(Equal("https://health.example.com:19443"))
		})
	})

	Context("with organization gateway parentRef", func() {
		It("should resolve port from organization gateway config", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:           "route",
				hostnames:      []interface{}{"app.org.example.com"},
				backendRefPort: 8080,
				parentRefs:     []interface{}{parentRef("org-gateway", "gw-ns")},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				PublicVirtualHost:            "example.com",
				PublicGatewayName:            "gateway-default",
				PublicGatewayNamespace:       "openchoreo-data-plane",
				OrganizationGatewayName:      "org-gateway",
				OrganizationGatewayNamespace: "gw-ns",
				OrganizationHTTPSPort:        31443,
			})
			endpoints := makeEndpoints(endpointEntry{name: "greeter", port: 8080})

			result := resolveEndpointURLStatuses(
				ctx,
				[]openchoreov1alpha1.Resource{makeResource(raw)},
				endpoints,
				nil,
				dp,
			)
			Expect(result).To(HaveLen(1))
			Expect(result[0].Name).To(Equal("greeter"))
			Expect(result[0].InvokeURL).To(Equal("https://app.org.example.com:31443"))
		})
	})

	Context("with parentRef namespace omitted (wildcard match)", func() {
		It("should match gateway when parentRef has no namespace", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:           "route",
				hostnames:      []interface{}{"app.example.com"},
				backendRefPort: 8080,
				parentRefs:     []interface{}{parentRef("gateway-default", "")},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				PublicVirtualHost:      "example.com",
				PublicGatewayName:      "gateway-default",
				PublicHTTPSPort:        30443,
				PublicGatewayNamespace: "openchoreo-data-plane",
			})
			endpoints := makeEndpoints(endpointEntry{name: "greeter", port: 8080})

			result := resolveEndpointURLStatuses(
				ctx,
				[]openchoreov1alpha1.Resource{makeResource(raw)},
				endpoints,
				nil,
				dp,
			)
			Expect(result).To(HaveLen(1))
			Expect(result[0].Name).To(Equal("greeter"))
			Expect(result[0].InvokeURL).To(Equal("https://app.example.com:30443"))
		})
	})

	Context("with nil Object in resource", func() {
		It("should skip resources with nil Object", func() {
			endpoints := makeEndpoints(endpointEntry{name: "greeter", port: 8080})
			result := resolveEndpointURLStatuses(
				ctx,
				[]openchoreov1alpha1.Resource{{ID: "empty", Object: nil}},
				endpoints,
				nil,
				makeDataPlane(openchoreov1alpha1.GatewaySpec{}),
			)
			Expect(result).To(BeEmpty())
		})
	})
})

var _ = Describe("extractBackendRefPort", func() {
	It("should return port from spec.rules[0].backendRefs[0].port", func() {
		raw := makeHTTPRouteJSON(httpRouteOpts{
			name:           "route",
			backendRefPort: 8080,
		})
		obj := &unstructured.Unstructured{}
		Expect(obj.UnmarshalJSON(raw)).To(Succeed())
		Expect(extractBackendRefPort(obj)).To(Equal(int64(8080)))
	})

	It("should return 0 when rules are missing", func() {
		raw, _ := json.Marshal(map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata":   map[string]interface{}{"name": "route"},
			"spec":       map[string]interface{}{},
		})
		obj := &unstructured.Unstructured{}
		Expect(obj.UnmarshalJSON(raw)).To(Succeed())
		Expect(extractBackendRefPort(obj)).To(Equal(int64(0)))
	})

	It("should return 0 when backendRefs are missing", func() {
		raw, _ := json.Marshal(map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata":   map[string]interface{}{"name": "route"},
			"spec": map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{},
				},
			},
		})
		obj := &unstructured.Unstructured{}
		Expect(obj.UnmarshalJSON(raw)).To(Succeed())
		Expect(extractBackendRefPort(obj)).To(Equal(int64(0)))
	})
})

var _ = Describe("extractFirstHostname", func() {
	It("should return the first hostname", func() {
		raw := makeHTTPRouteJSON(httpRouteOpts{
			name:      "route",
			hostnames: []interface{}{"first.example.com", "second.example.com"},
		})
		obj := &unstructured.Unstructured{}
		Expect(obj.UnmarshalJSON(raw)).To(Succeed())
		Expect(extractFirstHostname(obj)).To(Equal("first.example.com"))
	})

	It("should return empty string when hostnames are absent", func() {
		raw := makeHTTPRouteJSON(httpRouteOpts{name: "route"})
		obj := &unstructured.Unstructured{}
		Expect(obj.UnmarshalJSON(raw)).To(Succeed())
		Expect(extractFirstHostname(obj)).To(BeEmpty())
	})
})

var _ = Describe("extractFirstPathValue", func() {
	It("should return the path value", func() {
		raw := makeHTTPRouteJSON(httpRouteOpts{
			name:      "route",
			pathValue: "/api/v1",
		})
		obj := &unstructured.Unstructured{}
		Expect(obj.UnmarshalJSON(raw)).To(Succeed())
		Expect(extractFirstPathValue(obj)).To(Equal("/api/v1"))
	})

	It("should return empty string when no path match exists", func() {
		raw := makeHTTPRouteJSON(httpRouteOpts{
			name:           "route",
			backendRefPort: 8080,
		})
		obj := &unstructured.Unstructured{}
		Expect(obj.UnmarshalJSON(raw)).To(Succeed())
		Expect(extractFirstPathValue(obj)).To(BeEmpty())
	})
})

var _ = Describe("extractFirstParentRef", func() {
	It("should return name and namespace", func() {
		raw := makeHTTPRouteJSON(httpRouteOpts{
			name:       "route",
			parentRefs: []interface{}{parentRef("my-gw", "my-ns")},
		})
		obj := &unstructured.Unstructured{}
		Expect(obj.UnmarshalJSON(raw)).To(Succeed())
		name, ns := extractFirstParentRef(obj)
		Expect(name).To(Equal("my-gw"))
		Expect(ns).To(Equal("my-ns"))
	})

	It("should return empty strings when parentRefs are absent", func() {
		raw := makeHTTPRouteJSON(httpRouteOpts{name: "route"})
		obj := &unstructured.Unstructured{}
		Expect(obj.UnmarshalJSON(raw)).To(Succeed())
		name, ns := extractFirstParentRef(obj)
		Expect(name).To(BeEmpty())
		Expect(ns).To(BeEmpty())
	})
})

var _ = Describe("resolveGatewayPort", func() {
	It("should return 0 when name is empty", func() {
		dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{})
		Expect(resolveGatewayPort("", "ns", nil, dp)).To(Equal(int32(0)))
	})

	It("should return 0 when dataplane is nil", func() {
		Expect(resolveGatewayPort("gw", "ns", nil, nil)).To(Equal(int32(0)))
	})

	It("should return public port for matching public gateway", func() {
		dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
			PublicGatewayName:      "pub-gw",
			PublicGatewayNamespace: "gw-ns",
			PublicHTTPSPort:        30443,
		})
		Expect(resolveGatewayPort("pub-gw", "gw-ns", nil, dp)).To(Equal(int32(30443)))
	})

	It("should return standard HTTPS port when public port is 0", func() {
		dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
			PublicGatewayName:      "pub-gw",
			PublicGatewayNamespace: "gw-ns",
		})
		Expect(resolveGatewayPort("pub-gw", "gw-ns", nil, dp)).To(Equal(int32(standardHTTPSPort)))
	})

	It("should return organization port for matching org gateway", func() {
		dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
			OrganizationGatewayName:      "org-gw",
			OrganizationGatewayNamespace: "gw-ns",
			OrganizationHTTPSPort:        31443,
		})
		Expect(resolveGatewayPort("org-gw", "gw-ns", nil, dp)).To(Equal(int32(31443)))
	})

	It("should use default gateway name/ns when not specified", func() {
		dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
			PublicHTTPSPort: 30443,
		})
		Expect(resolveGatewayPort(defaultGatewayName, defaultGatewayNS, nil, dp)).To(Equal(int32(30443)))
	})

	It("should prefer environment config when PublicVirtualHost is set", func() {
		dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
			PublicGatewayName:      "pub-gw",
			PublicGatewayNamespace: "gw-ns",
			PublicHTTPSPort:        30443,
		})
		env := makeEnvironment(openchoreov1alpha1.GatewaySpec{
			PublicVirtualHost:      "env.example.com",
			PublicGatewayName:      "pub-gw",
			PublicGatewayNamespace: "gw-ns",
			PublicHTTPSPort:        443,
		})
		Expect(resolveGatewayPort("pub-gw", "gw-ns", env, dp)).To(Equal(int32(443)))
	})

	It("should match when parentRef namespace is empty (wildcard)", func() {
		dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
			PublicGatewayName:      "pub-gw",
			PublicGatewayNamespace: "gw-ns",
			PublicHTTPSPort:        30443,
		})
		Expect(resolveGatewayPort("pub-gw", "", nil, dp)).To(Equal(int32(30443)))
	})
})
