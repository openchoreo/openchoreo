// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package platformlogs queries the platform logs held by one observability plane.
// It backs the `logs` subcommand of both observabilityplane and
// clusterobservabilityplane, which differ only in how the plane is addressed.
package platformlogs

// PlaneKind says which observability plane resource holds the observer URL.
type PlaneKind int

const (
	// ClusterPlane addresses a cluster-scoped ClusterObservabilityPlane.
	ClusterPlane PlaneKind = iota
	// NamespacedPlane addresses a namespaced ObservabilityPlane.
	NamespacedPlane
)

// LogsParams is a single platform logs query as the CLI collects it.
//
// The coordinate filters are multi-value: values OR within a filter and the filters
// AND across each other. Selector is the exception, being a label selector whose
// commas mean AND.
type LogsParams struct {
	PlaneKind PlaneKind
	PlaneName string
	// Namespace is the OpenChoreo namespace holding the plane resource, for
	// NamespacedPlane only. It does not filter the logs - PodNamespaces does.
	Namespace string

	Clusters      []string
	PodNamespaces []string
	Pods          []string
	Containers    []string
	Selector      string
	Levels        []string
	Search        string

	Since  string // relative duration like "10m"; converted to an absolute window
	Tail   int    // number of entries to show from the end of the window
	Follow bool
	Output string // outputText or outputJSON
}
