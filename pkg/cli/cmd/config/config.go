/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package config

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// StoredConfig is the structure to store configuration contexts.
type StoredConfig struct {
	CurrentContext string              `yaml:"currentContext"`
	Clusters       []KubernetesCluster `yaml:"clusters"`
	Contexts       []Context           `yaml:"contexts"`
}

// KubernetesCluster defines K8s cluster configuration
type KubernetesCluster struct {
	Name       string `yaml:"name"`
	Kubeconfig string `yaml:"kubeconfig"`
	Context    string `yaml:"context"`
}

// Context represents a single named configuration context.
type Context struct {
	Name         string `yaml:"name"`
	Organization string `yaml:"organization,omitempty"`
	Project      string `yaml:"project,omitempty"`
	Component    string `yaml:"component,omitempty"`
	Environment  string `yaml:"environment,omitempty"`
	DataPlane    string `yaml:"dataPlane,omitempty"`
	ClusterRef   string `yaml:"clusterRef,omitempty"` // Reference to KubernetesCluster
}

func NewConfigCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.ConfigRoot.Use,
		Short: constants.ConfigRoot.Short,
		Long:  constants.ConfigRoot.Long,
	}

	// Add all subcommands using CommandBuilder
	cmd.AddCommand(
		newGetContextsCmd(impl),
		newSetContextCmd(impl),
		newUseContextCmd(impl),
		newCurrentContextCmd(impl),
	)
	return cmd
}

func newGetContextsCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ConfigGetContexts,
		RunE: func(fg *builder.FlagGetter) error {
			return impl.GetContexts()
		},
	}).Build()
}

func newSetContextCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigSetContext,
		Flags: []flags.Flag{
			flags.Organization,
			flags.Project,
			flags.Component,
			flags.DataPlane,
			flags.Environment,
			flags.Kubeconfig,
			flags.KubeContext,
		},
		RunE: func(fg *builder.FlagGetter) error {
			args := fg.GetArgs()
			if len(args) == 0 {
				return fmt.Errorf("context name is required")
			}
			return impl.SetContext(api.SetContextParams{
				Name:           args[0],
				Organization:   fg.GetString(flags.Organization),
				Project:        fg.GetString(flags.Project),
				Component:      fg.GetString(flags.Component),
				DataPlane:      fg.GetString(flags.DataPlane),
				Environment:    fg.GetString(flags.Environment),
				KubeconfigPath: fg.GetString(flags.Kubeconfig),
				KubeContext:    fg.GetString(flags.KubeContext),
			})
		},
	}).Build()

	// Require exactly one argument for the context name
	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func newUseContextCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ConfigUseContext,
		Flags:   []flags.Flag{flags.Name},
		RunE: func(fg *builder.FlagGetter) error {
			args := fg.GetArgs()
			if len(args) == 0 {
				return fmt.Errorf("context name is required")
			}
			return impl.UseContext(api.UseContextParams{
				Name: args[0],
			})
		},
	}).Build()
}

func newCurrentContextCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ConfigCurrentContext,
		RunE: func(fg *builder.FlagGetter) error {
			return impl.GetCurrentContext()
		},
	}).Build()
}
