// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/choreoctl/interactive"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

const (
	stateOrgSelect = iota
	stateProjSelect
	stateNameInput
	stateTypeSelect
	stateURLInput
	stateBranchInput
	statePathInput
	stateBuildTypeSelect
	stateDockerContextInput
	stateDockerfilePathInput
	stateBuildpackNameInput
	stateBuildpackVersionInput
)

type componentModel struct {
	interactive.BaseModel // Reuses common organization/project selection logic
	types                 []choreov1.ComponentType
	typeCursor            int

	// Component fields
	name   string
	url    string
	branch string
	path   string // Add path field

	// Build fields
	buildTypes  []string
	buildType   string
	buildCursor int

	// Docker fields
	dockerContext string
	dockerFile    string

	// Buildpack fields
	buildpacks        []string
	buildpackName     string
	buildpackCursor   int
	buildpackVersions []string
	buildpackVer      string
	versionCursor     int

	selected bool
	errorMsg string
	state    int
}

func (m componentModel) Init() tea.Cmd {
	return nil
}

func (m componentModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if interactive.IsQuitKey(keyMsg) {
		m.selected = false
		return m, tea.Quit
	}

	switch {
	case m.state <= stateProjSelect:
		return m.handleResourceSelection(keyMsg)
	case m.state <= statePathInput:
		return m.handleComponentConfig(keyMsg)
	default:
		return m.handleBuildConfig(keyMsg)
	}
}

// Handle organization/project selection
func (m componentModel) handleResourceSelection(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateOrgSelect:
		if interactive.IsEnterKey(keyMsg) {
			projects, err := m.FetchProjects()
			if err != nil {
				m.errorMsg = err.Error()
				m.selected = false
				return m, tea.Quit
			}
			if len(projects) == 0 {
				m.errorMsg = fmt.Sprintf("No projects found in organization '%s'. Please create a project first using 'choreoctl create project'",
					m.Organizations[m.OrgCursor])
				m.selected = false
				return m, tea.Quit
			}
			m.Projects = projects
			m.state = stateProjSelect
			m.errorMsg = ""
			return m, nil
		}
		m.OrgCursor = interactive.ProcessListCursor(keyMsg, m.OrgCursor, len(m.Organizations))

	case stateProjSelect:
		if interactive.IsEnterKey(keyMsg) {
			m.state = stateNameInput
			m.errorMsg = ""
			return m, nil
		}
		m.ProjCursor = interactive.ProcessListCursor(keyMsg, m.ProjCursor, len(m.Projects))
	}
	return m, nil
}

// Handle component configuration (name, type, URL, branch, path)
func (m componentModel) handleComponentConfig(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateNameInput:
		return m.handleNameInput(keyMsg)
	case stateTypeSelect:
		return m.handleTypeSelect(keyMsg)
	case stateURLInput:
		return m.handleURLInput(keyMsg)
	case stateBranchInput:
		if interactive.IsEnterKey(keyMsg) {
			m.state = statePathInput
			return m, nil
		}
		m.branch, _ = interactive.EditTextInputField(keyMsg, m.branch, len(m.branch))
	case statePathInput:
		if interactive.IsEnterKey(keyMsg) {
			m.state = stateBuildTypeSelect
			return m, nil
		}
		m.path, _ = interactive.EditTextInputField(keyMsg, m.path, len(m.path))
	}
	return m, nil
}

// Handle build configuration (Docker/Buildpack)
func (m componentModel) handleBuildConfig(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateBuildTypeSelect:
		return m.handleBuildTypeSelect(keyMsg)
	case stateDockerContextInput, stateDockerfilePathInput:
		return m.handleDockerConfig(keyMsg)
	case stateBuildpackNameInput, stateBuildpackVersionInput:
		return m.handleBuildpackConfig(keyMsg)
	}
	return m, nil
}

func (m componentModel) handleNameInput(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		if err := validation.ValidateComponentName(m.name); err != nil {
			m.errorMsg = err.Error()
			return m, nil
		}
		components, err := m.FetchComponents()
		if err != nil {
			m.errorMsg = fmt.Sprintf("Failed to check component existence: %v", err)
			return m, nil
		}
		for _, c := range components {
			if c == m.name {
				m.errorMsg = fmt.Sprintf("Component '%s' already exists in project '%s'",
					m.name, m.Projects[m.ProjCursor])
				return m, nil
			}
		}
		m.state = stateTypeSelect
		m.errorMsg = ""
		return m, nil
	}
	m.errorMsg = ""
	m.name, _ = interactive.EditTextInputField(keyMsg, m.name, len(m.name))
	return m, nil
}

