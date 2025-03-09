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
	"github.com/choreo-idp/choreo/internal/choreoctl/validation"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

const (
	dpStateOrgSelect = iota
	dpStateNameInput
	dpStateKubeClusterInput
	dpStateConnConfigInput
	dpStateCiliumInput
	dpStateScaleToZeroInput
	dpStateGatewayTypeInput
	dpStatePublicVirtualHostInput
	dpStateOrgVirtualHostInput
)

type dataPlaneModel struct {
	interactive.BaseModel // Reuse organization (and optionally project) selection

	// DataPlane-specific fields.
	name                  string
	kubernetesClusterName string
	connectionConfigRef   string
	enableCilium          bool
	enableScaleToZero     bool
	gatewayType           string
	publicVirtualHost     string
	orgVirtualHost        string

	selected bool
	errorMsg string
	state    int
}

func (m dataPlaneModel) Init() tea.Cmd {
	return nil
}

func (m dataPlaneModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if interactive.IsQuitKey(keyMsg) {
		m.selected = false
		return m, tea.Quit
	}

	switch m.state {
	case dpStateOrgSelect:
		if interactive.IsEnterKey(keyMsg) {
			if m.OrgCursor >= len(m.Organizations) {
				m.errorMsg = "Invalid organization selection"
				return m, nil
			}
			m.state = dpStateNameInput
			m.errorMsg = ""
			return m, nil
		}
		m.errorMsg = ""
		m.OrgCursor = interactive.ProcessListCursor(keyMsg, m.OrgCursor, len(m.Organizations))

	case dpStateNameInput:
		if interactive.IsEnterKey(keyMsg) {
			if err := validation.ValidateName("dataplane", m.name); err != nil {
				m.errorMsg = err.Error()
				return m, nil
			}
			m.state = dpStateKubeClusterInput
			m.errorMsg = ""
			return m, nil
		}
		m.errorMsg = ""
		m.name, _ = interactive.EditTextInputField(keyMsg, m.name, len(m.name))

	case dpStateKubeClusterInput:
		if interactive.IsEnterKey(keyMsg) {
			if m.kubernetesClusterName == "" {
				m.errorMsg = "Kubernetes cluster name cannot be empty"
				return m, nil
			}
			m.state = dpStateConnConfigInput
			m.errorMsg = ""
			return m, nil
		}
		m.errorMsg = ""
		m.kubernetesClusterName, _ = interactive.EditTextInputField(keyMsg, m.kubernetesClusterName, len(m.kubernetesClusterName))

	case dpStateConnConfigInput:
		if interactive.IsEnterKey(keyMsg) {
			if m.connectionConfigRef == "" {
				m.errorMsg = "Connection config reference cannot be empty"
				return m, nil
			}
			m.state = dpStateCiliumInput
			m.errorMsg = ""
			return m, nil
		}
		m.errorMsg = ""
		m.connectionConfigRef, _ = interactive.EditTextInputField(keyMsg, m.connectionConfigRef, len(m.connectionConfigRef))

	case dpStateCiliumInput:
		if interactive.IsEnterKey(keyMsg) {
			m.state = dpStateScaleToZeroInput
			m.errorMsg = ""
			return m, nil
		}
		switch keyMsg.String() {
		case "y", "Y":
			m.enableCilium = true
		case "n", "N":
			m.enableCilium = false
		}

	case dpStateScaleToZeroInput:
		if interactive.IsEnterKey(keyMsg) {
			m.state = dpStateGatewayTypeInput
			m.errorMsg = ""
			return m, nil
		}
		switch keyMsg.String() {
		case "y", "Y":
			m.enableScaleToZero = true
		case "n", "N":
			m.enableScaleToZero = false
		}

	case dpStateGatewayTypeInput:
		if interactive.IsEnterKey(keyMsg) {
			if m.gatewayType == "" {
				m.errorMsg = "Gateway type cannot be empty"
				return m, nil
			}
			m.state = dpStatePublicVirtualHostInput
			m.errorMsg = ""
			return m, nil
		}
		m.errorMsg = ""
		m.gatewayType, _ = interactive.EditTextInputField(keyMsg, m.gatewayType, len(m.gatewayType))

	case dpStatePublicVirtualHostInput:
		if interactive.IsEnterKey(keyMsg) {
			m.state = dpStateOrgVirtualHostInput
			m.errorMsg = ""
			return m, nil
		}
		m.publicVirtualHost, _ = interactive.EditTextInputField(keyMsg, m.publicVirtualHost, len(m.publicVirtualHost))

	case dpStateOrgVirtualHostInput:
		if interactive.IsEnterKey(keyMsg) {
			m.selected = true
			return m, tea.Quit
		}
		m.orgVirtualHost, _ = interactive.EditTextInputField(keyMsg, m.orgVirtualHost, len(m.orgVirtualHost))
	}

	return m, nil
}

