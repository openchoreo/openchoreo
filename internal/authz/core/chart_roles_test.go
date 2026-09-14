// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type chartValues struct {
	OpenchoreoAPI struct {
		Config struct {
			Security struct {
				Authorization struct {
					Bootstrap struct {
						Roles []struct {
							Name    string   `yaml:"name"`
							Actions []string `yaml:"actions"`
						} `yaml:"roles"`
					} `yaml:"bootstrap"`
				} `yaml:"authorization"`
			} `yaml:"security"`
		} `yaml:"config"`
	} `yaml:"openchoreoApi"`
}

func TestBootstrapRoles_ActionValidity(t *testing.T) {
	valuesPath := filepath.Join("..", "..", "..", "install", "helm", "openchoreo-control-plane", "values.yaml")
	data, err := os.ReadFile(valuesPath)
	require.NoError(t, err, "failed to read control plane values.yaml")

	var values chartValues
	err = yaml.Unmarshal(data, &values)
	require.NoError(t, err, "failed to parse control plane values.yaml")

	roles := values.OpenchoreoAPI.Config.Security.Authorization.Bootstrap.Roles
	require.NotEmpty(t, roles, "bootstrap.roles in values.yaml must not be empty")

	validActions := make(map[string]bool)
	for _, a := range ConcretePublicActions() {
		validActions[a.Name] = true
	}

	for _, role := range roles {
		t.Run(role.Name, func(t *testing.T) {
			require.NotEmpty(t, role.Actions, "role %s must have actions", role.Name)
			for _, action := range role.Actions {
				if action == "*" {
					continue
				}
				require.Truef(t, validActions[action],
					"role %q grants action %q which is not registered in core.ConcretePublicActions()",
					role.Name, action)
			}
		})
	}
}
