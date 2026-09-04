// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteconnect

import (
	"strconv"
	"strings"
)

// ResolveRequest is what occ sends to the control plane's remote-connect resolve
// endpoint. It is built from the local workload.yaml: the consuming
// component's identity plus its declared dependencies, resolved for one environment.
type ResolveRequest struct {
	// Namespace is the control-plane namespace (org) the component lives in.
	Namespace   string        `json:"namespace"`
	Project     string        `json:"project"`
	Component   string        `json:"component"`
	Environment string        `json:"environment"`
	Endpoints   []EndpointDep `json:"endpoints,omitempty"`
}

// EndpointDep mirrors a workload endpoint dependency (spec.dependencies.endpoints[]).
type EndpointDep struct {
	Project     string              `json:"project,omitempty"`
	Component   string              `json:"component"`
	Name        string              `json:"name"`
	Visibility  string              `json:"visibility"`
	EnvBindings EndpointEnvBindings `json:"envBindings"`
}

// EndpointEnvBindings mirrors ConnectionEnvBindings: the env var NAMES the app expects
// for each resolved address component.
type EndpointEnvBindings struct {
	Address  string `json:"address,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     string `json:"port,omitempty"`
	BasePath string `json:"basePath,omitempty"`
}

// ResolveResponse is the control plane's reply: how occ renders local env + connects,
// the CP-signed capability occ presents to the remote-agents, and the set of remote-agents occ
// dials directly. Each target names (AgentID) the remote-agent that serves it; the control
// plane provisions one remote-agent per provider project+env data-plane namespace on resolve,
// so a workload's dependencies fan out to one agent per provider namespace.
type ResolveResponse struct {
	Capability    string           `json:"capability"`
	Targets       []ResolvedTarget `json:"targets"`
	Unconnectable []Unconnectable  `json:"unconnectable,omitempty"`

	// Agents maps an agent ID (the provider's data-plane namespace) to the remote-agent occ
	// dials for the targets that name it. Each target's AgentID indexes this map. Empty
	// when there is nothing tunnellable, in which case occ opens no tunnels.
	Agents map[string]AgentEndpoint `json:"agents,omitempty"`
}

// AgentEndpoint is one remote-agent occ dials over TLS (layering yamux on top). With the
// shared SNI router, Endpoint is the same router address for every agent in a data plane;
// ServerName (the agent's SNI) and CABundle (the agent's own self-signed cert) are what
// distinguish and pin each agent.
type AgentEndpoint struct {
	// Endpoint is the "host:port" occ dials — the shared remote-connect SNI router.
	Endpoint string `json:"endpoint"`
	// CABundle is the PEM-encoded certificate occ pins for this agent. The agent presents
	// its own self-signed certificate, so occ trusts exactly this bundle. Empty means use
	// system roots.
	CABundle string `json:"caBundle,omitempty"`
	// ServerName is the SNI occ sends (routing it to this agent at the router) and the SAN
	// occ verifies the agent's certificate against.
	ServerName string `json:"serverName"`
}

// ResolvedTarget is one tunnellable dependency. Key matches the capability target key
// occ passes to OpenStream; the concrete host:port lives (signed) in the capability.
type ResolvedTarget struct {
	Key      string          `json:"key"`
	Proto    string          `json:"proto"` // "tcp"
	Endpoint *EndpointRender `json:"endpoint,omitempty"`
	// AgentID indexes ResolveResponse.Agents: the remote-agent occ opens this target's
	// streams against (the provider project+env data-plane namespace).
	AgentID string `json:"agentID,omitempty"`
}

// EndpointRender carries what occ needs to render endpoint env against a local port.
type EndpointRender struct {
	Scheme   string              `json:"scheme"`
	BasePath string              `json:"basePath,omitempty"`
	Bindings EndpointEnvBindings `json:"bindings"`
}

// Unconnectable reports a declared dependency that cannot be tunneled in v1.
type Unconnectable struct {
	Ref    string `json:"ref"`
	Reason string `json:"reason"`
}

// EndpointTargetKey identifies one endpoint dependency's stream, and is the key the
// remote-agent looks up in the capability to find its dial target. The provider project is
// part of it because two projects may each own a component of the same name — without it
// both dependencies collapse to one key and the lookup silently returns the wrong host.
// occ and the control plane must agree exactly, hence the shared helper.
func EndpointTargetKey(project, component, endpoint string) string {
	return "ep/" + project + "/" + component + "/" + endpoint
}

// RenderEnv produces the environment variables for a target, pointing the app at the
// local listener (localHost:localPort) instead of the real dependency address.
func RenderEnv(t ResolvedTarget, localHost string, localPort int) map[string]string {
	out := map[string]string{}
	if t.Endpoint == nil {
		return out
	}
	lp := strconv.Itoa(localPort)
	b := t.Endpoint.Bindings
	if b.Host != "" {
		out[b.Host] = localHost
	}
	if b.Port != "" {
		out[b.Port] = lp
	}
	if b.BasePath != "" {
		out[b.BasePath] = t.Endpoint.BasePath
	}
	if b.Address != "" {
		out[b.Address] = ComposeAddress(t.Endpoint.Scheme, localHost, localPort, t.Endpoint.BasePath)
	}
	return out
}

// ComposeAddress builds the connection string for an endpoint's `address` binding,
// mirroring the data plane's formatEndpointAddress: a scheme:// prefix only for
// http/https/ws/wss/tls, then host, then :port (when non-zero), then basePath (with a
// leading slash ensured) — for every scheme. Keeping this identical to the controller
// means the local `address` env var has the same shape the app sees in the cluster.
func ComposeAddress(scheme, host string, port int, basePath string) string {
	var sb strings.Builder
	if schemeUsesURLFormat(scheme) {
		sb.WriteString(scheme)
		sb.WriteString("://")
	}
	sb.WriteString(host)
	if port != 0 {
		sb.WriteString(":")
		sb.WriteString(strconv.Itoa(port))
	}
	if basePath != "" {
		if !strings.HasPrefix(basePath, "/") {
			sb.WriteString("/")
		}
		sb.WriteString(basePath)
	}
	return sb.String()
}

func schemeUsesURLFormat(scheme string) bool {
	switch scheme {
	case "http", "https", "ws", "wss", "tls":
		return true
	default:
		return false
	}
}
