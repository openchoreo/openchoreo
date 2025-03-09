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

package environment

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
)

type environmentListModel struct {
	interactive.BaseModel
	state    int
	selected bool
	errorMsg string
}

func (m environmentListModel) Init() tea.Cmd {
	return nil
}

func (m environmentListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if interactive.IsQuitKey(keyMsg) {
		m.selected = false
		return m, tea.Quit
	}

	if m.state == stateOrgSelect {
		if interactive.IsEnterKey(keyMsg) {
			m.selected = true
			return m, tea.Quit
		}
		m.OrgCursor = interactive.ProcessListCursor(keyMsg, m.OrgCursor, len(m.Organizations))
	}

	return m, nil
}

func (m environmentListModel) RenderProgress() string {
	var progress strings.Builder
	progress.WriteString("Selected resources:\n")

	if len(m.Organizations) > 0 {
		progress.WriteString(fmt.Sprintf("- organization: %s\n", m.Organizations[m.OrgCursor]))
	}

	return progress.String()
}

func (m environmentListModel) View() string {
	progress := m.RenderProgress()
	var view string

	if m.state == stateOrgSelect {
		view = m.RenderOrgSelection()
	}

	if m.errorMsg != "" {
		view += "\nError: " + m.errorMsg
	}

	return progress + view
}

func getEnvironmentInteractive(config constants.CRDConfig) error {
	baseModel, err := interactive.NewBaseModel()
	if err != nil {
		return err
	}

	model := environmentListModel{
		BaseModel: *baseModel,
		state:     stateOrgSelect,
	}

	finalModel, err := interactive.RunInteractiveModel(model)
	if err != nil {
		return fmt.Errorf("interactive mode failed: %w", err)
	}

	m, ok := finalModel.(environmentListModel)
	if !ok || !m.selected {
		if m.errorMsg != "" {
			return fmt.Errorf("%s", m.errorMsg)
		}
		return fmt.Errorf("environment listing cancelled")
	}

	params := api.GetEnvironmentParams{
		Organization: m.Organizations[m.OrgCursor],
	}

	err = getEnvironments(params, config)
	if err != nil {
		return err
	}

	return nil
}
