// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	k8syaml "sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// localHost is where every tunnel terminates, which is what lets one host binding
// serve several endpoints of the same resource.
const localHost = "127.0.0.1"

// tunnel is one yamux session to a project+env remote-agent; each accepted local
// connection opens a stream on it for a resolved target key.
type tunnel interface {
	OpenStream(key string) (net.Conn, error)
	// Fetch resolves one value-fetch key to the bytes the remote-agent read from its own
	// namespace. Separate from OpenStream because a fetch stream is a single
	// request/response, not a byte pipe.
	Fetch(key string) ([]byte, error)
	Close() error
}

// Remote implements the `remote` command logic.
type Remote struct {
	resolver Resolver
	// dialTunnel opens one yamux tunnel to a single remote-agent, presenting capability in
	// the handshake; called once per distinct agent a workload's targets fan out to.
	// Overridable in tests.
	dialTunnel func(ctx context.Context, agent remoteconnect.AgentEndpoint, capability string) (tunnel, error)
	// runShell spawns the subshell with the given environment; overridable in tests.
	runShell func(ctx context.Context, env []string) error
}

// New builds a Remote with production defaults.
func New(resolver Resolver) *Remote {
	return &Remote{
		resolver: resolver,
		dialTunnel: func(ctx context.Context, agent remoteconnect.AgentEndpoint, capability string) (tunnel, error) {
			return dialRemoteAgentTunnel(ctx, agent, capability)
		},
		runShell: runInteractiveShell,
	}
}

// workloadIdentity identifies a workload's owning component within a namespace, the
// key cross-workload dependency matching is done against.
type workloadIdentity struct {
	namespace string
	project   string
	component string
}

// localLink is an endpoint dependency that resolved to another workload passed on the
// same `occ remote` invocation, wired directly to a local host:port instead of being
// tunneled through the control plane.
type localLink struct {
	key         string // matches the server's "ep/<component>/<name>" key convention
	component   string // provider component name; looked up in ConnectParams.LocalOverrides
	envBindings remoteconnect.EndpointEnvBindings
	scheme      string
	basePath    string
	defaultPort int
}

func (l localLink) target(overrides map[string]LocalTarget) (string, int) {
	if t, ok := overrides[l.component]; ok {
		return t.Host, t.Port
	}
	return "127.0.0.1", l.defaultPort
}

func (l localLink) resolvedTarget() remoteconnect.ResolvedTarget {
	return remoteconnect.ResolvedTarget{
		Key:   l.key,
		Proto: "tcp",
		Endpoint: &remoteconnect.EndpointRender{
			Scheme:   l.scheme,
			BasePath: l.basePath,
			Bindings: l.envBindings,
		},
	}
}

