// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package logs

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/occ/interactive"
	"github.com/openchoreo/openchoreo/internal/occ/resources"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

const (
	stateOrgSelect = iota
	stateProjSelect
	stateCompSelect
	stateTypeSelect
	stateBuildSelect
	stateEnvSelect
	stateDeploymentSelect
)

// Log types
const (
	logTypeBuild      = "build"
	logTypeDeployment = "deployment"
)

type logModel struct {
	interactive.BaseModel
	builds       []string
	buildCursor  int
	environments []string
	envCursor    int
	deployments  []string
	deplCursor   int
	logTypes     []string
	typeCursor   int
	state        int
	selected     bool
	errorMsg     string
}

func (m logModel) FetchDeployments() ([]string, error) {
	if m.OrgCursor >= len(m.Organizations) ||
		m.ProjCursor >= len(m.Projects) ||
		m.CompCursor >= len(m.Components) ||
		m.envCursor >= len(m.environments) {
		return nil, fmt.Errorf("invalid selection indices for deployments")
	}

	k8sClient, err := resources.GetClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	pods := &corev1.PodList{}
	if err := k8sClient.List(context.Background(), pods,
		client.MatchingLabels{
			"organization-name": m.Organizations[m.OrgCursor],
			"project-name":      m.Projects[m.ProjCursor],
			"component-name":    m.Components[m.CompCursor],
			"environment-name":  m.environments[m.envCursor],
			"belong-to":         "user-workloads",
			"managed-by":        "choreo-deployment-controller",
		}); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	set := map[string]struct{}{}
	for _, pod := range pods.Items {
		if name := pod.Labels["deployment-name"]; name != "" {
			set[name] = struct{}{}
		}
	}

	deployments := make([]string, 0, len(set))
	for name := range set {
		deployments = append(deployments, name)
	}
	sort.Strings(deployments)
	return deployments, nil
}

func (m logModel) Init() tea.Cmd {
	return nil
}

func (m logModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if interactive.IsQuitKey(keyMsg) {
		m.selected = false
		return m, tea.Quit
	}

	switch m.state {
	case stateOrgSelect:
		if interactive.IsEnterKey(keyMsg) {
			projects, err := m.FetchProjects()
			if err != nil {
				m.errorMsg = err.Error()
				m.selected = false
				return m, tea.Quit
			}
			if len(projects) == 0 {
				m.errorMsg = fmt.Sprintf("No projects found in organization '%s'. Please create a project first using 'occ create project'",
					m.Organizations[m.OrgCursor])
				m.selected = false
				return m, tea.Quit
			}
			m.Projects = projects
			m.state = stateProjSelect
			m.errorMsg = ""
			return m, nil
		}
		m.OrgCursor = interactive.ProcessListCursor(keyMsg, m.OrgCursor, len(m.Organizations))

	case stateProjSelect:
		if interactive.IsEnterKey(keyMsg) {
			components, err := m.FetchComponents()
			if err != nil {
				m.errorMsg = err.Error()
				m.selected = false
				return m, tea.Quit
			}
			if len(components) == 0 {
				m.errorMsg = fmt.Sprintf("No components found in project '%s'. Please create a component first using 'occ create component'",
					m.Projects[m.ProjCursor])
				m.selected = false
				return m, tea.Quit
			}
			m.Components = components
			m.state = stateCompSelect
			m.errorMsg = ""
			return m, nil
		}
		m.ProjCursor = interactive.ProcessListCursor(keyMsg, m.ProjCursor, len(m.Projects))

	case stateCompSelect:
		if interactive.IsEnterKey(keyMsg) {
			m.state = stateTypeSelect
			m.logTypes = []string{logTypeBuild, logTypeDeployment}
			return m, nil
		}
		m.CompCursor = interactive.ProcessListCursor(keyMsg, m.CompCursor, len(m.Components))

	case stateTypeSelect:
		if interactive.IsEnterKey(keyMsg) {
			if m.logTypes[m.typeCursor] == logTypeBuild {
				builds, err := m.FetchBuildNames()
				if err != nil {
					m.errorMsg = err.Error()
					return m, tea.Quit
				}
				if len(builds) == 0 {
					m.errorMsg = fmt.Sprintf("No builds found in component '%s'", m.Components[m.CompCursor])
					return m, tea.Quit
				}
				m.builds = builds
				m.state = stateBuildSelect
			} else {
				environments, err := m.FetchEnvironments()
				if err != nil {
					m.errorMsg = err.Error()
					return m, tea.Quit
				}
				if len(environments) == 0 {
					m.errorMsg = fmt.Sprintf("No environments found in organization '%s'", m.Organizations[m.OrgCursor])
					return m, tea.Quit
				}
				m.environments = environments
				m.state = stateEnvSelect
			}
			return m, nil
		}
		m.typeCursor = interactive.ProcessListCursor(keyMsg, m.typeCursor, len(m.logTypes))

	case stateBuildSelect:
		if interactive.IsEnterKey(keyMsg) {
			m.selected = true
			return m, tea.Quit
		}
		m.buildCursor = interactive.ProcessListCursor(keyMsg, m.buildCursor, len(m.builds))

	case stateEnvSelect:
		if interactive.IsEnterKey(keyMsg) {
			deploymentList, err := m.FetchDeployments()
			if err != nil {
				m.errorMsg = fmt.Sprintf("Failed to get deployments: %v", err)
				return m, tea.Quit
			}
			if len(deploymentList) == 0 {
				m.errorMsg = fmt.Sprintf("No deployments found for component '%s' in environment '%s'",
					m.Components[m.CompCursor], m.environments[m.envCursor])
				return m, tea.Quit
			}

			m.deployments = deploymentList
			m.state = stateDeploymentSelect
			return m, nil
		}
		m.envCursor = interactive.ProcessListCursor(keyMsg, m.envCursor, len(m.environments))

	case stateDeploymentSelect:
		if interactive.IsEnterKey(keyMsg) {
			m.selected = true
			return m, tea.Quit
		}
		m.deplCursor = interactive.ProcessListCursor(keyMsg, m.deplCursor, len(m.deployments))
	}

	return m, nil
}

