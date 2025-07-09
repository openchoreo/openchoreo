// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/choreoctl/interactive"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

const (
	stateOrgSelect = iota
	stateProjSelect
	stateCompSelect
	stateDeploymentTrackSelect
	stateNameInput
	stateRevisionInput
)

type buildModel struct {
	interactive.BaseModel
	name                 string
	revision             string
	deploymentTracks     []openchoreov1alpha1.DeploymentTrack
	trackCursor          int
	deploymentTrack      *openchoreov1alpha1.DeploymentTrack
	selected             bool
	errorMsg             string
	state                int
	deploymentTrackNames []string // Add this field
}

func (m buildModel) Init() tea.Cmd {
	return nil
}

func (m buildModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if interactive.IsQuitKey(keyMsg) {
		m.selected = false
		return m, tea.Quit
	}

	switch m.state {
	case stateOrgSelect, stateProjSelect, stateCompSelect, stateDeploymentTrackSelect:
		return m.handleResourceSelection(keyMsg)
	case stateNameInput, stateRevisionInput:
		return m.handleBuildConfig(keyMsg)
	default:
		return m, nil
	}
}

// Update handleResourceSelection to add deployment track selection
func (m buildModel) handleResourceSelection(keyMsg tea.KeyMsg) (buildModel, tea.Cmd) {
	switch m.state {
	case stateOrgSelect:
		if interactive.IsEnterKey(keyMsg) {
			projects, err := m.FetchProjects()
			if err != nil {
				m.errorMsg = err.Error()
				return m, tea.Quit
			}
			if len(projects) == 0 {
				m.errorMsg = fmt.Sprintf("No projects found in organization '%s'", m.Organizations[m.OrgCursor])
				return m, tea.Quit
			}
			m.Projects = projects
			m.state = stateProjSelect
			m.errorMsg = ""
		} else {
			m.OrgCursor = interactive.ProcessListCursor(keyMsg, m.OrgCursor, len(m.Organizations))
		}
	case stateProjSelect:
		if interactive.IsEnterKey(keyMsg) {
			components, err := m.FetchComponents()
			if err != nil {
				m.errorMsg = err.Error()
				return m, tea.Quit
			}
			if len(components) == 0 {
				m.errorMsg = fmt.Sprintf("No components found in project '%s'", m.Projects[m.ProjCursor])
				return m, tea.Quit
			}
			m.Components = components
			m.state = stateCompSelect
		} else {
			m.ProjCursor = interactive.ProcessListCursor(keyMsg, m.ProjCursor, len(m.Projects))
		}
	case stateCompSelect:
		if interactive.IsEnterKey(keyMsg) {
			// Using the FetchDeploymentTracks method from interactive/base.go
			tracks, err := m.FetchDeploymentTracks()
			if err != nil {
				m.errorMsg = fmt.Sprintf("Failed to get deployment tracks: %v", err)
				return m, tea.Quit
			}
			if len(tracks) == 0 {
				m.errorMsg = "No deployment tracks found. Please create a deployment track first"
				return m, tea.Quit
			}

			// Get the actual DeploymentTrack objects now that we know they exist
			// We need to create a DeploymentTrackResource
			trackRes, err := kinds.NewDeploymentTrackResource(
				constants.DeploymentTrackV1Config,
				m.Organizations[m.OrgCursor],
				m.Projects[m.ProjCursor],
				m.Components[m.CompCursor],
			)
			if err != nil {
				m.errorMsg = fmt.Sprintf("Failed to create deployment track resource: %v", err)
				return m, tea.Quit
			}

			// List the deployment track objects
			trackObjects, err := trackRes.List()
			if err != nil {
				m.errorMsg = fmt.Sprintf("Failed to list deployment tracks: %v", err)
				return m, tea.Quit
			}

			m.deploymentTracks = make([]openchoreov1alpha1.DeploymentTrack, len(trackObjects))
			m.deploymentTrackNames = make([]string, len(trackObjects)) // Initialize the new array
			for i, trackWrapper := range trackObjects {
				m.deploymentTracks[i] = *trackWrapper.Resource
				m.deploymentTrackNames[i] = trackWrapper.LogicalName // Store logical names
			}

			m.state = stateDeploymentTrackSelect
			m.errorMsg = ""
		} else {
			m.CompCursor = interactive.ProcessListCursor(keyMsg, m.CompCursor, len(m.Components))
		}

	case stateDeploymentTrackSelect:
		if interactive.IsEnterKey(keyMsg) {
			m.deploymentTrack = &m.deploymentTracks[m.trackCursor]
			m.state = stateNameInput
			m.errorMsg = ""
		} else {
			m.trackCursor = interactive.ProcessListCursor(keyMsg, m.trackCursor, len(m.deploymentTracks))
		}
	}
	return m, nil
}