// Connect resolves each workload's dependencies, opens a local listener per tunnellable
// remote target, wires any dependency on another of the given workloads straight to a
// local host:port, renders the merged env bindings, and spawns a subshell (or prints the
// env with --print-env). Tunnels live until the subshell exits or ctx is cancelled.
func (d *Remote) Connect(ctx context.Context, p ConnectParams, out io.Writer) error {
	if len(p.WorkloadPaths) == 0 {
		return fmt.Errorf("at least one workload is required")
	}
	if p.Environment == "" {
		return fmt.Errorf("--env is required")
	}

	workloads := make([]*v1alpha1.Workload, 0, len(p.WorkloadPaths))
	byIdentity := make(map[workloadIdentity]*v1alpha1.Workload, len(p.WorkloadPaths))
	for _, path := range p.WorkloadPaths {
		wl, err := loadWorkloadFromFile(path)
		if err != nil {
			return err
		}
		namespace, err := workloadNamespace(wl, p.Namespace)
		if err != nil {
			return err
		}
		id := workloadIdentity{namespace: namespace, project: wl.Spec.Owner.ProjectName, component: wl.Spec.Owner.ComponentName}
		if existing, dup := byIdentity[id]; dup {
			return fmt.Errorf("duplicate workload for %s/%s/%s: %s and %s",
				namespace, id.project, id.component, existing.Spec.Owner.ComponentName, path)
		}
		byIdentity[id] = wl
		workloads = append(workloads, wl)
	}

	overrides := map[string]string{}
	// Names of env vars whose values were fetched from the data plane. Tracked so
	// --print-env can redact them rather than writing credentials to the terminal.
	sensitive := map[string]bool{}
	var listeners []net.Listener
	var tunnels []tunnel
	// Fetched file bindings live here for the life of the session. The cleanup below
	// runs on every return path — including the ctx-cancelled one that Ctrl-C takes —
	// so credentials written to disk do not outlive the tunnels that fetched them.
	files := newFileStore()
	defer func() {
		for _, ln := range listeners {
			_ = ln.Close()
		}
		for _, tn := range tunnels {
			_ = tn.Close()
		}
		files.cleanup()
	}()

	for _, wl := range workloads {
		namespace, err := workloadNamespace(wl, p.Namespace)
		if err != nil {
			return err
		}
		componentName := wl.Spec.Owner.ComponentName
		remoteEndpoints, links := splitDependencies(wl, namespace, byIdentity)

		fmt.Fprintf(out, "Connecting to %s/%s (%s)...\n", wl.Spec.Owner.ProjectName, componentName, p.Environment)

		hasResources := wl.Spec.Dependencies != nil && len(wl.Spec.Dependencies.Resources) > 0
		if len(remoteEndpoints) > 0 || hasResources {
			req := buildResolveRequest(wl, namespace, p.Environment, remoteEndpoints)
			resp, err := d.resolver.Resolve(ctx, req)
			if err != nil {
				return err
			}

			// Per resource, the in-cluster address each of its endpoints resolved to and
			// the local listener that now stands in for it.
			localAddrs := map[string][]addrSwap{}
			// Hoisted out of the target loop: fetching values needs the same tunnels the
			// listeners use, and a resource with no endpoint at all still needs one.
			agentTunnels := make(map[string]tunnel, len(resp.Agents))
			for id, agent := range resp.Agents {
				tn, terr := d.dialTunnel(ctx, agent, resp.Capability)
				if terr != nil {
					return terr
				}
				tunnels = append(tunnels, tn)
				agentTunnels[id] = tn
			}
			if len(resp.Targets) > 0 {
				// Route each target's streams to its own agent; same-namespace
				// dependencies share a tunnel.
				reporter := newStreamErrorReporter(out, resp.Capability)
				for _, t := range resp.Targets {
					tn, ok := agentTunnels[t.AgentID]
					if !ok {
						return fmt.Errorf("resolve returned no remote-agent %q for target %s", t.AgentID, t.Key)
					}

					ln, lerr := net.Listen("tcp", net.JoinHostPort(localHost, "0"))
					if lerr != nil {
						return fmt.Errorf("open local listener for %s: %w", t.Key, lerr)
					}
					listeners = append(listeners, ln)
					port := ln.Addr().(*net.TCPAddr).Port

					key := t.Key
					open := func() (net.Conn, error) { return tn.OpenStream(key) }
					go forward(ln, key, open, reporter.report)

					mergeOverrides(overrides, out, remoteconnect.RenderEnv(t, localHost, port))
					if t.Resource != nil && t.Resource.RemoteAddr != "" {
						localAddrs[t.Resource.Ref] = append(localAddrs[t.Resource.Ref],
							newAddrSwap(t.Resource.RemoteAddr, strconv.Itoa(port)))
					}
					fmt.Fprintf(out, "  %-28s -> %s:%d  (%s)\n", t.Key, localHost, port, targetKind(t))
				}
			}
			applyResourceBindings(overrides, out, resp, localAddrs)
			// After the tunnels: a fetched value travels over one, so this cannot run
			// before they are up.
			fetchBindings(overrides, sensitive, out, resp, agentTunnels, files, localAddrs, p.NoSecrets)
			// After both merges: an env var naming a mount path may have come from
			// StaticEnv or from a fetch, so the repoint must see the finished map.
			repointFilePaths(overrides, out, files)
			for _, u := range resp.Unconnectable {
				fmt.Fprintf(out, "  ! %s: %s\n", u.Ref, u.Reason)
			}
		}

		for _, link := range links {
			host, port := link.target(p.LocalOverrides)
			mergeOverrides(overrides, out, remoteconnect.RenderEnv(link.resolvedTarget(), host, port))
			fmt.Fprintf(out, "  %-28s -> %s:%d  (local)\n", link.key, host, port)
		}
	}

	if p.PrintEnv {
		printEnvBindings(out, overrides, sensitive)
		fmt.Fprintln(out, "\nTunnels open. Press Ctrl-C to disconnect.")
		<-ctx.Done()
		return nil
	}

	return d.runShell(ctx, mergeEnv(os.Environ(), overrides))
}

