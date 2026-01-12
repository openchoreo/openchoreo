// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/version"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	configContext "github.com/openchoreo/openchoreo/pkg/cli/cmd/config"
)

// serverVersionResponse represents the server version response from the API.
type serverVersionResponse struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	GitRevision string `json:"gitRevision"`
	BuildTime   string `json:"buildTime"`
	GoOS        string `json:"goOS"`
	GoArch      string `json:"goArch"`
	GoVersion   string `json:"goVersion"`
}

// NewVersionCmd creates the version command.
func NewVersionCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.Version,
		RunE: func(fg *builder.FlagGetter) error {
			// Print client version
			v := version.Get()
			fmt.Println("Client:")
			fmt.Printf("  Version:      %s\n", v.Version)
			fmt.Printf("  Git Revision: %s\n", v.GitRevision)
			fmt.Printf("  Build Time:   %s\n", v.BuildTime)
			fmt.Printf("  Go Version:   %s %s/%s\n", v.GoVersion, v.GoOS, v.GoArch)

			// Try to fetch server version if control plane is configured
			serverVersion, err := fetchServerVersion()
			if err != nil {
				fmt.Printf("\nServer: <not connected>\n")
				return nil
			}

			fmt.Println("\nServer:")
			fmt.Printf("  Version:      %s\n", serverVersion.Version)
			fmt.Printf("  Git Revision: %s\n", serverVersion.GitRevision)
			fmt.Printf("  Build Time:   %s\n", serverVersion.BuildTime)
			fmt.Printf("  Go Version:   %s %s/%s\n",
				serverVersion.GoVersion, serverVersion.GoOS, serverVersion.GoArch)

			return nil
		},
	}).Build()
}

// fetchServerVersion fetches the version information from the configured control plane.
func fetchServerVersion() (*serverVersionResponse, error) {
	// Load stored config to get control plane endpoint
	storedConfig, err := config.LoadStoredConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if storedConfig.CurrentContext == "" {
		return nil, fmt.Errorf("no current context set")
	}

	// Find current context
	var currentContext *configContext.Context
	for idx := range storedConfig.Contexts {
		if storedConfig.Contexts[idx].Name == storedConfig.CurrentContext {
			currentContext = &storedConfig.Contexts[idx]
			break
		}
	}

	if currentContext == nil {
		return nil, fmt.Errorf("current context '%s' not found", storedConfig.CurrentContext)
	}

	// Find control plane
	var controlPlane *configContext.ControlPlane
	for idx := range storedConfig.ControlPlanes {
		if storedConfig.ControlPlanes[idx].Name == currentContext.ControlPlane {
			controlPlane = &storedConfig.ControlPlanes[idx]
			break
		}
	}

	if controlPlane == nil || controlPlane.URL == "" {
		return nil, fmt.Errorf("control plane not configured")
	}

	// Create HTTP request with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := controlPlane.URL + "/version"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch server version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// Parse response
	var serverVersion serverVersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&serverVersion); err != nil {
		return nil, fmt.Errorf("failed to parse server version: %w", err)
	}

	return &serverVersion, nil
}
