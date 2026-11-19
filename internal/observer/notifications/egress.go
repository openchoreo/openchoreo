// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"context"
	"net"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/notifications/destination"
)

// maxResponseHeaderBytes bounds the response headers read from a webhook receiver.
const maxResponseHeaderBytes = 64 << 10

var defaultPolicy = destination.NewPolicy(destination.Options{})

var (
	// httpClient sends webhook notifications. It connects only to the destinations the policy
	// allows and does not follow redirects.
	httpClient = newWebhookClient(defaultPolicy)

	// smtpDial connects to SMTP servers, only at the destinations the policy allows.
	smtpDial func(ctx context.Context, network, address string) (net.Conn, error) = defaultPolicy.DialContext
)

// SetDestinationPolicy sets which addresses notifications may be sent to. It is meant to be
// called once at startup, before any notification is sent.
func SetDestinationPolicy(opts destination.Options) {
	policy := destination.NewPolicy(opts)
	httpClient = newWebhookClient(policy)
	smtpDial = policy.DialContext
}

// newWebhookClient returns a client that connects only to the destinations the policy allows and
// does not follow redirects. Each attempt is bounded by the caller's context, so it sets no timeout.
//
// A webhook sent through an HTTP proxy from the environment is not checked: the client connects
// only to the proxy, which decides where the request goes and must refuse internal destinations.
func newWebhookClient(policy *destination.Policy) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = policy.DialContext
	transport.MaxResponseHeaderBytes = maxResponseHeaderBytes
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
