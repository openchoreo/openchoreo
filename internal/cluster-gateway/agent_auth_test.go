// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// pemChain concatenates the PEM encodings of the given certificates, leaf first,
// matching the on-the-wire form a proxy forwards.
func pemChain(t *testing.T, certs ...*x509.Certificate) string {
	t.Helper()
	var b strings.Builder
	for _, c := range certs {
		b.Write(encodeCertToPEM(t, c))
	}
	return b.String()
}

// albHeaderValue mimics the AWS ALB X-Amzn-Mtls-Clientcert value: a URL-encoded,
// concatenated PEM chain.
func albHeaderValue(t *testing.T, certs ...*x509.Certificate) string {
	t.Helper()
	return url.PathEscape(pemChain(t, certs...))
}

// xfccHeaderValue mimics an Envoy/Istio X-Forwarded-Client-Cert element. cert is
// the leaf; chain, when non-empty, is included as the (preferred) Chain value.
func xfccHeaderValue(t *testing.T, cert *x509.Certificate, chain ...*x509.Certificate) string {
	t.Helper()
	el := fmt.Sprintf(`Hash=0123abcd;Cert=%q`, url.PathEscape(pemChain(t, cert)))
	if len(chain) > 0 {
		el += fmt.Sprintf(`;Chain=%q`, url.PathEscape(pemChain(t, chain...)))
	}
	el += `;Subject="CN=Test Client"`
	return el
}

func TestMTLSAuthenticator_Authenticate(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	leaf := generateTestClientCert(t, caCert, caKey)

	t.Run("extracts leaf and intermediates from handshake", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ws", nil)
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf, caCert}}

		creds, err := mtlsAuthenticator{}.Authenticate(req)
		require.NoError(t, err)
		assert.Equal(t, leaf, creds.clientCert)
		require.Len(t, creds.intermediates, 1)
		assert.Equal(t, caCert, creds.intermediates[0])
	})

	t.Run("no TLS connection is unauthenticated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ws", nil)
		req.TLS = nil

		_, err := mtlsAuthenticator{}.Authenticate(req)
		assert.ErrorIs(t, err, errNoClientCertificate)
	})

	t.Run("no peer certificates is unauthenticated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ws", nil)
		req.TLS = &tls.ConnectionState{PeerCertificates: nil}

		_, err := mtlsAuthenticator{}.Authenticate(req)
		assert.ErrorIs(t, err, errNoClientCertificate)
	})
}

