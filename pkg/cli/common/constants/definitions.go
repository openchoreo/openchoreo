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

package constants

import (
	"fmt"

	"github.com/choreo-idp/choreo/pkg/cli/common/messages"
)

type Command struct {
	Use     string
	Aliases []string
	Short   string
	Long    string
	Example string
}

var (
	Login = Command{
		Use:   "login",
		Short: "Login to Choreo",
		Long:  "Login to Choreo using your kubeconfig file's context.",
	}

	Logout = Command{
		Use:   "logout",
		Short: "Logout from Choreo",
	}

	Create = Command{
		Use:   "create",
		Short: "Create Choreo resources",
		Long: fmt.Sprintf(`Create Choreo resources like organizations, projects, and components.

Examples:
  # Create an organization interactively
  %[1]s create org

  # Create a project in an organization
  %[1]s create project --organization myorg --name myproject

  # Create a component in a project
  %[1]s create component --organization myorg --project myproject --name mycomponent --url https://github.com/org/repo`,
			messages.DefaultCLIName),
	}

	List = Command{
		Use:     "get",
		Short:   "Get Choreo resources",
		Aliases: []string{"list"},
		Long: fmt.Sprintf(`Get Choreo resources like organizations, projects, and components.

Examples:
  # Get all organizations
  %[1]s get org

  # Get projects in an organization
  %[1]s get project --organization myorg

  # Get components in a project
  %[1]s get component --organization myorg --project myproject

  # Get YAML output
  %[1]s get org -o yaml`,
			messages.DefaultCLIName),
	}

	Apply = Command{
		Use:   "apply",
		Short: "Apply Choreo resource configurations",
		Long: fmt.Sprintf(`Apply a configuration file to create or update Choreo resources.

	Examples:
	  # Apply an organization configuration
	  %[1]s apply -f organization.yaml`,
			messages.DefaultCLIName),
	}

	CreateProject = Command{
		Use:     "project",
		Aliases: []string{"proj", "projects"},
		Short:   "Create a project",
		Long: fmt.Sprintf(`Create a new project in an organization.

Examples:
  # Create a project interactively
  %[1]s create project

  # Create a project in a specific organization
  %[1]s create project --organization myorg --name myproject`,
			messages.DefaultCLIName),
	}

	CreateComponent = Command{
		Use:     "component",
		Aliases: []string{"comp", "components"},
		Short:   "Create a component",
	}

	CreateOrganization = Command{
		Use:     "organization",
		Aliases: []string{"org", "orgs", "organizations"},
		Short:   "Create an organization",
		Long: fmt.Sprintf(`Create a new organization in Choreo.

Examples:
  # Create an organization interactively
  %[1]s create org

  # Create an organization with specific details
  %[1]s create org --name myorg --display-name "My Organization" --description "My organization description"`,
			messages.DefaultCLIName),
	}

	ListOrganization = Command{
		Use:     "organization",
		Aliases: []string{"org", "orgs", "organizations"},
		Short:   "Get organizations",
		Long: fmt.Sprintf(`Get all organizations or get details of a specific organization.

Examples:
  # Get all organizations
  %[1]s get org

  # Get a specific organization
  %[1]s get org myorg

  # Output organization details in YAML format
  %[1]s get org -o yaml

  # Output specific organization in YAML format
  %[1]s get org myorg -o yaml`,
			messages.DefaultCLIName),
	}

	ListProject = Command{
		Use:     "project",
		Aliases: []string{"proj", "projects"},
		Short:   "Get projects",
		Long: fmt.Sprintf(`Get all projects in an organization or get details of a specific project.

Examples:
  # Get all projects in the current organization
  %[1]s get project

  # Get all projects in a specific organization
  %[1]s get project --organization myorg

  # Get a specific project
  %[1]s get project myproject --organization myorg

  # Output project details in YAML format
  %[1]s get project -o yaml --organization myorg

  # Output specific project in YAML format
  %[1]s get project myproject -o yaml --organization myorg`,
			messages.DefaultCLIName),
	}

	ListComponent = Command{
		Use:     "component",
		Aliases: []string{"comp", "components"},
		Short:   "Get components",
		Long: fmt.Sprintf(`Get all components in a project or get details of a specific component.

Examples:
  # Get all components in the current project
  %[1]s get component --organization myorg --project myproject

  # Get a specific component
  %[1]s get component mycomponent --organization myorg --project myproject

  # Output component details in YAML format
  %[1]s get component -o yaml --organization myorg --project myproject

  # Output specific component in YAML format
  %[1]s get component mycomponent -o yaml --organization myorg --project myproject`,
			messages.DefaultCLIName),
	}

	Logs = Command{
		Use:     "logs",
		Aliases: []string{"log"},
		Short:   "Get Choreo resource logs",
		Long: `Get logs from a pod in the current namespace.

This command allows you to view the logs of a specific pod. You can:
- Stream logs in real-time
- Get logs from a specific container
- View logs since a specific time
- Follow log output`,
		Example: `  # Get logs from a specific pod
  choreoctl logs pod-name

  # Stream logs from a pod
  choreoctl logs -f pod-name

  # Get logs from a specific container in a pod
  choreoctl logs pod-name -c container-name

  # Get logs since 1 hour
  choreoctl logs pod-name --since=1h`,
	}

	CreateBuild = Command{
		Use:     "build",
		Aliases: []string{"builds"},
		Short:   "Build a component",
		Long: `Build a component in the current project.

This command creates a new build for a component. You can:
- Create Docker builds
- Create Buildpack builds
- Specify build context and Dockerfile
- Set build arguments`,
		Example: `  # Create a build interactively
  choreoctl create build

  # Create a Docker build
  choreoctl create build --type docker --context . --dockerfile Dockerfile

  # Create a Buildpack build
  choreoctl create build --type buildpack --buildpack java

  # Create a build with specific name
  choreoctl create build my-build --type docker`,
	}

	ListBuild = Command{
		Use:     "build",
		Aliases: []string{"builds"},
		Short:   "Get builds",
		Long: `Get all builds in the current project or organization.
`,
		Example: `  # Get all builds
  choreoctl get builds

  # Get builds for a specific component
  choreoctl get builds  --organization myorg --project myproject --component my-component

  # Get builds in yaml format
  choreoctl get builds -o yaml
`,
	}
	ListDeployableArtifact = Command{
		Use:     "deployableartifact",
		Aliases: []string{"deployableartifacts"},
		Short:   "Get deployable artifacts",
		Long: `Get all deployable artifacts in the current project or organization.
`,
		Example: `  # Get all deployable artifacts
		  choreoctl get deployableartifacts

		  # Get deployable artifacts for a specific component
		  choreoctl get deployableartifacts  --organization myorg --project myproject --component my-component

		  # Get deployable artifacts in yaml format
		  choreoctl get deployableartifacts --organization myorg --project myproject --component my-component -o yaml
`,
	}
	ListDeployment = Command{
		Use:     "deployment",
		Aliases: []string{"deployments", "deploy"},
		Short:   "Get deployments",
		Long: `Get all deployments in the current project or organization.

This command allows you to:
- Get all deployments
- Filter by organization, project, and component
- Filter by environment and deployment track
- View deployments in different output formats`,
		Example: `  # Get all deployments
  choreoctl get deployments

  # Get deployments for a specific component
  choreoctl get deployments --organization myorg --project myproject --component mycomp

  # Get deployments for a specific environment
  choreoctl get deployments --organization myorg --project myproject --component mycomp --environment dev

  # Get deployments for a specific deployment track
  choreoctl get deployments --organization myorg --project myproject --component mycomp --deployment-track main

  # Get deployments in yaml format
  choreoctl get deployments -o yaml --organization myorg --project mycomp

  # Get details of a specific deployment
  choreoctl get deployment mydeployment --organization myorg --project myproject --component mycomp`,
	}

	CreateDeployment = Command{
		Use:     "deployment",
		Aliases: []string{"deployments", "deploy"},
		Short:   "Create a deployment",
		Long:    `Create a deployment in the specified organization, project and component.`,
		Example: `  # Create a deployment interactively
  choreoctl create deployment

  # Create a deployment with specific parameters
  choreoctl create deployment --name mydeployment --organization myorg --project myproj --component mycomp
  --environment dev --deployableartifact myartifact`,
	}

	CreateDeploymentTrack = Command{
		Use:     "deploymenttrack",
		Aliases: []string{"deptrack", "deptracks"},
		Short:   "Create a deployment track",
		Long:    `Create a deployment track in the specified organization, project and component.`,
		Example: `  # Create a deployment track interactively
  choreoctl create deploymenttrack

  # Create a deployment track with specific parameters
  choreoctl create deploymenttrack --name mytrack --org myorg --project myproj --component mycomp`,
	}

	ListDeploymentTrack = Command{
		Use:     "deploymenttrack [name]",
		Aliases: []string{"deptrack", "deptracks"},
		Short:   "Get deployment tracks",
		Long:    `Get deployment tracks in an organization, project and component.`,
		Example: `  # Get all deployment tracks
  choreoctl get deploymenttrack --org myorg --project myproj --component mycomp

  # Get specific deployment track
  choreoctl get deploymenttrack mytrack --org myorg --project myproj --component mycomp

  # Get in YAML format
  choreoctl get deploymenttrack -o yaml`,
	}

	ListEnvironment = Command{
		Use:     "environment [name]",
		Aliases: []string{"env", "environments", "envs"},
		Short:   "Get environments",
		Long: `Get all environments or a specific environment in an organization.
If no organization is specified, you will be prompted to select one interactively.`,
		Example: `  # Get all environments in an organization
  choreoctl get environment --organization myorg

  # Get a specific environment
  choreoctl get environment myenv --organization myorg

  # Get  environments in YAML format
  choreoctl get environment --organization myorg -o yaml

  # get environments interactively
  choreoctl get environment`,
	}

	CreateDataPlane = Command{
		Use:     "dataplane",
		Aliases: []string{"dp", "dataplanes"},
		Short:   "Create a data plane",
		Long:    `Create a data plane in the specified organization.`,
		Example: `  # Create a data plane interactively
  choreoctl create dataplane

  # Create a data plane with specific parameters
  choreoctl create dataplane --name mydp --organization myorg --cluster-name mycluster`,
	}

	ListDataPlane = Command{
		Use:     "dataplane [name]",
		Aliases: []string{"dp", "dataplanes"},
		Short:   "Get data planes",
		Long:    `Get all data planes or a specific data plane in an organization.`,
		Example: `  # Get all data planes
  choreoctl get dataplane --organization myorg

  # Get a specific data plane
  choreoctl get dataplane mydp --organization myorg

  # Get in YAML format
  choreoctl get dataplane --organization myorg -o yaml`,
	}

	ListEndpoint = Command{
		Use:     "endpoint [name]",
		Aliases: []string{"ep", "endpoints"},
		Short:   "Get endpoints",
		Long:    `Get all endpoints in an organization, project, component, and environment.`,
		Example: `  # Get all endpoints
  choreoctl get endpoint --org myorg --project myproj --component mycomp --environment dev

  # Get a specific endpoint
  choreoctl get endpoint myendpoint --org myorg --project myproj --component mycomp --environment dev

  # Get in YAML format
  choreoctl get endpoint --org myorg --project myproj --component mycomp --environment dev -o yaml`,
	}

	CreateEnvironment = Command{
		Use:     "environment",
		Aliases: []string{"env", "environments"},
		Short:   "Create an environment",
		Long:    `Create an environment in the specified organization.`,
		Example: `  # Create an environment interactively
  choreoctl create environment

  # Create an environment with specific parameters
  choreoctl create environment --name dev --organization myorg --dataplane-ref dp1 --dns-prefix dev`,
	}

	CreateDeployableArtifact = Command{
		Use:     "deployableartifact",
		Aliases: []string{"da", "artifact"},
		Short:   "Create a deployable artifact",
		Long:    `Create a deployable artifact in the specified organization, project and component.`,
		Example: `  # Create a deployable artifact interactively
  choreoctl create deployableartifact

  # Create a deployable artifact from a build
  choreoctl create deployableartifact --name myartifact --org myorg --project myproj --component mycomp`,
	}

	// ------------------------------------------------------------------------
	// Config Command Definitions
	// ------------------------------------------------------------------------

	// ConfigRoot holds usage and help texts for "config" command.
	ConfigRoot = Command{
		Use:   "config",
		Short: "Manage Choreo configuration contexts",
		Long: "Manage configuration contexts that store default values (e.g., organization, project, component) " +
			"for choreoctl commands.",
		Example: fmt.Sprintf(`  # Get  all available contexts
  %[1]s config get-contexts

  # Set or update a context
  %[1]s config set-context --name myctx --org myorg

  # Use a context
  %[1]s config use-context --name myctx

  # Show the current context's details
  %[1]s config current-context`, messages.DefaultCLIName),
	}

	// ConfigGetContexts holds usage and help texts for "config get-contexts" command.
	ConfigGetContexts = Command{
		Use:   "get-contexts",
		Short: "Get  all available contexts",
		Long:  "Get  all stored contexts along with an asterisk marking the current context.",
		Example: fmt.Sprintf(`  # Show all contexts
  %[1]s config get-contexts`, messages.DefaultCLIName),
	}

	// ConfigSetContext holds usage and help texts for "config set-context" command.
	ConfigSetContext = Command{
		Use:   "set-context",
		Short: "Create or update a context",
		Long:  "Set a context by specifying values for organization, project, component, build, environment, etc.",
		Example: fmt.Sprintf(`  # Set a context named myctx
  %[1]s config set-context myctx --org myorg --project myproj --environment dev`, messages.DefaultCLIName),
	}

	// ConfigUseContext holds usage and help texts for "config use-context" command.
	ConfigUseContext = Command{
		Use:   "use-context",
		Short: "Switch to a specified context",
		Long: "Set the active context so that subsequent commands will automatically use its " +
			"stored values for flags if omitted.",
		Example: fmt.Sprintf(`  # Switch to context myctx
  %[1]s config use-context --name myctx`, messages.DefaultCLIName),
	}

	// ConfigCurrentContext holds usage and help texts for "config current-context" command.
	ConfigCurrentContext = Command{
		Use:   "current-context",
		Short: "Display the current context",
		Long:  "Show which context is currently active, along with any stored configuration values.",
		Example: fmt.Sprintf(`  # Display the current context
  %[1]s config current-context`, messages.DefaultCLIName),
	}

	// ------------------------------------------------------------------------
	// Flag Descriptions (Used in config commands)
	// ------------------------------------------------------------------------

	// FlagContextNameDesc is used for the --name flag.
	FlagContextNameDesc = "Name of the context to create, update, or use"

	// FlagOrgDesc is used for the --org flag.
	FlagOrgDesc = "Organization name stored in this context"

	// FlagProjDesc is used for the --project flag.
	FlagProjDesc = "Project name stored in this context"

	// FlagComponentDesc is used for the --component flag.
	FlagComponentDesc = "Component name stored in this context"

	// FlagBuildDesc is used for the --build flag.
	FlagBuildDesc = "Build identifier stored in this context"

	// FlagDeploymentTrackDesc is used for the --deploymenttrack flag.
	FlagDeploymentTrackDesc = "Deployment track name stored in this context"

	// FlagEnvDesc is used for the --environment flag.
	FlagEnvDesc = "Environment name stored in this context"

	// FlagDataplaneDesc is used for the --dataplane flag.
	FlagDataplaneDesc = "Data plane reference stored in this context"

	// FlagDeployableArtifactDesc is used for the --deployableartifact flag.
	FlagDeployableArtifactDesc = "Deployable artifact name stored in this context"

	// ------------------------------------------------------------------------
	// Delete Command Definitions
	// ------------------------------------------------------------------------

	// Delete command definitions
	Delete = Command{
		Use:   "delete",
		Short: "Delete Choreo resources",
		Long:  "Delete resources in Choreo platform such as organizations, projects, components, etc.",
		Example: `  # Delete resources from a YAML file
  choreoctl delete -f resources.yaml`,
	}
)
