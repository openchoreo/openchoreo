// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func newTrafficCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "traffic",
		Short: "Show and split an endpoint's traffic",
		Long: `Show and split an endpoint's traffic. The endpoint holds the default split, which ` +
			`reaches each environment with its release; an environment can also have its own split, ` +
			`which applies there right away.`,
	}
	cmd.AddCommand(newTrafficGetCmd(f), newTrafficSetCmd(f), newTrafficResetCmd(f))
	return cmd
}

func newTrafficGetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get [ENDPOINT_NAME]",
		Short:   "Show the default split and what each environment serves",
		Example: `  occ endpoint traffic get trips --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).TrafficGet(TrafficGetParams{Namespace: flags.GetNamespace(cmd), EndpointName: args[0]})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newTrafficSetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set [ENDPOINT_NAME] BACKEND=PERCENT...",
		Short: "Save the default split, or one environment's own split",
		Long: `Save a split as whole percentages. The shares of the backends that take one add up to 100; ` +
			`a mirror's value is the share of requests copied to it. Without --env this is the default ` +
			`split and cuts a release; with --env it is that environment's own split and applies now. ` +
			`A split equal to the release's stores nothing.`,
		Example: `  # Give trips-go 5% in development, taken from trips-python
  occ endpoint traffic set trips trips-python=45 trips-stale=50 trips-go=5 --env development

  # Copy 25% of requests to a mirror in staging
  occ endpoint traffic set trips trips-shadow=25 --env staging

  # The default split for every environment
  occ endpoint traffic set trips trips-python=50 trips-stale=50 trips-go=0`,
		Args:    cobra.MinimumNArgs(2),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			split, err := parseSplit(args[1:])
			if err != nil {
				return err
			}
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).TrafficSet(TrafficSetParams{
				Namespace:    flags.GetNamespace(cmd),
				EndpointName: args[0],
				Environment:  flags.GetEnvironment(cmd),
				Split:        split,
				DryRun:       flags.GetDryRun(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddEnvironment(cmd)
	flags.AddDryRun(cmd)
	return cmd
}

func newTrafficResetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "reset [ENDPOINT_NAME]",
		Short:   "Drop an environment's own split, so it follows the default",
		Example: `  occ endpoint traffic reset trips --env development`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).TrafficReset(TrafficResetParams{
				Namespace:    flags.GetNamespace(cmd),
				EndpointName: args[0],
				Environment:  flags.GetEnvironment(cmd),
				DryRun:       flags.GetDryRun(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddEnvironment(cmd)
	flags.AddDryRun(cmd)
	return cmd
}

// parseSplit reads BACKEND=PERCENT pairs; a trailing % is allowed.
func parseSplit(pairs []string) (map[string]int32, error) {
	split := map[string]int32{}
	for _, pair := range pairs {
		name, raw, ok := strings.Cut(pair, "=")
		v, err := strconv.ParseInt(strings.TrimSuffix(raw, "%"), 10, 32)
		if !ok || name == "" || err != nil || v < 0 || v > 100 {
			return nil, fmt.Errorf("expected BACKEND=PERCENT with a whole percentage from 0 to 100, got %q", pair)
		}
		split[name] = int32(v)
	}
	return split, nil
}
