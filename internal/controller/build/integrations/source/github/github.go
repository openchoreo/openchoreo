package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v69/github"
	"gopkg.in/yaml.v3"

	"github.com/choreo-idp/choreo/internal/controller/build/common"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations/source"
)

type githubHandler struct {
	githubClient *github.Client
}

var _ source.SourceHandler[common.BuildContext] = (*githubHandler)(nil)

func NewGithubHandler(githubClient *github.Client) source.SourceHandler[common.BuildContext] {
	return &githubHandler{
		githubClient: githubClient,
	}
}

func (h *githubHandler) Name(ctx context.Context, builtCtx *common.BuildContext) string {
	return "SourceGithub"
}

func (h *githubHandler) FetchComponentDescriptor(ctx context.Context, buildCtx *common.BuildContext) (interface{}, error) {
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

	return config, nil
}
