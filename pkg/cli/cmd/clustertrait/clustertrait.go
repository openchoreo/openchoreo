// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewClusterTraitCmd(impl api.CommandImplementationInterface) *cobra.Command {
	clusterTraitCmd := &cobra.Command{
		Use:     constants.ClusterTrait.Use,
		Aliases: constants.ClusterTrait.Aliases,
		Short:   constants.ClusterTrait.Short,
		Long:    constants.ClusterTrait.Long,
	}

	clusterTraitCmd.AddCommand(
		newListClusterTraitCmd(impl),
	)

	return clusterTraitCmd
}

func newListClusterTraitCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListClusterTrait,
		Flags:   []flags.Flag{},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.ListClusterTraits()
		},
		PreRunE: auth.RequireLogin(impl),
	}).Build()
}
