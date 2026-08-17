// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package local

// ConnectParams are the inputs to `occ local`.
type ConnectParams struct {
	// WorkloadPaths are paths to one or more local workload.yaml files. When an
	// endpoint dependency declared by one workload matches another workload's own
	// (namespace, project, component) identity, it is wired directly to a local
	// host:port instead of being tunneled through the control plane.
	WorkloadPaths []string
	// Namespace is the control-plane namespace (org). Falls back to each workload
	// file's metadata.namespace when set there.
	Namespace string
	// Environment is the target environment to resolve dependencies for.
	Environment string
	// LocalOverrides overrides the local host:port for a cross-linked endpoint
	// dependency, keyed by the provider's component name. Absent entries default to
	// 127.0.0.1:<the provider's own declared endpoint port>.
	LocalOverrides map[string]LocalTarget
	// PrintEnv, when set, prints the resolved env bindings and holds the tunnels open
	// instead of spawning a subshell.
	PrintEnv bool
}

// LocalTarget is a local host:port a cross-linked dependency should point at directly.
type LocalTarget struct {
	Host string
	Port int
}
