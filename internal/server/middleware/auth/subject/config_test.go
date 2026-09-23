// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package subject

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestUserTypeConfig_DecodesAuthConfigYAML pins the yaml tags against the
// shape observer's auth-config.yaml actually uses, mirroring the anonymous
// struct LoadConfig unmarshals into.
//
// Worth a test of its own because observer reads this file with plain
// yaml.Unmarshal, which ignores unknown keys: a wrong or missing tag here
// does not fail, it silently leaves the field zero. readable_id_claim would
// then never reach the resolver, the audit fallback would quietly cover for
// it, and every other test in the tree would still pass.
func TestUserTypeConfig_DecodesAuthConfigYAML(t *testing.T) {
	const body = `
auth:
  subject_types:
    - type: "user"
      display_name: "User"
      priority: 1
      auth_mechanisms:
        - type: "jwt"
          readable_id_claim: "username"
          entitlement:
            claim: "groups"
            display_name: "User Group"
    - type: "service_account"
      display_name: "Service Account"
      priority: 2
      auth_mechanisms:
        - type: "jwt"
          readable_id_claim: "client_id"
          entitlement:
            claim: "client_id"
            display_name: "Client ID"
`

	var cfg struct {
		Auth struct {
			SubjectTypes []UserTypeConfig `yaml:"subject_types"`
		} `yaml:"auth"`
	}
	if err := yaml.Unmarshal([]byte(body), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	want := map[string]string{"user": "username", "service_account": "client_id"}
	if len(cfg.Auth.SubjectTypes) != len(want) {
		t.Fatalf("got %d subject types, want %d", len(cfg.Auth.SubjectTypes), len(want))
	}
	for _, ut := range cfg.Auth.SubjectTypes {
		if len(ut.AuthMechanisms) != 1 {
			t.Fatalf("%s: got %d mechanisms, want 1", ut.Type, len(ut.AuthMechanisms))
		}
		if got := ut.AuthMechanisms[0].ReadableIDClaim; got != want[ut.Type] {
			t.Errorf("%s: ReadableIDClaim = %q, want %q", ut.Type, got, want[ut.Type])
		}
	}
}

// TestUserTypeConfig_OmittedReadableIDClaimDecodesEmpty covers a mechanism
// that names no claim — it must stay empty so the audit layer's fallback is
// what fills the gap.
func TestUserTypeConfig_OmittedReadableIDClaimDecodesEmpty(t *testing.T) {
	const body = `
auth:
  subject_types:
    - type: "user"
      display_name: "User"
      priority: 1
      auth_mechanisms:
        - type: "jwt"
          entitlement:
            claim: "groups"
            display_name: "User Group"
`

	var cfg struct {
		Auth struct {
			SubjectTypes []UserTypeConfig `yaml:"subject_types"`
		} `yaml:"auth"`
	}
	if err := yaml.Unmarshal([]byte(body), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	if len(cfg.Auth.SubjectTypes) != 1 || len(cfg.Auth.SubjectTypes[0].AuthMechanisms) != 1 {
		t.Fatalf("unexpected decode result: %+v", cfg.Auth.SubjectTypes)
	}
	if got := cfg.Auth.SubjectTypes[0].AuthMechanisms[0].ReadableIDClaim; got != "" {
		t.Errorf("ReadableIDClaim = %q, want empty", got)
	}
}
