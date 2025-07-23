// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package webapplicationbinding

import (
	"context"
	"fmt"
	"strings"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

// setReadyStatus sets the WebApplicationBinding status to ready if all conditions are met in the Release.
func (r *Reconciler) setReadyStatus(ctx context.Context, webApplicationBinding *openchoreov1alpha1.WebApplicationBinding, release *openchoreov1alpha1.Release) error {
	// Count resources by health status
	totalResources := len(release.Status.Resources)

	// Handle the case where there are no resources
	if totalResources == 0 {
		message := "No resources to deploy"
		controller.MarkTrueCondition(webApplicationBinding, ConditionReady, ReasonAllResourcesReady, message)
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
			controller.MarkTrueCondition(webApplicationBinding, ConditionReady, ReasonResourcesReadyWithSuspended, message)
		} else {
			message := fmt.Sprintf("All %d resources are deployed and healthy", totalResources)
			controller.MarkTrueCondition(webApplicationBinding, ConditionReady, ReasonAllResourcesReady, message)
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
		controller.MarkFalseCondition(webApplicationBinding, ConditionReady, reason, message)
	}

	return nil
}

// updateEndpointStatus updates the WebApplicationBinding status with endpoint information
func (r *Reconciler) updateEndpointStatus(ctx context.Context, webApplicationBinding *openchoreov1alpha1.WebApplicationBinding) error {

	// Get endpoints from workload spec
	workloadEndpoints := webApplicationBinding.Spec.WorkloadSpec.Endpoints
	if len(workloadEndpoints) == 0 {
		return nil
	}

	// For web applications, we typically have one main HTTP endpoint
	// Web applications are always exposed through the gateway
	var endpoints []openchoreov1alpha1.EndpointStatus
	for name, ep := range workloadEndpoints {
		endpointStatus := openchoreov1alpha1.EndpointStatus{
			Name: name,
			Type: ep.Type,
		}

		// Project-level endpoint access is always available
		epaProject := makeEndpointAccess(webApplicationBinding, ep, openchoreov1alpha1.EndpointExposeLevelProject)
		endpointStatus.Project = epaProject

		// Web applications will always have a public endpoint if they are HTTP-based
		// TODO: Should we check WebApplicationClass for expose level configuration?
		if ep.Type == openchoreov1alpha1.EndpointTypeHTTP {
			// Add public endpoint access if configured
			// TODO: Check WebApplicationClass for public exposure configuration
			epaPublic := makeEndpointAccess(webApplicationBinding, ep, openchoreov1alpha1.EndpointExposeLevelPublic)
			endpointStatus.Public = epaPublic
		}

		endpoints = append(endpoints, endpointStatus)
	}

	webApplicationBinding.Status.Endpoints = endpoints
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

func makeEndpointAccess(webApplicationBinding *openchoreov1alpha1.WebApplicationBinding, ep openchoreov1alpha1.WorkloadEndpoint,
	exposeLevel openchoreov1alpha1.EndpointExposeLevel) *openchoreov1alpha1.EndpointAccess {

	var gatewayPort int32 = 8443 // TODO: Hardcoded for now with kube port-fwd, should be configurable.
	gatewayScheme := "https"     // TODO: Hardcoded for now, should be configurable

	if exposeLevel == openchoreov1alpha1.EndpointExposeLevelPublic {
		publicDomain := "choreoapps.localhost" // TODO: Get this from environment or config
		publicHost := fmt.Sprintf("%s-%s.%s", webApplicationBinding.Spec.Owner.ComponentName,
			webApplicationBinding.Spec.Environment, publicDomain)
		scheme := gatewayScheme
		// For web applications, use the root path as the base path
		basePath := "/"
		return &openchoreov1alpha1.EndpointAccess{
			Host:     publicHost,
			Port:     gatewayPort,
			Scheme:   scheme,
			BasePath: basePath,
			URI:      makeEndpointURI(scheme, publicHost, gatewayPort, basePath),
		}
	}
	// TODO: At the moment, there is no organization-level access for web applications.

	// Return project-level access by default
	serviceName := dpkubernetes.GenerateK8sName(webApplicationBinding.Name)
	scheme := getSchemeForEndpoint(ep)
	return &openchoreov1alpha1.EndpointAccess{
		Host:     serviceName,
		Port:     ep.Port,
		Scheme:   scheme,
		BasePath: "", // No base path for project-level access
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
