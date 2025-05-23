/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package dataplane

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
	dpStateOrgSelect = iota
	dpStateNameInput
	dpStateGatewayTypeInput
	dpStatePublicVirtualHostInput
	dpStateOrgVirtualHostInput
	dpStateClusterNameInput
	dpStateAPIServerURLInput
	dpStateCACertInput
	dpStateClientCertInput
	dpStateClientKeyInput
)

type dataPlaneModel struct {
	interactive.BaseModel // Reuse organization (and optionally project) selection

	// DataPlane-specific fields.
	name                  string
	gatewayType           string
	publicVirtualHost     string
	orgVirtualHost        string
	kubernetesClusterName string
	apiServerURL          string
	caCert                string
	clientCert            string
	clientKey             string

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
		return m.updateOrgSelect(keyMsg)
	case dpStateNameInput:
		return m.updateNameInput(keyMsg)
	case dpStateGatewayTypeInput:
		return m.updateGatewayTypeInput(keyMsg)
	case dpStatePublicVirtualHostInput:
		return m.updatePublicVirtualHostInput(keyMsg)
	case dpStateOrgVirtualHostInput:
		return m.updateOrgVirtualHostInput(keyMsg)
	case dpStateClusterNameInput:
		return m.updateClusterNameInput(keyMsg)
	case dpStateAPIServerURLInput:
		return m.updateAPIServerURLInput(keyMsg)
	case dpStateCACertInput:
		return m.updateCACertInput(keyMsg)
	case dpStateClientCertInput:
		return m.updateClientCertInput(keyMsg)
	case dpStateClientKeyInput:
		return m.updateClientKeyInput(keyMsg)
	default:
		return m, nil
	}
}

func (m dataPlaneModel) updateOrgSelect(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	return m, nil
}

func (m dataPlaneModel) updateNameInput(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		if err := validation.ValidateName("dataplane", m.name); err != nil {
			m.errorMsg = err.Error()
			return m, nil
		}
		m.state = dpStateGatewayTypeInput
		m.errorMsg = ""
		return m, nil
	}
	m.errorMsg = ""
	m.name, _ = interactive.EditTextInputField(keyMsg, m.name, len(m.name))
	return m, nil
}

func (m dataPlaneModel) updateGatewayTypeInput(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		if m.gatewayType == "" {
			m.gatewayType = "envoy"
		}
		m.state = dpStatePublicVirtualHostInput
		m.errorMsg = ""
		return m, nil
	}
	m.errorMsg = ""
	m.gatewayType, _ = interactive.EditTextInputField(keyMsg, m.gatewayType, len(m.gatewayType))
	return m, nil
}

func (m dataPlaneModel) updatePublicVirtualHostInput(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		if m.publicVirtualHost == "" {
			m.publicVirtualHost = "choreoapis.local"
		}
		m.state = dpStateOrgVirtualHostInput
		m.errorMsg = ""
		return m, nil
	}
	m.publicVirtualHost, _ = interactive.EditTextInputField(keyMsg, m.publicVirtualHost, len(m.publicVirtualHost))
	return m, nil
}

func (m dataPlaneModel) updateOrgVirtualHostInput(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		if m.orgVirtualHost == "" {
			m.orgVirtualHost = "internal.choreoapis.local"
		}
		m.state = dpStateClusterNameInput
		m.errorMsg = ""
		return m, nil
	}
	m.orgVirtualHost, _ = interactive.EditTextInputField(keyMsg, m.orgVirtualHost, len(m.orgVirtualHost))
	return m, nil
}

func (m dataPlaneModel) updateClusterNameInput(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		if m.kubernetesClusterName == "" {
			m.kubernetesClusterName = "kind-choreo-dp"
		}
		m.state = dpStateAPIServerURLInput
		m.errorMsg = ""
		return m, nil
	}
	m.kubernetesClusterName, _ = interactive.EditTextInputField(keyMsg, m.kubernetesClusterName, len(m.kubernetesClusterName))
	return m, nil
}

func (m dataPlaneModel) updateAPIServerURLInput(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		if m.apiServerURL == "" {
			m.errorMsg = "API Server URL cannot be empty"
			return m, nil
		}
		m.state = dpStateCACertInput
		m.errorMsg = ""
		return m, nil
	}
	m.apiServerURL, _ = interactive.EditTextInputField(keyMsg, m.apiServerURL, len(m.apiServerURL))
	return m, nil
}

