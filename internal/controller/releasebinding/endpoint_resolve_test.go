// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// urlToString converts an EndpointURL to a string representation.
func urlToString(u *openchoreov1alpha1.EndpointURL) string {
	if u == nil {
		return ""
	}

	url := u.Scheme + "://" + u.Host

	// Add port if it's not the default port for the scheme
	if u.Port != 0 && !((u.Scheme == "http" && u.Port == 80) || (u.Scheme == "https" && u.Port == 443)) {
		url += fmt.Sprintf(":%d", u.Port)
	}

	// Add path if present
	if u.Path != "" {
		if !strings.HasPrefix(u.Path, "/") {
			url += "/"
		}
		url += u.Path
	}

	return url
}

// httpRouteOpts configures the HTTPRoute JSON built by makeHTTPRouteJSON.
type httpRouteOpts struct {
	name      string
	labels    map[string]interface{}
	hostnames []interface{}
	pathValue string
}

// makeHTTPRouteJSON builds an unstructured HTTPRoute JSON blob for testing.
func makeHTTPRouteJSON(opts httpRouteOpts) []byte {
	metadata := map[string]interface{}{
		"name":      opts.name,
		"namespace": "default",
	}
	if len(opts.labels) > 0 {
		metadata["labels"] = opts.labels
	}

	route := map[string]interface{}{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "HTTPRoute",
		"metadata":   metadata,
	}

	spec := map[string]interface{}{}

	if len(opts.hostnames) > 0 {
		spec["hostnames"] = opts.hostnames
	}

	if opts.pathValue != "" {
		spec["rules"] = []interface{}{
			map[string]interface{}{
				"matches": []interface{}{
					map[string]interface{}{
						"path": map[string]interface{}{
							"type":  "PathPrefix",
							"value": opts.pathValue,
						},
					},
				},
			},
		}
	}

	route["spec"] = spec

	b, _ := json.Marshal(route)
	return b
}

func makeResource(raw []byte) openchoreov1alpha1.Resource {
	return openchoreov1alpha1.Resource{
		ID:     "test-resource",
		Object: &runtime.RawExtension{Raw: raw},
	}
}

// endpointEntry configures an entry passed to makeEndpoints.
type endpointEntry struct {
	name   string
	port   int32
	epType openchoreov1alpha1.EndpointType
}

