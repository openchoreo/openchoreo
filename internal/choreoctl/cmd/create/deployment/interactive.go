// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deployment

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/openchoreo/openchoreo/internal/choreoctl/interactive"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

const (
	stateOrgSelect = iota
	stateProjSelect
	stateCompSelect
	stateDeploymentTrackSelect
	stateEnvSelect
	stateDeployArtifactSelect
	stateNameInput
)

type deploymentModel struct {
	interactive.BaseModel
	environments        []string
	deployableArtifacts []string
	deploymentTracks    []string

	envCursor      int
	artifactCursor int
	trackCursor    int

	name     string
	selected bool
	errorMsg string
	state    int
}

func (m deploymentModel) Init() tea.Cmd {
	return nil
}

func (m deploymentModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if interactive.IsQuitKey(keyMsg) {
		m.selected = false
		return m, tea.Quit
	}

	switch {
	case m.state <= stateProjSelect:
		return m.handleResourceSelection(keyMsg)
	case m.state <= stateDeploymentTrackSelect:
		return m.handleTrackSelection(keyMsg)
	default:
		return m.handleDeploymentConfig(keyMsg)
	}
}

func (m deploymentModel) handleResourceSelection(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
				m.errorMsg = fmt.Sprintf("No projects found in organization '%s'. Please create a project first using 'choreoctl create project'",
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
			cmd, err := m.UpdateProjSelect(keyMsg)
			if err != nil {
				m.errorMsg = err.Error()
				return m, tea.Quit
			}
			m.state = stateCompSelect
			m.errorMsg = ""
			return m, cmd
		}
		m.ProjCursor = interactive.ProcessListCursor(keyMsg, m.ProjCursor, len(m.Projects))
	}
	return m, nil
}

func (m deploymentModel) handleTrackSelection(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateCompSelect:
		return m.handleComponentSelect(keyMsg)
	case stateDeploymentTrackSelect:
		return m.handleDeploymentTrackSelect(keyMsg)
	}
	return m, nil
}

func (m deploymentModel) handleDeploymentConfig(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateEnvSelect:
		return m.handleEnvironmentSelect(keyMsg)
	case stateDeployArtifactSelect:
		return m.handleArtifactSelect(keyMsg)
	case stateNameInput:
		return m.handleNameInput(keyMsg)
	}
	return m, nil
}

func (m deploymentModel) handleComponentSelect(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !interactive.IsEnterKey(keyMsg) {
		m.CompCursor = interactive.ProcessListCursor(keyMsg, m.CompCursor, len(m.Components))
		return m, nil
	}
	tracks, err := m.FetchDeploymentTracks()
	if err != nil {
		m.errorMsg = fmt.Sprintf("Failed to fetch deployment tracks: %v", err)
		return m, nil
	}
	if len(tracks) == 0 {
		m.errorMsg = fmt.Sprintf("No deployment tracks found in component '%s'. Please create a deployment track first using 'choreoctl create deploymenttrack'",
			m.Components[m.CompCursor])
		return m, tea.Quit
	}
	m.deploymentTracks = tracks
	m.state = stateDeploymentTrackSelect
	m.errorMsg = ""
	return m, nil
}

func (m deploymentModel) handleDeploymentTrackSelect(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		// After track selection, get environments
		environments, err := m.FetchEnvironments()
		if err != nil {
			m.errorMsg = fmt.Sprintf("Failed to fetch environments: %v", err)
			return m, nil
		}
		if len(environments) == 0 {
			m.errorMsg = fmt.Sprintf("No environments found for organization: %s", m.Organizations[m.OrgCursor])
			return m, tea.Quit
		}
		m.environments = environments
		m.state = stateEnvSelect
		m.errorMsg = ""
		return m, nil
	}
	m.trackCursor = interactive.ProcessListCursor(keyMsg, m.trackCursor, len(m.deploymentTracks))
	return m, nil
}

func (m deploymentModel) handleEnvironmentSelect(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		artifacts, err := m.FetchDeployableArtifacts()
		if err != nil {
			m.errorMsg = fmt.Sprintf("Failed to fetch deployable artifacts: %v", err)
			return m, nil
		}
		if len(artifacts) == 0 {
			m.errorMsg = fmt.Sprintf("No deployable artifacts found in component '%s'. Please create a deployable artifact first using 'choreoctl create deployableartifact'",
				m.Components[m.CompCursor])
			m.selected = false
			return m, tea.Quit
		}
		m.deployableArtifacts = artifacts
		m.state = stateDeployArtifactSelect
		m.errorMsg = ""
		return m, nil
	}
	m.envCursor = interactive.ProcessListCursor(keyMsg, m.envCursor, len(m.environments))
	return m, nil
}

