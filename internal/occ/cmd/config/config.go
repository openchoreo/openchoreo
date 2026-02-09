// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"

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

// AddContext creates a new configuration context.
func (c *ConfigContextImpl) AddContext(params api.AddContextParams) error {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate that the name is not already used by a context, control plane, or credential
	if err := validateNameUniqueness(cfg, params.Name); err != nil {
		return err
	}

	// Create control plane entry if it doesn't exist
	cpExists := false
	for _, cp := range cfg.ControlPlanes {
		if cp.Name == params.ControlPlane {
			cpExists = true
			break
		}
	}
	if !cpExists {
		cfg.ControlPlanes = append(cfg.ControlPlanes, configContext.ControlPlane{
			Name: params.ControlPlane,
		})
	}

	// Create credential entry if it doesn't exist
	credExists := false
	for _, cred := range cfg.Credentials {
		if cred.Name == params.Credentials {
			credExists = true
			break
		}
	}
	if !credExists {
		cfg.Credentials = append(cfg.Credentials, configContext.Credential{
			Name: params.Credentials,
		})
	}

	// Create the new context
	newCtx := configContext.Context{
		Name:         params.Name,
		ControlPlane: params.ControlPlane,
		Credentials:  params.Credentials,
		Namespace:    params.Namespace,
		Project:      params.Project,
		Component:    params.Component,
	}
	cfg.Contexts = append(cfg.Contexts, newCtx)

	if err := SaveStoredConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Created context: %s\n", params.Name)
	return nil
}

// ListContexts prints all available contexts with their details.
func (c *ConfigContextImpl) ListContexts() error {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if len(cfg.Contexts) == 0 {
		fmt.Println("No contexts stored.")
		return nil
	}

	headers := []string{"", "NAME", "CONTROLPLANE", "CREDENTIALS", "NAMESPACE", "PROJECT", "COMPONENT"}
	rows := make([][]string, 0, len(cfg.Contexts))

	for _, ctx := range cfg.Contexts {
		marker := " "
		if cfg.CurrentContext == ctx.Name {
			marker = "*"
		}

		rows = append(rows, []string{
			marker,
			formatValueOrPlaceholder(ctx.Name),
			formatValueOrPlaceholder(ctx.ControlPlane),
			formatValueOrPlaceholder(ctx.Credentials),
			formatValueOrPlaceholder(ctx.Namespace),
			formatValueOrPlaceholder(ctx.Project),
			formatValueOrPlaceholder(ctx.Component),
		})
	}

	return printTable(headers, rows)
}