func TestForwardedHeaderAuthenticator_Authenticate(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	leaf := generateTestClientCert(t, caCert, caKey)

	const header = "X-Amzn-Mtls-Clientcert"
	auth := forwardedHeaderAuthenticator{header: header}

	newReq := func(headerName, value string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/ws", nil)
		if value != "" {
			req.Header.Set(headerName, value)
		}
		return req
	}

	t.Run("ALB url-encoded PEM, leaf only", func(t *testing.T) {
		creds, err := auth.Authenticate(newReq(header, albHeaderValue(t, leaf)))
		require.NoError(t, err)
		assert.Equal(t, leaf.Raw, creds.clientCert.Raw)
		assert.Empty(t, creds.intermediates)
	})

	t.Run("ALB url-encoded PEM with intermediate", func(t *testing.T) {
		creds, err := auth.Authenticate(newReq(header, albHeaderValue(t, leaf, caCert)))
		require.NoError(t, err)
		assert.Equal(t, leaf.Raw, creds.clientCert.Raw)
		require.Len(t, creds.intermediates, 1)
		assert.Equal(t, caCert.Raw, creds.intermediates[0].Raw)
	})

	t.Run("already-decoded raw PEM passes through", func(t *testing.T) {
		creds, err := auth.Authenticate(newReq(header, pemChain(t, leaf)))
		require.NoError(t, err)
		assert.Equal(t, leaf.Raw, creds.clientCert.Raw)
	})

	t.Run("XFCC Cert element", func(t *testing.T) {
		a := forwardedHeaderAuthenticator{header: "X-Forwarded-Client-Cert"}
		creds, err := a.Authenticate(newReq("X-Forwarded-Client-Cert", xfccHeaderValue(t, leaf)))
		require.NoError(t, err)
		assert.Equal(t, leaf.Raw, creds.clientCert.Raw)
	})

	t.Run("XFCC prefers Chain over Cert", func(t *testing.T) {
		// Distinct cert in Chain proves Chain wins when both are present.
		chainLeaf := generateTestClientCert(t, caCert, caKey)
		a := forwardedHeaderAuthenticator{header: "X-Forwarded-Client-Cert"}
		creds, err := a.Authenticate(newReq("X-Forwarded-Client-Cert", xfccHeaderValue(t, leaf, chainLeaf, caCert)))
		require.NoError(t, err)
		assert.Equal(t, chainLeaf.Raw, creds.clientCert.Raw)
		require.Len(t, creds.intermediates, 1)
		assert.Equal(t, caCert.Raw, creds.intermediates[0].Raw)
	})

	t.Run("XFCC multiple hops uses the first (client-facing) element", func(t *testing.T) {
		hopLeaf := generateTestClientCert(t, caCert, caKey)
		value := xfccHeaderValue(t, leaf) + "," + xfccHeaderValue(t, hopLeaf)
		a := forwardedHeaderAuthenticator{header: "X-Forwarded-Client-Cert"}
		creds, err := a.Authenticate(newReq("X-Forwarded-Client-Cert", value))
		require.NoError(t, err)
		assert.Equal(t, leaf.Raw, creds.clientCert.Raw)
	})

	t.Run("missing header is unauthenticated", func(t *testing.T) {
		_, err := auth.Authenticate(newReq(header, ""))
		assert.ErrorIs(t, err, errNoClientCertificate)
	})

	t.Run("whitespace-only header is unauthenticated", func(t *testing.T) {
		_, err := auth.Authenticate(newReq(header, "   "))
		assert.ErrorIs(t, err, errNoClientCertificate)
	})

	t.Run("malformed PEM is rejected", func(t *testing.T) {
		_, err := auth.Authenticate(newReq(header, "-----BEGIN CERTIFICATE-----\nnot-base64\n-----END CERTIFICATE-----"))
		require.Error(t, err)
		assert.NotErrorIs(t, err, errNoClientCertificate)
	})

	t.Run("XFCC quoted comma before Cert does not truncate the element", func(t *testing.T) {
		a := forwardedHeaderAuthenticator{header: "X-Forwarded-Client-Cert"}
		value := `Subject="CN=Test, OU=Eng";` + xfccHeaderValue(t, leaf)
		creds, err := a.Authenticate(newReq("X-Forwarded-Client-Cert", value))
		require.NoError(t, err)
		assert.Equal(t, leaf.Raw, creds.clientCert.Raw)
	})

	t.Run("XFCC quoted semicolon in Subject cannot inject a Cert pair", func(t *testing.T) {
		// Subject content is client-controlled and follows Cert in Envoy's
		// element order; a quoted ";Cert=" inside it must not override the leaf.
		a := forwardedHeaderAuthenticator{header: "X-Forwarded-Client-Cert"}
		value := xfccHeaderValue(t, leaf) + `;Subject="CN=evil;Cert=bogus"`
		creds, err := a.Authenticate(newReq("X-Forwarded-Client-Cert", value))
		require.NoError(t, err)
		assert.Equal(t, leaf.Raw, creds.clientCert.Raw)
	})

	t.Run("XFCC escaped quote inside a quoted value stays quoted", func(t *testing.T) {
		// A \" must not toggle the quoted state: without escape handling the
		// comma after it would be treated as an element boundary, truncating
		// the element before Cert is reached.
		a := forwardedHeaderAuthenticator{header: "X-Forwarded-Client-Cert"}
		value := `Subject="CN=\"a, b\", OU=Eng";` + xfccHeaderValue(t, leaf)
		creds, err := a.Authenticate(newReq("X-Forwarded-Client-Cert", value))
		require.NoError(t, err)
		assert.Equal(t, leaf.Raw, creds.clientCert.Raw)
	})

	t.Run("XFCC ignores elements without key=value", func(t *testing.T) {
		a := forwardedHeaderAuthenticator{header: "X-Forwarded-Client-Cert"}
		value := "malformed;" + xfccHeaderValue(t, leaf)
		creds, err := a.Authenticate(newReq("X-Forwarded-Client-Cert", value))
		require.NoError(t, err)
		assert.Equal(t, leaf.Raw, creds.clientCert.Raw)
	})

	t.Run("invalid percent-escape falls back to the raw value", func(t *testing.T) {
		// A '%' not followed by two hex digits fails PathUnescape; the value is
		// then treated as plain PEM, so garbage after the certificate block is
		// ignored by the PEM decoder.
		creds, err := auth.Authenticate(newReq(header, pemChain(t, leaf)+"%zz"))
		require.NoError(t, err)
		assert.Equal(t, leaf.Raw, creds.clientCert.Raw)
	})

	t.Run("XFCC without Cert or Chain is rejected", func(t *testing.T) {
		a := forwardedHeaderAuthenticator{header: "X-Forwarded-Client-Cert"}
		_, err := a.Authenticate(newReq("X-Forwarded-Client-Cert", `Hash=abc;Subject="CN=x"`))
		require.Error(t, err)
	})
}

