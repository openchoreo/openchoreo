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

package deployableartifact

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/choreoctl/interactive"
	"github.com/choreo-idp/choreo/internal/choreoctl/validation"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

const (
	stateOrgSelect = iota
	stateProjSelect
	stateCompSelect
	stateNameInput
	stateArtifactTypeSelect
	stateBuildRefSelect
	stateImageTagInput
)

type deployableArtifactModel struct {
	interactive.BaseModel // Reuse common organization, project and component selection

	// Artifact-specific fields.
	builds        []string
	buildCursor   int
	artifactTypes []string
	typeCursor    int
	name          string
	imageTag      string
	selected      bool
	errorMsg      string
	state         int
}

func (m deployableArtifactModel) Init() tea.Cmd {
	return nil
}

func (m deployableArtifactModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if interactive.IsQuitKey(keyMsg) {
		m.selected = false
		return m, tea.Quit
	}

	switch m.state {
	// Update organization selection using BaseModel.
	case stateOrgSelect:
		if interactive.IsEnterKey(keyMsg) {
			cmd := m.UpdateOrgSelect(keyMsg)
			if m.State == interactive.StateProjSelect {
				m.state = stateProjSelect
			}
			m.errorMsg = ""
			return m, cmd
		}
		m.OrgCursor = interactive.ProcessListCursor(keyMsg, m.OrgCursor, len(m.Organizations))

	// Update project selection via BaseModel helper
	case stateProjSelect:
		if interactive.IsEnterKey(keyMsg) {
			cmd, err := m.UpdateProjSelect(keyMsg)
			if err != nil {
				m.errorMsg = err.Error()
				return m, nil
			}
			m.state = stateCompSelect
			m.errorMsg = ""
			return m, cmd
		}
		m.ProjCursor = interactive.ProcessListCursor(keyMsg, m.ProjCursor, len(m.Projects))

	// Component selection state
	case stateCompSelect:
		if interactive.IsEnterKey(keyMsg) {
			m.state = stateNameInput
			m.errorMsg = ""
			return m, nil
		}
		m.CompCursor = interactive.ProcessListCursor(keyMsg, m.CompCursor, len(m.Components))

	case stateNameInput:
		if interactive.IsEnterKey(keyMsg) {
			// Validate the artifact name.
			if err := validation.ValidateName("deployableartifact", m.name); err != nil {
				m.errorMsg = err.Error()
				return m, nil
			}
			// Check uniqueness
			artifacts, err := m.FetchDeployableArtifacts()
			if err != nil {
				m.errorMsg = fmt.Sprintf("Failed to check deployable artifact existence: %v", err)
				return m, nil
			}
			for _, a := range artifacts {
				if a == m.name {
					m.errorMsg = fmt.Sprintf("Deployable artifact '%s' already exists in component '%s'",
						m.name, m.Components[m.CompCursor])
					return m, nil
				}
			}
			m.artifactTypes = []string{"build", "image"}
			m.state = stateArtifactTypeSelect
			m.errorMsg = ""
			return m, nil
		}
		m.name, _ = interactive.EditTextInputField(keyMsg, m.name, len(m.name))

	case stateArtifactTypeSelect:
		if interactive.IsEnterKey(keyMsg) {
			if m.artifactTypes[m.typeCursor] == "build" {
				builds, err := m.FetchBuildNames()
				if err != nil {
					m.errorMsg = err.Error()
					return m, nil
				}
				if len(builds) == 0 {
					m.errorMsg = "No builds found. Please create a build first."
					return m, nil
				}
				m.builds = builds
				m.state = stateBuildRefSelect
			} else {
				m.state = stateImageTagInput
			}
			m.errorMsg = ""
			return m, nil
		}
		m.typeCursor = interactive.ProcessListCursor(keyMsg, m.typeCursor, len(m.artifactTypes))

	case stateImageTagInput:
		if interactive.IsEnterKey(keyMsg) {
			if m.imageTag == "" {
				m.errorMsg = "Image tag cannot be empty"
				return m, nil
			}
			m.selected = true
			return m, tea.Quit
		}
		m.imageTag, _ = interactive.EditTextInputField(keyMsg, m.imageTag, len(m.imageTag))

	// Select build reference.
	case stateBuildRefSelect:
		if interactive.IsEnterKey(keyMsg) {
			m.selected = true
			return m, tea.Quit
		}
		m.buildCursor = interactive.ProcessListCursor(keyMsg, m.buildCursor, len(m.builds))
	}

	return m, nil
}

func (m deployableArtifactModel) View() string {
	progress := m.RenderProgress()
	var view string

	switch m.state {
	case stateOrgSelect:
		view = m.RenderOrgSelection()
	case stateProjSelect:
		view = m.RenderProjSelection()
	case stateCompSelect:
		view = m.RenderComponentSelection()
	case stateArtifactTypeSelect:
		view = interactive.RenderListPrompt("Select artifact type:", m.artifactTypes, m.typeCursor)
	case stateNameInput:
		view = interactive.RenderInputPrompt("Enter deployable artifact name:", "", m.name, m.errorMsg)
	case stateImageTagInput:
		view = interactive.RenderInputPrompt("Enter image tag:", "", m.imageTag, m.errorMsg)
	case stateBuildRefSelect:
		view = interactive.RenderListPrompt("Select build:", m.builds, m.buildCursor)
	}

	return progress + view
}

func (m deployableArtifactModel) RenderProgress() string {
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
	if m.name != "" {
		progress.WriteString(fmt.Sprintf("- name: %s\n", m.name))
	}
	if len(m.builds) > 0 && m.state > stateBuildRefSelect {
		progress.WriteString(fmt.Sprintf("- build: %s\n", m.builds[m.buildCursor]))
	}
	if m.imageTag != "" {
		progress.WriteString(fmt.Sprintf("- image tag: %s\n", m.imageTag))
	}

	return progress.String()
}

func createDeployableArtifactInteractive(config constants.CRDConfig) error {
	baseModel, err := interactive.NewBaseModel()
	if err != nil {
		return err
	}

	model := deployableArtifactModel{
		BaseModel: *baseModel,
		state:     stateOrgSelect,
	}

	// Run the interactive model.
	finalModel, err := interactive.RunInteractiveModel(model)
	if err != nil {
		return fmt.Errorf("interactive mode failed: %w", err)
	}

	m, ok := finalModel.(deployableArtifactModel)
	if !ok || !m.selected {
		if m.errorMsg != "" {
			return fmt.Errorf("%s", m.errorMsg)
		}
		return fmt.Errorf("deployable artifact creation cancelled")
	}

	params := api.CreateDeployableArtifactParams{
		Name:         m.name,
		Organization: m.Organizations[m.OrgCursor],
		Project:      m.Projects[m.ProjCursor],
		Component:    m.Components[m.CompCursor],
	}

	if m.artifactTypes[m.typeCursor] == "build" {
		params.FromBuildRef = &choreov1.FromBuildRef{
			Name: m.builds[m.buildCursor],
		}
	} else {
		params.FromImageRef = &choreov1.FromImageRef{
			Tag: m.imageTag,
		}
	}

	return createDeployableArtifact(params, config)
}