func (m logModel) RenderProgress() string {
	var progress strings.Builder
	progress.WriteString("Selected resources:\n")

	if len(m.Organizations) > 0 {
		progress.WriteString(fmt.Sprintf("- organization: %s\n", m.Organizations[m.OrgCursor]))
	}
	if len(m.Projects) > 0 {
		progress.WriteString(fmt.Sprintf("- project: %s\n", m.Projects[m.ProjCursor]))
	}
	if len(m.Components) > 0 {
		progress.WriteString(fmt.Sprintf("- component: %s\n", m.Components[m.CompCursor]))
	}
	if len(m.logTypes) > 0 && m.state > stateTypeSelect {
		progress.WriteString(fmt.Sprintf("- type: %s\n", m.logTypes[m.typeCursor]))
	}
	if len(m.builds) > 0 {
		progress.WriteString(fmt.Sprintf("- build: %s\n", m.builds[m.buildCursor]))
	}
	if len(m.environments) > 0 && m.state > stateEnvSelect {
		progress.WriteString(fmt.Sprintf("- environment: %s\n", m.environments[m.envCursor]))
	}
	if len(m.deployments) > 0 && m.state > stateDeploymentSelect {
		progress.WriteString(fmt.Sprintf("- deployment: %s\n", m.deployments[m.deplCursor]))
	}

	return progress.String()
}

func (m logModel) View() string {
	progress := m.RenderProgress()
	var view string

	switch m.state {
	case stateOrgSelect:
		view = m.RenderOrgSelection()
	case stateProjSelect:
		view = m.RenderProjSelection()
	case stateCompSelect:
		view = m.RenderComponentSelection()
	case stateTypeSelect:
		view = interactive.RenderListPrompt("Select log type:", m.logTypes, m.typeCursor)
	case stateBuildSelect:
		view = interactive.RenderListPrompt("Select build:", m.builds, m.buildCursor)
	case stateEnvSelect:
		view = interactive.RenderListPrompt("Select environment:", m.environments, m.envCursor)
	case stateDeploymentSelect:
		view = interactive.RenderListPrompt("Select deployment:", m.deployments, m.deplCursor)
	}

	if m.errorMsg != "" {
		view += "\nError: " + m.errorMsg
	}

	return progress + view
}

func getLogsInteractive() error {
	baseModel, err := interactive.NewBaseModel()
	if err != nil {
		return err
	}

	model := logModel{
		BaseModel: *baseModel,
		state:     stateOrgSelect,
	}

	finalModel, err := interactive.RunInteractiveModel(model)
	if err != nil {
		return fmt.Errorf("interactive mode failed: %w", err)
	}

	m, ok := finalModel.(logModel)
	if !ok || !m.selected {
		if m.errorMsg != "" {
			return fmt.Errorf("%s", m.errorMsg)
		}
		return fmt.Errorf("log viewing cancelled")
	}

	params := api.LogParams{
		Organization: m.Organizations[m.OrgCursor],
		Project:      m.Projects[m.ProjCursor],
		Component:    m.Components[m.CompCursor],
		Type:         m.logTypes[m.typeCursor],
	}

	if m.logTypes[m.typeCursor] == logTypeBuild {
		params.Build = m.builds[m.buildCursor]
	} else {
		params.Environment = m.environments[m.envCursor]
		params.Deployment = m.deployments[m.deplCursor]
	}

	if err := handleLogs(params); err != nil {
		return err
	}

	flags := map[string]string{
		"type":         params.Type,
		"organization": params.Organization,
		"project":      params.Project,
		"component":    params.Component,
	}

	if params.Type == "build" {
		flags["build"] = params.Build
	} else {
		flags["environment"] = params.Environment
		flags["deployment"] = params.Deployment
	}

	return nil
}
