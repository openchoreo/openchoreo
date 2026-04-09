// Copyright 2026 The OpenChoreo Authors
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

func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the client and server version",
		RunE: func(cmd *cobra.Command, args []string) error {
			v := version.Get()
			fmt.Println("Client:")
			fmt.Printf("  Version:      %s\n", v.Version)
			fmt.Printf("  Git Revision: %s\n", v.GitRevision)
			fmt.Printf("  Build Time:   %s\n", v.BuildTime)
			fmt.Printf("  Go Version:   %s %s/%s\n", v.GoVersion, v.GoOS, v.GoArch)

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
	}
}

func fetchServerVersion() (*serverVersionResponse, error) {
	controlPlane, err := config.GetCurrentControlPlane()
	if err != nil {
		return nil, fmt.Errorf("failed to get control plane: %w", err)
	}

	if controlPlane.URL == "" {
		return nil, fmt.Errorf("control plane URL not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := controlPlane.URL + "/version"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch server version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var serverVersion serverVersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&serverVersion); err != nil {
		return nil, fmt.Errorf("failed to parse server version: %w", err)
	}

	return &serverVersion, nil
}
