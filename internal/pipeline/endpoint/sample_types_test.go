// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointpipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// progressiveTypesDir holds the EndpointTypes the progressive delivery samples
// apply; rendering them here keeps them in step with ${rules}.
var progressiveTypesDir = filepath.Join("..", "..", "..", "samples", "progressive-delivery", "endpoint-types")

const (
	kindVirtualService = "VirtualService"
	stableAdmin        = "stable-admin"
)

func loadProgressiveType(t *testing.T, file string) *v1alpha1.EndpointTypeSpec {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(progressiveTypesDir, file))
	require.NoError(t, err)
	var cet v1alpha1.ClusterEndpointType
	require.NoError(t, yaml.UnmarshalStrict(raw, &cet))
	spec := v1alpha1.EndpointTypeSpec(cet.Spec)
	return &spec
}

// progressiveInput is the stable backend at 90 and the canary at 10, with a
// preview of the canary under /next, testers reaching it by header, and a 10%
// mirror to a weight-0 shadow, built as the binding controller does: the path
// match first, as the controller sorts rules by how specific they are.
func progressiveInput(t *testing.T, file string, visibility v1alpha1.EndpointVisibility) *RenderInput {
	backends := fixtureBackends()
	shadow := backends[1]
	shadow.Name, shadow.ServiceName, shadow.Weight = "shadow", "greeter-v3-dev-5e6f", 0
	backends = append(backends, shadow)
	canary := backends[1]
	canary.Weight = 1
	return &RenderInput{
		EndpointTypeSpec: loadProgressiveType(t, file),
		Parameters:       rawExt(t, map[string]any{"pathPrefix": "/greeter"}),
		Visibility:       []v1alpha1.EndpointVisibility{visibility},
		Backends:         backends,
		Rules: []Rule{
			{Name: "preview", Backends: []Backend{canary}, Mirrors: []Mirror{},
				Matches: []Match{NewMatch(&PathMatch{Value: "/next"}, nil, nil)}},
			{Name: "testers", Backends: []Backend{canary}, Mirrors: []Mirror{},
				Matches: []Match{NewMatch(nil, []HeaderMatch{{Name: "X-Canary", Value: "true"}}, nil)}},
			{Name: "default", Matches: []Match{}, Backends: backends,
				Mirrors: []Mirror{{Backend: shadow, Percent: 10}}},
		},
		Metadata:    fixtureMetadata(),
		Environment: fixtureEnvironment(),
	}
}

func TestSampleType_HTTPProgressiveRendersRules(t *testing.T) {
	out, err := NewPipeline().Render(context.Background(), progressiveInput(t, "http.yaml", v1alpha1.EndpointVisibilityExternal))
	require.NoError(t, err)
	require.Len(t, out.Entries, 1)

	rules := out.Entries[0].Object["spec"].(map[string]any)["rules"].([]any)
	require.Len(t, rules, 3, "two match rules, then the default")

	testers := rules[1].(map[string]any)
	assert.Equal(t, []any{map[string]any{
		"path":    map[string]any{"type": "PathPrefix", "value": "/greeter"},
		"headers": []any{map[string]any{"type": "Exact", "name": "x-canary", "value": "true"}},
	}}, testers["matches"])
	assert.Equal(t, []any{map[string]any{"name": "greeter-v2-dev-3c4d", "port": float64(8080), "weight": float64(1)}},
		testers["backendRefs"])

	preview := rules[0].(map[string]any)
	assert.Equal(t, map[string]any{"type": "PathPrefix", "value": "/greeter/next"},
		preview["matches"].([]any)[0].(map[string]any)["path"])

	def := rules[2].(map[string]any)
	assert.Equal(t, []any{
		map[string]any{"name": "greeter-dev-1a2b", "port": float64(8080), "weight": float64(90)},
		map[string]any{"name": "greeter-v2-dev-3c4d", "port": float64(8080), "weight": float64(10)},
	}, def["backendRefs"], "the weight-0 shadow is not routed to")
	assert.Contains(t, def["filters"], map[string]any{"type": "RequestMirror", "requestMirror": map[string]any{
		"backendRef": map[string]any{"name": "greeter-v3-dev-5e6f", "port": float64(8080)}, "percent": float64(10)}})
}

