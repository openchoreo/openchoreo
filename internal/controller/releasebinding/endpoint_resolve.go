// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"fmt"
	"math"
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// clusterLocalSuffix is the default Kubernetes cluster domain suffix.
	clusterLocalSuffix = "svc.cluster.local"

	// URL scheme constants for endpoint types.
	schemeHTTP = "http"
	schemeGRPC = "grpc"
	schemeTCP  = "tcp"
	schemeUDP  = "udp"
)

// serviceInfo holds the name, namespace, and ports extracted from a rendered K8s Service resource.
type serviceInfo struct {
	name      string
	namespace string
	ports     []int32
}

// extractAllServiceInfos finds all v1/Service resources among the rendered resources and
// extracts their name, namespace, and spec.ports[].port values.
func extractAllServiceInfos(resources []openchoreov1alpha1.Resource) []serviceInfo {
	services := make([]serviceInfo, 0, len(resources))
	for i := range resources {
		res := &resources[i]
		if res.Object == nil || len(res.Object.Raw) == 0 {
			continue
		}

		obj := &unstructured.Unstructured{}
		if err := obj.UnmarshalJSON(res.Object.Raw); err != nil {
			continue
		}

		if obj.GetAPIVersion() != "v1" || obj.GetKind() != "Service" {
			continue
		}

		info := serviceInfo{
			name:      obj.GetName(),
			namespace: obj.GetNamespace(),
		}

		ports, found, err := unstructured.NestedSlice(obj.Object, "spec", "ports")
		if err == nil && found {
			for _, p := range ports {
				portMap, ok := p.(map[string]any)
				if !ok {
					continue
				}
				port, found, err := unstructured.NestedInt64(portMap, "port")
				if err == nil && found && port > 0 && port <= math.MaxInt32 {
					info.ports = append(info.ports, int32(port)) // #nosec G115 -- bounds checked above
				}
			}
		}
		services = append(services, info)
	}
	return services
}

// bestMatchingService picks the service with the most ports matching the workload endpoints.
// When multiple services tie, the first one encountered wins.
func bestMatchingService(services []serviceInfo, endpoints map[string]openchoreov1alpha1.WorkloadEndpoint) *serviceInfo {
	if len(services) == 0 {
		return nil
	}
	if len(services) == 1 {
		return &services[0]
	}

	// Build a set of endpoint ports for fast lookup.
	epPorts := make(map[int32]struct{}, len(endpoints))
	for _, ep := range endpoints {
		epPorts[ep.Port] = struct{}{}
	}

	var best *serviceInfo
	bestCount := -1
	for i := range services {
		count := 0
		for _, p := range services[i].ports {
			if _, ok := epPorts[p]; ok {
				count++
			}
		}
		if count > bestCount {
			bestCount = count
			best = &services[i]
		}
	}
	return best
}

// resolveServiceURLs builds EndpointURLStatus entries with ServiceURL populated for all
// workload endpoints that have a matching port in the rendered K8s Service.
// Endpoints that already have an EndpointURLStatus from gateway resolution are updated in place;
// endpoints without gateway URLs get new entries.
func resolveServiceURLs(
	ctx context.Context,
	resources []openchoreov1alpha1.Resource,
	endpoints map[string]openchoreov1alpha1.WorkloadEndpoint,
	existing []openchoreov1alpha1.EndpointURLStatus,
) []openchoreov1alpha1.EndpointURLStatus {
	logger := log.FromContext(ctx).WithName("endpoint-resolver")

	if len(endpoints) == 0 {
		return existing
	}

	services := extractAllServiceInfos(resources)
	svcInfo := bestMatchingService(services, endpoints)
	if svcInfo == nil {
		logger.Info("No rendered Service resource found, skipping service URL resolution")
		return existing
	}

	// Build a lookup of existing endpoint statuses by name.
	existingByName := make(map[string]int, len(existing))
	for i, ep := range existing {
		existingByName[ep.Name] = i
	}

	// Track which endpoint names already exist so we can append new ones.
	result := make([]openchoreov1alpha1.EndpointURLStatus, len(existing))
	copy(result, existing)

	// Collect endpoint names that don't already have an entry, so we can add them sorted.
	var newNames []string
	for name := range endpoints {
		if _, ok := existingByName[name]; !ok {
			newNames = append(newNames, name)
		}
	}
	sort.Strings(newNames)

	// Update existing entries with ServiceURL.
	for name, ep := range endpoints {
		svcURL := buildServiceURL(svcInfo, ep)
		if svcURL == nil {
			continue
		}

		if idx, ok := existingByName[name]; ok {
			result[idx].ServiceURL = svcURL
			result[idx].Type = ep.Type
		}
	}

	// Append new entries for endpoints not covered by gateway resolution.
	for _, name := range newNames {
		ep := endpoints[name]
		svcURL := buildServiceURL(svcInfo, ep)
		if svcURL == nil {
			continue
		}
		result = append(result, openchoreov1alpha1.EndpointURLStatus{
			Name:       name,
			Type:       ep.Type,
			ServiceURL: svcURL,
		})
	}

	return result
}

// buildServiceURL constructs an EndpointURL for the in-cluster Service address if the
// endpoint port matches one of the Service's ports.
func buildServiceURL(svc *serviceInfo, ep openchoreov1alpha1.WorkloadEndpoint) *openchoreov1alpha1.EndpointURL {
	if svc == nil {
		return nil
	}
	for _, svcPort := range svc.ports {
		if svcPort == ep.Port {
			return &openchoreov1alpha1.EndpointURL{
				Scheme: schemeForEndpointType(ep.Type),
				Host:   fmt.Sprintf("%s.%s.%s", svc.name, svc.namespace, clusterLocalSuffix),
				Port:   svcPort,
				Path:   ep.BasePath,
			}
		}
	}
	return nil
}

// schemeForEndpointType returns the URL scheme appropriate for the given endpoint type.
func schemeForEndpointType(epType openchoreov1alpha1.EndpointType) string {
	switch epType {
	case openchoreov1alpha1.EndpointTypeHTTP,
		openchoreov1alpha1.EndpointTypeREST,
		openchoreov1alpha1.EndpointTypeGraphQL,
		openchoreov1alpha1.EndpointTypeWebsocket:
		return schemeHTTP
	case openchoreov1alpha1.EndpointTypeGRPC:
		return schemeGRPC
	case openchoreov1alpha1.EndpointTypeTCP:
		return schemeTCP
	case openchoreov1alpha1.EndpointTypeUDP:
		return schemeUDP
	default:
		return schemeHTTP
	}
}
