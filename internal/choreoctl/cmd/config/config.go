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
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

	configContext "github.com/choreo-idp/choreo/pkg/cli/cmd/config"
	"github.com/choreo-idp/choreo/pkg/cli/flags"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

// ConfigContextImpl implements context-related commands.
type ConfigContextImpl struct{}

// NewConfigContextImpl creates a new instance of ConfigContextImpl.
func NewConfigContextImpl() *ConfigContextImpl {
	return &ConfigContextImpl{}
}

// GetContexts prints all available contexts with their details.
func (c *ConfigContextImpl) GetContexts() error {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if len(cfg.Contexts) == 0 {
		fmt.Println("No contexts stored.")
		return nil
	}

	// Create headers and rows for table
	headers := []string{"", "NAME", "ORGANIZATION", "PROJECT", "COMPONENT", "ENVIRONMENT", "DATAPLANE", "K8S CONFIG", "K8S CONTEXT"}
	rows := make([][]string, 0, len(cfg.Contexts))

	for _, ctx := range cfg.Contexts {
		// Current context marker
		marker := " "
		if cfg.CurrentContext == ctx.Name {
			marker = "*"
		}

		// Get cluster details
		kubeconfig := "-"
		kubecontext := "-"
		if ctx.ClusterRef != "" {
			for _, cluster := range cfg.Clusters {
				if cluster.Name == ctx.ClusterRef {
					kubeconfig = formatValueOrPlaceholder(cluster.Kubeconfig)
					kubecontext = formatValueOrPlaceholder(cluster.Context)
					break
				}
			}
		}

		// Format row with proper placeholders
		rows = append(rows, []string{
			marker,
			formatValueOrPlaceholder(ctx.Name),
			formatValueOrPlaceholder(ctx.Organization),
			formatValueOrPlaceholder(ctx.Project),
			formatValueOrPlaceholder(ctx.Component),
			formatValueOrPlaceholder(ctx.Environment),
			formatValueOrPlaceholder(ctx.DataPlane),
			kubeconfig,
			kubecontext,
		})
	}

	return printTable(headers, rows)
}

// GetCurrentContext prints the current context details.
func (c *ConfigContextImpl) GetCurrentContext() error {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.CurrentContext == "" {
		fmt.Println("No current context is set.")
		return nil
	}

	var currentCtx *configContext.Context
	for _, ctx := range cfg.Contexts {
		if ctx.Name == cfg.CurrentContext {
			ctxCopy := ctx
			currentCtx = &ctxCopy
			break
		}
	}

	if currentCtx == nil {
		return fmt.Errorf("current context %q not found in config", cfg.CurrentContext)
	}

	// Context details
	headers := []string{"PROPERTY", "VALUE"}
	rows := [][]string{
		{"Current Context", formatValueOrPlaceholder(currentCtx.Name)},
		{"Organization", formatValueOrPlaceholder(currentCtx.Organization)},
		{"Project", formatValueOrPlaceholder(currentCtx.Project)},
		{"Component", formatValueOrPlaceholder(currentCtx.Component)},
		{"Environment", formatValueOrPlaceholder(currentCtx.Environment)},
		{"Data Plane", formatValueOrPlaceholder(currentCtx.DataPlane)},
	}

	if err := printTable(headers, rows); err != nil {
		return err
	}

	// Print cluster info if available
	if currentCtx.ClusterRef != "" {
		for _, cluster := range cfg.Clusters {
			if cluster.Name == currentCtx.ClusterRef {
				fmt.Println("\nCluster:")
				clusterHeaders := []string{"PROPERTY", "VALUE"}
				clusterRows := [][]string{
					{"Name", formatValueOrPlaceholder(cluster.Name)},
					{"Kubeconfig", formatValueOrPlaceholder(cluster.Kubeconfig)},
					{"Context", formatValueOrPlaceholder(cluster.Context)},
				}
				return printTable(clusterHeaders, clusterRows)
			}
		}
	}

	return nil
}

