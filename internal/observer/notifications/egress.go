// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"context"
	"net"
	"net/http"
	"net/url"

	"github.com/openchoreo/openchoreo/internal/observer/notifications/destination"
)

// maxResponseHeaderBytes bounds the response headers read from a webhook receiver.
const maxResponseHeaderBytes = 64 << 10

var defaultPolicy = destination.NewPolicy(destination.Options{})

var (
	// httpClient sends webhook notifications. It connects only to the destinations the policy
	// allows and does not follow redirects.
	httpClient = newWebhookClient(defaultPolicy, http.ProxyFromEnvironment)

	// smtpDial connects to SMTP servers, only at the destinations the policy allows.
	smtpDial func(ctx context.Context, network, address string) (net.Conn, error) = defaultPolicy.DialContext
)

// SetDestinationPolicy sets which addresses notifications may be sent to. It is meant to be
// called once at startup, before any notification is sent.
func SetDestinationPolicy(opts destination.Options) {
	policy := destination.NewPolicy(opts)
	httpClient = newWebhookClient(policy, http.ProxyFromEnvironment)
	smtpDial = policy.DialContext
}

// newWebhookClient returns a client that connects only to the destinations the policy allows and
// does not follow redirects. proxy picks the HTTP proxy for a request, as http.Transport.Proxy
// does. Each attempt is bounded by the caller's context, so the client sets no timeout.
func newWebhookClient(policy *destination.Policy, proxy func(*http.Request) (*url.URL, error)) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = policy.DialContext
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		proxyURL, err := proxy(req)
		if err != nil || proxyURL == nil {
			return proxyURL, err
		}
		// Through a proxy, DialContext sees only the proxy's address, so check the webhook's host here.
		if err := policy.CheckHost(req.Context(), req.URL.Hostname()); err != nil {
			return nil, err
		}
		return proxyURL, nil
	}
	transport.MaxResponseHeaderBytes = maxResponseHeaderBytes
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