func (m componentModel) handleTypeSelect(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		// Validate that a component type is selected.
		if m.typeCursor < 0 || m.typeCursor >= len(m.types) {
			m.errorMsg = "Invalid component type selected"
			return m, nil
		}
		m.state = stateURLInput
		m.errorMsg = ""
		return m, nil
	}
	m.typeCursor = interactive.ProcessListCursor(keyMsg, m.typeCursor, len(m.types))
	return m, nil
}

func (m componentModel) handleURLInput(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		if err := validation.ValidateGitHubURL(m.url); err != nil {
			m.errorMsg = err.Error()
			return m, nil
		}
		m.state = stateBranchInput
		return m, nil
	}
	m.url, _ = interactive.EditTextInputField(keyMsg, m.url, len(m.url))
	return m, nil
}

func (m componentModel) handleBuildTypeSelect(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if interactive.IsEnterKey(keyMsg) {
		m.buildType = m.buildTypes[m.buildCursor]
		if m.buildType == constants.Docker {
			m.state = stateDockerContextInput
		} else {
			m.state = stateBuildpackNameInput
		}
		return m, nil
	}
	m.buildCursor = interactive.ProcessListCursor(keyMsg, m.buildCursor, len(m.buildTypes))
	return m, nil
}

func (m componentModel) handleDockerConfig(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateDockerContextInput:
		if interactive.IsEnterKey(keyMsg) {
			m.state = stateDockerfilePathInput
			return m, nil
		}
		m.dockerContext, _ = interactive.EditTextInputField(keyMsg, m.dockerContext, len(m.dockerContext))
	case stateDockerfilePathInput:
		if interactive.IsEnterKey(keyMsg) {
			m.selected = true
			return m, tea.Quit
		}
		m.dockerFile, _ = interactive.EditTextInputField(keyMsg, m.dockerFile, len(m.dockerFile))
	}
	return m, nil
}

func (m componentModel) handleBuildpackConfig(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateBuildpackNameInput:
		if interactive.IsEnterKey(keyMsg) {
			m.buildpackName = m.buildpacks[m.buildpackCursor]
			m.buildpackVersions = choreov1.SupportedVersions[choreov1.BuildpackName(m.buildpackName)]
			if len(m.buildpackVersions) == 0 {
				m.errorMsg = "no versions available for selected buildpack"
				return m, nil
			}
			m.state = stateBuildpackVersionInput
			return m, nil
		}
		m.buildpackCursor = interactive.ProcessListCursor(keyMsg, m.buildpackCursor, len(m.buildpacks))
	case stateBuildpackVersionInput:
		if interactive.IsEnterKey(keyMsg) {
			m.buildpackVer = m.buildpackVersions[m.versionCursor]
			m.selected = true
			return m, tea.Quit
		}
		m.versionCursor = interactive.ProcessListCursor(keyMsg, m.versionCursor, len(m.buildpackVersions))
	}
	return m, nil
}

func (m componentModel) View() string {
	progress := m.RenderProgress()
	var view string
	switch m.state {
	case stateOrgSelect:
		view = m.RenderOrgSelection()
	case stateProjSelect:
		view = m.RenderProjSelection()
	case stateNameInput:
		view = interactive.RenderInputPrompt("Enter component name:", "", m.name, m.errorMsg)
	case stateTypeSelect:
		typeOptions := make([]string, len(m.types))
		for i, t := range m.types {
			typeOptions[i] = string(t)
		}
		view = interactive.RenderListPrompt("Select component type:", typeOptions, m.typeCursor)
	case stateURLInput:
		view = interactive.RenderInputPrompt("Enter git repository URL:", "", m.url, m.errorMsg)
	case stateBranchInput:
		view = interactive.RenderInputPrompt("Enter git branch (default: main):", "", m.branch, m.errorMsg)
	case statePathInput:
		view = interactive.RenderInputPrompt("Enter source code path (default: /):", "", m.path, m.errorMsg)
	case stateBuildTypeSelect:
		view = interactive.RenderListPrompt("Select build type:", m.buildTypes, m.buildCursor)
	case stateDockerContextInput:
		view = interactive.RenderInputPrompt("Enter Docker context path:", "/", m.dockerContext, m.errorMsg)
	case stateDockerfilePathInput:
		view = interactive.RenderInputPrompt("Enter Dockerfile path:", "Dockerfile", m.dockerFile, m.errorMsg)
	case stateBuildpackNameInput:
		view = interactive.RenderListPrompt("Select buildpack type:", m.buildpacks, m.buildpackCursor)
	case stateBuildpackVersionInput:
		view = interactive.RenderListPrompt("Select buildpack version:", m.buildpackVersions, m.versionCursor)
	}
	return progress + view
}

