// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/clustercomponenttype"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewClusterComponentTypeCmd() *cobra.Command {
	clusterComponentTypeCmd := &cobra.Command{
		Use:     constants.ClusterComponentType.Use,
		Aliases: constants.ClusterComponentType.Aliases,
		Short:   constants.ClusterComponentType.Short,
		Long:    constants.ClusterComponentType.Long,
	}

	clusterComponentTypeCmd.AddCommand(
		newListClusterComponentTypeCmd(),
	)

	return clusterComponentTypeCmd
}

func newListClusterComponentTypeCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListClusterComponentType,
		Flags:   []flags.Flag{},
		RunE: func(fg *builder.FlagGetter) error {
			return clustercomponenttype.New().List()
		},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
	}).Build()
}
