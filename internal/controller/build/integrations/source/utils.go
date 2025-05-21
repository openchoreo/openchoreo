/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package source

import (
	"fmt"
	"path"
	"strings"

	"github.com/openchoreo/openchoreo/internal/controller/build/integrations"
)

func ExtractRepositoryInfo(repoURL string) (string, string, error) {
	if repoURL == "" {
		return "", "", fmt.Errorf("repository URL is empty")
	}
	if strings.Split(repoURL, "/")[0] != "https:" {
		return "", "", fmt.Errorf("invalid repository URL")
	}
	urlSegments := strings.Split(repoURL, "/")
	start := 0
	len := len(urlSegments)
	if len > 2 {
		start = len - 2
	}
	owner := urlSegments[start]
	repo := urlSegments[start+1]
	return owner, repo, nil
}

func MakeComponentDescriptorPath(buildCtx *integrations.BuildContext) string {
	componentManifestPath := "./.choreo/component.yaml"
	if buildCtx.Build.Spec.Path != "" {
		componentManifestPath = path.Clean(fmt.Sprintf(".%s/.choreo/component.yaml", buildCtx.Build.Spec.Path))
	}
	return componentManifestPath
}
