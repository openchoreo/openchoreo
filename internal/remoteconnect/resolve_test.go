// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteconnect

import "testing"

const localHostStr = "127.0.0.1"

func TestRenderEnvEndpoint(t *testing.T) {
	tgt := ResolvedTarget{
		Key:   "ep/backend-api/http",
		Proto: "tcp",
		Endpoint: &EndpointRender{
			Scheme:   "http",
			BasePath: "/api",
			Bindings: EndpointEnvBindings{Address: "BACKEND_API_URL", Host: "BACKEND_HOST", Port: "BACKEND_PORT", BasePath: "BACKEND_PATH"},
		},
	}
	env := RenderEnv(tgt, localHostStr, 9090)
	if got := env["BACKEND_API_URL"]; got != "http://127.0.0.1:9090/api" {
		t.Errorf("address = %q", got)
	}
	if env["BACKEND_HOST"] != localHostStr || env["BACKEND_PORT"] != "9090" || env["BACKEND_PATH"] != "/api" {
		t.Errorf("component bindings wrong: %+v", env)
	}
}

func TestRenderEnvResource(t *testing.T) {
	tgt := ResolvedTarget{
		Key:   ResourceTargetKey("doclet-postgres", "client"),
		Proto: "tcp",
		Resource: &ResourceRender{
			Ref:      "doclet-postgres",
			Endpoint: "client",
			HostEnv:  "DB_HOST",
			PortEnv:  "DB_PORT",
		},
	}
	env := RenderEnv(tgt, localHostStr, 55432)
	if env["DB_HOST"] != localHostStr || env["DB_PORT"] != "55432" {
		t.Errorf("host/port rewrite wrong: %+v", env)
	}
	// Non-tunnel bindings arrive via ResolveResponse.Resources, not per target, so a
	// resource target renders nothing but its own host/port.
	if len(env) != 2 {
		t.Errorf("expected only host/port from a resource target, got %+v", env)
	}
}

// A resource with two endpoints rewrites each port binding independently while both
// share one host binding -- every tunnel terminates on the same local host.
func TestRenderEnvResourceMultipleEndpoints(t *testing.T) {
	client := ResolvedTarget{
		Key:      ResourceTargetKey("tc-broker", "client"),
		Proto:    "tcp",
		Resource: &ResourceRender{Ref: "tc-broker", Endpoint: "client", HostEnv: "BROKER_HOST", PortEnv: "BROKER_PORT"},
	}
	monitor := ResolvedTarget{
		Key:      ResourceTargetKey("tc-broker", "monitor"),
		Proto:    "tcp",
		Resource: &ResourceRender{Ref: "tc-broker", Endpoint: "monitor", HostEnv: "BROKER_HOST", PortEnv: "BROKER_MONITOR_PORT"},
	}
	env := RenderEnv(client, localHostStr, 61452)
	for k, v := range RenderEnv(monitor, localHostStr, 61460) {
		env[k] = v
	}
	if env["BROKER_HOST"] != localHostStr {
		t.Errorf("shared host binding wrong: %+v", env)
	}
	if env["BROKER_PORT"] != "61452" || env["BROKER_MONITOR_PORT"] != "61460" {
		t.Errorf("per-endpoint port bindings wrong: %+v", env)
	}
}

func TestResourceTargetKey(t *testing.T) {
	if got := ResourceTargetKey("tc-broker", "monitor"); got != "res/tc-broker/monitor" {
		t.Errorf("ResourceTargetKey = %q", got)
	}
}

func TestComposeAddress(t *testing.T) {
	cases := []struct {
		scheme, basePath, want string
	}{
		{"http", "/api", "http://127.0.0.1:8080/api"},
		{"http", "api", "http://127.0.0.1:8080/api"}, // leading slash ensured
		{"https", "", "https://127.0.0.1:8080"},
		{"grpc", "", "127.0.0.1:8080"},    // no scheme:// prefix
		{"tcp", "/x", "127.0.0.1:8080/x"}, // path appended for all schemes (matches DP)
	}
	for _, c := range cases {
		if got := ComposeAddress(c.scheme, localHostStr, 8080, c.basePath); got != c.want {
			t.Errorf("ComposeAddress(%q,%q) = %q, want %q", c.scheme, c.basePath, got, c.want)
		}
	}
}