func (m dataPlaneModel) View() string {
	progress := m.RenderProgress()
	var view string
	switch m.state {
	case dpStateOrgSelect:
		view = m.RenderOrgSelection()
	case dpStateNameInput:
		view = interactive.RenderInputPrompt("Enter data plane name:", "", m.name, m.errorMsg)
	case dpStateKubeClusterInput:
		view = interactive.RenderInputPrompt("Enter Kubernetes cluster name:", "", m.kubernetesClusterName, m.errorMsg)
	case dpStateConnConfigInput:
		view = interactive.RenderInputPrompt("Enter connection config ref:", "", m.connectionConfigRef, m.errorMsg)
	case dpStateCiliumInput:
		view = interactive.RenderInputPrompt("Enable Cilium? (y/n):", "", fmt.Sprintf("%v", m.enableCilium), m.errorMsg)
	case dpStateScaleToZeroInput:
		view = interactive.RenderInputPrompt("Enable scale to zero? (y/n):", "", fmt.Sprintf("%v", m.enableScaleToZero), m.errorMsg)
	case dpStateGatewayTypeInput:
		view = interactive.RenderInputPrompt("Enter gateway type:", "", m.gatewayType, m.errorMsg)
	case dpStatePublicVirtualHostInput:
		view = interactive.RenderInputPrompt("Enter public virtual host:", "", m.publicVirtualHost, m.errorMsg)
	case dpStateOrgVirtualHostInput:
		view = interactive.RenderInputPrompt("Enter organization virtual host:", "", m.orgVirtualHost, m.errorMsg)
	default:
		view = ""
	}
	return progress + view
}

func (m dataPlaneModel) RenderProgress() string {
	var progress strings.Builder
	progress.WriteString("Selected inputs:\n")

	if len(m.Organizations) > 0 {
		progress.WriteString(fmt.Sprintf("- organization: %s\n", m.Organizations[m.OrgCursor]))
	}

	if m.name != "" {
		progress.WriteString(fmt.Sprintf("- name: %s\n", m.name))
	}

	if m.kubernetesClusterName != "" {
		progress.WriteString(fmt.Sprintf("- kubernetes cluster: %s\n", m.kubernetesClusterName))
	}

	if m.connectionConfigRef != "" {
		progress.WriteString(fmt.Sprintf("- connection config: %s\n", m.connectionConfigRef))
	}

	progress.WriteString(fmt.Sprintf("- enable cilium: %v\n", m.enableCilium))
	progress.WriteString(fmt.Sprintf("- enable scale to zero: %v\n", m.enableScaleToZero))

	if m.gatewayType != "" {
		progress.WriteString(fmt.Sprintf("- gateway type: %s\n", m.gatewayType))
	}

	if m.publicVirtualHost != "" {
		progress.WriteString(fmt.Sprintf("- public virtual host: %s\n", m.publicVirtualHost))
	}
	if m.orgVirtualHost != "" {
		progress.WriteString(fmt.Sprintf("- organization virtual host: %s\n", m.orgVirtualHost))
	}

	return progress.String()
}

func createDataPlaneInteractive(config constants.CRDConfig) error {
	baseModel, err := interactive.NewBaseModel()
	if err != nil {
		return err
	}

	model := dataPlaneModel{
		BaseModel: *baseModel,
		state:     dpStateOrgSelect,
	}

	finalModel, err := interactive.RunInteractiveModel(model)
	if err != nil {
		return fmt.Errorf("interactive mode failed: %w", err)
	}

	m, ok := finalModel.(dataPlaneModel)
	if !ok || !m.selected {
		if m.errorMsg != "" {
			return fmt.Errorf("%s", m.errorMsg)
		}
		return fmt.Errorf("data plane creation cancelled")
	}

	return createDataPlane(api.CreateDataPlaneParams{
		Name:                    m.name,
		Organization:            m.Organizations[m.OrgCursor],
		KubernetesClusterName:   m.kubernetesClusterName,
		ConnectionConfigRef:     m.connectionConfigRef,
		EnableCilium:            m.enableCilium,
		EnableScaleToZero:       m.enableScaleToZero,
		GatewayType:             m.gatewayType,
		PublicVirtualHost:       m.publicVirtualHost,
		OrganizationVirtualHost: m.orgVirtualHost,
	}, config)
}