func (m dataPlaneModel) updateCACertInput(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		if m.caCert == "" {
			m.errorMsg = "CA Certificate cannot be empty"
			return m, nil
		}
		m.state = dpStateClientCertInput
		m.errorMsg = ""
		return m, nil
	}
	m.caCert, _ = interactive.EditTextInputField(keyMsg, m.caCert, len(m.caCert))
	return m, nil
}

func (m dataPlaneModel) updateClientCertInput(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		if m.clientCert == "" {
			m.errorMsg = "Client Certificate cannot be empty"
			return m, nil
		}
		m.state = dpStateClientKeyInput
		m.errorMsg = ""
		return m, nil
	}
	m.clientCert, _ = interactive.EditTextInputField(keyMsg, m.clientCert, len(m.clientCert))
	return m, nil
}

func (m dataPlaneModel) updateClientKeyInput(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		if m.clientKey == "" {
			m.errorMsg = "Client Key cannot be empty"
			return m, nil
		}
		m.selected = true
		return m, tea.Quit
	}
	m.clientKey, _ = interactive.EditTextInputField(keyMsg, m.clientKey, len(m.clientKey))
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
	case dpStateGatewayTypeInput:
		view = interactive.RenderInputPrompt("Enter gateway type:",
			"envoy", m.gatewayType, m.errorMsg)
	case dpStatePublicVirtualHostInput:
		view = interactive.RenderInputPrompt("Enter public virtual host:",
			"choreoapis.local", m.publicVirtualHost, m.errorMsg)
	case dpStateOrgVirtualHostInput:
		view = interactive.RenderInputPrompt("Enter organization virtual host:",
			"internal.choreoapis.local", m.orgVirtualHost, m.errorMsg)
	case dpStateClusterNameInput:
		view = interactive.RenderInputPrompt("Enter DataPlane Cluster Name:", "kind-choreo-dp", m.kubernetesClusterName, m.errorMsg)
	case dpStateAPIServerURLInput:
		view = interactive.RenderInputPrompt("Enter Kubernetes API server URL:", "", m.apiServerURL, m.errorMsg)
	case dpStateCACertInput:
		view = interactive.RenderInputPrompt("Enter CA certificate:", "", m.caCert, m.errorMsg)
	case dpStateClientCertInput:
		view = interactive.RenderInputPrompt("Enter client certificate:", "", m.clientCert, m.errorMsg)
	case dpStateClientKeyInput:
		view = interactive.RenderInputPrompt("Enter client key:", "", m.clientKey, m.errorMsg)
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

	if m.gatewayType != "" {
		progress.WriteString(fmt.Sprintf("- gateway type: %s\n", m.gatewayType))
	}

	if m.publicVirtualHost != "" {
		progress.WriteString(fmt.Sprintf("- public virtual host: %s\n", m.publicVirtualHost))
	}

	if m.orgVirtualHost != "" {
		progress.WriteString(fmt.Sprintf("- organization virtual host: %s\n", m.orgVirtualHost))
	}

	if m.kubernetesClusterName != "" {
		progress.WriteString(fmt.Sprintf("- dataplane cluster name: %s\n", m.kubernetesClusterName))
	}

	if m.apiServerURL != "" {
		progress.WriteString(fmt.Sprintf("- api server url: %s\n", m.apiServerURL))
	}

	if m.caCert != "" {
		progress.WriteString("- ca cert: [provided]\n")
	}

	if m.clientCert != "" {
		progress.WriteString("- client cert: [provided]\n")
	}

	if m.clientKey != "" {
		progress.WriteString("- client key: [provided]\n")
	}

	return progress.String() + "\n"
}

func createDataPlaneInteractive(config constants.CRDConfig) error {
	baseModel, err := interactive.NewBaseModel()
	if err != nil {
		return err
	}

	model := dataPlaneModel{
		BaseModel: *baseModel,
		state:     dpStateOrgSelect,
		// Provide default values that users can modify
		gatewayType:       "envoy",
		publicVirtualHost: "choreoapis.local",
		orgVirtualHost:    "internal.choreoapis.local",
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
		EnableCilium:            true,
		EnableScaleToZero:       true,
		GatewayType:             m.gatewayType,
		PublicVirtualHost:       m.publicVirtualHost,
		OrganizationVirtualHost: m.orgVirtualHost,
		APIServerURL:            m.apiServerURL,
		CACert:                  m.caCert,
		ClientCert:              m.clientCert,
		ClientKey:               m.clientKey,
	}, config)
}
