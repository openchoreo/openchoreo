// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
)

// extractClientCertFromHeader parses the agent's client certificate out of an HTTP header
// instead of the TLS handshake. It is used when a TLS-terminating load balancer sits in
// front of the public listener and forwards the certificate it received from the agent.
//
// The header value must be the standard base64 encoding of one or more concatenated
// PEM-encoded certificates: the leaf certificate first, followed by any intermediates.
func extractClientCertFromHeader(r *http.Request, headerName string) (*x509.Certificate, []*x509.Certificate, error) {
	headerValue := r.Header.Get(headerName)
	if headerValue == "" {
		return nil, nil, fmt.Errorf("header %q is empty", headerName)
	}

	pemData, err := base64.StdEncoding.DecodeString(headerValue)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to base64-decode header %q: %w", headerName, err)
	}

	var certs []*x509.Certificate
	rest := pemData
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse certificate from header %q: %w", headerName, err)
		}
		certs = append(certs, cert)
	}

	if len(certs) == 0 {
		return nil, nil, fmt.Errorf("no PEM-encoded certificate found in header %q", headerName)
	}

	return certs[0], certs[1:], nil
}
