// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
)

// TestTimeoutLadderIncludesServerWrite pins the full deadline ladder the
// resource tree endpoints depend on: agent < gateway < client < walk < server
// write. If the server write deadline dropped at or below the walk budget, a
// legitimate tree response would be cut off before the walk gave up.
func TestTimeoutLadderIncludesServerWrite(t *testing.T) {
	write := TimeoutsDefaults().Write
	if protocol.AgentRequestTimeout >= protocol.GatewayTunnelTimeout {
		t.Errorf("agent %v must be < gateway %v", protocol.AgentRequestTimeout, protocol.GatewayTunnelTimeout)
	}
	if protocol.GatewayTunnelTimeout >= protocol.ClientRequestTimeout {
		t.Errorf("gateway %v must be < client %v", protocol.GatewayTunnelTimeout, protocol.ClientRequestTimeout)
	}
	if protocol.ClientRequestTimeout >= protocol.WalkTimeout {
		t.Errorf("client %v must be < walk %v", protocol.ClientRequestTimeout, protocol.WalkTimeout)
	}
	if write <= protocol.WalkTimeout {
		t.Errorf("server write timeout %v must exceed the resource-tree walk budget %v",
			write, protocol.WalkTimeout)
	}
}

func TestTLSConfig_Validate(t *testing.T) {
	tests := []struct {
		name           string
		cfg            TLSConfig
		expectedErrors config.ValidationErrors
	}{
		{
			name: "disabled skips all validation",
			cfg: TLSConfig{
				Enabled: false,
				// Missing required fields but should pass because disabled
			},
			expectedErrors: nil,
		},
		{
			name: "enabled requires cert_file and key_file",
			cfg: TLSConfig{
				Enabled: true,
			},
			expectedErrors: config.ValidationErrors{
				{Field: "tls.cert_file", Message: "is required"},
				{Field: "tls.key_file", Message: "is required"},
			},
		},
		{
			name: "enabled with only cert_file requires key_file",
			cfg: TLSConfig{
				Enabled:  true,
				CertFile: "/path/to/cert.pem",
			},
			expectedErrors: config.ValidationErrors{
				{Field: "tls.key_file", Message: "is required"},
			},
		},
		{
			name: "enabled with only key_file requires cert_file",
			cfg: TLSConfig{
				Enabled: true,
				KeyFile: "/path/to/key.pem",
			},
			expectedErrors: config.ValidationErrors{
				{Field: "tls.cert_file", Message: "is required"},
			},
		},
		{
			name: "enabled with both files is valid",
			cfg: TLSConfig{
				Enabled:  true,
				CertFile: "/path/to/cert.pem",
				KeyFile:  "/path/to/key.pem",
			},
			expectedErrors: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.cfg.Validate(config.NewPath("tls"))
			if diff := cmp.Diff(tt.expectedErrors, errs); diff != "" {
				t.Errorf("validation errors mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
