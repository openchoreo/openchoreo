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

package project

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/choreo-idp/choreo/internal/choreoctl/interactive"
	"github.com/choreo-idp/choreo/internal/choreoctl/validation"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

const (
	stateOrgSelect = iota
	stateNameInput
)

type projectModel struct {
	interactive.BaseModel
	state    int
	name     string
	selected bool
	errorMsg string
}

func (m projectModel) Init() tea.Cmd {
	return nil
}

func (m projectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			m.state = stateNameInput
			m.errorMsg = ""
			return m, nil
		}
		m.OrgCursor = interactive.ProcessListCursor(keyMsg, m.OrgCursor, len(m.Organizations))

	case stateNameInput:
		if interactive.IsEnterKey(keyMsg) {
			if err := validation.ValidateProjectName(m.name); err != nil {
				m.errorMsg = err.Error()
				return m, nil
			}

			// Fetch projects for uniqueness check
			projects, err := m.FetchProjects()
			if err != nil {
				m.errorMsg = err.Error()
				return m, nil
			}

			// Check for duplicate project name
			for _, p := range projects {
				if p == m.name {
					m.errorMsg = fmt.Sprintf("Project '%s' already exists in organization '%s'",
						m.name, m.Organizations[m.OrgCursor])
					return m, nil
				}
			}

			m.selected = true
			return m, tea.Quit
		}
		m.errorMsg = ""
		m.name, _ = interactive.EditTextInputField(keyMsg, m.name, 256)
	}

	return m, nil
}

func (m projectModel) View() string {
	progress := m.RenderProgress()
	switch m.state {
	case stateOrgSelect:
		return progress + m.RenderOrgSelection()
	case stateNameInput:
		return progress + interactive.RenderInputPrompt("Enter project name:", "", m.name, m.errorMsg)
	}
	return progress
}

func createProjectInteractive(config constants.CRDConfig) error {
	baseModel, err := interactive.NewBaseModel()
	if err != nil {
		return err
	}

	model := projectModel{
		BaseModel: *baseModel,
		state:     stateOrgSelect,
	}

	finalModel, err := interactive.RunInteractiveModel(model)
	if err != nil {
		return err
	}

	m, ok := finalModel.(projectModel)
	if !ok || !m.selected {
		return fmt.Errorf("project creation cancelled")
	}

	return createProject(api.CreateProjectParams{
		Name:         m.name,
		Organization: m.Organizations[m.OrgCursor],
	}, config)
}
