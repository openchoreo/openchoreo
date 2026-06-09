// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
)

func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Choreo configuration contexts",
		Long:  "Manage Choreo configuration contexts, control planes, and credentials.",
	}
	cmd.AddCommand(
		newContextCmd(),
		newControlPlaneCmd(),
		newCredentialsCmd(),
	)
	return cmd
}

// ---- context ----

func newContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage configuration contexts",
	}
	cmd.AddCommand(
		newContextAddCmd(),
		newContextListCmd(),
		newContextDeleteCmd(),
		newContextUpdateCmd(),
		newContextUseCmd(),
	)
	return cmd
}

func newContextAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [CONTEXT_NAME]",
		Short: "Add a new configuration context",
		Args:  cmdutil.ExactOneArgWithUsage(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return New().AddContext(AddContextParams{
				Name:         args[0],
				ControlPlane: flags.GetControlPlane(cmd),
				Credentials:  flags.GetCredentials(cmd),
				Namespace:    flags.GetNamespace(cmd),
				Project:      flags.GetProject(cmd),
				Component:    flags.GetComponent(cmd),
			})
		},
	}
	flags.AddControlPlane(cmd)
	flags.AddCredentials(cmd)
	flags.AddNamespace(cmd)
	flags.AddProject(cmd)
	flags.AddComponent(cmd)
	_ = cmd.MarkFlagRequired("controlplane")
	_ = cmd.MarkFlagRequired("credentials")
	return cmd
}

func newContextListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configuration contexts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return New().ListContexts()
		},
	}
}

func newContextDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [CONTEXT_NAME]",
		Short: "Delete a configuration context",
		Args:  cmdutil.ExactOneArgWithUsage(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return New().DeleteContext(DeleteContextParams{Name: args[0]})
		},
	}
}

func newContextUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [CONTEXT_NAME]",
		Short: "Update an existing configuration context",
		Args:  cmdutil.ExactOneArgWithUsage(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return New().UpdateContext(UpdateContextParams{
				Name:         args[0],
				Namespace:    flags.GetNamespace(cmd),
				Project:      flags.GetProject(cmd),
				Component:    flags.GetComponent(cmd),
				ControlPlane: flags.GetControlPlane(cmd),
				Credentials:  flags.GetCredentials(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddProject(cmd)
	flags.AddComponent(cmd)
	flags.AddControlPlane(cmd)
	flags.AddCredentials(cmd)
	return cmd
}

func newContextUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use [CONTEXT_NAME]",
		Short: "Switch to a configuration context",
		Args:  cmdutil.ExactOneArgWithUsage(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return New().UseContext(UseContextParams{Name: args[0]})
		},
	}
}

// ---- controlplane ----

func newControlPlaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "controlplane",
		Short: "Manage control plane configurations",
	}
	cmd.AddCommand(
		newControlPlaneAddCmd(),
		newControlPlaneListCmd(),
		newControlPlaneUpdateCmd(),
		newControlPlaneDeleteCmd(),
	)
	return cmd
}

func newControlPlaneAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [CONTROLPLANE_NAME]",
		Short: "Add a new control plane configuration",
		Args:  cmdutil.ExactOneArgWithUsage(),
		RunE: func(cmd *cobra.Command, args []string) error {
			url, err := cmd.Flags().GetString("url")
			if err != nil {
				return err
			}
			return New().AddControlPlane(AddControlPlaneParams{
				Name: args[0],
				URL:  url,
			})
		},
	}
	flags.AddURL(cmd)
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

func newControlPlaneListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all control plane configurations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return New().ListControlPlanes()
		},
	}
}

func newControlPlaneUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [CONTROLPLANE_NAME]",
		Short: "Update a control plane configuration",
		Args:  cmdutil.ExactOneArgWithUsage(),
		RunE: func(cmd *cobra.Command, args []string) error {
			url, _ := cmd.Flags().GetString("url")
			return New().UpdateControlPlane(UpdateControlPlaneParams{
				Name: args[0],
				URL:  url,
			})
		},
	}
	flags.AddURL(cmd)
	return cmd
}

func newControlPlaneDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [CONTROLPLANE_NAME]",
		Short: "Delete a control plane configuration",
		Args:  cmdutil.ExactOneArgWithUsage(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return New().DeleteControlPlane(DeleteControlPlaneParams{Name: args[0]})
		},
	}
}

// ---- credentials ----

func newCredentialsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "credentials",
		Short: "Manage credentials configurations",
	}
	cmd.AddCommand(
		newCredentialsAddCmd(),
		newCredentialsListCmd(),
		newCredentialsDeleteCmd(),
	)
	return cmd
}

func newCredentialsAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add [CREDENTIALS_NAME]",
		Short: "Add a new credentials configuration",
		Args:  cmdutil.ExactOneArgWithUsage(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("credentials name is required")
			}
			return New().AddCredentials(AddCredentialsParams{Name: args[0]})
		},
	}
}

func newCredentialsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all credentials configurations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return New().ListCredentials()
		},
	}
}

func newCredentialsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [CREDENTIALS_NAME]",
		Short: "Delete a credentials configuration",
		Args:  cmdutil.ExactOneArgWithUsage(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return New().DeleteCredentials(DeleteCredentialsParams{Name: args[0]})
		},
	}
}
