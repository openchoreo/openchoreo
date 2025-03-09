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

package build

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/choreo-idp/choreo/internal/choreoctl/interactive"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

const (
	stateOrgSelect = iota
	stateProjSelect
	stateCompSelect
)

type buildListModel struct {
	interactive.BaseModel // Reuses Organizations, Projects, Components, and their cursors.
	state                 int
	selected              bool
	errorMsg              string
}

func (m buildListModel) Init() tea.Cmd {
	return nil
}

func (m buildListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.OrgCursor >= len(m.Organizations) {
				m.errorMsg = "Invalid organization selection"
				return m, nil
			}
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
			if m.ProjCursor >= len(m.Projects) {
				m.errorMsg = "Invalid project selection"
				return m, nil
			}
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
			if m.CompCursor >= len(m.Components) {
				m.errorMsg = "Invalid component selection"
				return m, nil
			}
			m.selected = true
			return m, tea.Quit
		}
		m.CompCursor = interactive.ProcessListCursor(keyMsg, m.CompCursor, len(m.Components))
	}

	return m, nil
}

func (m buildListModel) RenderProgress() string {
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

	return progress.String()
}

func (m buildListModel) View() string {
	progress := m.RenderProgress()
	var view string

	switch m.state {
	case stateOrgSelect:
		view = m.RenderOrgSelection()
	case stateProjSelect:
		view = m.RenderProjSelection()
	case stateCompSelect:
		view = m.RenderComponentSelection()
	}

	if m.errorMsg != "" {
		view += "\nError: " + m.errorMsg
	}

	return progress + view
}

func getBuildInteractive(config constants.CRDConfig) error {
	baseModel, err := interactive.NewBaseModel()
	if err != nil {
		return err
	}

	model := buildListModel{
		BaseModel: *baseModel,
		state:     stateOrgSelect,
	}

	finalModel, err := interactive.RunInteractiveModel(model)
	if err != nil {
		return fmt.Errorf("interactive mode failed: %w", err)
	}

	m, ok := finalModel.(buildListModel)
	if !ok || !m.selected {
		if m.errorMsg != "" {
			return fmt.Errorf("%s", m.errorMsg)
		}
		return fmt.Errorf("build listing cancelled")
	}

	params := api.GetBuildParams{
		Organization: m.Organizations[m.OrgCursor],
		Project:      m.Projects[m.ProjCursor],
		Component:    m.Components[m.CompCursor],
	}

	err = getBuilds(params, config)
	if err != nil {
		return err
	}

	return nil
}
