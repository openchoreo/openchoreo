// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- NewConfigCmd structure ---

func TestNewConfigCmd_Use(t *testing.T) {
	cmd := NewConfigCmd()
	assert.Equal(t, "config", cmd.Use)
}

func TestNewConfigCmd_Subcommands(t *testing.T) {
	cmd := NewConfigCmd()
	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.ElementsMatch(t, []string{"context", "controlplane", "credentials"}, names)
}

// --- context subcommands ---

func TestContextCmd_Subcommands(t *testing.T) {
	cmd := newContextCmd()
	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.ElementsMatch(t, []string{"add", "list", "delete", "update", "use"}, names)
}

func TestContextAddCmd_MissingArg(t *testing.T) {
	cmd := newContextAddCmd()
	err := cmd.Args(cmd, []string{})
	assert.Error(t, err)
}

func TestContextDeleteCmd_MissingArg(t *testing.T) {
	cmd := newContextDeleteCmd()
	err := cmd.Args(cmd, []string{})
	assert.Error(t, err)
}

func TestContextUseCmd_MissingArg(t *testing.T) {
	cmd := newContextUseCmd()
	err := cmd.Args(cmd, []string{})
	assert.Error(t, err)
}

func TestContextUpdateCmd_MissingArg(t *testing.T) {
	cmd := newContextUpdateCmd()
	err := cmd.Args(cmd, []string{})
	assert.Error(t, err)
}

func TestContextListCmd_NoArgs(t *testing.T) {
	cmd := newContextListCmd()
	err := cmd.Args(cmd, []string{})
	assert.NoError(t, err)
}

// --- controlplane subcommands ---

func TestControlPlaneCmd_Subcommands(t *testing.T) {
	cmd := newControlPlaneCmd()
	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.ElementsMatch(t, []string{"add", "list", "update", "delete"}, names)
}

func TestControlPlaneAddCmd_MissingArg(t *testing.T) {
	cmd := newControlPlaneAddCmd()
	err := cmd.Args(cmd, []string{})
	assert.Error(t, err)
}

func TestControlPlaneDeleteCmd_MissingArg(t *testing.T) {
	cmd := newControlPlaneDeleteCmd()
	err := cmd.Args(cmd, []string{})
	assert.Error(t, err)
}

func TestControlPlaneListCmd_NoArgs(t *testing.T) {
	cmd := newControlPlaneListCmd()
	err := cmd.Args(cmd, []string{})
	assert.NoError(t, err)
}

// --- credentials subcommands ---

func TestCredentialsCmd_Subcommands(t *testing.T) {
	cmd := newCredentialsCmd()
	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.ElementsMatch(t, []string{"add", "list", "delete"}, names)
}

func TestCredentialsAddCmd_MissingArg(t *testing.T) {
	cmd := newCredentialsAddCmd()
	err := cmd.Args(cmd, []string{})
	assert.Error(t, err)
}

func TestCredentialsDeleteCmd_MissingArg(t *testing.T) {
	cmd := newCredentialsDeleteCmd()
	err := cmd.Args(cmd, []string{})
	assert.Error(t, err)
}

func TestCredentialsListCmd_NoArgs(t *testing.T) {
	cmd := newCredentialsListCmd()
	err := cmd.Args(cmd, []string{})
	assert.NoError(t, err)
}
