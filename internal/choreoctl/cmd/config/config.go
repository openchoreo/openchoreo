// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	configContext "github.com/openchoreo/openchoreo/pkg/cli/cmd/config"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
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
	headers := []string{"", "NAME", "ORGANIZATION", "PROJECT", "COMPONENT", "ENVIRONMENT", "DATAPLANE"}
	rows := make([][]string, 0, len(cfg.Contexts))

	for _, ctx := range cfg.Contexts {
		// Current context marker
		marker := " "
		if cfg.CurrentContext == ctx.Name {
			marker = "*"
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

	// Print control plane info if available
	if cfg.ControlPlane != nil {
		fmt.Println("\nControl Plane:")
		cpHeaders := []string{"PROPERTY", "VALUE"}
		tokenDisplay := "-"
		if cfg.ControlPlane.Token != "" {
			tokenDisplay = maskToken(cfg.ControlPlane.Token)
		}
		cpRows := [][]string{
			{"Type", cfg.ControlPlane.Type},
			{"Endpoint", cfg.ControlPlane.Endpoint},
			{"Token", tokenDisplay},
		}
		return printTable(cpHeaders, cpRows)
	}

	return nil
}

// SetContext creates or updates a configuration context with the given parameters.
func (c *ConfigContextImpl) SetContext(params api.SetContextParams) error {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create new context
	newCtx := configContext.Context{
		Name:         params.Name,
		Organization: params.Organization,
		Project:      params.Project,
		Component:    params.Component,
		Environment:  params.Environment,
		DataPlane:    params.DataPlane,
	}

	// Update or create the context
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

	// Find current context
	var curCtx *configContext.Context

	for _, c := range cfg.Contexts {
		if c.Name == cfg.CurrentContext {
			ctxCopy := c // Create copy to avoid pointer to loop variable
			curCtx = &ctxCopy
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
}

// getDefaultContextValues returns the default context values based on
// environment variables or predefined defaults aligned with Helm chart values
func getDefaultContextValues() DefaultContextValues {
	return DefaultContextValues{
		ContextName:  getEnvOrDefault("CHOREO_DEFAULT_CONTEXT", "default"),
		Organization: getEnvOrDefault("CHOREO_DEFAULT_ORG", "default"),
		Project:      getEnvOrDefault("CHOREO_DEFAULT_PROJECT", "default"),
		DataPlane:    getEnvOrDefault("CHOREO_DEFAULT_DATAPLANE", "default"),
		Environment:  getEnvOrDefault("CHOREO_DEFAULT_ENV", "development"),
	}
}

// getDefaultControlPlaneValues returns the default control plane configuration
func getDefaultControlPlaneValues() (string, string) {
	endpoint := getEnvOrDefault("CHOREO_API_ENDPOINT", "http://localhost:8080")
	token := getEnvOrDefault("CHOREO_API_TOKEN", "")
	return endpoint, token
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
		// Load existing config or create new if not exists
		cfg, err := LoadStoredConfig()
		if err != nil {
			return err
		}

		// If no contexts exist, create default context
		if len(cfg.Contexts) == 0 {
			// Get default values
			defaults := getDefaultContextValues()

			// Create default context
			defaultContext := configContext.Context{
				Name:         defaults.ContextName,
				Organization: defaults.Organization,
				Project:      defaults.Project,
				DataPlane:    defaults.DataPlane,
				Environment:  defaults.Environment,
			}
			cfg.Contexts = append(cfg.Contexts, defaultContext)

			// Set as current context
			cfg.CurrentContext = defaultContext.Name

			// Set default control plane configuration
			if cfg.ControlPlane == nil {
				endpoint, token := getDefaultControlPlaneValues()
				cfg.ControlPlane = &configContext.ControlPlane{
					Type:     "local",
					Endpoint: endpoint,
					Token:    token,
				}
			}

			// Save the config file
			if err := SaveStoredConfig(cfg); err != nil {
				return fmt.Errorf("failed to save default config: %w", err)
			}
		}
	}

	return nil
}

// SetControlPlane sets the control plane configuration
func (c *ConfigContextImpl) SetControlPlane(params api.SetControlPlaneParams) error {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Determine control plane type based on endpoint
	cpType := "remote"
	if strings.HasPrefix(params.Endpoint, "http://localhost") || strings.HasPrefix(params.Endpoint, "http://127.0.0.1") {
		cpType = "local"
	}

	// Create or update control plane configuration
	cfg.ControlPlane = &configContext.ControlPlane{
		Type:     cpType,
		Endpoint: params.Endpoint,
		Token:    params.Token,
	}

	if err := SaveStoredConfig(cfg); err != nil {
		return fmt.Errorf("failed to save control plane config: %w", err)
	}

	fmt.Printf("Control plane configured successfully:\n")
	fmt.Printf("  Type: %s\n", cpType)
	fmt.Printf("  Endpoint: %s\n", params.Endpoint)
	if params.Token != "" {
		fmt.Printf("  Token: %s\n", maskToken(params.Token))
	}

	return nil
}

// maskToken masks the token for display purposes
func maskToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}
