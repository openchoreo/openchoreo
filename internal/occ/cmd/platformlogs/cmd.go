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

// longDesc sets expectations this command's name does not. `<resource> logs` elsewhere in
// occ means the logs of that resource, whereas here the plane is the source; and the store
// it reads holds every container the plane's collector sees, not only OpenChoreo's own.
const longDesc = `Query logs by raw Kubernetes coordinates from everything the named observability
plane collects. That is OpenChoreo's own components - the control plane's controller-manager,
openchoreo-api and cluster-gateway, the data plane agents and gateways, the workflow and
observability plane infrastructure - and also the third-party infrastructure deployed
alongside them, and the workloads running on the planes it watches.

Entries come back with no ownership check, so this reads user workload logs as well. That
is why it needs the cluster-scoped 'platformlogs:view' permission, which is granted to the
platform-engineer and admin roles only. Treat the output as privileged, and use
'occ component logs' for a single component's runtime logs, correlated by project,
component and environment and checked against ownership.

Each observability plane holds its own store, so --cluster selects between the clusters
feeding this one. Multi-value filters match any of their values, and different filters
must all match.

Narrow to OpenChoreo's own components with --selector, which reads the labels OpenChoreo
stamps on its own pods:
  openchoreo.dev/plane=controlplane|dataplane|workflowplane|observabilityplane
  openchoreo.dev/plane-id=<planeID>   narrows to one instance of a plane

The control plane is a singleton and carries no plane-id. Everything OpenChoreo does not
ship - third-party infrastructure and user workloads alike - carries no plane label at all,
and is reached by --pod-namespace, --pod or its own labels.`

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
