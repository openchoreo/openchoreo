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
	Resources   []ResourceDep `json:"resources,omitempty"`
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

// ResourceDep mirrors a workload resource dependency (spec.dependencies.resources[]).
type ResourceDep struct {
	Ref         string            `json:"ref"`
	EnvBindings map[string]string `json:"envBindings,omitempty"` // outputName -> env var name
	// FileBindings mirrors the workload's fileBindings: outputName -> the container path
	// the app reads the value from. The referenced output is always Secret- or
	// ConfigMap-backed (the API rejects mounting a plain value), so every entry needs a
	// data-plane read; occ materializes each into a session-scoped file.
	FileBindings map[string]string `json:"fileBindings,omitempty"`
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

	// Resources carries the env each resource dependency contributes that does not
	// depend on a tunnel. It is separate from Targets because a resource with no
	// declared endpoints still contributes env while producing no target at all.
	Resources []ResourceBindings `json:"resources,omitempty"`

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
	Resource *ResourceRender `json:"resource,omitempty"`
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

// ResourceRender carries what occ needs to point one resource endpoint's env vars at
// a local listener. HostEnv and PortEnv name the env vars the consuming workload bound
// to this endpoint's host and port outputs; each is set to the local tunnel address.
// One of these exists per declared endpoint, so a resource with two endpoints yields
// two targets and each rewrites only its own port binding.
type ResourceRender struct {
	// Ref is the resource dependency this endpoint belongs to, for display.
	Ref string `json:"ref"`
	// Endpoint is the endpoint name declared on the ResourceType.
	Endpoint string `json:"endpoint"`
	// HostEnv is the env var set to the local tunnel host, empty when the workload
	// did not bind this endpoint's host output.
	HostEnv string `json:"hostEnv,omitempty"`
	// PortEnv is the env var set to the local tunnel port, empty when the workload
	// did not bind this endpoint's port output.
	PortEnv string `json:"portEnv,omitempty"`

	// RemoteAddr is the "host:port" this endpoint resolves to inside the cluster. occ
	// replaces occurrences of it in the same resource's other bindings with the local
	// listener address, so a composed value -- a connection URL, a driver-specific
	// connection string -- points at the tunnel rather than an address the developer's
	// machine cannot resolve. Only the full host:port pair is matched, never a bare
	// host, so a value that legitimately names the host for another purpose (a TLS
	// server name, an admin URL on a different port) is left alone.
	RemoteAddr string `json:"remoteAddr,omitempty"`
}

// ResourceBindings is the tunnel-independent env a resource dependency contributes:
// values the control plane resolved directly, the reference-backed bindings occ must
// fetch over the tunnel, and the ones that could not be resolved at all. A resource
// that declares no endpoints contributes only this and opens no tunnel.
type ResourceBindings struct {
	Ref string `json:"ref"`
	// StaticEnv holds resolved plain values verbatim.
	StaticEnv map[string]string `json:"staticEnv,omitempty"`
	// FetchEnv maps an env var name to the capability key that authorizes fetching its
	// value from the data plane. The values are deliberately NOT in this response: they
	// travel occ <- remote-agent over the tunnel so that secret material never passes
	// through the control plane. occ resolves each key against the agent named by the
	// grant, then merges the results into the process environment.
	FetchEnv map[string]string `json:"fetchEnv,omitempty"`
	// FetchFile maps a container mount path to the capability key that authorizes
	// fetching its content, mirroring the workload's fileBindings. occ writes each
	// value into a session-scoped directory and points the app at it.
	FetchFile map[string]string `json:"fetchFile,omitempty"`
	// OmittedSecretEnv lists env vars whose value could not be made available, with the
	// reason. Reported to the developer rather than silently dropped, because an empty
	// env var is a worse failure than a named one.
	OmittedSecretEnv []OmittedBinding `json:"omittedSecretEnv,omitempty"`

	// FetchAgentID indexes ResolveResponse.Agents: the remote-agent holding this
	// resource's FetchEnv/FetchFile values. Set whenever either is non-empty.
	FetchAgentID string `json:"fetchAgentID,omitempty"`
}

// OmittedBinding is one binding that could not be resolved, and why. Target is the env
// var name, or the mount path for a file binding.
type OmittedBinding struct {
	Target string `json:"target"`
	Reason string `json:"reason"`
	// File distinguishes a mount path from an env var name, so occ can word its report
	// accurately without parsing Target.
	File bool `json:"file,omitempty"`
}

// Unconnectable reports a declared dependency that cannot be tunneled (e.g. a
// resource whose endpoint is embedded in a composite secret output).
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

// ResourceTargetKey identifies one resource endpoint's stream. The endpoint name is
// part of it because a resource type may declare several endpoints (a client port and a
// monitoring port, say) and the agent resolves a stream by key alone, so each endpoint
// needs its own. occ and the control plane must agree exactly, hence the shared helper.
func ResourceTargetKey(ref, endpoint string) string {
	return ResourceRefKey(ref) + "/" + endpoint
}

// ResourceRefKey identifies a whole resource dependency, for reporting a failure that
// is not specific to one endpoint (the resource is unavailable, or not authorized). It
// is the prefix of every ResourceTargetKey for that ref, so a reader sees the same
// "res/<ref>" identifier whether the problem was the resource or one of its endpoints.
func ResourceRefKey(ref string) string {
	return "res/" + ref
}

// SecretGrantKey identifies one fetch of one resource output's value. It shares the
// "<ref>" segment with ResourceRefKey so a reader sees which dependency a failed fetch
// belongs to, but sits in its own "sec/" space: the remote-agent selects between dialing
// and reading on this prefix, and the two spaces must never collide. occ and the
// control plane must agree exactly, hence the shared helper.
func SecretGrantKey(ref, output string) string {
	return "sec/" + ref + "/" + output
}

// IsSecretGrantKey reports whether key is in the fetch space. The remote-agent uses it to
// cross-check the control plane's answer against what it asked for: a key from one
// space answered with the other is a protocol violation, not a value to use.
func IsSecretGrantKey(key string) bool {
	return strings.HasPrefix(key, "sec/")
}

// RenderEnv produces the environment variables for a target, pointing the app at the
// local listener (localHost:localPort) instead of the real dependency address.
func RenderEnv(t ResolvedTarget, localHost string, localPort int) map[string]string {
	out := map[string]string{}
	lp := strconv.Itoa(localPort)
	switch {
	case t.Endpoint != nil:
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
	case t.Resource != nil:
		if t.Resource.HostEnv != "" {
			out[t.Resource.HostEnv] = localHost
		}
		if t.Resource.PortEnv != "" {
			out[t.Resource.PortEnv] = lp
		}
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
