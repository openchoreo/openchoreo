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

package source

import (
	"fmt"
	"path"
	"strings"

	"github.com/choreo-idp/choreo/internal/controller/build/integrations"
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
