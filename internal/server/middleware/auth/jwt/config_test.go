// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"net/http"
	"testing"
)

func TestJWKSURLTLSInsecureSkipVerify(t *testing.T) {
	tests := []struct {
		name                         string
		jwksURLTLSInsecureSkipVerify bool
		wantInsecureSkipVerify       bool
	}{
		{
			name:                         "TLS verification enabled (default)",
			jwksURLTLSInsecureSkipVerify: false,
			wantInsecureSkipVerify:       false,
		},
		{
			name:                         "TLS verification disabled",
			jwksURLTLSInsecureSkipVerify: true,
			wantInsecureSkipVerify:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				JWKSURLTLSInsecureSkipVerify: tt.jwksURLTLSInsecureSkipVerify,
			}
			config.setDefaults()

			if config.HTTPClient == nil {
				t.Fatal("HTTPClient should be initialized")
			}

			transport, ok := config.HTTPClient.Transport.(*http.Transport)
			if !ok {
				t.Fatal("HTTPClient.Transport should be *http.Transport")
			}

			if transport.TLSClientConfig == nil {
				t.Fatal("TLSClientConfig should be initialized")
			}

			if transport.TLSClientConfig.InsecureSkipVerify != tt.wantInsecureSkipVerify {
				t.Errorf("InsecureSkipVerify = %v, want %v",
					transport.TLSClientConfig.InsecureSkipVerify, tt.wantInsecureSkipVerify)
			}
		})
	}
}
