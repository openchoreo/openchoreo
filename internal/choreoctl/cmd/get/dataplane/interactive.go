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

package dataplane

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/choreo-idp/choreo/internal/choreoctl/interactive"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

type dataPlaneListModel struct {
	interactive.BaseModel
	selected bool
	errorMsg string
}

func (m dataPlaneListModel) Init() tea.Cmd {
	return nil
}

func (m dataPlaneListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if interactive.IsQuitKey(keyMsg) {
		m.selected = false
		return m, tea.Quit
	}

	if interactive.IsEnterKey(keyMsg) {
		m.selected = true
		return m, tea.Quit
	}

	m.OrgCursor = interactive.ProcessListCursor(keyMsg, m.OrgCursor, len(m.Organizations))
	return m, nil
}

func (m dataPlaneListModel) View() string {
	view := ""
	if m.errorMsg != "" {
		view += m.errorMsg + "\n"
	}
	view += interactive.RenderListPrompt("Select organization:", m.Organizations, m.OrgCursor)
	return view
}

func (m dataPlaneListModel) RenderProgress() string {
	var progress strings.Builder
	progress.WriteString("Selected resources:\n")

	if len(m.Organizations) > 0 {
		progress.WriteString(fmt.Sprintf("- organization: %s\n", m.Organizations[m.OrgCursor]))
	}

	return progress.String()
}

func getDataPlaneInteractive(config constants.CRDConfig) error {
	baseModel, err := interactive.NewBaseModel()
	if err != nil {
		return err
	}

	model := dataPlaneListModel{
		BaseModel: *baseModel,
	}

	finalModel, err := interactive.RunInteractiveModel(model)
	if err != nil {
		return fmt.Errorf("interactive mode failed: %w", err)
	}

	m, ok := finalModel.(dataPlaneListModel)
	if !ok || !m.selected {
		if m.errorMsg != "" {
			return fmt.Errorf("%s", m.errorMsg)
		}
		return fmt.Errorf("data plane listing cancelled")
	}

	params := api.GetDataPlaneParams{
		Organization: m.Organizations[m.OrgCursor],
	}

	err = getDataPlanes(params, config)
	if err != nil {
		return err
	}

	return nil
}
