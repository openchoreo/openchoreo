// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/notifications/destination"
)

// everyPrivateRange lists every private address range.
var everyPrivateRange = destination.Entries{Prefixes: []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("fc00::/7"),
}}

// countingServer returns a server that counts the requests it receives.
func countingServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if handler != nil {
			handler(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return ts, &hits
}

func TestSendWebhookRefusesDisallowedDestinations(t *testing.T) {
	// Even with every private range listed, loopback and link-local addresses are refused.
	withDestinationPolicy(t, destination.Options{Private: everyPrivateRange})
	ts, hits := countingServer(t, nil)
	_, port, err := net.SplitHostPort(ts.Listener.Addr().String())
	require.NoError(t, err)

	urls := map[string]string{
		"loopback address":               ts.URL + "/hook",
		"name that resolves to loopback": "http://localhost:" + port + "/hook",
		"unspecified address":            "http://0.0.0.0:" + port + "/hook",
		"IPv4-mapped loopback":           "http://[::ffff:127.0.0.1]:" + port + "/hook",
		"cloud metadata address":         "http://169.254.169.254/latest/meta-data/",
	}
	for name, url := range urls {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := SendWebhookWithConfig(ctx, &WebhookConfig{URL: url}, map[string]interface{}{"a": 1})
			require.Error(t, err)
			assert.ErrorIs(t, err, destination.ErrNotAllowed)
			assert.Equal(t, ReasonChannelConfig, ReasonOf(err), "a refused destination is not retried")
			assert.Equal(t, "destination address not allowed", DetailOf(err))
		})
	}
	assert.Zero(t, hits.Load(), "no request may reach a refused destination")
}

func TestSendWebhookDoesNotFollowRedirects(t *testing.T) {
	target, targetHits := countingServer(t, nil)
	for _, status := range []int{
		http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			redirector, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL+"/internal", status)
			})

			err := SendWebhookWithConfig(context.Background(), &WebhookConfig{URL: redirector.URL + "/hook?token=s3cr3t"},
				map[string]interface{}{"a": 1})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "redirect")
			assert.NotContains(t, err.Error(), target.URL, "the redirect's Location is left out")
			assert.NotContains(t, err.Error(), "s3cr3t")
			assert.Equal(t, ReasonChannelConfig, ReasonOf(err), "a redirect is not retried")
		})
	}
	assert.Zero(t, targetHits.Load(), "a redirect target must not be contacted")
}

func TestSendEmailRefusesDisallowedDestinations(t *testing.T) {
	withDestinationPolicy(t, destination.Options{Private: everyPrivateRange})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	var accepted atomic.Int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			_ = conn.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := &NotificationChannelConfig{Type: "email", Email: EmailConfig{
		SMTP: SMTPConfig{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, From: "alerts@example.org"},
		To:   []string{"oncall@example.org"},
	}}
	err = SendEmailWithConfig(ctx, cfg, "subject", "body")
	require.Error(t, err)
	assert.ErrorIs(t, err, destination.ErrNotAllowed)
	assert.Equal(t, ReasonChannelConfig, ReasonOf(err))

	// Give a connection that got through time to be accepted before checking that none did.
	time.Sleep(50 * time.Millisecond)
	assert.Zero(t, accepted.Load(), "no connection may reach a refused SMTP server")
}

func TestSendWebhookThroughProxyChecksTheTarget(t *testing.T) {
	proxy, proxyHits := countingServer(t, nil)
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	origClient := httpClient
	t.Cleanup(func() { httpClient = origClient })
	// Loopback is allowed so that the client can reach the proxy, which listens on loopback.
	httpClient = newWebhookClient(destination.NewPolicy(destination.Options{AllowLoopback: true}),
		func(*http.Request) (*url.URL, error) { return proxyURL, nil })

	for _, target := range []string{"http://169.254.169.254/latest/meta-data/", "http://10.43.0.9:9098/api/v1alpha1/alerts/webhook"} {
		err := SendWebhookWithConfig(context.Background(), &WebhookConfig{URL: target}, map[string]interface{}{"a": 1})
		require.Error(t, err, target)
		assert.ErrorIs(t, err, destination.ErrNotAllowed, target)
		assert.Equal(t, ReasonChannelConfig, ReasonOf(err), target)
	}
	assert.Zero(t, proxyHits.Load(), "a refused target is not handed to the proxy")

	err = SendWebhookWithConfig(context.Background(), &WebhookConfig{URL: "http://8.8.8.8/hook"}, map[string]interface{}{"a": 1})
	require.NoError(t, err)
	assert.Equal(t, int32(1), proxyHits.Load(), "an allowed target goes through the proxy")
}

func TestUnknownProtectedAddressesAreRetried(t *testing.T) {
	err := fmt.Errorf("dial: %w", destination.ErrProtectionUnavailable)
	assert.Equal(t, ReasonConnection, ReasonOf(err))
	assert.True(t, ReasonOf(err).Retryable())
}
