// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package organization

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/openchoreo/openchoreo/internal/choreoctl/interactive"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

const (
	stateNameInput = iota
	stateDisplayNameInput
)

type organizationModel struct {
	interactive.BaseModel // Embeds common interactive helpers.
	state                 int
	name                  string
	displayName           string
	selected              bool
	errorMsg              string
}

func (m organizationModel) Init() tea.Cmd {
	return nil
}

func (m organizationModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if interactive.IsQuitKey(keyMsg) {
		m.selected = false
		return m, tea.Quit
	}

	switch m.state {
	case stateNameInput:
		if interactive.IsEnterKey(keyMsg) {
			if err := validation.ValidateOrganizationName(m.name); err != nil {
				m.errorMsg = err.Error()
				return m, nil
			}
			m.state = stateDisplayNameInput
			m.errorMsg = ""
			return m, nil
		}
		m.name, _ = interactive.EditTextInputField(keyMsg, m.name, len(m.name))
	case stateDisplayNameInput:
		if interactive.IsEnterKey(keyMsg) {
			m.selected = true
			m.errorMsg = ""
			return m, tea.Quit
		}
		m.displayName, _ = interactive.EditTextInputField(keyMsg, m.displayName, 256)
	}

	return m, nil
}

func (m organizationModel) View() string {
	var view string

	switch m.state {
	case stateNameInput:
		view = interactive.RenderInputPrompt("Enter organization name:", "", m.name, m.errorMsg)
	case stateDisplayNameInput:
		view = interactive.RenderInputPrompt("Enter display name:", "", m.displayName, m.errorMsg)
	default:
		view = ""
	}

	return view
}

func createOrganizationInteractive(config constants.CRDConfig) error {
	model := organizationModel{
		state: stateNameInput,
	}

	finalModel, err := interactive.RunInteractiveModel(model)
	if err != nil {
		return err
	}

	m, ok := finalModel.(organizationModel)
	if !ok || !m.selected {
		return fmt.Errorf("organization creation cancelled")
	}

	return createOrganization(api.CreateOrganizationParams{
		Name:        m.name,
		DisplayName: m.displayName,
	}, config)
}
