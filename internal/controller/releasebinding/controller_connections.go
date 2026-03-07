// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	pipelinecontext "github.com/openchoreo/openchoreo/internal/pipeline/component/context"
)

// buildConnectionTargets extracts ConnectionTarget entries from workload connections.
// This is a pure function with no API calls.
func buildConnectionTargets(
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	connections []openchoreov1alpha1.WorkloadConnection,
) []openchoreov1alpha1.ConnectionTarget {
	if len(connections) == 0 {
		return nil
	}
	targets := make([]openchoreov1alpha1.ConnectionTarget, 0, len(connections))
	for _, conn := range connections {
		project := conn.Project
		if project == "" {
			project = releaseBinding.Spec.Owner.ProjectName
		}
		targets = append(targets, openchoreov1alpha1.ConnectionTarget{
			Namespace:   releaseBinding.Namespace,
			Project:     project,
			Component:   conn.Component,
			Endpoint:    conn.Name,
			Visibility:  conn.Visibility,
			Environment: releaseBinding.Spec.Environment,
		})
	}
	return targets
}

// resolveConnections resolves all connection targets by looking up dependency
// ReleaseBindings and extracting endpoint URLs from their status.
func (r *Reconciler) resolveConnections(
	ctx context.Context,
	targets []openchoreov1alpha1.ConnectionTarget,
) ([]openchoreov1alpha1.ResolvedConnection, []openchoreov1alpha1.PendingConnection, error) {
	if len(targets) == 0 {
		return nil, nil, nil
	}

	var resolved []openchoreov1alpha1.ResolvedConnection
	var pending []openchoreov1alpha1.PendingConnection

	for _, target := range targets {
		resolvedConn, pendingConn, err := r.resolveConnection(ctx, target)
		if err != nil {
			return nil, nil, err
		}
		if pendingConn != nil {
			pending = append(pending, *pendingConn)
		} else if resolvedConn != nil {
			resolved = append(resolved, *resolvedConn)
		}
	}

	return resolved, pending, nil
}

// resolveConnection attempts to resolve a single connection target by looking up the
// dependency ReleaseBinding and extracting the endpoint URL from its status.
// It returns a non-nil error only for transient API failures that should trigger a requeue.
func (r *Reconciler) resolveConnection(
	ctx context.Context,
	conn openchoreov1alpha1.ConnectionTarget,
) (*openchoreov1alpha1.ResolvedConnection, *openchoreov1alpha1.PendingConnection, error) {
	indexKey := controller.MakeReleaseBindingOwnerEnvKey(conn.Project, conn.Component, conn.Environment)

	var rbList openchoreov1alpha1.ReleaseBindingList
	if err := r.List(ctx, &rbList,
		client.InNamespace(conn.Namespace),
		client.MatchingFields{controller.IndexKeyReleaseBindingOwnerEnv: indexKey}); err != nil {
		return nil, nil, fmt.Errorf("failed to list ReleaseBindings for component %s/%s: %w", conn.Project, conn.Component, err)
	}

	if len(rbList.Items) == 0 {
		return nil, &openchoreov1alpha1.PendingConnection{
			Namespace: conn.Namespace,
			Project:   conn.Project,
			Component: conn.Component,
			Endpoint:  conn.Endpoint,
			Reason:    fmt.Sprintf("ReleaseBinding not found for component %s/%s", conn.Project, conn.Component),
		}, nil
	}

	if len(rbList.Items) > 1 {
		return nil, &openchoreov1alpha1.PendingConnection{
			Namespace: conn.Namespace,
			Project:   conn.Project,
			Component: conn.Component,
			Endpoint:  conn.Endpoint,
			Reason:    fmt.Sprintf("multiple ReleaseBindings found for component %s/%s in environment %s", conn.Project, conn.Component, conn.Environment),
		}, nil
	}

	rb := &rbList.Items[0]

	if rb.Spec.State == openchoreov1alpha1.ReleaseStateUndeploy {
		return nil, &openchoreov1alpha1.PendingConnection{
			Namespace: conn.Namespace,
			Project:   conn.Project,
			Component: conn.Component,
			Endpoint:  conn.Endpoint,
			Reason:    "component is undeployed",
		}, nil
	}

	for _, ep := range rb.Status.Endpoints {
		if ep.Name != conn.Endpoint {
			continue
		}
		url := resolveURLForVisibility(ep, conn.Visibility)
		if url == nil {
			return nil, &openchoreov1alpha1.PendingConnection{
				Namespace: conn.Namespace,
				Project:   conn.Project,
				Component: conn.Component,
				Endpoint:  conn.Endpoint,
				Reason:    fmt.Sprintf("endpoint %q has no URL for visibility %s", conn.Endpoint, conn.Visibility),
			}, nil
		}
		return &openchoreov1alpha1.ResolvedConnection{
			Namespace:  conn.Namespace,
			Project:    conn.Project,
			Component:  conn.Component,
			Endpoint:   conn.Endpoint,
			Visibility: conn.Visibility,
			URL:        *url,
		}, nil, nil
	}

	return nil, &openchoreov1alpha1.PendingConnection{
		Namespace: conn.Namespace,
		Project:   conn.Project,
		Component: conn.Component,
		Endpoint:  conn.Endpoint,
		Reason:    fmt.Sprintf("endpoint %q not yet resolved", conn.Endpoint),
	}, nil
}