func createComponentInteractive(config constants.CRDConfig) error {
	baseModel, err := interactive.NewBaseModel()
	if err != nil {
		return err
	}

	// Initialize all supported component types
	componentTypes := []choreov1.ComponentType{
		choreov1.ComponentTypeScheduledTask,
		choreov1.ComponentTypeWebApplication,
		choreov1.ComponentTypeService,
	}

	model := componentModel{
		BaseModel:  *baseModel,
		state:      stateOrgSelect,
		types:      componentTypes,
		buildTypes: []string{constants.Docker, constants.Buildpack},
		buildpacks: getBuildpackTypes(),
	}

	finalModel, err := interactive.RunInteractiveModel(model)
	if err != nil {
		return fmt.Errorf("interactive mode failed: %w", err)
	}

	m, ok := finalModel.(componentModel)
	if !ok || !m.selected {
		if m.errorMsg != "" {
			return fmt.Errorf("%s", m.errorMsg)
		}
		return fmt.Errorf("component creation cancelled")
	}

	params := api.CreateComponentParams{
		Organization:     m.Organizations[m.OrgCursor],
		Project:          m.Projects[m.ProjCursor],
		Name:             m.name,
		DisplayName:      m.name, // Use name as display name if not provided
		GitRepositoryURL: m.url,
		Branch:           defaultIfEmpty(m.branch, "main"),
		Path:             defaultIfEmpty(m.path, "/"),
		Type:             m.types[m.typeCursor],
	}

	if m.buildType == constants.Docker {
		params.DockerFile = m.dockerFile
		params.DockerContext = m.dockerContext
	} else {
		params.BuildpackName = m.buildpackName
		params.BuildpackVersion = m.buildpackVer
	}

	// Call the new createComponent function with config
	if err := createComponent(params, config); err != nil {
		return fmt.Errorf("failed to create component: %w", err)
	}

	return nil
}

func getBuildpackTypes() []string {
	keys := make([]string, 0, len(choreov1.SupportedVersions))
	for k := range choreov1.SupportedVersions {
		keys = append(keys, string(k))
	}
	return keys
}

func (m componentModel) RenderProgress() string {
	var progress strings.Builder
	progress.WriteString("Selected inputs:\n")

	if len(m.Organizations) > 0 {
		progress.WriteString(fmt.Sprintf("- organization: %s\n", m.Organizations[m.OrgCursor]))
	}

	if len(m.Projects) > 0 {
		progress.WriteString(fmt.Sprintf("- project: %s\n", m.Projects[m.ProjCursor]))
	}

	if m.name != "" {
		progress.WriteString(fmt.Sprintf("- name: %s\n", m.name))
	}

	if m.state > stateTypeSelect && m.typeCursor < len(m.types) {
		progress.WriteString(fmt.Sprintf("- type: %s\n", m.types[m.typeCursor]))
	}

	if m.url != "" {
		progress.WriteString(fmt.Sprintf("- git repository: %s\n", m.url))
	}

	if m.branch != "" {
		progress.WriteString(fmt.Sprintf("- branch: %s\n", m.branch))
	}

	if m.path != "" {
		progress.WriteString(fmt.Sprintf("- path: %s\n", m.path))
	}

	if m.buildType != "" {
		progress.WriteString(fmt.Sprintf("- build type: %s\n", m.buildType))

		if m.buildType == constants.Docker {
			if m.dockerContext != "" {
				progress.WriteString(fmt.Sprintf("- docker context: %s\n", m.dockerContext))
			}
			if m.dockerFile != "" {
				progress.WriteString(fmt.Sprintf("- dockerfile path: %s\n", m.dockerFile))
			}
		}

		if m.buildType == constants.Buildpack {
			if m.buildpackName != "" {
				progress.WriteString(fmt.Sprintf("- buildpack: %s\n", m.buildpackName))
			}
			if m.buildpackVer != "" {
				progress.WriteString(fmt.Sprintf("- buildpack version: %s\n", m.buildpackVer))
			}
		}
	}

	return progress.String()
}

func defaultIfEmpty(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}