func (m deploymentModel) handleArtifactSelect(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		if m.artifactCursor >= len(m.deployableArtifacts) {
			m.errorMsg = "Invalid deployable artifact selection"
			return m, nil
		}
		m.state = stateNameInput
		m.errorMsg = ""
		return m, nil
	}
	m.artifactCursor = interactive.ProcessListCursor(keyMsg, m.artifactCursor, len(m.deployableArtifacts))
	return m, nil
}

func (m deploymentModel) handleNameInput(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		if err := validation.ValidateName("deployment", m.name); err != nil {
			m.errorMsg = err.Error()
			return m, nil
		}

		deployments, err := m.FetchDeployments()
		if err != nil {
			m.errorMsg = fmt.Sprintf("Failed to check deployment existence: %v", err)
			return m, nil
		}

		// Check uniqueness
		for _, d := range deployments {
			if d == m.name {
				m.errorMsg = fmt.Sprintf("Deployment '%s' already exists in environment '%s'",
					m.name, m.environments[m.envCursor])
				return m, nil
			}
		}

		m.selected = true
		return m, tea.Quit
	}
	m.name, _ = interactive.EditTextInputField(keyMsg, m.name, len(m.name))
	return m, nil
}

func (m deploymentModel) View() string {
	progress := m.RenderProgress()
	var view string

	switch m.state {
	case stateOrgSelect:
		view = m.RenderOrgSelection()
	case stateProjSelect:
		view = m.RenderProjSelection()
	case stateCompSelect:
		view = m.RenderComponentSelection()
	case stateDeploymentTrackSelect:
		view = interactive.RenderListPrompt("Select deployment track:", m.deploymentTracks, m.trackCursor)
	case stateEnvSelect:
		view = interactive.RenderListPrompt("Select environment:", m.environments, m.envCursor)
	case stateDeployArtifactSelect:
		view = interactive.RenderListPrompt("Select deployable artifact:", m.deployableArtifacts, m.artifactCursor)
	case stateNameInput:
		view = interactive.RenderInputPrompt("Enter deployment name:", "", m.name, m.errorMsg)
	default:
		view = ""
	}

	if m.errorMsg != "" {
		view += "\nError: " + m.errorMsg
	}

	return progress + view
}

func (m deploymentModel) RenderProgress() string {
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
	if len(m.deploymentTracks) > 0 && m.state > stateDeploymentTrackSelect {
		progress.WriteString(fmt.Sprintf("- deployment track: %s\n", m.deploymentTracks[m.trackCursor]))
	}
	if len(m.environments) > 0 {
		progress.WriteString(fmt.Sprintf("- environment: %s\n", m.environments[m.envCursor]))
	}
	if len(m.deployableArtifacts) > 0 && m.state > stateDeployArtifactSelect {
		progress.WriteString(fmt.Sprintf("- deployable artifact: %s\n", m.deployableArtifacts[m.artifactCursor]))
	}
	if m.name != "" {
		progress.WriteString(fmt.Sprintf("- name: %s\n", m.name))
	}

	return progress.String()
}

func createDeploymentInteractive(config constants.CRDConfig) error {
	baseModel, err := interactive.NewBaseModel()
	if err != nil {
		return err
	}

	model := deploymentModel{
		BaseModel: *baseModel,
		state:     stateOrgSelect,
	}

	finalModel, err := interactive.RunInteractiveModel(model)
	if err != nil {
		return fmt.Errorf("interactive mode failed: %w", err)
	}

	m, ok := finalModel.(deploymentModel)
	if !ok || !m.selected {
		if m.errorMsg != "" {
			return fmt.Errorf("%s", m.errorMsg)
		}
		return fmt.Errorf("deployment creation cancelled")
	}

	return createDeployment(api.CreateDeploymentParams{
		Name:               m.name,
		Organization:       m.Organizations[m.OrgCursor],
		Project:            m.Projects[m.ProjCursor],
		Component:          m.Components[m.CompCursor],
		Environment:        m.environments[m.envCursor],
		DeployableArtifact: m.deployableArtifacts[m.artifactCursor],
		DeploymentTrack:    m.deploymentTracks[m.trackCursor],
	}, config)
}
