// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// errNoClientCertificate is returned by every AgentAuthenticator when the
// request carries no usable client certificate. A missing certificate is always
// treated as unauthenticated, regardless of mode.
var errNoClientCertificate = errors.New("no client certificate presented")

// agentCredentials is the input to per-CR certificate verification: the leaf
// client certificate and any intermediates presented alongside it. It is the
// common output of every AgentAuthenticator, so the rest of the gateway is
// unaware of how the certificate was obtained. The ordering matches
// tls.ConnectionState.PeerCertificates (leaf first).
type agentCredentials struct {
	clientCert    *x509.Certificate
	intermediates []*x509.Certificate
}

// AgentAuthenticator extracts the agent's client certificate chain from an
// inbound WebSocket request. Implementations differ only in WHERE the chain
// comes from (TLS handshake vs a forwarded header); the trust anchors and the
// per-CR verification performed afterwards are identical in every mode.
type AgentAuthenticator interface {
	// Authenticate returns the client certificate chain for the request, or an
	// error if no usable certificate is present.
	Authenticate(r *http.Request) (*agentCredentials, error)
}

// mtlsAuthenticator reads the client certificate from the TLS handshake. This
// is the default and preserves the gateway's original behavior.
type mtlsAuthenticator struct{}

func (mtlsAuthenticator) Authenticate(r *http.Request) (*agentCredentials, error) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return nil, errNoClientCertificate
	}
	return &agentCredentials{
		clientCert:    r.TLS.PeerCertificates[0],
		intermediates: r.TLS.PeerCertificates[1:],
	}, nil
}

// forwardedHeaderAuthenticator reads the client certificate chain from a header
// set by a trusted TLS-terminating proxy. This is only safe when the public
// listener is reachable exclusively from that proxy (see Server.Start warning).
type forwardedHeaderAuthenticator struct {
	header string
}

func (a forwardedHeaderAuthenticator) Authenticate(r *http.Request) (*agentCredentials, error) {
	raw := strings.TrimSpace(r.Header.Get(a.header))
	if raw == "" {
		// A trusted proxy omits the header entirely when the client presents no
		// certificate, so an absent header is unauthenticated, not an error state.
		return nil, errNoClientCertificate
	}

	// parseForwardedCertChain returns at least one certificate on success.
	certs, err := parseForwardedCertChain(raw)
	if err != nil {
		return nil, err
	}

	return &agentCredentials{
		clientCert:    certs[0],
		intermediates: certs[1:],
	}, nil
}

// buildAgentAuthenticator resolves the configured agent authentication mode into
// an AgentAuthenticator, returning an error for an unrecognized mode so it is
// surfaced at startup rather than silently defaulting.
func buildAgentAuthenticator(cfg *Config) (AgentAuthenticator, error) {
	switch cfg.AgentAuthMode {
	case "", AgentAuthModeMTLS:
		return mtlsAuthenticator{}, nil
	case AgentAuthModeForwardedHeader:
		header := cfg.AgentAuthForwardedHeader
		if header == "" {
			header = DefaultForwardedHeaderName
		}
		return forwardedHeaderAuthenticator{header: header}, nil
	default:
		return nil, fmt.Errorf("invalid agent auth mode %q: must be %q or %q",
			cfg.AgentAuthMode, AgentAuthModeMTLS, AgentAuthModeForwardedHeader)
	}
}

// parseForwardedCertChain parses a client certificate chain from a forwarded
// header value. It supports the two encodings emitted by TLS-terminating proxies:
//
//   - AWS ALB (X-Amzn-Mtls-Clientcert): the whole value is a URL-encoded,
//     concatenated PEM chain, leaf first.
//   - Envoy/Istio XFCC (X-Forwarded-Client-Cert): a comma-separated list of
//     semicolon-delimited key=value elements. The first element is the
//     client-facing hop; its Chain (preferred) or Cert holds URL-encoded PEM.
//
// The leaf certificate is returned first, followed by any intermediates.
func parseForwardedCertChain(raw string) ([]*x509.Certificate, error) {
	// URL-encoded PEM begins with literal "-----BEGIN" (dashes are not
	// percent-encoded); XFCC begins with a "key=" token instead.
	if strings.HasPrefix(raw, "-----BEGIN") {
		return decodeURLEncodedPEM(raw)
	}
	return parseXFCCChain(raw)
}

// parseXFCCChain parses the Envoy/Istio X-Forwarded-Client-Cert format. Only the
// first (client-facing) element is used; its Chain value is preferred because it
// includes the leaf and issuers, falling back to Cert (leaf only).
func parseXFCCChain(raw string) ([]*x509.Certificate, error) {
	// The first element is the client-facing hop; drop any subsequent proxy
	// hops. Both delimiters must be quote-aware: Envoy quotes values that
	// contain delimiters (e.g. Subject="CN=Test, OU=Eng"), and Subject content
	// is client-controlled, so a naive split could let a crafted Subject inject
	// a bogus Cert pair.
	element := splitUnquoted(raw, ',')[0]

	var leafPEM, chainPEM string
	for _, pair := range splitUnquoted(element, ';') {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		// URL-encoded PEM values are wrapped in double quotes by Envoy.
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "cert":
			leafPEM = value
		case "chain":
			chainPEM = value
		}
	}

	source := chainPEM
	if source == "" {
		source = leafPEM
	}
	if source == "" {
		return nil, fmt.Errorf("forwarded certificate: XFCC header has no Cert or Chain element")
	}

	return decodeURLEncodedPEM(source)
}

// splitUnquoted splits s at each occurrence of sep that is not inside a
// double-quoted section, honoring backslash escapes within quotes. It always
// returns at least one element.
func splitUnquoted(s string, sep byte) []string {
	var parts []string
	start := 0
	inQuotes := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if inQuotes && i+1 < len(s) {
				i++ // skip the escaped character
			}
		case '"':
			inQuotes = !inQuotes
		case sep:
			if !inQuotes {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// decodeURLEncodedPEM percent-decodes a URL-encoded PEM chain and parses every
// CERTIFICATE block within it. url.PathUnescape is used rather than
// QueryUnescape so that literal '+' characters in base64 bodies are preserved.
// Values that are already plain PEM (no percent-escapes) pass through unchanged.
func decodeURLEncodedPEM(encoded string) ([]*x509.Certificate, error) {
	decoded, err := url.PathUnescape(encoded)
	if err != nil {
		decoded = encoded
	}
	certs, err := parseCACertificates([]byte(decoded))
	if err != nil {
		return nil, fmt.Errorf("forwarded certificate: %w", err)
	}
	return certs, nil
}