// workloadNamespace resolves a workload's effective namespace: its own
// metadata.namespace if set, else fallback (--namespace).
func workloadNamespace(wl *v1alpha1.Workload, fallback string) (string, error) {
	if wl.Namespace != "" {
		return wl.Namespace, nil
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("namespace is required: set metadata.namespace in the workload file or pass --namespace")
}

// splitDependencies partitions wl's declared endpoint dependencies into those that
// resolve to another workload passed on this invocation (localLinks) and those that
// still need remote resolution against the control plane.
func splitDependencies(wl *v1alpha1.Workload, namespace string, byIdentity map[workloadIdentity]*v1alpha1.Workload) (remote []v1alpha1.WorkloadConnection, links []localLink) {
	deps := wl.Spec.Dependencies
	if deps == nil {
		return nil, nil
	}
	consumerProject := wl.Spec.Owner.ProjectName
	for _, e := range deps.Endpoints {
		providerProject := e.Project
		if providerProject == "" {
			providerProject = consumerProject
		}
		id := workloadIdentity{namespace: namespace, project: providerProject, component: e.Component}
		if provider, ok := byIdentity[id]; ok {
			if ep, epOK := provider.Spec.Endpoints[e.Name]; epOK {
				links = append(links, localLink{
					key:       remoteconnect.EndpointTargetKey(providerProject, e.Component, e.Name),
					component: e.Component,
					envBindings: remoteconnect.EndpointEnvBindings{
						Address:  e.EnvBindings.Address,
						Host:     e.EnvBindings.Host,
						Port:     e.EnvBindings.Port,
						BasePath: e.EnvBindings.BasePath,
					},
					scheme:      schemeForEndpointType(ep.Type),
					basePath:    ep.BasePath,
					defaultPort: int(ep.Port),
				})
				continue
			}
			// Matched component but not the named endpoint - fall through to remote
			// resolution, which surfaces a clear "endpoint not found" Unconnectable.
		}
		remote = append(remote, e)
	}
	return remote, links
}

// schemeForEndpointType mirrors the control plane's endpoint-type -> URL scheme
// mapping (internal/controller/releasebinding's schemeForEndpointType) so a local link
// renders an `address` binding identically to what a remote resolve would have produced.
func schemeForEndpointType(t v1alpha1.EndpointType) string {
	switch t {
	case v1alpha1.EndpointTypeHTTP, v1alpha1.EndpointTypeGraphQL:
		return "http"
	case v1alpha1.EndpointTypeWebsocket:
		return "ws"
	case v1alpha1.EndpointTypeGRPC:
		return "grpc"
	case v1alpha1.EndpointTypeTCP:
		return "tcp"
	case v1alpha1.EndpointTypeUDP:
		return "udp"
	default:
		return "http"
	}
}

// mergeOverrides copies src into dst, warning when a key is already set to a different
// value by an earlier workload in this invocation.
func mergeOverrides(dst map[string]string, out io.Writer, src map[string]string) {
	for k, v := range src {
		if existing, ok := dst[k]; ok && existing != v {
			fmt.Fprintf(out, "  ! warning: %s set by multiple workloads (%q vs %q); using %q\n", k, existing, v, v)
		}
		dst[k] = v
	}
}

// forward accepts local connections and pipes each over a fresh yamux stream to the
// remote-agent (opened via open).
func forward(ln net.Listener, key string, open func() (net.Conn, error), report func(string, error)) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return // listener closed
		}
		go func(local net.Conn) {
			stream, serr := open()
			if serr != nil {
				// Without this the app saw only a connection reset, which made an expired
				// session look like the dependency had gone away.
				report(key, serr)
				_ = local.Close()
				return
			}
			remoteconnect.Pipe(local, stream)
		}(conn)
	}
}

// streamErrorReporter prints the first failure for each dependency. A dependency that
// fails once usually fails for every subsequent connection, so repeating it would bury
// the session in noise.
type streamErrorReporter struct {
	out     io.Writer
	expiry  time.Time
	mu      sync.Mutex
	printed map[string]bool
}