// DeleteContext removes a configuration context by name.
func (c *ConfigContextImpl) DeleteContext(params api.DeleteContextParams) error {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	found := false
	for i, ctx := range cfg.Contexts {
		if ctx.Name == params.Name {
			cfg.Contexts = append(cfg.Contexts[:i], cfg.Contexts[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("context %q not found", params.Name)
	}

	// Clear current context if it was the deleted one
	if cfg.CurrentContext == params.Name {
		cfg.CurrentContext = ""
	}

	if err := SaveStoredConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Deleted context: %s\n", params.Name)
	return nil
}

// UpdateContext updates an existing configuration context.
func (c *ConfigContextImpl) UpdateContext(params api.UpdateContextParams) error {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	found := false
	for i := range cfg.Contexts {
		if cfg.Contexts[i].Name == params.Name {
			if params.Namespace != "" {
				cfg.Contexts[i].Namespace = params.Namespace
			}
			if params.Project != "" {
				cfg.Contexts[i].Project = params.Project
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("context %q not found", params.Name)
	}

	if err := SaveStoredConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Updated context: %s\n", params.Name)
	return nil
}

// validateNameUniqueness checks that the given name is not already used by a context, control plane, or credential.
func validateNameUniqueness(cfg *configContext.StoredConfig, name string) error {
	for _, ctx := range cfg.Contexts {
		if ctx.Name == name {
			return fmt.Errorf("name %q is already used by a context", name)
		}
	}
	for _, cp := range cfg.ControlPlanes {
		if cp.Name == name {
			return fmt.Errorf("name %q is already used by a control plane", name)
		}
	}
	for _, cred := range cfg.Credentials {
		if cred.Name == name {
			return fmt.Errorf("name %q is already used by a credential", name)
		}
	}
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

// DescribeContext prints the details of a named context.
func (c *ConfigContextImpl) DescribeContext(params api.DescribeContextParams) error {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	for _, ctx := range cfg.Contexts {
		if ctx.Name == params.Name {
			return printContextDetails(cfg, &ctx)
		}
	}

	return fmt.Errorf("context %q not found", params.Name)
}

// printContextDetails prints the details of a context in a property/value table.
func printContextDetails(cfg *configContext.StoredConfig, ctx *configContext.Context) error {
	headers := []string{"PROPERTY", "VALUE"}
	rows := [][]string{
		{"Name", formatValueOrPlaceholder(ctx.Name)},
		{"Control Plane", formatValueOrPlaceholder(ctx.ControlPlane)},
		{"Credentials", formatValueOrPlaceholder(ctx.Credentials)},
		{"Namespace", formatValueOrPlaceholder(ctx.Namespace)},
		{"Project", formatValueOrPlaceholder(ctx.Project)},
		{"Component", formatValueOrPlaceholder(ctx.Component)},
		{"Mode", formatValueOrPlaceholder(ctx.Mode)},
		{"Root Directory Path", formatValueOrPlaceholder(ctx.RootDirectoryPath)},
	}

	if err := printTable(headers, rows); err != nil {
		return err
	}

	// Print control plane info if available and not in file-system mode
	if ctx.ControlPlane != "" && ctx.Mode != configContext.ModeFileSystem {
		for _, cp := range cfg.ControlPlanes {
			if cp.Name == ctx.ControlPlane {
				fmt.Println("\nControl Plane:")
				cpHeaders := []string{"PROPERTY", "VALUE"}
				tokenDisplay := "-"
				for _, cred := range cfg.Credentials {
					if cred.Name == ctx.Credentials && cred.Token != "" {
						tokenDisplay = maskToken(cred.Token)
						break
					}
				}
				cpRows := [][]string{
					{"Name", cp.Name},
					{"URL", cp.URL},
					{"Token", tokenDisplay},
				}
				return printTable(cpHeaders, cpRows)
			}
		}
	}

	return nil
}

// ApplyContextDefaults loads the stored config and sets default flag values
// from the current context, if not already provided.
func ApplyContextDefaults(cmd *cobra.Command) error {
	// Skip for config commands to avoid circular dependencies
	if cmd.Parent() != nil && (cmd.Parent().Name() == "config" || cmd.Parent().Name() == "context" || cmd.Parent().Name() == "controlplane") {
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
	applyIfNotSet(cmd, flags.Namespace.Name, curCtx.Namespace)
	applyIfNotSet(cmd, flags.Project.Name, curCtx.Project)
	applyIfNotSet(cmd, flags.Component.Name, curCtx.Component)

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
	Namespace    string
	Project      string
	Credentials  string
	ControlPlane string
}

// getDefaultContextValues returns the default context values based on
// environment variables or predefined defaults aligned with Helm chart values
func getDefaultContextValues() DefaultContextValues {
	return DefaultContextValues{
		ContextName:  getEnvOrDefault("CHOREO_DEFAULT_CONTEXT", "default"),
		Namespace:    getEnvOrDefault("CHOREO_DEFAULT_ORG", "default"),
		Project:      getEnvOrDefault("CHOREO_DEFAULT_PROJECT", "default"),
		Credentials:  getEnvOrDefault("CHOREO_DEFAULT_CREDENTIAL", "default"),
		ControlPlane: getEnvOrDefault("CHOREO_DEFAULT_CONTROLPLANE", "default"),
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
				Namespace:    defaults.Namespace,
				Project:      defaults.Project,
				Credentials:  defaults.Credentials,
				ControlPlane: defaults.ControlPlane,
			}
			cfg.Contexts = append(cfg.Contexts, defaultContext)

			// Set as current context
			cfg.CurrentContext = defaultContext.Name

			// Set default control plane configuration if not exists
			if len(cfg.ControlPlanes) == 0 {
				endpoint, _ := getDefaultControlPlaneValues()
				cfg.ControlPlanes = []configContext.ControlPlane{
					{
						Name: defaults.ControlPlane,
						URL:  endpoint,
					},
				}
			}

			if len(cfg.Credentials) == 0 {
				cfg.Credentials = []configContext.Credential{
					{
						Name: defaults.Credentials,
					},
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

// AddControlPlane adds a new control plane configuration.
func (c *ConfigContextImpl) AddControlPlane(params api.AddControlPlaneParams) error {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate that the name is not already used
	if err := validateNameUniqueness(cfg, params.Name); err != nil {
		return err
	}

	cfg.ControlPlanes = append(cfg.ControlPlanes, configContext.ControlPlane{
		Name: params.Name,
		URL:  params.URL,
	})

	if err := SaveStoredConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Created control plane: %s\n", params.Name)
	return nil
}

// ListControlPlanes prints all control plane configurations.
func (c *ConfigContextImpl) ListControlPlanes() error {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if len(cfg.ControlPlanes) == 0 {
		fmt.Println("No control planes stored.")
		return nil
	}

	headers := []string{"NAME", "URL"}
	rows := make([][]string, 0, len(cfg.ControlPlanes))

	for _, cp := range cfg.ControlPlanes {
		rows = append(rows, []string{
			formatValueOrPlaceholder(cp.Name),
			formatValueOrPlaceholder(cp.URL),
		})
	}

	return printTable(headers, rows)
}

// UpdateControlPlane updates an existing control plane configuration.
func (c *ConfigContextImpl) UpdateControlPlane(params api.UpdateControlPlaneParams) error {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	found := false
	for idx := range cfg.ControlPlanes {
		if cfg.ControlPlanes[idx].Name == params.Name {
			if params.URL != "" {
				cfg.ControlPlanes[idx].URL = params.URL
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("control plane %q not found", params.Name)
	}

	if err := SaveStoredConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Updated control plane: %s\n", params.Name)
	return nil
}

// DeleteControlPlane removes a control plane configuration by name.
func (c *ConfigContextImpl) DeleteControlPlane(params api.DeleteControlPlaneParams) error {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	found := false
	for i, cp := range cfg.ControlPlanes {
		if cp.Name == params.Name {
			cfg.ControlPlanes = append(cfg.ControlPlanes[:i], cfg.ControlPlanes[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("control plane %q not found", params.Name)
	}

	if err := SaveStoredConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Deleted control plane: %s\n", params.Name)
	return nil
}

// maskToken masks the token for display purposes
func maskToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

// GetCurrentContext returns the current context
func GetCurrentContext() (*configContext.Context, error) {
	cfg, err := LoadStoredConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.CurrentContext == "" {
		return nil, fmt.Errorf("no current context set")
	}

	// Find current context
	for idx := range cfg.Contexts {
		if cfg.Contexts[idx].Name == cfg.CurrentContext {
			return &cfg.Contexts[idx], nil
		}
	}

	return nil, fmt.Errorf("current context '%s' not found", cfg.CurrentContext)
}

// GetCurrentCredential returns the credential for the current context
func GetCurrentCredential() (*configContext.Credential, error) {
	currentContext, err := GetCurrentContext()
	if err != nil {
		return nil, err
	}

	if currentContext.Credentials == "" {
		return nil, fmt.Errorf("no credentials associated with current context")
	}

	cfg, err := LoadStoredConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Find credential
	for idx := range cfg.Credentials {
		if cfg.Credentials[idx].Name == currentContext.Credentials {
			return &cfg.Credentials[idx], nil
		}
	}

	return nil, fmt.Errorf("credential '%s' not found", currentContext.Credentials)
}

// GetCurrentControlPlane returns the control plane for the current context
func GetCurrentControlPlane() (*configContext.ControlPlane, error) {
	currentContext, err := GetCurrentContext()
	if err != nil {
		return nil, err
	}

	cfg, err := LoadStoredConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Find control plane
	for idx := range cfg.ControlPlanes {
		if cfg.ControlPlanes[idx].Name == currentContext.ControlPlane {
			return &cfg.ControlPlanes[idx], nil
		}
	}

	return nil, fmt.Errorf("control plane '%s' not found", currentContext.ControlPlane)
}
