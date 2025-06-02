// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

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
	stateDataPlaneSelect
	stateNameInput
	stateIsProductionInput
	stateDNSPrefixInput
)

type environmentModel struct {
	interactive.BaseModel // Embeds common organization selection logic

	// Environment-specific fields.
	state        int
	dataPlanes   []string
	dpCursor     int
	name         string
	isProduction bool
	dnsPrefix    string
	selected     bool
	errorMsg     string
}

func (m environmentModel) Init() tea.Cmd {
	return nil
}

func (m environmentModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			dataPlanes, err := m.FetchDataPlanes()
			if err != nil {
				m.errorMsg = fmt.Sprintf("Failed to get data planes: %v", err)
				return m, nil
			}
			if len(dataPlanes) == 0 {
				m.errorMsg = fmt.Sprintf("No data planes found in organization '%s'. Please create a data plane first using 'choreoctl create dataplane'",
					m.Organizations[m.OrgCursor])
				m.selected = false
				return m, tea.Quit
			}
			m.dataPlanes = dataPlanes
			m.state = stateDataPlaneSelect
			m.errorMsg = ""
			return m, nil
		}
		m.OrgCursor = interactive.ProcessListCursor(keyMsg, m.OrgCursor, len(m.Organizations))

	case stateDataPlaneSelect:
		if interactive.IsEnterKey(keyMsg) {
			m.state = stateNameInput
			m.errorMsg = ""
			return m, nil
		}
		m.dpCursor = interactive.ProcessListCursor(keyMsg, m.dpCursor, len(m.dataPlanes))

	case stateNameInput:
		if interactive.IsEnterKey(keyMsg) {
			// First validate the environment name format
			if err := validation.ValidateName("environment", m.name); err != nil {
				m.errorMsg = err.Error()
				return m, nil
			}
			environments, err := m.FetchEnvironments()
			if err != nil {
				m.errorMsg = fmt.Sprintf("Failed to check environment existence: %v", err)
				return m, nil
			}
			// check for duplicate environment name
			for _, e := range environments {
				if e == m.name {
					m.errorMsg = fmt.Sprintf("Environment '%s' already exists in organization '%s'",
						m.name, m.Organizations[m.OrgCursor])
					return m, nil
				}
			}

			m.state = stateIsProductionInput
			m.errorMsg = ""
			return m, nil
		}
		m.errorMsg = ""
		m.name, _ = interactive.EditTextInputField(keyMsg, m.name, len(m.name))

	case stateIsProductionInput:
		if interactive.IsEnterKey(keyMsg) {
			m.state = stateDNSPrefixInput
			m.errorMsg = ""
			return m, nil
		}
		// Toggle isProduction with simple key input ("y" for yes, "n" for no)
		switch keyMsg.String() {
		case "y", "Y":
			m.isProduction = true
		case "n", "N":
			m.isProduction = false
		}

	case stateDNSPrefixInput:
		if interactive.IsEnterKey(keyMsg) {
			m.selected = true
			return m, tea.Quit
		}
		m.dnsPrefix, _ = interactive.EditTextInputField(keyMsg, m.dnsPrefix, len(m.dnsPrefix))
	}

	return m, nil
}

func (m environmentModel) View() string {
	progress := m.RenderProgress()

	switch m.state {
	case stateOrgSelect:
		return progress + m.RenderOrgSelection()
	case stateDataPlaneSelect:
		return progress + interactive.RenderListPrompt("Select data plane:", m.dataPlanes, m.dpCursor)
	case stateNameInput:
		return progress + interactive.RenderInputPrompt("Enter environment name:", "", m.name, m.errorMsg)
	case stateIsProductionInput:
		return progress + interactive.RenderInputPrompt("Is this a production environment? (y/n):", "", fmt.Sprintf("%v", m.isProduction), m.errorMsg)
	case stateDNSPrefixInput:
		return progress + interactive.RenderInputPrompt("Enter DNS prefix:", "", m.dnsPrefix, m.errorMsg)
	default:
		return progress
	}
}

func (m environmentModel) RenderProgress() string {
	var progress strings.Builder
	progress.WriteString("Selected inputs:\n")

	if len(m.Organizations) > 0 {
		progress.WriteString(fmt.Sprintf("- organization: %s\n", m.Organizations[m.OrgCursor]))
	}

	if m.state > stateOrgSelect && len(m.dataPlanes) > 0 {
		progress.WriteString(fmt.Sprintf("- data plane: %s\n", m.dataPlanes[m.dpCursor]))
	}

	if m.state > stateDataPlaneSelect && m.name != "" {
		progress.WriteString(fmt.Sprintf("- environment: %s\n", m.name))
	}

	if m.state > stateNameInput {
		progress.WriteString(fmt.Sprintf("- production: %v\n", m.isProduction))
	}

	if m.state > stateIsProductionInput && m.dnsPrefix != "" {
		progress.WriteString(fmt.Sprintf("- dns prefix: %s\n", m.dnsPrefix))
	}

	return progress.String()
}

func createEnvironmentInteractive(config constants.CRDConfig) error {
	baseModel, err := interactive.NewBaseModel()
	if err != nil {
		return err
	}

	model := environmentModel{
		BaseModel: *baseModel,
		state:     stateOrgSelect,
	}

	finalModel, err := interactive.RunInteractiveModel(model)
	if err != nil {
		return fmt.Errorf("interactive mode failed: %w", err)
	}

	m, ok := finalModel.(environmentModel)
	if !ok || !m.selected {
		if m.errorMsg != "" {
			return fmt.Errorf("%s", m.errorMsg)
		}
		return fmt.Errorf("environment creation cancelled")
	}

	return createEnvironment(api.CreateEnvironmentParams{
		Name:         m.name,
		Organization: m.Organizations[m.OrgCursor],
		DataPlaneRef: m.dataPlanes[m.dpCursor],
		IsProduction: m.isProduction,
		DNSPrefix:    m.dnsPrefix,
	}, config)
}