// resolveURLForVisibility extracts the appropriate URL from an EndpointURLStatus
// based on the requested visibility level.
func resolveURLForVisibility(
	ep openchoreov1alpha1.EndpointURLStatus,
	visibility openchoreov1alpha1.EndpointVisibility,
) *openchoreov1alpha1.EndpointURL {
	switch visibility {
	case openchoreov1alpha1.EndpointVisibilityProject, openchoreov1alpha1.EndpointVisibilityNamespace:
		return ep.ServiceURL
	case openchoreov1alpha1.EndpointVisibilityExternal:
		if ep.ExternalURLs != nil {
			if ep.ExternalURLs.HTTPS != nil {
				return ep.ExternalURLs.HTTPS
			}
			if ep.ExternalURLs.HTTP != nil {
				return ep.ExternalURLs.HTTP
			}
			if ep.ExternalURLs.TLS != nil {
				return ep.ExternalURLs.TLS
			}
		}
		return nil
	default:
		return nil
	}
}

// buildConnectionItems builds a list of ConnectionItem from workload connections,
// each carrying its own pre-computed env vars from the resolved connections in ReleaseBinding status.
func buildConnectionItems(
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	connections []openchoreov1alpha1.WorkloadConnection,
) []pipelinecontext.ConnectionItem {
	if len(connections) == 0 {
		return nil
	}

	// Build lookup: "namespace/project/component/endpoint/visibility" → ResolvedConnection
	resolved := make(map[string]openchoreov1alpha1.ResolvedConnection, len(releaseBinding.Status.ResolvedConnections))
	for _, rc := range releaseBinding.Status.ResolvedConnections {
		resolved[connectionKey(rc.Namespace, rc.Project, rc.Component, rc.Endpoint, string(rc.Visibility))] = rc
	}

	items := make([]pipelinecontext.ConnectionItem, 0, len(connections))
	for _, conn := range connections {
		project := conn.Project
		if project == "" {
			project = releaseBinding.Spec.Owner.ProjectName
		}

		item := pipelinecontext.ConnectionItem{
			Namespace:  releaseBinding.Namespace,
			Project:    project,
			Component:  conn.Component,
			Endpoint:   conn.Name,
			Visibility: string(conn.Visibility),
		}

		key := connectionKey(releaseBinding.Namespace, project, conn.Component, conn.Name, string(conn.Visibility))
		if rc, ok := resolved[key]; ok {
			item.EnvVars = buildEnvVarsForConnection(conn, rc)
		}

		items = append(items, item)
	}
	return items
}

