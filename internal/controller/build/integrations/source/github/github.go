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

package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v69/github"
	"gopkg.in/yaml.v3"

	"github.com/choreo-idp/choreo/internal/controller/build/integrations"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations/source"
)

type githubHandler struct {
	githubClient *github.Client
}

var _ source.SourceHandler[integrations.BuildContext] = (*githubHandler)(nil)

func NewGithubHandler(githubClient *github.Client) source.SourceHandler[integrations.BuildContext] {
	return &githubHandler{
		githubClient: githubClient,
	}
}

func (h *githubHandler) Name(ctx context.Context, builtCtx *integrations.BuildContext) string {
	return "SourceGithub"
}

func (h *githubHandler) FetchComponentDescriptor(ctx context.Context, buildCtx *integrations.BuildContext) (*source.Config, error) {
	owner, repositoryName, err := source.ExtractRepositoryInfo(buildCtx.Component.Spec.Source.GitRepository.URL)
	if err != nil {
		return nil, fmt.Errorf("bad git repository url: %w", err)
	}
	// If the build has a specific git revision, use it. Otherwise, use the default branch.
	ref := buildCtx.Build.Spec.Branch
	if buildCtx.Build.Spec.GitRevision != "" {
		ref = buildCtx.Build.Spec.GitRevision
	}

	componentYaml, _, _, err := h.githubClient.Repositories.GetContents(ctx, owner, repositoryName,
		source.MakeComponentDescriptorPath(buildCtx), &github.RepositoryContentGetOptions{Ref: ref})
	if err != nil {
		return nil, fmt.Errorf("failed to get component.yaml from the repository buildName:%s;owner:%s;repo:%s;%w", buildCtx.Build.Name, owner, repositoryName, err)
	}
	componentYamlContent, err := componentYaml.GetContent()
	if err != nil {
		return nil, fmt.Errorf("failed to get content of component.yaml from the repository  buildName:%s;owner:%s;repo:%s;%w", buildCtx.Build.Name, owner, repositoryName, err)
	}
	config := source.Config{}
	err = yaml.Unmarshal([]byte(componentYamlContent), &config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal component.yaml from the repository buildName:%s;owner:%s;repo:%s;%w", buildCtx.Build.Name, owner, repositoryName, err)
	}

	return &config, nil
}
