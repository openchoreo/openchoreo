// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymenttrack

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
	stateNameInput
	stateAPIVersionInput
	stateAutoDeployInput
)

type deploymentTrackModel struct {
	interactive.BaseModel
	name       string
	apiVersion string
	autoDeploy bool
	selected   bool
	errorMsg   string
	state      int
}

func (m deploymentTrackModel) Init() tea.Cmd {
	return nil
}

func (m deploymentTrackModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			components, err := m.FetchComponents()
			if err != nil {
				m.errorMsg = err.Error()
				m.selected = false
				return m, tea.Quit
			}
			if len(components) == 0 {
				m.errorMsg = fmt.Sprintf("No components found in project '%s'. Please create a component first using 'choreoctl create component'",
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
			m.state = stateNameInput
			m.errorMsg = ""
			return m, nil
		}
		m.CompCursor = interactive.ProcessListCursor(keyMsg, m.CompCursor, len(m.Components))

	case stateNameInput:
		if interactive.IsEnterKey(keyMsg) {
			// Validate name format
			if err := validation.ValidateName("deploymenttrack", m.name); err != nil {
				m.errorMsg = err.Error()
				return m, nil
			}

			// Check uniqueness
			tracks, err := m.FetchDeploymentTracks()
			if err != nil {
				m.errorMsg = fmt.Sprintf("Failed to check deployment track existence: %v", err)
				return m, nil
			}
			for _, t := range tracks {
				if t == m.name {
					m.errorMsg = fmt.Sprintf("Deployment track '%s' already exists in component '%s'",
						m.name, m.Components[m.CompCursor])
					return m, nil
				}
			}
			m.state = stateAPIVersionInput
			m.errorMsg = ""
			return m, nil
		}
		m.errorMsg = ""
		m.name, _ = interactive.EditTextInputField(keyMsg, m.name, len(m.name))

	case stateAPIVersionInput:
		if interactive.IsEnterKey(keyMsg) {
			if m.apiVersion == "" {
				m.errorMsg = "API version cannot be empty"
				return m, nil
			}
			m.state = stateAutoDeployInput
			m.errorMsg = ""
			return m, nil
		}
		m.apiVersion, _ = interactive.EditTextInputField(keyMsg, m.apiVersion, len(m.apiVersion))

	case stateAutoDeployInput:
		if interactive.IsEnterKey(keyMsg) {
			m.selected = true
			return m, tea.Quit
		}
		switch keyMsg.String() {
		case "y", "Y":
			m.autoDeploy = true
		case "n", "N":
			m.autoDeploy = false
		}
	}
	return m, nil
}

func (m deploymentTrackModel) View() string {
	progress := m.RenderProgress()
	var view string

	switch m.state {
	case stateOrgSelect:
		view = m.RenderOrgSelection()
	case stateProjSelect:
		view = m.RenderProjSelection()
	case stateCompSelect:
		view = m.RenderComponentSelection()
	case stateNameInput:
		view = interactive.RenderInputPrompt("Enter deployment track name:", "", m.name, m.errorMsg)
	case stateAPIVersionInput:
		view = interactive.RenderInputPrompt("Enter API version:", "", m.apiVersion, m.errorMsg)
	case stateAutoDeployInput:
		view = interactive.RenderInputPrompt("Enable auto deploy? (y/n):", "", fmt.Sprintf("%v", m.autoDeploy), m.errorMsg)
	}

	return progress + view
}

func (m deploymentTrackModel) RenderProgress() string {
	var progress strings.Builder
	progress.WriteString("Selected inputs:\n")

	if len(m.Organizations) > 0 {
		progress.WriteString(fmt.Sprintf("- organization: %s\n", m.Organizations[m.OrgCursor]))
	}
	if len(m.Projects) > 0 {
		progress.WriteString(fmt.Sprintf("- project: %s\n", m.Projects[m.ProjCursor]))
	}
	if len(m.Components) > 0 {
		progress.WriteString(fmt.Sprintf("- component: %s\n", m.Components[m.CompCursor]))
	}
	if m.name != "" {
		progress.WriteString(fmt.Sprintf("- name: %s\n", m.name))
	}
	if m.apiVersion != "" {
		progress.WriteString(fmt.Sprintf("- api version: %s\n", m.apiVersion))
	}
	if m.state > stateAPIVersionInput {
		progress.WriteString(fmt.Sprintf("- auto deploy: %v\n", m.autoDeploy))
	}

	return progress.String()
}

func createDeploymentTrackInteractive(config constants.CRDConfig) error {
	baseModel, err := interactive.NewBaseModel()
	if err != nil {
		return err
	}

	model := deploymentTrackModel{
		BaseModel: *baseModel,
		state:     stateOrgSelect,
	}

	finalModel, err := interactive.RunInteractiveModel(model)
	if err != nil {
		return fmt.Errorf("interactive mode failed: %w", err)
	}

	m, ok := finalModel.(deploymentTrackModel)
	if !ok || !m.selected {
		if m.errorMsg != "" {
			return fmt.Errorf("%s", m.errorMsg)
		}
		return fmt.Errorf("deployment track creation cancelled")
	}

	return createDeploymentTrack(api.CreateDeploymentTrackParams{
		Name:         m.name,
		Organization: m.Organizations[m.OrgCursor],
		Project:      m.Projects[m.ProjCursor],
		Component:    m.Components[m.CompCursor],
		APIVersion:   m.apiVersion,
		AutoDeploy:   m.autoDeploy,
	}, config)
}
