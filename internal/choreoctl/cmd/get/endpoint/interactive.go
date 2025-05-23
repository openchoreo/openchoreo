/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package endpoint

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/openchoreo/openchoreo/internal/choreoctl/interactive"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

const (
	stateOrgSelect = iota
	stateProjSelect
	stateCompSelect
	stateEnvSelect
)

type endpointListModel struct {
	interactive.BaseModel
	state    int
	selected bool
	errorMsg string
}

func (m endpointListModel) Init() tea.Cmd {
	return nil
}

func (m endpointListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			environments, err := m.FetchEnvironments()
			if err != nil {
				m.errorMsg = err.Error()
				m.selected = false
				return m, tea.Quit
			}
			if len(environments) == 0 {
				m.selected = true
				return m, tea.Quit
			}
			m.Environments = environments
			m.state = stateEnvSelect
			m.errorMsg = ""
			return m, nil
		}
		m.CompCursor = interactive.ProcessListCursor(keyMsg, m.CompCursor, len(m.Components))

	case stateEnvSelect:
		if interactive.IsEnterKey(keyMsg) {
			m.selected = true
			return m, tea.Quit
		}
		m.EnvCursor = interactive.ProcessListCursor(keyMsg, m.EnvCursor, len(m.Environments))
	}

	return m, nil
}

func (m endpointListModel) View() string {
	var progress string
	if m.state > stateOrgSelect {
		progress += fmt.Sprintf("Organization: %s\n", m.Organizations[m.OrgCursor])
	}
	if m.state > stateProjSelect {
		progress += fmt.Sprintf("Project: %s\n", m.Projects[m.ProjCursor])
	}
	if m.state > stateCompSelect {
		progress += fmt.Sprintf("Component: %s\n", m.Components[m.CompCursor])
	}

	var view string
	switch m.state {
	case stateOrgSelect:
		view = interactive.RenderListPrompt("Select organization:", m.Organizations, m.OrgCursor)
	case stateProjSelect:
		view = interactive.RenderListPrompt("Select project:", m.Projects, m.ProjCursor)
	case stateCompSelect:
		view = interactive.RenderListPrompt("Select component:", m.Components, m.CompCursor)
	case stateEnvSelect:
		view = interactive.RenderListPrompt("Select environment:", m.Environments, m.EnvCursor)
	default:
		view = ""
	}

	if m.errorMsg != "" {
		view += "\nError: " + m.errorMsg
	}
	return progress + view
}

func (m endpointListModel) RenderProgress() string {
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
	if len(m.Environments) > 0 {
		progress.WriteString(fmt.Sprintf("- environment: %s\n", m.Environments[m.EnvCursor]))
	}

	return progress.String()
}

func getEndpointInteractive(config constants.CRDConfig) error {
	baseModel, err := interactive.NewBaseModel()
	if err != nil {
		return err
	}

	model := endpointListModel{
		BaseModel: *baseModel,
		state:     stateOrgSelect,
	}

	finalModel, err := interactive.RunInteractiveModel(model)
	if err != nil {
		return fmt.Errorf("interactive mode failed: %w", err)
	}

	m, ok := finalModel.(endpointListModel)
	if !ok || !m.selected {
		if m.errorMsg != "" {
			return fmt.Errorf("%s", m.errorMsg)
		}
		return fmt.Errorf("endpoint listing cancelled")
	}

	params := api.GetEndpointParams{
		Organization: m.Organizations[m.OrgCursor],
		Project:      m.Projects[m.ProjCursor],
		Component:    m.Components[m.CompCursor],
	}
	if len(m.Environments) > 0 {
		params.Environment = m.Environments[m.EnvCursor]
	}

	err = getEndpoints(params, config)
	if err != nil {
		return err
	}

	flags := map[string]string{
		"organization": m.Organizations[m.OrgCursor],
		"project":      m.Projects[m.ProjCursor],
		"component":    m.Components[m.CompCursor],
	}
	if len(m.Environments) > 0 {
		flags["environment"] = m.Environments[m.EnvCursor]
	}

	return nil
}