// SetContext creates or updates a configuration context with the given parameters.
func (c *ConfigContextImpl) SetContext(params api.SetContextParams) error {
	// 1. Load the stored choreo config
	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 2. Fill missing kubeconfig/context from current context if needed
	c.fillKubeconfigAndContextFromCurrent(cfg, &params)

	// 3. Fall back to default kubeconfig if still unset
	if err := c.fallbackToDefaultKubeconfig(&params); err != nil {
		return err
	}

	// 4. Validate kubeconfig and context
	if err := c.validateKubeconfigAndContext(&params); err != nil {
		return err
	}

	// 5. Reuse or create a cluster reference
	clusterRef, err := c.getOrCreateCluster(cfg, params)
	if err != nil {
		return err
	}

	// 6. Update or create the context
	if err := c.updateOrCreateContext(cfg, params, clusterRef); err != nil {
		return err
	}

	return nil
}

// fillKubeconfigAndContextFromCurrent updates params with kubeconfig/context from current context if they are missing.
func (c *ConfigContextImpl) fillKubeconfigAndContextFromCurrent(cfg *configContext.StoredConfig, params *api.SetContextParams) {
	if params.KubeconfigPath != "" || params.KubeContext != "" {
		return
	}
	if cfg.CurrentContext == "" {
		return
	}
	for _, ctx := range cfg.Contexts {
		if ctx.Name == cfg.CurrentContext {
			for _, cluster := range cfg.Clusters {
				if cluster.Name == ctx.ClusterRef {
					params.KubeconfigPath = cluster.Kubeconfig
					params.KubeContext = cluster.Context
					return
				}
			}
		}
	}
}

// fallbackToDefaultKubeconfig sets the default kubeconfig path if none is provided.
func (c *ConfigContextImpl) fallbackToDefaultKubeconfig(params *api.SetContextParams) error {
	if params.KubeconfigPath != "" {
		return nil
	}
	defaultPath, err := GetDefaultKubeconfigPath()
	if err != nil {
		return fmt.Errorf("failed to get default kubeconfig path: %w", err)
	}
	params.KubeconfigPath = defaultPath
	return nil
}

// validateKubeconfigAndContext ensures the kubeconfig file is valid and sets context if empty.
func (c *ConfigContextImpl) validateKubeconfigAndContext(params *api.SetContextParams) error {
	k8sCfg, err := clientcmd.LoadFromFile(params.KubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig from %s: %w", params.KubeconfigPath, err)
	}
	if params.KubeContext == "" {
		if k8sCfg.CurrentContext == "" {
			return fmt.Errorf("no current context in kubeconfig; please specify --kube-context")
		}
		params.KubeContext = k8sCfg.CurrentContext
	}
	return nil
}

// getOrCreateCluster reuses or creates a cluster entry for the given kubeconfig and context.
func (c *ConfigContextImpl) getOrCreateCluster(cfg *configContext.StoredConfig, params api.SetContextParams) (string, error) {
	absPath, err := filepath.Abs(params.KubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("cannot resolve kubeconfig path: %w", err)
	}

	// Check if a matching cluster already exists
	for i := range cfg.Clusters {
		cPath, cErr := filepath.Abs(cfg.Clusters[i].Kubeconfig)
		if cErr != nil {
			continue
		}
		if cPath == absPath && cfg.Clusters[i].Context == params.KubeContext {
			// Found exact match, reuse it
			return cfg.Clusters[i].Name, nil
		}
	}

	// Create a new cluster with a unique name
	clusterName := fmt.Sprintf("cluster-%s", params.KubeContext)
	newCluster := configContext.KubernetesCluster{
		Name:       clusterName,
		Kubeconfig: params.KubeconfigPath,
		Context:    params.KubeContext,
	}
	cfg.Clusters = append(cfg.Clusters, newCluster)
	return clusterName, nil
}