func TestSampleType_IstioRendersRules(t *testing.T) {
	out, err := NewPipeline().Render(context.Background(), progressiveInput(t, "istio-virtualservice.yaml", v1alpha1.EndpointVisibilityNamespace))
	require.NoError(t, err)
	require.Len(t, out.Entries, 3, "callers of any backend's Service get the Endpoint's routing, one VirtualService each")
	for _, e := range out.Entries {
		assert.Len(t, e.Object["spec"].(map[string]any)["hosts"], 1)
	}

	spec := out.Entries[0].Object["spec"].(map[string]any)
	http := spec["http"].([]any)
	require.Len(t, http, 3)
	assert.Equal(t, map[string]any{"regex": `/next(/.*)?`},
		http[0].(map[string]any)["match"].([]any)[0].(map[string]any)["uri"])
	assert.Equal(t, map[string]any{"x-canary": map[string]any{"exact": "true"}},
		http[1].(map[string]any)["match"].([]any)[0].(map[string]any)["headers"])
	mirrors := http[2].(map[string]any)["mirrors"].([]any)
	require.Len(t, mirrors, 1)
	assert.Equal(t, map[string]any{"value": float64(10)}, mirrors[0].(map[string]any)["percentage"])
}

func TestSampleType_GAMMARendersRules(t *testing.T) {
	out, err := NewPipeline().Render(context.Background(), progressiveInput(t, "gamma-httproute.yaml", v1alpha1.EndpointVisibilityNamespace))
	require.NoError(t, err)
	require.Len(t, out.Entries, 1)

	spec := out.Entries[0].Object["spec"].(map[string]any)
	parents := spec["parentRefs"].([]any)
	require.Len(t, parents, 3, "callers of any backend's Service get the Endpoint's routing")
	assert.Equal(t, map[string]any{"group": "", "kind": "Service", "name": "greeter-dev-1a2b", "port": float64(8080)}, parents[0])

	rules := spec["rules"].([]any)
	require.Len(t, rules, 3)
	assert.Equal(t, []any{map[string]any{
		"headers": []any{map[string]any{"type": "Exact", "name": "x-canary", "value": "true"}},
	}}, rules[1].(map[string]any)["matches"])
	assert.NotContains(t, rules[2].(map[string]any), "matches", "the default rule matches everything")
	assert.Len(t, rules[2].(map[string]any)["backendRefs"], 2)
}

func TestSampleTypes_DeclareCapabilities(t *testing.T) {
	all := []v1alpha1.EndpointCapability{
		v1alpha1.EndpointCapabilityMatch, v1alpha1.EndpointCapabilityRegularExpression,
		v1alpha1.EndpointCapabilityMirror, v1alpha1.EndpointCapabilityMirrorPercent,
	}
	assert.ElementsMatch(t, all, loadProgressiveType(t, "http.yaml").Capabilities)
	assert.ElementsMatch(t, all, loadProgressiveType(t, "istio-virtualservice.yaml").Capabilities)
	assert.ElementsMatch(t, all, loadProgressiveType(t, "istio-http.yaml").Capabilities)
	assert.ElementsMatch(t, all, loadProgressiveType(t, "cilium-http.yaml").Capabilities)
	assert.Equal(t, []v1alpha1.EndpointCapability{v1alpha1.EndpointCapabilityMatch},
		loadProgressiveType(t, "gamma-httproute.yaml").Capabilities)
}

// renderKinds renders a sample type for the given visibilities and returns the
// rendered objects by kind.
func renderKinds(t *testing.T, file string, visibility ...v1alpha1.EndpointVisibility) map[string]map[string]any {
	t.Helper()
	input := progressiveInput(t, file, visibility[0])
	input.Visibility = visibility
	out, err := NewPipeline().Render(context.Background(), input)
	require.NoError(t, err)
	kinds := map[string]map[string]any{}
	for _, e := range out.Entries {
		kinds[e.Object["kind"].(string)] = e.Object
	}
	return kinds
}