func newStreamErrorReporter(out io.Writer, capability string) *streamErrorReporter {
	r := &streamErrorReporter{out: out, printed: map[string]bool{}}
	if exp, ok := remoteconnect.CapabilityExpiry(capability); ok {
		r.expiry = exp
	}
	return r
}

func (r *streamErrorReporter) report(key string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.printed[key] {
		return
	}
	r.printed[key] = true
	if !r.expiry.IsZero() && time.Now().After(r.expiry) {
		fmt.Fprintf(r.out, "  ! %s: session expired at %s — exit and re-run `occ remote` to reconnect\n",
			key, r.expiry.Local().Format(time.Kitchen))
		return
	}
	fmt.Fprintf(r.out, "  ! %s: %v\n", key, err)
}

func targetKind(t remoteconnect.ResolvedTarget) string {
	if t.Resource != nil {
		return "resource/" + t.Resource.Endpoint
	}
	return "endpoint"
}

// applyResourceBindings merges the env each resource dependency contributes directly:
// values the control plane resolved itself, re-pointed at the local listeners where they
// embed a tunneled address, plus a report of the bindings that could not be resolved at
// all. Values behind a Secret/ConfigMap reference are not here — they are fetched over
// the tunnel by fetchBindings. Resources with no endpoint at all are included, which is
// what lets a dependency with nothing to dial still supply its configuration.
func applyResourceBindings(
	overrides map[string]string,
	out io.Writer,
	resp *remoteconnect.ResolveResponse,
	localAddrs map[string][]addrSwap,
) {
	tunneledRefs := make(map[string]bool, len(resp.Targets))
	for _, t := range resp.Targets {
		if t.Resource != nil {
			tunneledRefs[t.Resource.Ref] = true
		}
	}
	for _, rb := range resp.Resources {
		mergeOverrides(overrides, out, rewriteAddrs(rb.StaticEnv, localAddrs[rb.Ref],
			remoteconnect.ResourceRefKey(rb.Ref), out))
		if !tunneledRefs[rb.Ref] {
			// Either the type declares nothing dialable, or the binding is still pinned
			// to a ResourceRelease cut before it did. Either way the addresses below are
			// the in-cluster ones as published, which this machine may not resolve.
			fmt.Fprintf(out, "  %-28s (no endpoint tunneled; %d binding(s) resolved as published in-cluster)\n",
				remoteconnect.ResourceRefKey(rb.Ref), len(rb.StaticEnv))
		}
		for _, om := range rb.OmittedSecretEnv {
			what := "value"
			if om.File {
				what = "file"
			}
			fmt.Fprintf(out, "  ! %s: %s %s not resolved (%s)\n",
				remoteconnect.ResourceRefKey(rb.Ref), om.Target, what, om.Reason)
		}
	}
}

// addrSwap pairs the in-cluster address an endpoint resolved to with the local
// listener that replaces it, split into parts so a composed value carrying the host
// and port at separate positions can be re-pointed too.
type addrSwap struct {
	remote     string // in-cluster "host:port"
	local      string // "127.0.0.1:<local port>"
	remoteHost string
	remotePort string
	localPort  string
}

// newAddrSwap splits an endpoint's in-cluster address so both the fused pair and its
// parts can be substituted. A remote address that will not split yields a swap that
// only ever matches as a whole.
func newAddrSwap(remote, localPort string) addrSwap {
	sw := addrSwap{
		remote:    remote,
		local:     net.JoinHostPort(localHost, localPort),
		localPort: localPort,
	}
	if host, port, err := net.SplitHostPort(remote); err == nil {
		sw.remoteHost, sw.remotePort = host, port
	}
	return sw
}

// splittable reports whether this swap can be applied to a host and a port held at
// separate positions in one value.
func (s addrSwap) splittable() bool {
	return s.remoteHost != "" && s.remotePort != ""
}

