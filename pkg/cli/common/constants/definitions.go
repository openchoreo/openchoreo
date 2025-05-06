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

	"github.com/openchoreo/openchoreo/pkg/cli/common/messages"
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

	Version = Command{
		Use:   "version",
		Short: "Show the version information",
		Long:  "Show the version information of the Choreo CLI.",
	}

	Create = Command{
		Use:   "create",
		Short: "Create Choreo resources",
		Long: fmt.Sprintf(`Create Choreo resources like organizations, projects, and components.

Examples:
  # Create an organization interactively
  %[1]s create organization --interactive

  # Create a project in an organization
  %[1]s create project --organization acme-corp --name online-store

  # Create a component in a project
  %[1]s create component --organization acme-corp --project online-store --name product-catalog \
   --git-repository-url https://github.com/org/repo`, messages.DefaultCLIName),
	}

	List = Command{
		Use:     "get",
		Short:   "List Choreo resources",
		Aliases: []string{"list"},
		Long: fmt.Sprintf(`List Choreo resources like organizations, projects, and components.

Examples:
  # List all organizations
  %[1]s get organization

  # List projects in an organization
  %[1]s get project --organization acme-corp

  # List components in a project
  %[1]s get component --organization acme-corp --project online-store

  # Output organization details in YAML format
  %[1]s get organization -o yaml`,
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
  %[1]s create project --interactive

  # Create a project in a specific organization
  %[1]s create project --organization acme-corp --name online-store`,
			messages.DefaultCLIName),
	}

	CreateComponent = Command{
		Use:     "component",
		Aliases: []string{"comp", "components"},
		Short:   "Create a new component in a project",
		Long: fmt.Sprintf(`Create a new component in the specified project and organization.

Examples:
  # Create a component interactively
  %[1]s create component --interactive

  # Create a component with Git repository
  %[1]s create component --name product-catalog --organization acme-corp --project online-store \
    --display-name "Product Catalog" --git-repository-url https://github.com/acme-corp/product-catalog --type Service

  # Create a component with build configuration
  %[1]s create component --name product-catalog --organization acme-corp --project online-store \
    --type Service --git-repository-url https://github.com/acme-corp/product-catalog --branch main \
	--path / --docker-context ./src --dockerfile-path ./src/Dockerfile`,
			messages.DefaultCLIName),
	}

	CreateOrganization = Command{
		Use:     "organization",
		Aliases: []string{"org", "orgs", "organizations"},
		Short:   "Create an organization",
		Long: fmt.Sprintf(`Create a new organization in Choreo.

Examples:
  # Create an organization interactively
  %[1]s create organization --interactive

  # Create an organization with specific details
  %[1]s create organization --name acme-corp --display-name "ACME" --description "ACME Corporation"`,
			messages.DefaultCLIName),
	}

	ListOrganization = Command{
		Use:     "organization",
		Aliases: []string{"org", "orgs", "organizations"},
		Short:   "List organizations",
		Long: fmt.Sprintf(`List all organizations or get details of a specific organization.

Examples:
  # List all organizations
  %[1]s get organization

  # List a specific organization
  %[1]s get organization acme-corp

  # Output organization details in YAML format
  %[1]s get organization -o yaml

  # Output specific organization in YAML format
  %[1]s get organization acme-corp -o yaml`,
			messages.DefaultCLIName),
	}

	ListProject = Command{
		Use:     "project",
		Aliases: []string{"proj", "projects"},
		Short:   "List projects",
		Long: fmt.Sprintf(`List all projects in an organization or get details of a specific project.

Examples:
  # List all projects in the current organization
  %[1]s get project

  # List all projects in a specific organization
  %[1]s get project --organization acme-corp

  # List a specific project
  %[1]s get project online-store --organization acme-corp

  # Output project details in YAML format
  %[1]s get project -o yaml --organization acme-corp

  # Output specific project in YAML format
  %[1]s get project online-store -o yaml --organization acme-corp`,
			messages.DefaultCLIName),
	}

	ListComponent = Command{
		Use:     "component",
		Aliases: []string{"comp", "components"},
		Short:   "List components",
		Long: fmt.Sprintf(`List all components in a project or get details of a specific component.

Examples:
  # List all components in the current project
  %[1]s get component --organization acme-corp --project online-store

  # List a specific component
  %[1]s get component product-catalog --organization acme-corp --project online-store

  # Output component details in YAML format
  %[1]s get component -o yaml --organization acme-corp --project online-store

  # Output specific component in YAML format
  %[1]s get component product-catalog -o yaml --organization acme-corp --project online-store`,
			messages.DefaultCLIName),
	}

	Logs = Command{
		Use:     "logs",
		Aliases: []string{"log"},
		Short:   "Get logs for Choreo resources",
		Long: `Get logs for Choreo resources such as build and deployment.

This command allows you to:
- Stream logs in real-time
- Get logs from a specific build or deployment
- Follow log output`,
		Example: `  # Get logs from a specific build
  choreoctl logs --type build --build product-catalog-build-01 --organization acme-corp --project online-store \
  --component product-catalog

  # Get logs from a specific deployment
  choreoctl logs --type deployment --deployment product-catalog-dev-01 --organization acme-corp --project online-store \
  --component product-catalog --environment development

  # Get last 100 lines of logs from a specific build
  choreoctl logs --type build --build product-catalog-build-01 --organization acme-corp --project online-store \
  --component product-catalog --tail 100

  # Stream logs from a specific build
  choreoctl logs --type build --build product-catalog-build-01 --organization acme-corp --project online-store \
   --component product-catalog --follow
  `,
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
- Define custom build arguments`,
		Example: `  # Create a build interactively
  choreoctl create build --interactive

  # Create a Docker build
  choreoctl create build --name product-catalog-build-01 --organization acme-corp --project online-store \
    --component product-catalog --docker-context ./src --dockerfile-path ./src/Dockerfile --deployment-track main

  # Create a Buildpack build
  choreoctl create build --name product-catalog-build-01 --organization acme-corp --project online-store \
    --component product-catalog --buildpack-name java --buildpack-version  --deployment-track main

  # Create a build with revision and branch
  choreoctl create build --name product-catalog-build-01 --organization acme-corp --project online-store \
    --component product-catalog --branch main --revision abc123 --auto-build true`,
	}

	ListBuild = Command{
		Use:     "build",
		Aliases: []string{"builds"},
		Short:   "List builds",
		Long: `List all builds in the current project or organization.
`,
		Example: `  # List all builds
  choreoctl get build

  # List builds for a specific component
  choreoctl get build  --organization acme-corp --project online-store --component product-catalog

  # List builds in yaml format
  choreoctl get build -o yaml
`,
	}
	ListDeployableArtifact = Command{
		Use:     "deployableartifact",
		Aliases: []string{"deployableartifacts"},
		Short:   "List deployable artifacts",
		Long: `List all deployable artifacts in the current project or organization.
`,
		Example: `  # List all deployable artifacts
		  choreoctl get deployableartifact

		  # List deployable artifacts for a specific component
		  choreoctl get deployableartifact  --organization acme-corp --project online-store --component product-catalog

		  # List deployable artifacts in yaml format
		  choreoctl get deployableartifact --organization acme-corp --project online-store --component product-catalog -o yaml
`,
	}
	ListDeployment = Command{
		Use:     "deployment",
		Aliases: []string{"deployments", "deploy"},
		Short:   "List deployments",
		Long: `List all deployments in the current project or organization.

This command allows you to:
- List all deployments
- Filter by organization, project, and component
- Filter by environment and deployment track
- View deployments in different output formats`,
		Example: `  # List all deployments
  choreoctl get deployment

  # List deployments for a specific component
  choreoctl get deployment --organization acme-corp --project online-store --component product-catalog

  # List deployments for a specific environment
  choreoctl get deployment --organization acme-corp --project online-store --component product-catalog \
  --environment dev

  # List deployments for a specific deployment track
  choreoctl get deployment --organization acme-corp --project online-store --component product-catalog \
   --deployment-track main

  # List deployments in yaml format
  choreoctl get deployment -o yaml --organization acme-corp --project product-catalog

  # List details of a specific deployment
  choreoctl get deployment product-catalog-dev-01 --organization acme-corp --project online-store \
   --component product-catalog`,
	}

	CreateDeployment = Command{
		Use:     "deployment",
		Aliases: []string{"deployments", "deploy"},
		Short:   "Create a deployment",
		Long:    `Create a deployment in the specified organization, project and component.`,
		Example: `  # Create a deployment interactively
  choreoctl create deployment --interactive

  # Create a deployment with specific parameters
  choreoctl create deployment --name product-catalog-dev-01 --organization acme-corp --project online-store \
    --component product-catalog --environment development --deployableartifact product-catalog-artifact`,
	}

	CreateDeploymentTrack = Command{
		Use:     "deploymenttrack",
		Aliases: []string{"deptrack", "deptracks"},
		Short:   "Create a deployment track",
		Long:    `Create a deployment track in the specified organization, project and component.`,
		Example: `  # Create a deployment track interactively
  choreoctl create deploymenttrack --interactive

  # Create a deployment track with specific parameters
  choreoctl create deploymenttrack --name main-track --organization acme-corp --project online-store \
    --component product-catalog --api-version v1 --auto-deploy true`,
	}

	ListDeploymentTrack = Command{
		Use:     "deploymenttrack [name]",
		Aliases: []string{"deptrack", "deptracks"},
		Short:   "List deployment tracks",
		Long:    `List deployment tracks in an organization, project and component.`,
		Example: `  # List all deployment tracks
  choreoctl get deploymenttrack --organization acme-corp --project online-store --component product-catalog

  # List specific deployment track
  choreoctl get deploymenttrack main-track --organization acme-corp --project online-store --component product-catalog

  # Output deployment tracks in YAML format
  choreoctl get deploymenttrack -o yaml`,
	}

	ListEnvironment = Command{
		Use:     "environment [name]",
		Aliases: []string{"env", "environments", "envs"},
		Short:   "List environments",
		Long: `List all environments or a specific environment in an organization.
If no organization is specified, you will be prompted to select one interactively.`,
		Example: `  # List all environments in an organization
  choreoctl get environment --organization acme-corp

  # List a specific environment
  choreoctl get environment development --organization acme-corp

  # Output environments in YAML format
  choreoctl get environment --organization acme-corp -o yaml

  # get environments interactively
  choreoctl get environment --interactive`,
	}

	CreateDataPlane = Command{
		Use:     "dataplane",
		Aliases: []string{"dp", "dataplanes"},
		Short:   "Create a data plane",
		Long:    `Create a data plane in the specified organization.`,
		Example: `  # Create a data plane interactively
  choreoctl create dataplane --interactive

  # Create a data plane with specific parameters
  choreoctl create dataplane --name primary-dataplane --organization acme-corp --cluster-name k8s-cluster-01 \
    --connection-config kubeconfig --enable-cilium --enable-scale-to-zero --gateway-type envoy \
    --public-virtual-host api.example.com`,
	}

	ListDataPlane = Command{
		Use:     "dataplane [name]",
		Aliases: []string{"dp", "dataplanes"},
		Short:   "List data planes",
		Long:    `List all data planes or a specific data plane in an organization.`,
		Example: `  # List all data planes
  choreoctl get dataplane --organization acme-corp

  # List a specific data plane
  choreoctl get dataplane primary-dataplane --organization acme-corp

  # Output data plane details in YAML format
  choreoctl get dataplane --organization acme-corp -o yaml`,
	}

	ListEndpoint = Command{
		Use:     "endpoint [name]",
		Aliases: []string{"ep", "endpoints"},
		Short:   "List endpoints",
		Long:    `List all endpoints in an organization, project, component, and environment.`,
		Example: `  # List all endpoints
  choreoctl get endpoint --organization acme-corp --project online-store --component product-catalog \
  --environment dev

  # List a specific endpoint
  choreoctl get endpoint product-ep --organization acme-corp --project online-store --component product-catalog \
   --environment dev

  # Output endpoint details in YAML format
  choreoctl get endpoint --organization acme-corp --project online-store --component product-catalog \
  --environment development -o yaml`,
	}

	CreateEnvironment = Command{
		Use:     "environment",
		Aliases: []string{"env", "environments"},
		Short:   "Create an environment",
		Long:    `Create an environment in the specified organization.`,
		Example: `  # Create an environment interactively
  choreoctl create environment --interactive

  # Create a development environment
  choreoctl create environment --name dev --organization acme-corp --dataplane-ref primary-dataplane --dns-prefix dev

  # Create a production environment
  choreoctl create environment --name production --organization acme-corp --dataplane-ref primary-dataplane \
    --dns-prefix prod --production`,
	}

	CreateDeployableArtifact = Command{
		Use:     "deployableartifact",
		Aliases: []string{"da", "artifact"},
		Short:   "Create a deployable artifact",
		Long:    `Create a deployable artifact in the specified organization, project and component.`,
		Example: `  # Create a deployable artifact interactively
  choreoctl create deployableartifact --interactive

  # Create a deployable artifact from a build
  choreoctl create deployableartifact --name product-catalog-artifact --organization acme-corp \
    --project online-store --component product-catalog --build product-catalog-build-01

  # Create a deployable artifact from an image
  choreoctl create deployableartifact --name product-catalog-artifact --organization acme-corp \
    --project online-store --component product-catalog --from-image-ref product-catalog:latest`,
	}

	CreateDeploymentPipeline = Command{
		Use:     "deploymentpipeline",
		Aliases: []string{"deppipe", "deppipes", "deploymentpipelines"},
		Short:   "Create a deployment pipeline",
		Long:    `Create a deployment pipeline in the specified organization.`,
		Example: `  # Create a deployment pipeline with specific parameters
  choreoctl create deploymentpipeline --name dev-stage-prod --organization acme-corp \
   --environment-order "development,staging,production"`,
	}

	ListDeploymentPipeline = Command{
		Use:     "deploymentpipeline [name]",
		Aliases: []string{"deppipe", "deppipes", "deploymentpipelines"},
		Short:   "List deployment pipelines",
		Long:    `List all deployment pipelines or a specific deployment pipeline in an organization.`,
		Example: `  # List all deployment pipelines
  choreoctl get deploymentpipeline --organization acme-corp

  # List a specific deployment pipeline
  choreoctl get deploymentpipeline default-pipeline --organization acme-corp

  # Output deployment pipeline details in YAML format
  choreoctl get deploymentpipeline --organization acme-corp -o yaml`,
	}

	ListConfigurationGroup = Command{
		Use:     "configurationgroup [name]",
		Aliases: []string{"cg", "configurationgroup"},
		Short:   "List configuration groups",
		Long:    `List all configuration groups or a specific configuration group in an organization.`,
		Example: `  # List all configuration groups
  choreoctl get configurationgroup --organization acme-corp

  # List a specific configuration group
  choreoctl get configurationgroup config-group-1 --organization acme-corp

  # Output configuration group details in YAML format
  choreoctl get configurationgroup --organization acme-corp -o yaml`,
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
		Example: fmt.Sprintf(`  # List all stored configuration contexts
  %[1]s config get-contexts

  # Set or update a configuration context
  %[1]s config set-context --name acme-corp-context --organization acme-corp

  # Use a configuration context
  %[1]s config use-context --name acme-corp-context

  # Show the current configuration context's details
  %[1]s config current-context`, messages.DefaultCLIName),
	}

	// ConfigGetContexts holds usage and help texts for "config get-contexts" command.
	ConfigGetContexts = Command{
		Use:   "get-contexts",
		Short: "List all available configuration contexts",
		Long:  "List all stored configuration contexts, with an asterisk (*) marking the currently active context",
		Example: fmt.Sprintf(`  # Show all configuration contexts
  %[1]s config get-contexts`, messages.DefaultCLIName),
	}

	// ConfigSetContext holds usage and help texts for "config set-context" command.
	ConfigSetContext = Command{
		Use:   "set-context",
		Short: "Create or update a configuration context",
		Long:  "Configure a context by specifying values for organization, project, component, build, environment, etc.",
		Example: fmt.Sprintf(`  # Set a configuration context named acme-corp-context
  %[1]s config set-context acme-corp-context --organization acme-corp \
    --project online-store --environment dev`,
			messages.DefaultCLIName),
	}

	// ConfigUseContext holds usage and help texts for "config use-context" command.
	ConfigUseContext = Command{
		Use:   "use-context",
		Short: "Switch to a specified configuration context",
		Long:  "Set the active context, so subsequent commands automatically use its stored values when flags are omitted.",
		Example: fmt.Sprintf(`  # Switch to the configuration context named acme-corp-context
  %[1]s config use-context --name acme-corp-context`, messages.DefaultCLIName),
	}

	// ConfigCurrentContext holds usage and help texts for "config current-context" command.
	ConfigCurrentContext = Command{
		Use:   "current-context",
		Short: "Display the currently active configuration context",
		Long:  "Display the currently active configuration context, including any stored configuration values.",
		Example: fmt.Sprintf(`  # Display the currently active configuration context
  %[1]s config current-context`, messages.DefaultCLIName),
	}

	// ------------------------------------------------------------------------
	// Flag Descriptions (Used in config commands)
	// ------------------------------------------------------------------------

	// FlagContextNameDesc is used for the --name flag.
	FlagContextNameDesc = "Name of the configuration context to create, update, or use"

	// FlagOrgDesc is used for the --organization flag.
	FlagOrgDesc = "Organization name stored in this configuration context"

	// FlagProjDesc is used for the --project flag.
	FlagProjDesc = "Project name stored in this configuration context"

	// FlagComponentDesc is used for the --component flag.
	FlagComponentDesc = "Component name stored in this configuration context"

	// FlagBuildDesc is used for the --build flag.
	FlagBuildDesc = "Build identifier stored in this configuration context"

	// FlagDeploymentTrackDesc is used for the --deploymenttrack flag.
	FlagDeploymentTrackDesc = "Deployment track name stored in this configuration context"

	// FlagEnvDesc is used for the --environment flag.
	FlagEnvDesc = "Environment name stored in this configuration context"

	// FlagDataplaneDesc is used for the --dataplane flag.
	FlagDataplaneDesc = "Data plane reference stored in this configuration context"

	// FlagDeployableArtifactDesc is used for the --deployableartifact flag.
	FlagDeployableArtifactDesc = "Deployable artifact name stored in this configuration context"

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