func (m buildModel) handleBuildConfig(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateNameInput:
		if interactive.IsEnterKey(keyMsg) {
			if err := validation.ValidateName("build", m.name); err != nil {
				m.errorMsg = err.Error()
				return m, nil
			}
			// Set selected to true after name input since revision is optional
			m.selected = true
			m.state = stateRevisionInput
			return m, nil
		}
		m.name, _ = interactive.EditTextInputField(keyMsg, m.name, len(m.name))
		return m, nil

	case stateRevisionInput:
		if interactive.IsEnterKey(keyMsg) {
			// Keep selected as true even if revision is skipped
			return m, tea.Quit
		}
		m.revision, _ = interactive.EditTextInputField(keyMsg, m.revision, len(m.revision))
		return m, nil
	}
	return m, nil
}

// Update View to show deployment track selection
func (m buildModel) View() string {
	progress := m.RenderProgress()
	switch m.state {
	case stateOrgSelect:
		return progress + m.RenderOrgSelection()
	case stateProjSelect:
		return progress + m.RenderProjSelection()
	case stateCompSelect:
		return progress + m.RenderComponentSelection()
	case stateDeploymentTrackSelect:
		// No need to create a new array, use the stored names
		return progress + interactive.RenderListPrompt("Select deployment track:", m.deploymentTrackNames, m.trackCursor)
	case stateNameInput:
		return progress + interactive.RenderInputPrompt("Enter build name:", "", m.name, m.errorMsg)
	case stateRevisionInput:
		return progress + interactive.RenderInputPrompt("Enter git revision (optional, press Enter to use latest):", "", m.revision, m.errorMsg)
	}
	return progress
}

func createBuildInteractive(config constants.CRDConfig) error {
	baseModel, err := interactive.NewBaseModel()
	if err != nil {
		return err
	}

	model := buildModel{
		BaseModel: *baseModel,
		state:     stateOrgSelect,
	}

	finalModel, err := interactive.RunInteractiveModel(model)
	if err != nil {
		return fmt.Errorf("interactive mode failed: %w", err)
	}

	m, ok := finalModel.(buildModel)
	if !ok || !m.selected {
		if m.errorMsg != "" {
			return fmt.Errorf("%s", m.errorMsg)
		}
		return fmt.Errorf("build creation cancelled")
	}

	params := api.CreateBuildParams{
		Organization:    m.Organizations[m.OrgCursor],
		Project:         m.Projects[m.ProjCursor],
		Component:       m.Components[m.CompCursor],
		Name:            m.name,
		DeploymentTrack: m.deploymentTrackNames[m.trackCursor], // Use logical name
		Revision:        defaultIfEmpty(m.revision, ""),
	}

	// Enrich params with deployment track configuration
	if m.deploymentTrack != nil && m.deploymentTrack.Spec.BuildTemplateSpec != nil {
		buildSpec := m.deploymentTrack.Spec.BuildTemplateSpec
		params.Branch = buildSpec.Branch
		params.Path = buildSpec.Path

		if buildSpec.BuildConfiguration != nil {
			if buildSpec.BuildConfiguration.Docker != nil {
				params.Docker = &openchoreov1alpha1.DockerConfiguration{
					Context:        buildSpec.BuildConfiguration.Docker.Context,
					DockerfilePath: buildSpec.BuildConfiguration.Docker.DockerfilePath,
				}
			} else if buildSpec.BuildConfiguration.Buildpack != nil {
				params.Buildpack = &openchoreov1alpha1.BuildpackConfiguration{
					Name:    buildSpec.BuildConfiguration.Buildpack.Name,
					Version: buildSpec.BuildConfiguration.Buildpack.Version,
				}
			}
		}
	}

	err = createBuild(params, config)
	if err != nil {
		return fmt.Errorf("failed to create build: %w", err)
	}

	return nil
}

// Update RenderProgress to show selected deployment track
func (m buildModel) RenderProgress() string {
	var progress strings.Builder
	progress.WriteString("Selected inputs:\n")

	if len(m.Organizations) > 0 {
		progress.WriteString(fmt.Sprintf("- organization: %s\n", m.Organizations[m.OrgCursor]))
	}

	if len(m.Projects) > 0 {
		progress.WriteString(fmt.Sprintf("- project: %s\n", m.Projects[m.ProjCursor]))
	}

	if len(m.Components) > 0 {
		progress.WriteString(fmt.Sprintf("- component: %s\n", m.Components[m.CompCursor]))
	}

	if len(m.deploymentTracks) > 0 && m.state > stateDeploymentTrackSelect {
		progress.WriteString(fmt.Sprintf("- deployment track: %s\n", m.deploymentTrackNames[m.trackCursor]))
	}

	if m.name != "" {
		progress.WriteString(fmt.Sprintf("- name: %s\n", m.name))
	}

	revision := "latest"
	if m.revision != "" {
		revision = m.revision
	}
	progress.WriteString(fmt.Sprintf("- revision: %s\n", revision))

	return progress.String()
}

func defaultIfEmpty(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}