// rewriteAddrs re-points a resource's composed bindings at its tunnels, so a connection
// URL or driver connection string resolved for the cluster works from the developer's
// machine. Two shapes are handled, in order:
//
// Fused -- "redis://SVC:6379" -- has the host and port adjacent, so the pair is
// substituted as one string. Being that specific is what keeps the rewrite safe: it
// cannot match a host named with some other port, nor a bare hostname.
//
// Split -- "host=SVC,port=6379,password=x" -- holds the two apart, so they are
// substituted individually. That is only attempted for an endpoint whose fused pair is
// absent from the value, and only when the value carries both the host AND that
// endpoint's port as a delimited token. A value naming the host alone (a TLS server
// name) or with a different port (an admin URL) satisfies neither condition and is left
// as resolved, and reported. Each split rewrite is reported too, since substituting a
// bare host is the weaker inference of the two.
func rewriteAddrs(env map[string]string, swaps []addrSwap, ref string, out io.Writer) map[string]string {
	if len(env) == 0 || len(swaps) == 0 {
		return env
	}
	// Local listener ports, so a split rewrite never consumes a port an earlier rewrite
	// just produced. Ephemeral ports are assigned by the OS and could coincide with
	// another endpoint's in-cluster port; substituting into that would silently undo the
	// earlier endpoint's rewrite.
	localPorts := make(map[string]bool, len(swaps))
	for _, sw := range swaps {
		localPorts[sw.localPort] = true
	}

	rewritten := make(map[string]string, len(env))
	for k, v := range env {
		got := v
		for _, sw := range swaps {
			got = strings.ReplaceAll(got, sw.remote, sw.local)
		}
		// Decided against the original value, not the partially rewritten one: several
		// endpoints of a resource share a host output, so the first rewrite would
		// otherwise hide the host from the endpoints still to be applied.
		for _, sw := range swaps {
			if !sw.splittable() || strings.Contains(v, sw.remote) {
				continue
			}
			if !strings.Contains(v, sw.remoteHost) || !containsPortToken(v, sw.remotePort) {
				continue
			}
			if localPorts[sw.remotePort] {
				// This endpoint's in-cluster port is some endpoint's local port, so a
				// substitution here cannot be told apart from one already applied. Left
				// alone, and reported below as still pointing into the cluster.
				continue
			}
			got = strings.ReplaceAll(got, sw.remoteHost, localHost)
			got = replacePortToken(got, sw.remotePort, sw.localPort)
			fmt.Fprintf(out, "  ~ %s: %s had its host and port re-pointed separately\n", ref, k)
		}
		if got == v {
			// Unchanged, but mentions a tunneled host: the value embeds the host with
			// some other port, so it still points into the cluster.
			for _, sw := range swaps {
				if sw.remoteHost != "" && strings.Contains(v, sw.remoteHost) {
					fmt.Fprintf(out, "  ! %s: %s still points at %s and was not re-pointed at a tunnel\n", ref, k, sw.remoteHost)
					break
				}
			}
		}
		rewritten[k] = got
	}
	return rewritten
}

// containsPortToken reports whether port appears in s as a standalone token.
func containsPortToken(s, port string) bool {
	return findPortToken(s, port, 0) >= 0
}

// replacePortToken replaces every standalone occurrence of port in s with local.
func replacePortToken(s, port, local string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		j := findPortToken(s, port, i)
		if j < 0 {
			b.WriteString(s[i:])
			break
		}
		b.WriteString(s[i:j])
		b.WriteString(local)
		i = j + len(port)
	}
	return b.String()
}

// findPortToken returns the index of the first standalone occurrence of port in s at or
// after from, or -1. Standalone means not flanked by a character that could belong to a
// hostname or a longer number, so the 6379 in "cache6379.ns" or "63790" is not a match.
func findPortToken(s, port string, from int) int {
	for i := from; ; {
		j := strings.Index(s[i:], port)
		if j < 0 {
			return -1
		}
		j += i
		if !isAddrChar(s, j-1) && !isAddrChar(s, j+len(port)) {
			return j
		}
		i = j + 1
	}
}

