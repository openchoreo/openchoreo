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