// updateOrCreateContext merges or creates the context and saves config.
func (c *ConfigContextImpl) updateOrCreateContext(
	cfg *configContext.StoredConfig,
	params api.SetContextParams,
	clusterRef string,
) error {
	newCtx := configContext.Context{
		Name:         params.Name,
		Organization: params.Organization,
		Project:      params.Project,
		Component:    params.Component,
		Environment:  params.Environment,
		DataPlane:    params.DataPlane,
		ClusterRef:   clusterRef,
	}

	found := false
	for i := range cfg.Contexts {
		if cfg.Contexts[i].Name == params.Name {
			// Preserve existing fields if not provided
			if params.Organization == "" {
				newCtx.Organization = cfg.Contexts[i].Organization
			}
			if params.Project == "" {
				newCtx.Project = cfg.Contexts[i].Project
			}
			if params.Component == "" {
				newCtx.Component = cfg.Contexts[i].Component
			}
			if params.Environment == "" {
				newCtx.Environment = cfg.Contexts[i].Environment
			}
			if params.DataPlane == "" {
				newCtx.DataPlane = cfg.Contexts[i].DataPlane
			}
			cfg.Contexts[i] = newCtx
			found = true
			break
		}
	}
	if !found {
		cfg.Contexts = append(cfg.Contexts, newCtx)
	}

	if err := SaveStoredConfig(cfg); err != nil {
		return fmt.Errorf("failed to save updated config: %w", err)
	}

	action := "Updated"
	if !found {
		action = "Created"
	}
	fmt.Printf("%s context: %s\n", action, params.Name)

	return nil
}

// UseContext sets the current context to the context with the given name.
func (c *ConfigContextImpl) UseContext(params api.UseContextParams) error {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	found := false
	for _, ctx := range cfg.Contexts {
		if ctx.Name == params.Name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("context '%s' not found", params.Name)
	}
	cfg.CurrentContext = params.Name
	if err := SaveStoredConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Printf("Now using context: %s\n", params.Name)
	return nil
}

// Add helper function to manage clusters

func (c *ConfigContextImpl) AddCluster(cluster *configContext.KubernetesCluster) error {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check for duplicate cluster names
	for _, existing := range cfg.Clusters {
		if existing.Name == cluster.Name {
			return fmt.Errorf("cluster %s already exists", cluster.Name)
		}
	}

	cfg.Clusters = append(cfg.Clusters, *cluster)
	return SaveStoredConfig(cfg)
}

// Add function to get cluster by name

func (c *ConfigContextImpl) GetCluster(name string) (*configContext.KubernetesCluster, error) {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	for _, cluster := range cfg.Clusters {
		if cluster.Name == name {
			return &cluster, nil
		}
	}
	return nil, fmt.Errorf("cluster %s not found", name)
}

// ApplyContextDefaults loads the stored config and sets default flag values
// from the current context, if not already provided.
func ApplyContextDefaults(cmd *cobra.Command) error {
	// Skip for certain commands to avoid circular dependencies
	if cmd.Parent() != nil && cmd.Parent().Name() == "config" {
		return nil
	}

	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// No defaults to apply if no current context
	if cfg.CurrentContext == "" {
		return nil
	}

	// Find current context and its cluster
	var curCtx *configContext.Context
	var curCluster *configContext.KubernetesCluster

	for _, c := range cfg.Contexts {
		if c.Name == cfg.CurrentContext {
			ctxCopy := c // Create copy to avoid pointer to loop variable
			curCtx = &ctxCopy

			// Find associated cluster
			if curCtx.ClusterRef != "" {
				for _, cluster := range cfg.Clusters {
					if cluster.Name == curCtx.ClusterRef {
						clusterCopy := cluster
						curCluster = &clusterCopy
						break
					}
				}
			}
			break
		}
	}

	if curCtx == nil {
		return fmt.Errorf("current context %q not found", cfg.CurrentContext)
	}

	// Apply context-based defaults only if flags not explicitly set
	applyIfNotSet(cmd, flags.Organization.Name, curCtx.Organization)
	applyIfNotSet(cmd, flags.Project.Name, curCtx.Project)
	applyIfNotSet(cmd, flags.Environment.Name, curCtx.Environment)
	applyIfNotSet(cmd, flags.Component.Name, curCtx.Component)
	applyIfNotSet(cmd, flags.DataPlane.Name, curCtx.DataPlane)

	// Apply cluster config if available
	if curCluster != nil {
		if cmd.Flags().Lookup("kubeconfig") != nil {
			applyIfNotSet(cmd, "kubeconfig", curCluster.Kubeconfig)
		}
		if cmd.Flags().Lookup("context") != nil {
			applyIfNotSet(cmd, "context", curCluster.Context)
		}
	}

	return nil
}

