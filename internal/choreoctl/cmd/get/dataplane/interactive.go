// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/openchoreo/openchoreo/internal/choreoctl/interactive"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
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