// isAddrChar reports whether s[i] is a character a hostname or number can be made of.
// An index outside s is a delimiter, not a character.
func isAddrChar(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	c := s[i]
	return c == '.' || c == '-' || c == '_' ||
		('0' <= c && c <= '9') || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

// loadWorkloadFromFile reads a YAML file and returns its Workload document.
func loadWorkloadFromFile(path string) (*v1alpha1.Workload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workload file: %w", err)
	}
	for _, doc := range splitYAMLDocs(data) {
		var probe struct {
			Kind string `json:"kind"`
		}
		if err := k8syaml.Unmarshal(doc, &probe); err != nil {
			continue
		}
		if probe.Kind != "Workload" {
			continue
		}
		var wl v1alpha1.Workload
		if err := k8syaml.Unmarshal(doc, &wl); err != nil {
			return nil, fmt.Errorf("parse Workload: %w", err)
		}
		return &wl, nil
	}
	return nil, fmt.Errorf("no Workload document found in %s", path)
}

// splitYAMLDocs splits a multi-document YAML byte slice on `---` separators.
func splitYAMLDocs(data []byte) [][]byte {
	var docs [][]byte
	for part := range bytes.SplitSeq(data, []byte("\n---")) {
		if trimmed := bytes.TrimSpace(part); len(trimmed) > 0 {
			docs = append(docs, trimmed)
		}
	}
	return docs
}

// buildResolveRequest maps a Workload's declared dependencies into a ResolveRequest.
// endpoints is the subset of the workload's declared endpoint dependencies that still
// need remote resolution (cross-linked ones are excluded by the caller).
func buildResolveRequest(wl *v1alpha1.Workload, namespace, env string, endpoints []v1alpha1.WorkloadConnection) remoteconnect.ResolveRequest {
	req := remoteconnect.ResolveRequest{
		Namespace:   namespace,
		Project:     wl.Spec.Owner.ProjectName,
		Component:   wl.Spec.Owner.ComponentName,
		Environment: env,
	}
	for _, e := range endpoints {
		req.Endpoints = append(req.Endpoints, remoteconnect.EndpointDep{
			Project:    e.Project,
			Component:  e.Component,
			Name:       e.Name,
			Visibility: e.Visibility,
			EnvBindings: remoteconnect.EndpointEnvBindings{
				Address:  e.EnvBindings.Address,
				Host:     e.EnvBindings.Host,
				Port:     e.EnvBindings.Port,
				BasePath: e.EnvBindings.BasePath,
			},
		})
	}
	if wl.Spec.Dependencies != nil {
		for _, r := range wl.Spec.Dependencies.Resources {
			req.Resources = append(req.Resources, remoteconnect.ResourceDep{
				Ref:          r.Ref,
				EnvBindings:  r.EnvBindings,
				FileBindings: r.FileBindings,
			})
		}
	}
	return req
}

// mergeEnv overlays overrides onto a base environment ("KEY=VALUE" slice).
func mergeEnv(base []string, overrides map[string]string) []string {
	merged := make(map[string]string, len(base)+len(overrides))
	for _, kv := range base {
		if k, v, ok := strings.Cut(kv, "="); ok {
			merged[k] = v
		}
	}
	maps.Copy(merged, overrides)
	out := make([]string, 0, len(merged))
	for k, v := range merged {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// printEnvBindings lists the resolved bindings for --print-env. Values fetched from the
// data plane are redacted: they are the same credentials the cluster keeps in a Secret,
// and --print-env output lands in a terminal, scrollback, and often a pasted bug report.
// The name is still shown so the developer can see the binding was resolved.
func printEnvBindings(out io.Writer, env map[string]string, sensitive map[string]bool) {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintln(out, "\nEnvironment bindings:")
	for _, k := range keys {
		if sensitive[k] {
			fmt.Fprintf(out, "  export %s=<hidden; set in the subshell>\n", k)
			continue
		}
		fmt.Fprintf(out, "  export %s=%s\n", k, env[k])
	}
}

func runInteractiveShell(ctx context.Context, env []string) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.CommandContext(ctx, shell)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Put the shell in its own process group and make it the terminal's foreground group
	// (Unix) so it can read stdin without blocking on SIGTTIN. occ does not reap the
	// shell's background jobs — that lifecycle belongs to the shell, which warns on exit
	// if jobs are still running and SIGHUPs them.
	setSubshellProcessGroup(cmd)
	err := cmd.Run()
	// The subshell held the terminal foreground; reclaim it before occ returns so the
	// parent shell isn't handed a background terminal. Background jobs the user started
	// are the shell's to manage — zsh/bash SIGHUP their jobs on exit.
	restoreTerminalForeground()
	return err
}