func TestBuildAgentAuthenticator(t *testing.T) {
	t.Run("empty mode defaults to mtls", func(t *testing.T) {
		a, err := buildAgentAuthenticator(&Config{})
		require.NoError(t, err)
		assert.IsType(t, mtlsAuthenticator{}, a)
	})

	t.Run("explicit mtls", func(t *testing.T) {
		a, err := buildAgentAuthenticator(&Config{AgentAuthMode: AgentAuthModeMTLS})
		require.NoError(t, err)
		assert.IsType(t, mtlsAuthenticator{}, a)
	})

	t.Run("forwarded-header with explicit header", func(t *testing.T) {
		a, err := buildAgentAuthenticator(&Config{
			AgentAuthMode:            AgentAuthModeForwardedHeader,
			AgentAuthForwardedHeader: "X-Forwarded-Client-Cert",
		})
		require.NoError(t, err)
		fh, ok := a.(forwardedHeaderAuthenticator)
		require.True(t, ok)
		assert.Equal(t, "X-Forwarded-Client-Cert", fh.header)
	})

	t.Run("forwarded-header defaults header to ALB", func(t *testing.T) {
		a, err := buildAgentAuthenticator(&Config{AgentAuthMode: AgentAuthModeForwardedHeader})
		require.NoError(t, err)
		fh, ok := a.(forwardedHeaderAuthenticator)
		require.True(t, ok)
		assert.Equal(t, DefaultForwardedHeaderName, fh.header)
	})

	t.Run("invalid mode is an error", func(t *testing.T) {
		_, err := buildAgentAuthenticator(&Config{AgentAuthMode: "bogus"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid agent auth mode")
	})
}

// TestForwardedHeaderMatchesHandshakeVerification encodes the core promise of the
// feature: forwarded-header extraction feeds byte-identical credentials into the
// per-CR verifier, so a given client certificate is accepted for exactly the same
// CRs regardless of how it reached the gateway. If this parity ever breaks, the
// forwarded-header path would silently diverge from the trusted mtls path.
func TestForwardedHeaderMatchesHandshakeVerification(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	leaf := generateTestClientCert(t, caCert, caKey)
	caPEM := encodeCertToPEM(t, caCert)

	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp1", Namespace: "ns"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "prod",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: string(caPEM)},
			},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(dp).Build()
	s := &Server{k8sClient: fakeClient, logger: testLogger()}

	// mtls path: certificate from the TLS handshake.
	mtlsReq := httptest.NewRequest(http.MethodGet, "/ws", nil)
	mtlsReq.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}
	mtlsCreds, err := mtlsAuthenticator{}.Authenticate(mtlsReq)
	require.NoError(t, err)

	// forwarded-header path: same certificate, forwarded by an ALB-style proxy.
	fwdReq := httptest.NewRequest(http.MethodGet, "/ws", nil)
	fwdReq.Header.Set(DefaultForwardedHeaderName, albHeaderValue(t, leaf))
	fwdCreds, err := forwardedHeaderAuthenticator{header: DefaultForwardedHeaderName}.Authenticate(fwdReq)
	require.NoError(t, err)

	mtlsValid, err := s.verifyClientCertificatePerCR(mtlsCreds.clientCert, mtlsCreds.intermediates, "dataplane", "prod")
	require.NoError(t, err)
	fwdValid, err := s.verifyClientCertificatePerCR(fwdCreds.clientCert, fwdCreds.intermediates, "dataplane", "prod")
	require.NoError(t, err)

	assert.Equal(t, []string{"ns/dp1"}, mtlsValid)
	assert.Equal(t, mtlsValid, fwdValid, "forwarded-header verification must match handshake verification")
}
