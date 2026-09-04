// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package remote implements `occ remote` (tunnel one or more workloads' dependencies
// to the developer's machine).
package remote

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
)

// NewRemoteCmd builds the `remote` command.
func NewRemoteCmd() *cobra.Command {
	var (
		printEnv  bool
		noSecrets bool
		localArgs []string
	)

	cmd := &cobra.Command{
		Use:   "remote <workload.yaml> [workload.yaml...]",
		Short: "Tunnel one or more workloads' dependencies to your machine",
		Long: "Resolve each workload's declared dependencies for an environment, open a local " +
			"TCP listener for each remote one, and start a subshell whose environment points at " +
			"those listeners — so the app runs locally against the environment's real upstreams.\n\n" +
			"When multiple workload files are given and one declares an endpoint dependency on " +
			"another workload's own component, that dependency is wired directly to a local " +
			"host:port instead of being tunneled — see --local.\n\n" +
			"Values a dependency publishes through a Secret or ConfigMap are read in the data " +
			"plane and returned over the tunnel, so they never pass through the control plane. " +
			"They are placed in the subshell's environment (or, for file bindings, in a " +
			"session-scoped directory removed on exit) and never printed. Use --no-secrets to " +
			"skip fetching them.",
		Example: "  occ remote comp1/workload.yaml --env development\n" +
			"  occ remote comp1/workload.yaml comp2/workload.yaml --env development\n" +
			"  occ remote comp1/workload.yaml comp2/workload.yaml --local comp2=127.0.0.1:9091 --env development",
		Args:    cobra.MinimumNArgs(1),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			overrides, err := parseLocalOverrides(localArgs)
			if err != nil {
				return err
			}

			resolver, err := newHTTPResolver()
			if err != nil {
				return err
			}
			// Tunnels live for the session; Ctrl-C / SIGTERM tears everything down.
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return New(resolver).Connect(ctx, ConnectParams{
				WorkloadPaths:  args,
				Namespace:      flags.GetNamespace(cmd),
				Environment:    flags.GetEnvironment(cmd),
				LocalOverrides: overrides,
				PrintEnv:       printEnv,
				NoSecrets:      noSecrets,
			}, cmd.OutOrStdout())
		},
	}

	cmd.Flags().BoolVar(&printEnv, "print-env", false,
		"Print resolved env bindings and hold tunnels open instead of spawning a subshell")
	cmd.Flags().BoolVar(&noSecrets, "no-secrets", false,
		"Do not fetch secret- or configmap-backed dependency values; report them as "+
			"unresolved instead, so no credential enters the local process")
	cmd.Flags().StringArrayVar(&localArgs, "local", nil,
		"Override the local host:port for a cross-linked dependency's provider component "+
			"(component=host:port), e.g. --local comp2=127.0.0.1:9091. Repeatable. Defaults to "+
			"127.0.0.1:<the provider's declared endpoint port> when not given.")
	flags.AddNamespace(cmd)
	flags.AddEnvironment(cmd)
	return cmd
}

// parseLocalOverrides parses --local component=host:port values into a lookup keyed
// by component name.
func parseLocalOverrides(raw []string) (map[string]LocalTarget, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	overrides := make(map[string]LocalTarget, len(raw))
	for _, v := range raw {
		component, hostport, ok := strings.Cut(v, "=")
		if !ok || component == "" || hostport == "" {
			return nil, fmt.Errorf("invalid --local value %q: expected component=host:port", v)
		}
		host, portStr, err := net.SplitHostPort(hostport)
		if err != nil {
			return nil, fmt.Errorf("invalid --local value %q: %w", v, err)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid --local value %q: port must be numeric", v)
		}
		overrides[component] = LocalTarget{Host: host, Port: port}
	}
	return overrides, nil
}
