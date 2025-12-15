// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"fmt"

	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetWebhookBaseURLFromIngress retrieves the webhook base URL from the openchoreo-api Ingress resource.
// It constructs the URL from the first ingress rule host and determines the protocol based on TLS configuration.
//
// Returns the base URL (e.g., "https://api.example.com") or an error if the Ingress is not found or misconfigured.
func GetWebhookBaseURLFromIngress(ctx context.Context, k8sClient client.Client, namespace, ingressName string) (string, error) {
	// Get the Ingress resource
	ingress := &networkingv1.Ingress{}
	err := k8sClient.Get(ctx, client.ObjectKey{
		Name:      ingressName,
		Namespace: namespace,
	}, ingress)

	if err != nil {
		return "", fmt.Errorf("failed to get ingress %s/%s: %w", namespace, ingressName, err)
	}

	// Check for explicit annotation override
	if annotationURL, ok := ingress.Annotations["openchoreo.dev/webhook-base-url"]; ok && annotationURL != "" {
		return annotationURL, nil
	}

	// Extract the first host from ingress rules
	if len(ingress.Spec.Rules) == 0 {
		return "", fmt.Errorf("ingress %s/%s has no rules configured", namespace, ingressName)
	}

	host := ingress.Spec.Rules[0].Host
	if host == "" {
		return "", fmt.Errorf("ingress %s/%s rule has no host configured", namespace, ingressName)
	}

	// Determine protocol based on TLS configuration
	protocol := "http"
	if len(ingress.Spec.TLS) > 0 {
		// If TLS is configured for this host, use https
		for _, tls := range ingress.Spec.TLS {
			for _, tlsHost := range tls.Hosts {
				if tlsHost == host {
					protocol = "https"
					break
				}
			}
			if protocol == "https" {
				break
			}
		}
	}

	// Construct the base URL
	baseURL := fmt.Sprintf("%s://%s", protocol, host)

	return baseURL, nil
}