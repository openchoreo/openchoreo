// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterauthzrolebinding

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clusterauthzrolebinding"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	cliargs "github.com/openchoreo/openchoreo/pkg/cli/common/args"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

// NewClusterAuthzRoleBindingCmd returns the parent command for authz cluster role binding operations
func NewClusterAuthzRoleBindingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.ClusterAuthzRoleBinding.Use,
		Aliases: constants.ClusterAuthzRoleBinding.Aliases,
		Short:   constants.ClusterAuthzRoleBinding.Short,
		Long:    constants.ClusterAuthzRoleBinding.Long,
	}

	cmd.AddCommand(
		newListClusterAuthzRoleBindingCmd(),
		newGetClusterAuthzRoleBindingCmd(),
		newDeleteClusterAuthzRoleBindingCmd(),
	)

	return cmd
}

func newListClusterAuthzRoleBindingCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListClusterAuthzRoleBinding,
		Flags:   []flags.Flag{},
		RunE: func(fg *builder.FlagGetter) error {
			return clusterauthzrolebinding.New().List()
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}

func newGetClusterAuthzRoleBindingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetClusterAuthzRoleBinding.Use,
		Short:   constants.GetClusterAuthzRoleBinding.Short,
		Long:    constants.GetClusterAuthzRoleBinding.Long,
		Example: constants.GetClusterAuthzRoleBinding.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clusterauthzrolebinding.New().Get(clusterauthzrolebinding.GetParams{
				Name: args[0],
			})
		},
	}

	return cmd
}

func newDeleteClusterAuthzRoleBindingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteClusterAuthzRoleBinding.Use,
		Short:   constants.DeleteClusterAuthzRoleBinding.Short,
		Long:    constants.DeleteClusterAuthzRoleBinding.Long,
		Example: constants.DeleteClusterAuthzRoleBinding.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			return clusterauthzrolebinding.New().Delete(clusterauthzrolebinding.DeleteParams{
				Name: args[0],
			})
		},
	}

	return cmd
}