func makeEndpoints(entries ...endpointEntry) map[string]openchoreov1alpha1.WorkloadEndpoint {
	m := make(map[string]openchoreov1alpha1.WorkloadEndpoint, len(entries))
	for _, e := range entries {
		epType := e.epType
		if epType == "" {
			epType = openchoreov1alpha1.EndpointTypeHTTP
		}
		m[e.name] = openchoreov1alpha1.WorkloadEndpoint{
			Port: e.port,
			Type: epType,
		}
	}
	return m
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

// extLabels returns labels marking an HTTPRoute for the named endpoint with external visibility.
func extLabels(name string) map[string]interface{} {
	return map[string]interface{}{
		labels.LabelKeyEndpointName:       name,
		labels.LabelKeyEndpointVisibility: string(openchoreov1alpha1.EndpointVisibilityExternal),
	}
}

// intLabels returns labels marking an HTTPRoute for the named endpoint with internal visibility.
func intLabels(name string) map[string]interface{} {
	return map[string]interface{}{
		labels.LabelKeyEndpointName:       name,
		labels.LabelKeyEndpointVisibility: string(openchoreov1alpha1.EndpointVisibilityInternal),
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

	Context("when HTTPRoute has no endpoint-name label", func() {
		It("should skip the HTTPRoute", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:      "route",
				hostnames: []interface{}{"app.example.com"},
				// no labels
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

	Context("when HTTPRoute endpoint-name label does not match any endpoint", func() {
		It("should skip the HTTPRoute", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:      "route",
				hostnames: []interface{}{"app.example.com"},
				labels: map[string]interface{}{
					labels.LabelKeyEndpointName:       "unknown-endpoint",
					labels.LabelKeyEndpointVisibility: "external",
				},
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

	Context("when endpoint type is not HTTP-compatible", func() {
		It("should skip HTTPRoutes for TCP endpoints", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:      "route",
				hostnames: []interface{}{"app.example.com"},
				labels:    extLabels("tcp-endpoint"),
			})
			endpoints := makeEndpoints(endpointEntry{
				name:   "tcp-endpoint",
				port:   9000,
				epType: openchoreov1alpha1.EndpointTypeTCP,
			})
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
				name:   "route",
				labels: extLabels("greeter"),
				// no hostnames
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
					External: &openchoreov1alpha1.GatewayEndpointSpec{
						HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 30443},
					},
				},
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
			Expect(result[0].ExternalURLs).To(BeNil())
			Expect(result[0].InternalURLs).To(BeNil())
		})
	})

	Context("when no gateway endpoint is configured for the visibility", func() {
		It("should skip the HTTPRoute", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:      "route",
				labels:    extLabels("greeter"),
				hostnames: []interface{}{"app.example.com"},
			})
			// External visibility but no external gateway configured
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
					Internal: &openchoreov1alpha1.GatewayEndpointSpec{
						HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 31443},
					},
				},
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
			Expect(result[0].ExternalURLs).To(BeNil())
			Expect(result[0].InternalURLs).To(BeNil())
		})
	})

	Context("with HTTPS-only external gateway", func() {
		It("should produce a single HTTPS invoke URL", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:      "route",
				labels:    extLabels("greeter"),
				hostnames: []interface{}{"app.example.com"},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
					External: &openchoreov1alpha1.GatewayEndpointSpec{
						HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 30443},
					},
				},
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
			Expect(urlToString(result[0].ExternalURLs.HTTPS)).To(Equal("https://app.example.com:30443"))
		})
	})

	Context("with HTTP-only external gateway", func() {
		It("should produce a single HTTP invoke URL", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:      "route",
				labels:    extLabels("greeter"),
				hostnames: []interface{}{"app.example.com"},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
					External: &openchoreov1alpha1.GatewayEndpointSpec{
						HTTP: &openchoreov1alpha1.GatewayListenerSpec{Port: 30080},
					},
				},
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
			Expect(urlToString(result[0].ExternalURLs.HTTP)).To(Equal("http://app.example.com:30080"))
		})
	})

	Context("with HTTP and HTTPS external gateway", func() {
		It("should produce both HTTP and HTTPS invoke URLs", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:      "route",
				labels:    extLabels("greeter"),
				hostnames: []interface{}{"app.example.com"},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
					External: &openchoreov1alpha1.GatewayEndpointSpec{
						HTTP:  &openchoreov1alpha1.GatewayListenerSpec{Port: 30080},
						HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 30443},
					},
				},
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
			Expect(urlToString(result[0].ExternalURLs.HTTP)).To(Equal("http://app.example.com:30080"))
			Expect(urlToString(result[0].ExternalURLs.HTTPS)).To(Equal("https://app.example.com:30443"))
		})
	})

	Context("with HTTP, HTTPS and TLS external gateway", func() {
		It("should produce HTTP, HTTPS and TLS invoke URLs", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:      "route",
				labels:    extLabels("greeter"),
				hostnames: []interface{}{"app.example.com"},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
					External: &openchoreov1alpha1.GatewayEndpointSpec{
						HTTP:  &openchoreov1alpha1.GatewayListenerSpec{Port: 30080},
						HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 30443},
						TLS:   &openchoreov1alpha1.GatewayListenerSpec{Port: 30444},
					},
				},
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
			Expect(urlToString(result[0].ExternalURLs.HTTP)).To(Equal("http://app.example.com:30080"))
			Expect(urlToString(result[0].ExternalURLs.HTTPS)).To(Equal("https://app.example.com:30443"))
			Expect(urlToString(result[0].ExternalURLs.TLS)).To(Equal("https://app.example.com:30444"))
		})
	})

	Context("with path in route", func() {
		It("should include path in all invoke URLs", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:      "route",
				labels:    extLabels("greeter"),
				hostnames: []interface{}{"app.example.com"},
				pathValue: "/api/v1",
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
					External: &openchoreov1alpha1.GatewayEndpointSpec{
						HTTP:  &openchoreov1alpha1.GatewayListenerSpec{Port: 30080},
						HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 30443},
					},
				},
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
			Expect(urlToString(result[0].ExternalURLs.HTTP)).To(Equal("http://app.example.com:30080/api/v1"))
			Expect(urlToString(result[0].ExternalURLs.HTTPS)).To(Equal("https://app.example.com:30443/api/v1"))
		})
	})

	Context("with listener port 0 (standard port implied)", func() {
		It("should produce URL without port suffix", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:      "route",
				labels:    extLabels("greeter"),
				hostnames: []interface{}{"app.example.com"},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
					External: &openchoreov1alpha1.GatewayEndpointSpec{
						HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 0},
					},
				},
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
			Expect(urlToString(result[0].ExternalURLs.HTTPS)).To(Equal("https://app.example.com"))
		})
	})

	Context("with internal visibility", func() {
		It("should use the internal gateway endpoint", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:      "route",
				labels:    intLabels("greeter"),
				hostnames: []interface{}{"app.internal.example.com"},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
					External: &openchoreov1alpha1.GatewayEndpointSpec{
						HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 30443},
					},
					Internal: &openchoreov1alpha1.GatewayEndpointSpec{
						HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 31443},
					},
				},
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
			Expect(urlToString(result[0].InternalURLs.HTTPS)).To(Equal("https://app.internal.example.com:31443"))
		})
	})

	Context("with environment gateway override", func() {
		It("should use environment endpoint spec instead of dataplane spec", func() {
			raw := makeHTTPRouteJSON(httpRouteOpts{
				name:      "route",
				labels:    extLabels("greeter"),
				hostnames: []interface{}{"app.env.example.com"},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
					External: &openchoreov1alpha1.GatewayEndpointSpec{
						HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 30443},
					},
				},
			})
			env := makeEnvironment(openchoreov1alpha1.GatewaySpec{
				Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
					External: &openchoreov1alpha1.GatewayEndpointSpec{
						HTTP:  &openchoreov1alpha1.GatewayListenerSpec{Port: 80},
						HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 443},
					},
				},
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
			Expect(urlToString(result[0].ExternalURLs.HTTP)).To(Equal("http://app.env.example.com"))
			Expect(urlToString(result[0].ExternalURLs.HTTPS)).To(Equal("https://app.env.example.com"))
		})
	})

	Context("with multiple endpoints and HTTPRoutes", func() {
		It("should resolve each HTTPRoute to the correct endpoint", func() {
			route1 := makeHTTPRouteJSON(httpRouteOpts{
				name:      "route-greeter",
				labels:    extLabels("greeter"),
				hostnames: []interface{}{"greeter.example.com"},
				pathValue: "/greet",
			})
			route2 := makeHTTPRouteJSON(httpRouteOpts{
				name:      "route-health",
				labels:    intLabels("health"),
				hostnames: []interface{}{"health.internal.example.com"},
			})
			dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
				Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
					External: &openchoreov1alpha1.GatewayEndpointSpec{
						HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 19443},
					},
					Internal: &openchoreov1alpha1.GatewayEndpointSpec{
						HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 31443},
					},
				},
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
			Expect(urlToString(result[0].ExternalURLs.HTTPS)).To(Equal("https://greeter.example.com:19443/greet"))
			Expect(result[1].Name).To(Equal("health"))
			Expect(urlToString(result[1].InternalURLs.HTTPS)).To(Equal("https://health.internal.example.com:31443"))
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

var _ = Describe("extractFirstHostname", func() {
	It("should return the first hostname", func() {
		raw := makeHTTPRouteJSON(httpRouteOpts{
			name:      "route",
			hostnames: []interface{}{"first.example.com", "second.example.com"},
		})
		obj := unmarshalHTTPRoute(raw)
		Expect(extractFirstHostname(obj)).To(Equal("first.example.com"))
	})

	It("should return empty string when hostnames are absent", func() {
		raw := makeHTTPRouteJSON(httpRouteOpts{name: "route"})
		obj := unmarshalHTTPRoute(raw)
		Expect(extractFirstHostname(obj)).To(BeEmpty())
	})
})

var _ = Describe("extractFirstPathValue", func() {
	It("should return the path value", func() {
		raw := makeHTTPRouteJSON(httpRouteOpts{
			name:      "route",
			pathValue: "/api/v1",
		})
		obj := unmarshalHTTPRoute(raw)
		Expect(extractFirstPathValue(obj)).To(Equal("/api/v1"))
	})

	It("should return empty string when no path match exists", func() {
		raw := makeHTTPRouteJSON(httpRouteOpts{name: "route"})
		obj := unmarshalHTTPRoute(raw)
		Expect(extractFirstPathValue(obj)).To(BeEmpty())
	})
})

var _ = Describe("resolveGatewayEndpointByVisibility", func() {
	It("should return nil when dataplane is nil", func() {
		Expect(resolveGatewayEndpointByVisibility(openchoreov1alpha1.EndpointVisibilityExternal, nil, nil)).To(BeNil())
	})

	It("should return nil when no ingress is configured", func() {
		dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{})
		Expect(resolveGatewayEndpointByVisibility(openchoreov1alpha1.EndpointVisibilityExternal, nil, dp)).To(BeNil())
	})

	It("should return external endpoint for external visibility", func() {
		extEP := &openchoreov1alpha1.GatewayEndpointSpec{
			HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 30443},
		}
		dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
			Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
				External: extEP,
				Internal: &openchoreov1alpha1.GatewayEndpointSpec{
					HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 31443},
				},
			},
		})
		Expect(resolveGatewayEndpointByVisibility(openchoreov1alpha1.EndpointVisibilityExternal, nil, dp)).To(Equal(extEP))
	})

	It("should return internal endpoint for internal visibility", func() {
		intEP := &openchoreov1alpha1.GatewayEndpointSpec{
			HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 31443},
		}
		dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
			Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
				External: &openchoreov1alpha1.GatewayEndpointSpec{
					HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 30443},
				},
				Internal: intEP,
			},
		})
		Expect(resolveGatewayEndpointByVisibility(openchoreov1alpha1.EndpointVisibilityInternal, nil, dp)).To(Equal(intEP))
	})

	It("should return nil when external endpoint is absent for external visibility", func() {
		dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
			Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
				Internal: &openchoreov1alpha1.GatewayEndpointSpec{
					HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 31443},
				},
			},
		})
		Expect(resolveGatewayEndpointByVisibility(openchoreov1alpha1.EndpointVisibilityExternal, nil, dp)).To(BeNil())
	})

	It("should use internal endpoint for project visibility (non-external)", func() {
		intEP := &openchoreov1alpha1.GatewayEndpointSpec{
			HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 31443},
		}
		dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
			Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
				Internal: intEP,
			},
		})
		Expect(resolveGatewayEndpointByVisibility(openchoreov1alpha1.EndpointVisibilityProject, nil, dp)).To(Equal(intEP))
	})

	It("should use environment config when environment ingress is configured", func() {
		dpEP := &openchoreov1alpha1.GatewayEndpointSpec{
			HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 30443},
		}
		envEP := &openchoreov1alpha1.GatewayEndpointSpec{
			HTTP:  &openchoreov1alpha1.GatewayListenerSpec{Port: 80},
			HTTPS: &openchoreov1alpha1.GatewayListenerSpec{Port: 443},
		}
		dp := makeDataPlane(openchoreov1alpha1.GatewaySpec{
			Ingress: &openchoreov1alpha1.GatewayNetworkSpec{External: dpEP},
		})
		env := makeEnvironment(openchoreov1alpha1.GatewaySpec{
			Ingress: &openchoreov1alpha1.GatewayNetworkSpec{External: envEP},
		})
		Expect(resolveGatewayEndpointByVisibility(openchoreov1alpha1.EndpointVisibilityExternal, env, dp)).To(Equal(envEP))
	})
})

// unmarshalHTTPRoute is a test helper that unmarshals raw JSON into an Unstructured object.
func unmarshalHTTPRoute(raw []byte) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	_ = obj.UnmarshalJSON(raw)
	return obj
}