func TestSampleType_MeshTypesServeGatewayAndMesh(t *testing.T) {
	for file, meshKind := range map[string]string{"istio-http.yaml": kindVirtualService, "cilium-http.yaml": "CiliumEnvoyConfig"} {
		t.Run(file, func(t *testing.T) {
			both := renderKinds(t, file, v1alpha1.EndpointVisibilityExternal, v1alpha1.EndpointVisibilityNamespace)
			assert.Len(t, both, 2)
			assert.Contains(t, both, "HTTPRoute")
			assert.Contains(t, both, meshKind)

			external := renderKinds(t, file, v1alpha1.EndpointVisibilityExternal)
			assert.Equal(t, []string{"HTTPRoute"}, keys(external), "gateway callers only")

			for _, v := range []v1alpha1.EndpointVisibility{v1alpha1.EndpointVisibilityNamespace, v1alpha1.EndpointVisibilityInternal} {
				assert.Equal(t, []string{meshKind}, keys(renderKinds(t, file, v)), "in-cluster callers of %s", v)
			}
		})
	}
}

func TestSampleType_IstioHTTPSplitsTheSameInBothDirections(t *testing.T) {
	kinds := renderKinds(t, "istio-http.yaml", v1alpha1.EndpointVisibilityExternal, v1alpha1.EndpointVisibilityNamespace)

	rules := kinds["HTTPRoute"]["spec"].(map[string]any)["rules"].([]any)
	require.Len(t, rules, 3)
	assert.Equal(t, []any{
		map[string]any{"name": "greeter-dev-1a2b", "port": float64(8080), "weight": float64(90)},
		map[string]any{"name": "greeter-v2-dev-3c4d", "port": float64(8080), "weight": float64(10)},
	}, rules[2].(map[string]any)["backendRefs"])

	http := kinds[kindVirtualService]["spec"].(map[string]any)["http"].([]any)
	require.Len(t, http, 3)
	route := http[2].(map[string]any)["route"].([]any)
	weights := make([]any, 0, len(route))
	for _, d := range route {
		weights = append(weights, d.(map[string]any)["weight"])
	}
	assert.Equal(t, []any{float64(90), float64(10)}, weights)
}

func TestSampleType_CiliumHTTPRendersRules(t *testing.T) {
	cec := renderKinds(t, "cilium-http.yaml", v1alpha1.EndpointVisibilityNamespace)["CiliumEnvoyConfig"]
	spec := cec["spec"].(map[string]any)
	assert.Len(t, spec["services"], 3, "callers of any backend's Service go through Envoy")

	resources := spec["resources"].([]any)
	require.Len(t, resources, 5, "a listener, its routes, and a cluster per Service")
	hcm := resources[0].(map[string]any)["filter_chains"].([]any)[0].(map[string]any)["filters"].([]any)[0].(map[string]any)["typed_config"].(map[string]any)
	assert.Equal(t, []any{map[string]any{"upgrade_type": "websocket"}}, hcm["upgrade_configs"], "WebSocket handshakes pass on")
	clusters := map[string]bool{}
	for _, r := range resources[2:] {
		clusters[r.(map[string]any)["name"].(string)] = true
	}

	vhost := resources[1].(map[string]any)["virtual_hosts"].([]any)[0].(map[string]any)
	routes := vhost["routes"].([]any)
	require.Len(t, routes, 3, "two match routes, then the default")

	testers := routes[1].(map[string]any)["match"].(map[string]any)
	assert.Equal(t, "/", testers["prefix"])
	assert.Equal(t, []any{map[string]any{"name": "x-canary", "string_match": map[string]any{"exact": "true"}}},
		testers["headers"])
	preview := routes[0].(map[string]any)["match"].(map[string]any)
	assert.Equal(t, map[string]any{"regex": `/next(/.*)?`}, preview["safe_regex"])
	assert.NotContains(t, preview, "prefix")

	def := routes[2].(map[string]any)["route"].(map[string]any)
	weighted := def["weighted_clusters"].(map[string]any)["clusters"].([]any)
	require.Len(t, weighted, 2, "the weight-0 shadow is not routed to")
	for _, c := range weighted {
		assert.True(t, clusters[c.(map[string]any)["name"].(string)], "routes name clusters the config defines")
	}
	for name := range clusters {
		assert.True(t, strings.HasSuffix(name, ":8080"),
			"%s: Cilium names a Service's endpoints by port once its ports are listed", name)
	}
	assert.Equal(t, float64(90), weighted[0].(map[string]any)["weight"])
	mirrors := def["request_mirror_policies"].([]any)
	require.Len(t, mirrors, 1)
	assert.Equal(t, map[string]any{"default_value": map[string]any{"numerator": float64(10), "denominator": "HUNDRED"}},
		mirrors[0].(map[string]any)["runtime_fraction"])
	assert.True(t, clusters[mirrors[0].(map[string]any)["cluster"].(string)])
}