// buildEnvVarsForConnection builds the env var list for a single resolved connection.
func buildEnvVarsForConnection(
	conn openchoreov1alpha1.WorkloadConnection,
	rc openchoreov1alpha1.ResolvedConnection,
) []pipelinecontext.ConnectionEnvVar {
	envVars := make([]pipelinecontext.ConnectionEnvVar, 0, 4)

	if conn.EnvBindings.Address != "" {
		envVars = append(envVars, pipelinecontext.ConnectionEnvVar{
			Name:  conn.EnvBindings.Address,
			Value: formatEndpointAddress(rc.URL),
		})
	}

	if conn.EnvBindings.Host != "" {
		envVars = append(envVars, pipelinecontext.ConnectionEnvVar{
			Name:  conn.EnvBindings.Host,
			Value: rc.URL.Host,
		})
	}

	if conn.EnvBindings.Port != "" {
		portStr := ""
		if rc.URL.Port != 0 {
			portStr = strconv.Itoa(int(rc.URL.Port))
		}
		envVars = append(envVars, pipelinecontext.ConnectionEnvVar{
			Name:  conn.EnvBindings.Port,
			Value: portStr,
		})
	}

	if conn.EnvBindings.BasePath != "" {
		envVars = append(envVars, pipelinecontext.ConnectionEnvVar{
			Name:  conn.EnvBindings.BasePath,
			Value: rc.URL.Path,
		})
	}

	return envVars
}

// formatEndpointAddress formats an EndpointURL into a protocol-appropriate connection string.
// For schemes that use URL format (http, https, ws, wss, tls): scheme://host:port/path
// For schemes without URL format (grpc, tcp, udp) or empty: host:port
func formatEndpointAddress(url openchoreov1alpha1.EndpointURL) string {
	var sb strings.Builder

	if schemeUsesURLFormat(url.Scheme) {
		sb.WriteString(url.Scheme)
		sb.WriteString("://")
	}

	sb.WriteString(url.Host)

	if url.Port != 0 {
		sb.WriteString(":")
		sb.WriteString(strconv.Itoa(int(url.Port)))
	}

	if url.Path != "" {
		if !strings.HasPrefix(url.Path, "/") {
			sb.WriteString("/")
		}
		sb.WriteString(url.Path)
	}

	return sb.String()
}

// schemeUsesURLFormat returns true for schemes that should include the scheme:// prefix
// in the formatted address (HTTP-family and TLS protocols).
func schemeUsesURLFormat(scheme string) bool {
	switch scheme {
	case "http", "https", "ws", "wss", "tls":
		return true
	default:
		return false
	}
}

// allConnectionsResolved checks whether all connection targets have been resolved.
func allConnectionsResolved(
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	connections []openchoreov1alpha1.WorkloadConnection,
) bool {
	if len(connections) == 0 {
		return true
	}
	resolved := make(map[string]struct{}, len(releaseBinding.Status.ResolvedConnections))
	for _, rc := range releaseBinding.Status.ResolvedConnections {
		resolved[connectionKey(rc.Namespace, rc.Project, rc.Component, rc.Endpoint, string(rc.Visibility))] = struct{}{}
	}
	for _, conn := range connections {
		project := conn.Project
		if project == "" {
			project = releaseBinding.Spec.Owner.ProjectName
		}
		if _, ok := resolved[connectionKey(releaseBinding.Namespace, project, conn.Component, conn.Name, string(conn.Visibility))]; !ok {
			return false
		}
	}
	return true
}

// connectionKey builds a lookup key for a connection target.
func connectionKey(namespace, project, component, endpoint, visibility string) string {
	return namespace + "/" + project + "/" + component + "/" + endpoint + "/" + visibility
}

// setConnectionsCondition sets the ConnectionsResolved condition on the ReleaseBinding.
func setConnectionsCondition(
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	allResolved bool,
) {
	if len(releaseBinding.Status.ConnectionTargets) == 0 {
		controller.MarkTrueCondition(releaseBinding, ConditionConnectionsResolved,
			ReasonNoConnections, "No connections to resolve")
		return
	}

	if allResolved {
		resolvedCount := len(releaseBinding.Status.ResolvedConnections)
		controller.MarkTrueCondition(releaseBinding, ConditionConnectionsResolved,
			ReasonAllConnectionsResolved,
			fmt.Sprintf("All %d connections resolved", resolvedCount))
		return
	}

	pendingCount := len(releaseBinding.Status.PendingConnections)
	resolvedCount := len(releaseBinding.Status.ResolvedConnections)
	controller.MarkFalseCondition(releaseBinding, ConditionConnectionsResolved,
		ReasonConnectionsPending,
		fmt.Sprintf("%d connections pending, %d resolved", pendingCount, resolvedCount))
}
