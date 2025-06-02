// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/openchoreo/openchoreo/internal/choreoctl/cmd/config"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type ApplyImpl struct{}

func NewApplyImpl() *ApplyImpl {
	return &ApplyImpl{}
}

func (i *ApplyImpl) Apply(params api.ApplyParams) error {
	if err := validation.ValidateParams(validation.CmdApply, validation.ResourceApply, params); err != nil {
		return err
	}

	// TODO: Properly fix this, This is a quick fix to support remote URLs for samples
	isRemoteURL := strings.HasPrefix(params.FilePath, "http://") ||
		strings.HasPrefix(params.FilePath, "https://")

	// Only perform file existence/permission checks if NOT a remote URL
	if !isRemoteURL {
		if _, err := os.Stat(params.FilePath); os.IsNotExist(err) {
			return fmt.Errorf("file %s does not exist", params.FilePath)
		}

		if _, err := os.ReadFile(params.FilePath); err != nil {
			if os.IsPermission(err) {
				return fmt.Errorf("permission denied: %s", params.FilePath)
			}
			return fmt.Errorf("error reading file: %s", params.FilePath)
		}
	}

	kubeconfig, context, err := config.GetStoredKubeConfigValues()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig values: %w", err)
	}

	// Execute kubectl apply ToDo: move to use k8s client instead of kubectl
	cmd := exec.Command("kubectl",
		"--kubeconfig", kubeconfig,
		"--context", context,
		"apply",
		"-f", params.FilePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error applying file: %s", params.FilePath)
	}

	if isRemoteURL {
		fmt.Printf("Successfully applied the remote file: %s\n", params.FilePath)
	} else {
		fmt.Printf("Successfully applied file: %s\n", params.FilePath)
	}
	return nil
}
