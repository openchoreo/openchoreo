// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractClientCertFromHeader_Success(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	headerValue := base64.StdEncoding.EncodeToString(encodeCertToPEM(t, clientCert))

	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("X-Client-Cert", headerValue)

	leaf, intermediates, err := extractClientCertFromHeader(r, "X-Client-Cert")
	require.NoError(t, err)
	assert.Equal(t, clientCert.Raw, leaf.Raw)
	assert.Empty(t, intermediates)
}

func TestExtractClientCertFromHeader_LeafAndIntermediate(t *testing.T) {
	rootCA, rootKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, rootCA, rootKey)

	headerValue := base64.StdEncoding.EncodeToString(
		append(encodeCertToPEM(t, clientCert), encodeCertToPEM(t, rootCA)...),
	)

	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("X-Client-Cert", headerValue)

	leaf, intermediates, err := extractClientCertFromHeader(r, "X-Client-Cert")
	require.NoError(t, err)
	assert.Equal(t, clientCert.Raw, leaf.Raw)
	require.Len(t, intermediates, 1)
	assert.Equal(t, rootCA.Raw, intermediates[0].Raw)
}

func TestExtractClientCertFromHeader_MissingHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)

	_, _, err := extractClientCertFromHeader(r, "X-Client-Cert")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestExtractClientCertFromHeader_InvalidBase64(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("X-Client-Cert", "not-valid-base64!!!")

	_, _, err := extractClientCertFromHeader(r, "X-Client-Cert")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base64-decode")
}

func TestExtractClientCertFromHeader_NoCertInPEM(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("X-Client-Cert", base64.StdEncoding.EncodeToString([]byte("not a pem block")))

	_, _, err := extractClientCertFromHeader(r, "X-Client-Cert")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no PEM-encoded certificate")
}
