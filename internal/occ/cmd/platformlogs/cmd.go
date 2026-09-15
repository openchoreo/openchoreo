// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package platformlogs

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

const shortDesc = "Query platform logs collected by this observability plane"

// longDesc explains what platform logs are and how they differ from the component logs
// the CLI already serves, since `<resource> logs` elsewhere in occ means the logs of
// that resource rather than the logs it holds.
const longDesc = `Query logs from OpenChoreo's own platform components - the control plane
(controller-manager, openchoreo-api, cluster-gateway), the data plane agents and gateways,
and the workflow and observability plane infrastructure.

This is the operator's view of the platform itself, not of user workloads. Entries are
addressed by raw Kubernetes coordinates rather than by project, component and environment,
so use 'occ component logs' for a deployed component's runtime logs.

Logs are read from the store held by the named observability plane; each plane holds its
own store, so --cluster selects between the clusters feeding this one. Multi-value filters
match any of their values, and different filters must all match.

Plane attribution is expressed through --selector, which OpenChoreo stamps on its own pods:
  openchoreo.dev/plane=controlplane|dataplane|workflowplane|observabilityplane
  openchoreo.dev/plane-id=<planeID>   narrows to one instance of a plane

The control plane is a singleton and carries no plane-id. Components OpenChoreo does not
ship carry no plane label at all, and are reached by --pod-namespace or their own labels.

Requires the cluster-scoped 'platformlogs:view' permission.`

const clusterPlaneExample = `  # Control plane components over the last 10 minutes
  occ clusterobservabilityplane logs default --selector openchoreo.dev/plane=controlplane --since 10m

  # A single container, followed
  occ cop logs default --pod-namespace openchoreo-control-plane --container manager -f

  # Errors from two clusters, as JSON
  occ cop logs default --cluster clusterX,clusterY --level ERROR -o json

  # One data plane instance
  occ cop logs default -l openchoreo.dev/plane=dataplane,openchoreo.dev/plane-id=eu-1`

const namespacedPlaneExample = `  # Control plane components over the last 10 minutes
  occ observabilityplane logs primary-observabilityplane --namespace acme-corp \
    --selector openchoreo.dev/plane=controlplane --since 10m

  # Errors from one pod, followed
  occ op logs primary-observabilityplane -n acme-corp --pod controller-manager-7f58b689b5-pwsb5 --level ERROR -f`

// NewClusterPlaneLogsCmd builds the `logs` subcommand of clusterobservabilityplane.
func NewClusterPlaneLogsCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "logs [CLUSTER_OBSERVABILITY_PLANE_NAME]",
		Short:   shortDesc,
		Long:    longDesc,
		Example: clusterPlaneExample,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			params := logsParams(cmd)
			params.PlaneKind = ClusterPlane
			params.PlaneName = args[0]
			return New(cl).Logs(params)
		},
	}
	addLogsFlags(cmd)
	return cmd
}

// NewNamespacedPlaneLogsCmd builds the `logs` subcommand of observabilityplane.
func NewNamespacedPlaneLogsCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "logs [OBSERVABILITYPLANE_NAME]",
		Short:   shortDesc,
		Long:    longDesc,
		Example: namespacedPlaneExample,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			params := logsParams(cmd)
			params.PlaneKind = NamespacedPlane
			params.PlaneName = args[0]
			params.Namespace = flags.GetNamespace(cmd)
			if err := cmdutil.RequireFields("logs", "observabilityplane", map[string]string{
				"namespace": params.Namespace,
			}); err != nil {
				return err
			}
			return New(cl).Logs(params)
		},
	}
	addLogsFlags(cmd)
	// The OpenChoreo namespace holding the plane resource, not the pod namespace the
	// logs are filtered by - that is --pod-namespace.
	flags.AddNamespace(cmd)
	return cmd
}

// addLogsFlags registers the filters both variants share.
func addLogsFlags(cmd *cobra.Command) {
	cmd.Flags().StringSlice("cluster", nil,
		"Clusters the entries were collected from, as named on each cluster's logs collector (comma-separated)")
	cmd.Flags().StringSlice("pod-namespace", nil,
		"Kubernetes namespaces of the pods, e.g. openchoreo-control-plane (comma-separated)")
	cmd.Flags().StringSlice("pod", nil, "Pod names (comma-separated)")
	// No short alias: `-c` denotes --component elsewhere in the CLI.
	cmd.Flags().StringSlice("container", nil, "Container names within the pods (comma-separated)")
	cmd.Flags().StringP("selector", "l", "",
		"Label selector over the pod labels, e.g. openchoreo.dev/plane=controlplane. Commas mean AND; equality-based selectors only")
	cmd.Flags().StringSlice("level", nil,
		"Log levels to include: DEBUG, INFO, WARN, ERROR (comma-separated)")
	cmd.Flags().String("search", "", "Only return entries whose message contains this text")
	cmd.Flags().StringP("output", "o", outputText,
		"Output format: 'text' or 'json' (json emits one entry per line)")
	flags.AddSince(cmd)
	flags.AddTail(cmd)
	flags.AddFollow(cmd)
}

// logsParams reads the shared filters off the command.
func logsParams(cmd *cobra.Command) LogsParams {
	clusters, _ := cmd.Flags().GetStringSlice("cluster")
	podNamespaces, _ := cmd.Flags().GetStringSlice("pod-namespace")
	pods, _ := cmd.Flags().GetStringSlice("pod")
	containers, _ := cmd.Flags().GetStringSlice("container")
	selector, _ := cmd.Flags().GetString("selector")
	levels, _ := cmd.Flags().GetStringSlice("level")
	search, _ := cmd.Flags().GetString("search")
	output, _ := cmd.Flags().GetString("output")

	return LogsParams{
		Clusters:      clusters,
		PodNamespaces: podNamespaces,
		Pods:          pods,
		Containers:    containers,
		Selector:      selector,
		Levels:        levels,
		Search:        search,
		Since:         flags.GetSince(cmd),
		Tail:          flags.GetTail(cmd),
		Follow:        flags.GetFollow(cmd),
		Output:        output,
	}
}