// Helper function to apply flag value if not already set
func applyIfNotSet(cmd *cobra.Command, flagName, value string) {
	if value != "" && !cmd.Flags().Changed(flagName) {
		if flag := cmd.Flags().Lookup(flagName); flag != nil {
			_ = cmd.Flags().Set(flagName, value)
		}
	}
}

// DefaultContextValues defines default values for context initialization
type DefaultContextValues struct {
	ContextName  string
	Organization string
	Project      string
	DataPlane    string
	Environment  string
	ClusterName  string
}

// getDefaultContextValues returns the default context values based on
// environment variables or predefined defaults aligned with Helm chart values
func getDefaultContextValues() DefaultContextValues {
	return DefaultContextValues{
		ContextName:  getEnvOrDefault("CHOREO_DEFAULT_CONTEXT", "default"),
		Organization: getEnvOrDefault("CHOREO_DEFAULT_ORG", "default-org"),
		Project:      getEnvOrDefault("CHOREO_DEFAULT_PROJECT", "default-project"),
		DataPlane:    getEnvOrDefault("CHOREO_DEFAULT_DATAPLANE", "default-dataplane"),
		Environment:  getEnvOrDefault("CHOREO_DEFAULT_ENV", "development"),
		ClusterName:  getEnvOrDefault("CHOREO_DEFAULT_CLUSTER", "default-cluster"),
	}
}

// getEnvOrDefault returns the value of the environment variable or the default value if not set
func getEnvOrDefault(envVar, defaultValue string) string {
	if value := os.Getenv(envVar); value != "" {
		return value
	}
	return defaultValue
}

// EnsureContext creates and sets a default context if none exists.
func EnsureContext() error {
	if !IsConfigFileExists() {
		// Get default kubeconfig path and context
		kubeconfigPath, kubeContext, err := GetDefaultKubeconfigWithContext()
		if err != nil {
			return err
		}

		// Load existing config or create new if not exists
		cfg, err := LoadStoredConfig()
		if err != nil {
			return err
		}

		// If no contexts exist, create default context with cluster
		if len(cfg.Contexts) == 0 {
			// Get default values
			defaults := getDefaultContextValues()

			// Add default cluster mapping
			defaultCluster := configContext.KubernetesCluster{
				Name:       defaults.ClusterName,
				Kubeconfig: kubeconfigPath,
				Context:    kubeContext,
			}
			cfg.Clusters = append(cfg.Clusters, defaultCluster)

			// Create default context
			defaultContext := configContext.Context{
				Name:         defaults.ContextName,
				Organization: defaults.Organization,
				Project:      defaults.Project,
				DataPlane:    defaults.DataPlane,
				Environment:  defaults.Environment,
				ClusterRef:   defaultCluster.Name,
			}
			cfg.Contexts = append(cfg.Contexts, defaultContext)

			// Set as current context
			cfg.CurrentContext = defaultContext.Name

			// Save the config file
			if err := SaveStoredConfig(cfg); err != nil {
				return fmt.Errorf("failed to save default config: %w", err)
			}
		}
	}

	return nil
}
