// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package servicebinding

import (
	"context"
	"fmt"
	"path"
	"strings"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// setReadyStatus sets the ServiceBinding status to ready if all conditions are met in the Release.
func (r *Reconciler) setReadyStatus(ctx context.Context, serviceBinding *openchoreov1alpha1.ServiceBinding, release *openchoreov1alpha1.Release) error {
	// Count resources by health status
	totalResources := len(release.Status.Resources)

	// Handle the case where there are no resources
	if totalResources == 0 {
		message := "No resources to deploy"
		controller.MarkTrueCondition(serviceBinding, ConditionReady, ReasonAllResourcesReady, message)
		return nil
	}

	healthyCount := 0
	progressingCount := 0
	degradedCount := 0
	suspendedCount := 0

	// Check all resources using their health status
	for _, resource := range release.Status.Resources {
		switch resource.HealthStatus {
		case openchoreov1alpha1.HealthStatusHealthy:
			healthyCount++
		case openchoreov1alpha1.HealthStatusSuspended:
			suspendedCount++
		case openchoreov1alpha1.HealthStatusProgressing, openchoreov1alpha1.HealthStatusUnknown:
			// Treat both progressing and unknown as progressing
			progressingCount++
		case openchoreov1alpha1.HealthStatusDegraded:
			degradedCount++
		default:
			// Treat any unrecognized health status as progressing
			progressingCount++
		}
	}

	// Check if all resources are ready (healthy or suspended)
	allResourcesReady := (healthyCount + suspendedCount) == totalResources

	// Set the ready condition based on resource health status
	if allResourcesReady {
		// Use appropriate ready reason
		if suspendedCount > 0 {
			message := fmt.Sprintf("All %d resources are ready (%d suspended)", totalResources, suspendedCount)
			controller.MarkTrueCondition(serviceBinding, ConditionReady, ReasonResourcesReadyWithSuspended, message)
		} else {
			message := fmt.Sprintf("All %d resources are deployed and healthy", totalResources)
			controller.MarkTrueCondition(serviceBinding, ConditionReady, ReasonAllResourcesReady, message)
		}
	} else {
		// Build a status message with counts
		var statusParts []string

		if progressingCount > 0 {
			statusParts = append(statusParts, fmt.Sprintf("%d/%d progressing", progressingCount, totalResources))
		}
		if degradedCount > 0 {
			statusParts = append(statusParts, fmt.Sprintf("%d/%d degraded", degradedCount, totalResources))
		}
		if healthyCount > 0 {
			statusParts = append(statusParts, fmt.Sprintf("%d/%d healthy", healthyCount, totalResources))
		}
		if suspendedCount > 0 {
			statusParts = append(statusParts, fmt.Sprintf("%d/%d suspended", suspendedCount, totalResources))
		}

		// Determine reason using priority: Progressing > Degraded
		var reason controller.ConditionReason
		var message string

		if progressingCount > 0 {
			// If any resource is progressing, the whole binding is progressing
			reason = ReasonResourceHealthProgressing
		} else {
			// Only degraded resources
			reason = ReasonResourceHealthDegraded
		}

		message = fmt.Sprintf("Resources status: %s", strings.Join(statusParts, ", "))
		controller.MarkFalseCondition(serviceBinding, ConditionReady, reason, message)
	}

	return nil
}

// updateEndpointStatus updates the ServiceBinding status with endpoint information
// TODO: Fix this code
func (r *Reconciler) updateEndpointStatus(ctx context.Context, serviceBinding *openchoreov1alpha1.ServiceBinding) error {

	// Get endpoints from workload spec
	workloadEndpoints := serviceBinding.Spec.WorkloadSpec.Endpoints
	if len(workloadEndpoints) == 0 {
		return nil
	}

	// Build endpoint status for each endpoint
	var endpoints []openchoreov1alpha1.EndpointStatus
	for name, ep := range workloadEndpoints {
		endpointStatus := openchoreov1alpha1.EndpointStatus{
			Name: name,
			Type: ep.Type,
		}

		// Project-level endpoint access is always available
		epaProject := makeEndpointAccess(serviceBinding, ep, openchoreov1alpha1.ServiceAPI{}, openchoreov1alpha1.EndpointExposeLevelProject)
		endpointStatus.Project = epaProject

		// Check if the endpoint is exposed at different levels
		// Only HTTP and REST endpoints are supported for expose levels at the moment
		apiConfig, ok := serviceBinding.Spec.APIs[name]
		if ok {
			switch apiConfig.Type {
			case openchoreov1alpha1.EndpointTypeREST:
				for _, level := range apiConfig.RESTEndpoint.ExposeLevels {
					if level == openchoreov1alpha1.ExposeLevelPublic {
						endpointStatus.Public = makeEndpointAccess(serviceBinding, ep, *apiConfig, openchoreov1alpha1.EndpointExposeLevelPublic)
					} else if level == openchoreov1alpha1.ExposeLevelOrganization {
						endpointStatus.Organization = makeEndpointAccess(serviceBinding, ep, *apiConfig, openchoreov1alpha1.EndpointExposeLevelOrganization)
					} else if level == openchoreov1alpha1.ExposeLevelProject {
						// Project-level access is already set above
					}
				}
			}
		}

		endpoints = append(endpoints, endpointStatus)
	}

	serviceBinding.Status.Endpoints = endpoints
	return nil
}

// getSchemeForEndpoint determines the scheme based on endpoint configuration
func getSchemeForEndpoint(ep openchoreov1alpha1.WorkloadEndpoint) string {
	// Use the endpoint type to determine scheme
	switch ep.Type {
	case openchoreov1alpha1.EndpointTypeGRPC:
		return "grpc"
	case openchoreov1alpha1.EndpointTypeWebsocket:
		return "ws"
	case openchoreov1alpha1.EndpointTypeHTTP, openchoreov1alpha1.EndpointTypeREST:
		// Check if HTTPS based on port
		if ep.Port == 443 || ep.Port == 8443 {
			return "https"
		}
		return "http"
	case openchoreov1alpha1.EndpointTypeGraphQL:
		// GraphQL typically uses HTTP
		if ep.Port == 443 || ep.Port == 8443 {
			return "https"
		}
		return "http"
	case openchoreov1alpha1.EndpointTypeTCP, openchoreov1alpha1.EndpointTypeUDP:
		// No scheme for raw TCP/UDP
		return ""
	default:
		// Default to empty to let the client decide
		return ""
	}
}

func makeEndpointAccess(serviceBinding *openchoreov1alpha1.ServiceBinding, ep openchoreov1alpha1.WorkloadEndpoint,
	apiConfig openchoreov1alpha1.ServiceAPI, exposeLevel openchoreov1alpha1.EndpointExposeLevel) *openchoreov1alpha1.EndpointAccess {

	var gatewayPort int32 = 8443 // TODO: Hardcoded for now with kube port-fwd, should be configurable.
	gatewayScheme := "https"     // TODO: Hardcoded for now, should be configurable

	if exposeLevel == openchoreov1alpha1.EndpointExposeLevelPublic {
		publicDomain := "choreoapis.localhost" // TODO: Get this from environment or config
		publicHost := fmt.Sprintf("%s.%s", serviceBinding.Spec.Environment, publicDomain)
		scheme := gatewayScheme
		basePath := ""
		if apiConfig.Type == openchoreov1alpha1.EndpointTypeREST && apiConfig.RESTEndpoint != nil {
			// For REST APIs, we need to use the base path from the API config
			basePath = path.Clean(path.Join("/", serviceBinding.Spec.Owner.ProjectName,
				serviceBinding.Spec.Owner.ComponentName, apiConfig.RESTEndpoint.Backend.BasePath))
		}
		return &openchoreov1alpha1.EndpointAccess{
			Host:     publicHost,
			Port:     gatewayPort,
			Scheme:   scheme,
			BasePath: basePath,
			URI:      makeEndpointURI(scheme, publicHost, gatewayPort, basePath),
		}
	} else if exposeLevel == openchoreov1alpha1.EndpointExposeLevelOrganization {
		orgDomain := "choreoapis.internal" // TODO: Get this from environment or config
		orgHost := fmt.Sprintf("%s.%s", serviceBinding.Spec.Environment, orgDomain)
		scheme := gatewayScheme
		basePath := ""
		if apiConfig.Type == openchoreov1alpha1.EndpointTypeREST && apiConfig.RESTEndpoint != nil {
			// For REST APIs, we need to use the base path from the API config
			basePath = path.Clean(path.Join("/", serviceBinding.Spec.Owner.ProjectName,
				serviceBinding.Spec.Owner.ComponentName, apiConfig.RESTEndpoint.Backend.BasePath))
		}
		return &openchoreov1alpha1.EndpointAccess{
			Host:     orgHost,
			Port:     gatewayPort,
			Scheme:   scheme,
			BasePath: basePath,
			URI:      makeEndpointURI(scheme, orgHost, gatewayPort, basePath),
		}
	}

	// Return project-level access by default
	serviceName := dpkubernetes.GenerateK8sName(serviceBinding.Name)
	scheme := getSchemeForEndpoint(ep)
	return &openchoreov1alpha1.EndpointAccess{
		Host:     serviceName,
		Port:     ep.Port,
		Scheme:   scheme,
		BasePath: "", // TODO: How to get this?
		URI:      makeEndpointURI(scheme, serviceName, ep.Port, ""),
	}
}

// makeEndpointURI constructs the complete URI for an endpoint
func makeEndpointURI(scheme, host string, port int32, basePath string) string {
	if scheme == "" {
		// For raw TCP/UDP, just return host:port
		return fmt.Sprintf("%s:%d", host, port)
	}

	// For HTTP-based protocols, check if we need to include the port
	includePort := true
	if (scheme == "http" && port == 80) || (scheme == "https" && port == 443) {
		includePort = false
	}

	var url string
	if includePort {
		url = fmt.Sprintf("%s://%s:%d", scheme, host, port)
	} else {
		url = fmt.Sprintf("%s://%s", scheme, host)
	}

	// Add a base path if provided
	if basePath != "" {
		if !strings.HasPrefix(basePath, "/") {
			basePath = "/" + basePath
		}
		url += basePath
	}

	return url
}
