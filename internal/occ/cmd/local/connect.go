// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package local

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
	"strings"
	"sync"
	"time"

	k8syaml "sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/depconnect"
)

// tunnel is one yamux session to a project+env dep-agent; each accepted local
// connection opens a stream on it for a resolved target key.
type tunnel interface {
	OpenStream(key string) (net.Conn, error)
	Close() error
}

// Dev implements the `local` command logic.
type Dev struct {
	resolver Resolver
	// dialTunnel opens one yamux tunnel to a single dep-agent, presenting capability in
	// the handshake; called once per distinct agent a workload's targets fan out to.
	// Overridable in tests.
	dialTunnel func(ctx context.Context, agent depconnect.AgentEndpoint, capability string) (tunnel, error)
	// runShell spawns the subshell with the given environment; overridable in tests.
	runShell func(ctx context.Context, env []string) error
}

// New builds a Dev with production defaults.
func New(resolver Resolver) *Dev {
	return &Dev{
		resolver: resolver,
		dialTunnel: func(ctx context.Context, agent depconnect.AgentEndpoint, capability string) (tunnel, error) {
			return dialDepAgentTunnel(ctx, agent, capability)
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
// same `occ local` invocation, wired directly to a local host:port instead of being
// tunneled through the control plane.
type localLink struct {
	key         string // matches the server's "ep/<component>/<name>" key convention
	component   string // provider component name; looked up in ConnectParams.LocalOverrides
	envBindings depconnect.EndpointEnvBindings
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

func (l localLink) resolvedTarget() depconnect.ResolvedTarget {
	return depconnect.ResolvedTarget{
		Key:   l.key,
		Proto: "tcp",
		Endpoint: &depconnect.EndpointRender{
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
func (d *Dev) Connect(ctx context.Context, p ConnectParams, out io.Writer) error {
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
	var listeners []net.Listener
	var tunnels []tunnel
	defer func() {
		for _, ln := range listeners {
			_ = ln.Close()
		}
		for _, tn := range tunnels {
			_ = tn.Close()
		}
	}()

	for _, wl := range workloads {
		namespace, err := workloadNamespace(wl, p.Namespace)
		if err != nil {
			return err
		}
		componentName := wl.Spec.Owner.ComponentName
		remoteEndpoints, links := splitDependencies(wl, namespace, byIdentity)

		fmt.Fprintf(out, "Connected to %s/%s (%s):\n", wl.Spec.Owner.ProjectName, componentName, p.Environment)

		if len(remoteEndpoints) > 0 {
			req := buildResolveRequest(wl, namespace, p.Environment, remoteEndpoints)
			resp, err := d.resolver.Resolve(ctx, req)
			if err != nil {
				return err
			}

			if len(resp.Targets) > 0 {
				// Open one tunnel per distinct dep-agent the targets fan out to (one per
				// provider project+env namespace), then route each target's streams to
				// its agent. Same-namespace dependencies share a tunnel.
				reporter := newStreamErrorReporter(out, resp.Capability)
				agentTunnels := make(map[string]tunnel, len(resp.Agents))
				for id, agent := range resp.Agents {
					tn, terr := d.dialTunnel(ctx, agent, resp.Capability)
					if terr != nil {
						return terr
					}
					tunnels = append(tunnels, tn)
					agentTunnels[id] = tn
				}

				for _, t := range resp.Targets {
					tn, ok := agentTunnels[t.AgentID]
					if !ok {
						return fmt.Errorf("resolve returned no dep-agent %q for target %s", t.AgentID, t.Key)
					}

					ln, lerr := net.Listen("tcp", "127.0.0.1:0")
					if lerr != nil {
						return fmt.Errorf("open local listener for %s: %w", t.Key, lerr)
					}
					listeners = append(listeners, ln)
					port := ln.Addr().(*net.TCPAddr).Port

					key := t.Key
					open := func() (net.Conn, error) { return tn.OpenStream(key) }
					go forward(ln, key, open, reporter.report)

					mergeOverrides(overrides, out, depconnect.RenderEnv(t, "127.0.0.1", port))
					fmt.Fprintf(out, "  %-28s -> 127.0.0.1:%d  (endpoint)\n", t.Key, port)
				}
			}
			for _, u := range resp.Unconnectable {
				fmt.Fprintf(out, "  ! %s: %s\n", u.Ref, u.Reason)
			}
		}

		for _, link := range links {
			host, port := link.target(p.LocalOverrides)
			mergeOverrides(overrides, out, depconnect.RenderEnv(link.resolvedTarget(), host, port))
			fmt.Fprintf(out, "  %-28s -> %s:%d  (local)\n", link.key, host, port)
		}
	}

	if p.PrintEnv {
		printEnvBindings(out, overrides)
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
					key:       depconnect.EndpointTargetKey(providerProject, e.Component, e.Name),
					component: e.Component,
					envBindings: depconnect.EndpointEnvBindings{
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
const schemeHTTP = "http"

func schemeForEndpointType(t v1alpha1.EndpointType) string {
	switch t {
	case v1alpha1.EndpointTypeHTTP, v1alpha1.EndpointTypeGraphQL:
		return schemeHTTP
	case v1alpha1.EndpointTypeWebsocket:
		return "ws"
	case v1alpha1.EndpointTypeGRPC:
		return "grpc"
	case v1alpha1.EndpointTypeTCP:
		return "tcp"
	case v1alpha1.EndpointTypeUDP:
		return "udp"
	default:
		return schemeHTTP
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
// dep-agent (opened via open).
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
			depconnect.Pipe(local, stream)
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
	if exp, ok := depconnect.CapabilityExpiry(capability); ok {
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
		fmt.Fprintf(r.out, "  ! %s: session expired at %s — exit and re-run `occ local` to reconnect\n",
			key, r.expiry.Local().Format(time.Kitchen))
		return
	}
	fmt.Fprintf(r.out, "  ! %s: %v\n", key, err)
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
func buildResolveRequest(wl *v1alpha1.Workload, namespace, env string, endpoints []v1alpha1.WorkloadConnection) depconnect.ResolveRequest {
	req := depconnect.ResolveRequest{
		Namespace:   namespace,
		Project:     wl.Spec.Owner.ProjectName,
		Component:   wl.Spec.Owner.ComponentName,
		Environment: env,
	}
	for _, e := range endpoints {
		req.Endpoints = append(req.Endpoints, depconnect.EndpointDep{
			Project:    e.Project,
			Component:  e.Component,
			Name:       e.Name,
			Visibility: e.Visibility,
			EnvBindings: depconnect.EndpointEnvBindings{
				Address:  e.EnvBindings.Address,
				Host:     e.EnvBindings.Host,
				Port:     e.EnvBindings.Port,
				BasePath: e.EnvBindings.BasePath,
			},
		})
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

func printEnvBindings(out io.Writer, env map[string]string) {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintln(out, "\nEnvironment bindings:")
	for _, k := range keys {
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