func keys(m map[string]map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestSampleTypes_NeedOneBasePath(t *testing.T) {
	for _, file := range []string{"http.yaml", "istio-http.yaml", "cilium-http.yaml", "istio-virtualservice.yaml", "gamma-httproute.yaml"} {
		t.Run(file, func(t *testing.T) {
			input := progressiveInput(t, file, v1alpha1.EndpointVisibilityNamespace)
			input.Backends[1].BasePath = "/v2"
			_, err := NewPipeline().Render(context.Background(), input)
			require.ErrorContains(t, err, "needs every backend to serve under the same base path")
		})
	}
}

func TestSampleTypes_JoinMeshPathMatchesToTheBasePath(t *testing.T) {
	withBase := func(t *testing.T, file string) *RenderInput {
		input := progressiveInput(t, file, v1alpha1.EndpointVisibilityNamespace)
		for i := range input.Backends {
			input.Backends[i].BasePath = "/api.v1"
		}
		for i := range input.Rules {
			for j := range input.Rules[i].Backends {
				input.Rules[i].Backends[j].BasePath = "/api.v1"
			}
		}
		return input
	}

	out, err := NewPipeline().Render(context.Background(), withBase(t, "istio-virtualservice.yaml"))
	require.NoError(t, err)
	http := out.Entries[0].Object["spec"].(map[string]any)["http"].([]any)
	assert.Equal(t, map[string]any{"regex": `/api\.v1/next(/.*)?`},
		http[0].(map[string]any)["match"].([]any)[0].(map[string]any)["uri"])

	out, err = NewPipeline().Render(context.Background(), withBase(t, "gamma-httproute.yaml"))
	require.NoError(t, err)
	rules := out.Entries[0].Object["spec"].(map[string]any)["rules"].([]any)
	assert.Equal(t, map[string]any{"type": "PathPrefix", "value": "/api.v1/next"},
		rules[0].(map[string]any)["matches"].([]any)[0].(map[string]any)["path"])
}

func TestSampleType_CiliumHTTPDefinesEachServiceOnce(t *testing.T) {
	input := progressiveInput(t, "cilium-http.yaml", v1alpha1.EndpointVisibilityNamespace)
	withSecondStableHandle(input)
	out, err := NewPipeline().Render(context.Background(), input)
	require.NoError(t, err)
	spec := out.Entries[0].Object["spec"].(map[string]any)
	assert.Len(t, spec["services"], 3)
	assert.Len(t, spec["resources"], 5, "a listener, its routes, and one cluster for each of the 3 Services")
}

func TestSampleType_CiliumHTTPRedirectsOnlyTheBackendsPorts(t *testing.T) {
	input := progressiveInput(t, "cilium-http.yaml", v1alpha1.EndpointVisibilityNamespace)
	admin := input.Backends[0]
	admin.Name, admin.Port = stableAdmin, 9090
	input.Backends = append(input.Backends, admin)
	spec := renderObjects(t, input)["CiliumEnvoyConfig"]["spec"].(map[string]any)
	assert.Contains(t, spec["services"], map[string]any{
		"name": input.Backends[0].ServiceName, "namespace": input.Backends[0].Namespace,
		"ports": []any{float64(8080), float64(9090)},
	}, "one entry per Service, with only the ports its backends use")
	for _, svc := range spec["services"].([]any) {
		assert.NotEmpty(t, svc.(map[string]any)["ports"], "no list would redirect every port")
	}
}

// virtualServicePorts renders a type and returns, for each VirtualService
// host, the ports each of its routes matches.
func virtualServicePorts(t *testing.T, input *RenderInput) map[string][][]any {
	t.Helper()
	out, err := NewPipeline().Render(context.Background(), input)
	require.NoError(t, err)
	byHost := map[string][][]any{}
	for _, e := range out.Entries {
		if e.Object["kind"] != kindVirtualService {
			continue
		}
		spec := e.Object["spec"].(map[string]any)
		require.Len(t, spec["hosts"], 1)
		host := spec["hosts"].([]any)[0].(string)
		for _, route := range spec["http"].([]any) {
			matches := route.(map[string]any)["match"].([]any)
			ports := make([]any, 0, len(matches))
			for _, m := range matches {
				ports = append(ports, m.(map[string]any)["port"])
			}
			byHost[host] = append(byHost[host], ports)
		}
	}
	return byHost
}

func TestSampleTypes_RouteOnlyTheBackendsPorts(t *testing.T) {
	for _, file := range []string{"istio-http.yaml", "istio-virtualservice.yaml"} {
		t.Run(file, func(t *testing.T) {
			input := progressiveInput(t, file, v1alpha1.EndpointVisibilityNamespace)
			admin := input.Backends[0]
			admin.Name, admin.Port = stableAdmin, 9090
			input.Backends = append(input.Backends, admin)
			stable := admin.ServiceName + "." + admin.Namespace + ".svc.cluster.local"
			byHost := virtualServicePorts(t, input)
			require.Len(t, byHost, 3)
			for host, routes := range byHost {
				want := []any{float64(8080)}
				if host == stable {
					want = []any{float64(8080), float64(9090)}
				}
				// Each rule has one match, or none for the default: one per port.
				for _, ports := range routes {
					assert.ElementsMatch(t, want, ports, "routes for %s match its backends' ports only", host)
				}
			}
		})
	}
}

func TestSampleTypes_MatchEachServiceOnItsOwnPorts(t *testing.T) {
	for _, file := range []string{"istio-http.yaml", "istio-virtualservice.yaml"} {
		t.Run(file, func(t *testing.T) {
			// The stable Service is called on 8080 and the canary's on 9090, so
			// a call to the stable Service on 9090 keeps its own route.
			input := progressiveInput(t, file, v1alpha1.EndpointVisibilityNamespace)
			canary := input.Backends[1].ServiceName
			for i := range input.Backends {
				if input.Backends[i].ServiceName == canary {
					input.Backends[i].Port = 9090
				}
			}
			for i := range input.Rules {
				for j := range input.Rules[i].Backends {
					if input.Rules[i].Backends[j].ServiceName == canary {
						input.Rules[i].Backends[j].Port = 9090
					}
				}
			}
			byHost := virtualServicePorts(t, input)
			require.Len(t, byHost, 3)
			for host, routes := range byHost {
				want := []any{float64(8080)}
				if strings.HasPrefix(host, canary+".") {
					want = []any{float64(9090)}
				}
				for _, ports := range routes {
					assert.Equal(t, want, ports, "routes for %s", host)
				}
			}
		})
	}
}

func TestSampleTypes_MatchHostAsAuthority(t *testing.T) {
	for _, file := range []string{"istio-http.yaml", "istio-virtualservice.yaml"} {
		t.Run(file, func(t *testing.T) {
			input := progressiveInput(t, file, v1alpha1.EndpointVisibilityNamespace)
			input.Rules[0].Matches = []Match{NewMatch(nil, []HeaderMatch{{Name: "Host", Value: "beta.example.com"}}, nil)}
			input.Rules[1].Matches = []Match{NewMatch(nil, []HeaderMatch{
				{Name: "Host", Type: HeaderMatchRegularExpression, Value: `.*\.beta\.example\.com`},
				{Name: "X-Canary", Value: "true"},
			}, nil)}
			routes := renderObjects(t, input)[kindVirtualService]["spec"].(map[string]any)["http"].([]any)

			exact := routes[0].(map[string]any)["match"].([]any)[0].(map[string]any)
			assert.Equal(t, map[string]any{"exact": "beta.example.com"}, exact["authority"])
			assert.NotContains(t, exact, "headers")

			regex := routes[1].(map[string]any)["match"].([]any)[0].(map[string]any)
			assert.Equal(t, map[string]any{"regex": `.*\.beta\.example\.com`}, regex["authority"])
			assert.Equal(t, map[string]any{"x-canary": map[string]any{"exact": "true"}}, regex["headers"],
				"the other headers stay headers")
		})
	}
}

func TestSampleTypes_ListEachMeshHostOnce(t *testing.T) {
	for _, file := range []string{"istio-http.yaml", "istio-virtualservice.yaml"} {
		t.Run(file, func(t *testing.T) {
			input := progressiveInput(t, file, v1alpha1.EndpointVisibilityNamespace)
			withSecondStableHandle(input)
			input.Rules[2].Backends = input.Backends
			spec := renderObjects(t, input)[kindVirtualService]["spec"].(map[string]any)
			assert.Len(t, spec["hosts"], 1)
			assert.Len(t, spec["http"], 3, "every rule keeps its route")
			assert.Len(t, virtualServicePorts(t, input), 3, "the stable Service once, with v2 and the shadow")
		})
	}
}

// withBasePath gives every backend, in the input and in its rules, one base
// path.
func withBasePath(input *RenderInput, path string) *RenderInput {
	for i := range input.Backends {
		input.Backends[i].BasePath = path
	}
	for i := range input.Rules {
		for j := range input.Rules[i].Backends {
			input.Rules[i].Backends[j].BasePath = path
		}
	}
	return input
}

func meshURIs(t *testing.T, file string, input *RenderInput) []string {
	t.Helper()
	out, err := NewPipeline().Render(context.Background(), input)
	require.NoError(t, err)
	var regexes []string
	for _, e := range out.Entries {
		if e.Object["kind"] != kindVirtualService {
			continue
		}
		for _, h := range e.Object["spec"].(map[string]any)["http"].([]any) {
			matches, _ := h.(map[string]any)["match"].([]any)
			for _, m := range matches {
				if uri, ok := m.(map[string]any)["uri"]; ok {
					regexes = append(regexes, uri.(map[string]any)["regex"].(string))
				}
			}
		}
	}
	require.NotEmpty(t, regexes, file)
	return regexes
}

func TestSampleTypes_MatchTheBasePathLiterally(t *testing.T) {
	for _, file := range []string{"istio-virtualservice.yaml", "istio-http.yaml"} {
		t.Run(file, func(t *testing.T) {
			input := withBasePath(progressiveInput(t, file, v1alpha1.EndpointVisibilityNamespace), "/v(1)+")
			re := regexp.MustCompile("^(?:" + meshURIs(t, file, input)[0] + ")$")
			assert.True(t, re.MatchString("/v(1)+/next"))
			assert.False(t, re.MatchString("/v11/next"), "( ) and + are characters of the path")
		})
	}
}

func TestSampleTypes_RenderARootPrefixAsNoPathCondition(t *testing.T) {
	for _, file := range []string{"istio-virtualservice.yaml", "istio-http.yaml"} {
		t.Run(file, func(t *testing.T) {
			input := withBasePath(progressiveInput(t, file, v1alpha1.EndpointVisibilityNamespace), "/api")
			input.Rules[0].Matches = []Match{NewMatch(&PathMatch{Value: "/"}, []HeaderMatch{{Name: "x-beta", Value: "1"}}, nil)}
			input.Rules = append(input.Rules[:1], input.Rules[2:]...)
			out, err := NewPipeline().Render(context.Background(), input)
			require.NoError(t, err)
			beta := 0
			for _, e := range out.Entries {
				if e.Object["kind"] != kindVirtualService {
					continue
				}
				for _, h := range e.Object["spec"].(map[string]any)["http"].([]any) {
					matches, _ := h.(map[string]any)["match"].([]any)
					for _, m := range matches {
						m := m.(map[string]any)
						if headers, ok := m["headers"].(map[string]any); ok && headers["x-beta"] != nil {
							beta++
						}
						assert.NotContains(t, m, "uri",
							`a "/" prefix selects every request, as a match without a path does`)
					}
				}
			}
			assert.Positive(t, beta, "the x-beta route is rendered")
		})
	}
}

func TestSampleTypes_JoinAPrefixWithATrailingSlash(t *testing.T) {
	// istio-http serves each Endpoint under /<endpoint name> first.
	for file, want := range map[string]string{
		"http.yaml":        "/greeter/next",
		"istio-http.yaml":  "/greeter-api/greeter/next",
		"cilium-http.yaml": "/greeter/next",
	} {
		t.Run(file, func(t *testing.T) {
			input := progressiveInput(t, file, v1alpha1.EndpointVisibilityExternal)
			input.Parameters = rawExt(t, map[string]any{"pathPrefix": "/greeter/"})
			out, err := NewPipeline().Render(context.Background(), input)
			require.NoError(t, err)
			routes := 0
			for _, e := range out.Entries {
				if e.Object["kind"] != "HTTPRoute" {
					continue
				}
				routes++
				rules := e.Object["spec"].(map[string]any)["rules"].([]any)
				assert.Equal(t, map[string]any{"type": "PathPrefix", "value": want},
					rules[0].(map[string]any)["matches"].([]any)[0].(map[string]any)["path"])
			}
			assert.Equal(t, 1, routes, "one HTTPRoute for external callers")
		})
	}
}

func TestSampleType_CiliumHTTPMatchesHostAsAuthority(t *testing.T) {
	input := progressiveInput(t, "cilium-http.yaml", v1alpha1.EndpointVisibilityNamespace)
	input.Rules[1].Matches = []Match{NewMatch(nil, []HeaderMatch{{Name: "Host", Value: "beta.example.com"}}, nil)}
	cec := renderObjects(t, input)["CiliumEnvoyConfig"]
	vhost := cec["spec"].(map[string]any)["resources"].([]any)[1].(map[string]any)["virtual_hosts"].([]any)[0].(map[string]any)
	headers := vhost["routes"].([]any)[1].(map[string]any)["match"].(map[string]any)["headers"].([]any)
	assert.Equal(t, ":authority", headers[0].(map[string]any)["name"], "Envoy keeps Host in :authority")
}

func TestSampleType_GAMMANamesEachServiceOnce(t *testing.T) {
	input := progressiveInput(t, "gamma-httproute.yaml", v1alpha1.EndpointVisibilityNamespace)
	withSecondStableHandle(input)
	route := renderObjects(t, input)["HTTPRoute"]
	assert.Len(t, route["spec"].(map[string]any)["parentRefs"], 3)
}

// withSecondStableHandle adds a second backend on the stable component's
// endpoint, at another weight: one Service behind two backends.
func withSecondStableHandle(input *RenderInput) {
	again := input.Backends[0]
	again.Name, again.Weight = "stable-again", 5
	input.Backends = append(input.Backends, again)
}

func renderObjects(t *testing.T, input *RenderInput) map[string]map[string]any {
	t.Helper()
	out, err := NewPipeline().Render(context.Background(), input)
	require.NoError(t, err)
	kinds := map[string]map[string]any{}
	for _, e := range out.Entries {
		kinds[e.Object["kind"].(string)] = e.Object
	}
	return kinds
}

func TestSampleType_CiliumHTTPDefinesAClusterPerServicePort(t *testing.T) {
	input := progressiveInput(t, "cilium-http.yaml", v1alpha1.EndpointVisibilityNamespace)
	admin := input.Backends[0]
	admin.Name, admin.Port = stableAdmin, 9090
	input.Backends = append(input.Backends, admin)
	spec := renderObjects(t, input)["CiliumEnvoyConfig"]["spec"].(map[string]any)
	resources := spec["resources"].([]any)[2:]
	clusters := make([]string, 0, len(resources))
	for _, r := range resources {
		clusters = append(clusters, r.(map[string]any)["name"].(string))
	}
	stable := admin.Namespace + "/" + admin.ServiceName
	assert.Contains(t, clusters, stable+":8080")
	assert.Contains(t, clusters, stable+":9090")
	assert.Len(t, clusters, 4)
}

func TestSampleTypes_NeedTheComponentsService(t *testing.T) {
	for _, file := range []string{"http.yaml", "istio-http.yaml", "cilium-http.yaml", "istio-virtualservice.yaml", "gamma-httproute.yaml"} {
		t.Run(file, func(t *testing.T) {
			input := progressiveInput(t, file, v1alpha1.EndpointVisibilityNamespace)
			input.Backends[2].ServiceName = ""
			_, err := NewPipeline().Render(context.Background(), input)
			require.ErrorContains(t, err, "its ComponentType renders none")
		})
	}
}

func TestSampleTypes_HoldAtMost16RulesInAnHTTPRoute(t *testing.T) {
	for file, visibility := range map[string]v1alpha1.EndpointVisibility{
		"http.yaml":            v1alpha1.EndpointVisibilityExternal,
		"istio-http.yaml":      v1alpha1.EndpointVisibilityExternal,
		"cilium-http.yaml":     v1alpha1.EndpointVisibilityExternal,
		"gamma-httproute.yaml": v1alpha1.EndpointVisibilityNamespace,
	} {
		t.Run(file, func(t *testing.T) {
			input := progressiveInput(t, file, visibility)
			for len(input.Rules) < 17 {
				extra := input.Rules[1]
				extra.Name = fmt.Sprintf("testers-%d", len(input.Rules))
				input.Rules = append([]Rule{extra}, input.Rules...)
			}
			_, err := NewPipeline().Render(context.Background(), input)
			require.ErrorContains(t, err, "at most 16 rules")
		})
	}
}

func TestSampleTypes_JoinABasePathEndingInASlash(t *testing.T) {
	out, err := NewPipeline().Render(context.Background(),
		withBasePath(progressiveInput(t, "gamma-httproute.yaml", v1alpha1.EndpointVisibilityNamespace), "/api/"))
	require.NoError(t, err)
	rules := out.Entries[0].Object["spec"].(map[string]any)["rules"].([]any)
	assert.Equal(t, map[string]any{"type": "PathPrefix", "value": "/api/next"},
		rules[0].(map[string]any)["matches"].([]any)[0].(map[string]any)["path"])

	input := withBasePath(progressiveInput(t, "istio-virtualservice.yaml", v1alpha1.EndpointVisibilityNamespace), "/api/")
	assert.Equal(t, `/api/next(/.*)?`, meshURIs(t, "istio-virtualservice.yaml", input)[0])

	out, err = NewPipeline().Render(context.Background(),
		withBasePath(progressiveInput(t, "http.yaml", v1alpha1.EndpointVisibilityExternal), "/api/"))
	require.NoError(t, err)
	filters := out.Entries[0].Object["spec"].(map[string]any)["rules"].([]any)[0].(map[string]any)["filters"].([]any)
	assert.Equal(t, "/api/next",
		filters[0].(map[string]any)["urlRewrite"].(map[string]any)["path"].(map[string]any)["replacePrefixMatch"])
}
