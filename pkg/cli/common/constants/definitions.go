// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

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
		Short: "Login to OpenChoreo CLI",
		Long:  "Login to OpenChoreo CLI",
	}

	Logout = Command{
		Use:   "logout",
		Short: "Logout from OpenChoreo CLI",
	}

	Version = Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print version information.",
	}

	Create = Command{
		Use:   "create",
		Short: "Create OpenChoreo resources",
		Long: fmt.Sprintf(`Create OpenChoreo resources like namespaces, projects, and components.

Examples:
  # Create a project in a namespace
  %[1]s create project --namespace acme-corp --name online-store

  # Create a component in a project
  %[1]s create component --namespace acme-corp --project online-store --name product-catalog \
   --git-repository-url https://github.com/org/repo`, messages.DefaultCLIName),
	}

	List = Command{
		Use:     "get",
		Short:   "List OpenChoreo resources",
		Aliases: []string{"list"},
		Long: fmt.Sprintf(`List OpenChoreo resources like namespaces, projects, and components.

Examples:
  # List all namespaces
  %[1]s get namespace

  # List projects in a namespace
  %[1]s get project --namespace acme-corp

  # List components in a project
  %[1]s get component --namespace acme-corp --project online-store

  # Output namespace details in YAML format
  %[1]s get namespace -o yaml`,
			messages.DefaultCLIName),
	}

	Apply = Command{
		Use:   "apply",
		Short: "Apply OpenChoreo resources by file name",
		Long: fmt.Sprintf(`Apply a configuration file to create or update OpenChoreo resources.

	Examples:
	  # Apply a namespace configuration
	  %[1]s apply -f namespace.yaml`,
			messages.DefaultCLIName),
	}

	CreateProject = Command{
		Use:     "project",
		Aliases: []string{"proj", "projects"},
		Short:   "Create a project",
		Long: fmt.Sprintf(`Create a new project in a namespace.

Examples:
  # Create a project in a specific namespace
  %[1]s create project --namespace acme-corp --name online-store`,
			messages.DefaultCLIName),
	}

	CreateComponent = Command{
		Use:     "component",
		Aliases: []string{"comp", "components"},
		Short:   "Create a new component in a project",
		Long: fmt.Sprintf(`Create a new component in the specified project and namespace.

Examples:
  # Create a component with Git repository
  %[1]s create component --name product-catalog --namespace acme-corp --project online-store \
    --display-name "Product Catalog" --git-repository-url https://github.com/acme-corp/product-catalog --type Service

  # Create a component with build configuration
  %[1]s create component --name product-catalog --namespace acme-corp --project online-store \
    --type Service --git-repository-url https://github.com/acme-corp/product-catalog --branch main \
	--path / --docker-context ./src --dockerfile-path ./src/Dockerfile`,
			messages.DefaultCLIName),
	}

	CreateNamespace = Command{
		Use:     "namespace",
		Aliases: []string{"namespaces", "ns"},
		Short:   "Create a namespace",
		Long: fmt.Sprintf(`Create a new namespace in Choreo.

Examples:
  # Create a namespace with specific details
  %[1]s create namespace --name acme-corp --display-name "ACME" --description "ACME Corporation"`,
			messages.DefaultCLIName),
	}

	CreateWorkload = Command{
		Use:     "workload",
		Aliases: []string{"wl"},
		Short:   "Create a workload from a descriptor file",
		Long: fmt.Sprintf(`Create a workload from a workload descriptor file.

The workload descriptor (workload.yaml) should be located alongside your source code
and describes the endpoints and configuration for your workload.

Examples:
  # Create workload from descriptor
  %[1]s create workload workload.yaml --namespace acme-corp --project online-store \
    --component product-catalog --image myimage:latest

  # Create workload and save to file
  %[1]s create workload workload.yaml -o acme-corp -p online-store -c product-catalog \
    --image myimage:latest --output workload-cr.yaml`,
			messages.DefaultCLIName),
	}

	Scaffold = Command{
		Use:   "scaffold",
		Short: "Generate scaffolded resource YAML files",
		Long: fmt.Sprintf(`Generate scaffolded resource YAML files from existing CRDs.

Examples:
  # Scaffold a component from a ComponentType
  %[1]s scaffold component --name my-app --type deployment/web-app`, messages.DefaultCLIName),
	}

	ScaffoldComponent = Command{
		Use:     "component",
		Aliases: []string{"comp"},
		Short:   "Scaffold a Component YAML from ComponentType and Traits",
		Long: fmt.Sprintf(`Generate a Component YAML file based on a ComponentType definition.

The command fetches the ComponentType and any specified Traits from the cluster,
applies default values, and generates a YAML file with required fields as
placeholders and optional fields as commented examples.

The --namespace and --project flags can be omitted if set in the current context.

Examples:
  # Scaffold a basic component
  %[1]s scaffold component --name my-app --type deployment/web-app

  # Scaffold with traits
  %[1]s scaffold component --name my-app --type deployment/web-app --traits storage,ingress

  # Scaffold with workflow
  %[1]s scaffold component --name my-app --type deployment/web-app --workflow docker-build

  # Output to file
  %[1]s scaffold component --name my-app --type deployment/web-app -o my-app.yaml`, messages.DefaultCLIName),
	}

	ListNamespace = Command{
		Use:     "namespace",
		Aliases: []string{"namespaces", "ns"},
		Short:   "List namespaces",
		Long: fmt.Sprintf(`List all namespaces or get details of a specific namespace.

Examples:
  # List all namespaces
  %[1]s get namespace

  # List a specific namespace
  %[1]s get namespace acme-corp

  # Output namespace details in YAML format
  %[1]s get namespace -o yaml

  # Output specific namespace in YAML format
  %[1]s get namespace acme-corp -o yaml`,
			messages.DefaultCLIName),
	}

	ListProject = Command{
		Use:     "project",
		Aliases: []string{"proj", "projects"},
		Short:   "List projects",
		Long: fmt.Sprintf(`List all projects in a namespace or get details of a specific project.

Examples:
  # List all projects in the current namespace
  %[1]s get project

  # List all projects in a specific namespace
  %[1]s get project --namespace acme-corp

  # List a specific project
  %[1]s get project online-store --namespace acme-corp

  # Output project details in YAML format
  %[1]s get project -o yaml --namespace acme-corp

  # Output specific project in YAML format
  %[1]s get project online-store -o yaml --namespace acme-corp`,
			messages.DefaultCLIName),
	}

	ListComponent = Command{
		Use:     "component",
		Aliases: []string{"comp", "components"},
		Short:   "List components",
		Long: fmt.Sprintf(`List all components in a project or get details of a specific component.

Examples:
  # List all components in the current project
  %[1]s get component --namespace acme-corp --project online-store

  # List a specific component
  %[1]s get component product-catalog --namespace acme-corp --project online-store

  # Output component details in YAML format
  %[1]s get component -o yaml --namespace acme-corp --project online-store

  # Output specific component in YAML format
  %[1]s get component product-catalog -o yaml --namespace acme-corp --project online-store`,
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
  occ logs --type build --build product-catalog-build-01 --namespace acme-corp --project online-store \
  --component product-catalog

  # Get logs from a specific deployment
  occ logs --type deployment --deployment product-catalog-dev-01 --namespace acme-corp --project online-store \
  --component product-catalog --environment development

  # Get last 100 lines of logs from a specific build
  occ logs --type build --build product-catalog-build-01 --namespace acme-corp --project online-store \
  --component product-catalog --tail 100

  # Stream logs from a specific build
  occ logs --type build --build product-catalog-build-01 --namespace acme-corp --project online-store \
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
		Example: `  # Create a build
  occ create build --name product-catalog-build-01 --namespace acme-corp --project online-store \
    --component product-catalog --docker-context ./src --dockerfile-path ./src/Dockerfile --deployment-track main

  # Create a Buildpack build
  occ create build --name product-catalog-build-01 --namespace acme-corp --project online-store \
    --component product-catalog --buildpack-name java --buildpack-version  --deployment-track main

  # Create a build with revision and branch
  occ create build --name product-catalog-build-01 --namespace acme-corp --project online-store \
    --component product-catalog --branch main --revision abc123 --auto-build true`,
	}

	ListBuild = Command{
		Use:     "build",
		Aliases: []string{"builds"},
		Short:   "List builds",
		Long: `List all builds in the current project or namespace.
`,
		Example: `  # List all builds
  occ get build

  # List builds for a specific component
  occ get build  --namespace acme-corp --project online-store --component product-catalog

  # List builds in yaml format
  occ get build -o yaml
`,
	}
	ListDeployableArtifact = Command{
		Use:     "deployableartifact",
		Aliases: []string{"deployableartifacts"},
		Short:   "List deployable artifacts",
		Long: `List all deployable artifacts in the current project or namespace.
`,
		Example: `  # List all deployable artifacts
		  occ get deployableartifact

		  # List deployable artifacts for a specific component
		  occ get deployableartifact  --namespace acme-corp --project online-store --component product-catalog

		  # List deployable artifacts in yaml format
		  occ get deployableartifact --namespace acme-corp --project online-store --component product-catalog -o yaml
`,
	}
	ListDeployment = Command{
		Use:     "deployment",
		Aliases: []string{"deployments", "deploy"},
		Short:   "List deployments",
		Long: `List all deployments in the current project or namespace.

This command allows you to:
- List all deployments
- Filter by namespace, project, and component
- Filter by environment and deployment track
- View deployments in different output formats`,
		Example: `  # List all deployments
  occ get deployment

  # List deployments for a specific component
  occ get deployment --namespace acme-corp --project online-store --component product-catalog

  # List deployments for a specific environment
  occ get deployment --namespace acme-corp --project online-store --component product-catalog \
  --environment dev

  # List deployments for a specific deployment track
  occ get deployment --namespace acme-corp --project online-store --component product-catalog \
   --deployment-track main

  # List deployments in yaml format
  occ get deployment -o yaml --namespace acme-corp --project product-catalog

  # List details of a specific deployment
  occ get deployment product-catalog-dev-01 --namespace acme-corp --project online-store \
   --component product-catalog`,
	}

	CreateDeployment = Command{
		Use:     "deployment",
		Aliases: []string{"deployments", "deploy"},
		Short:   "Create a deployment",
		Long:    `Create a deployment in the specified namespace, project and component.`,
		Example: `  # Create a deployment with specific parameters
  occ create deployment --name product-catalog-dev-01 --namespace acme-corp --project online-store \
    --component product-catalog --environment development --deployableartifact product-catalog-artifact`,
	}

	CreateDeploymentTrack = Command{
		Use:     "deploymenttrack",
		Aliases: []string{"deptrack", "deptracks"},
		Short:   "Create a deployment track",
		Long:    `Create a deployment track in the specified namespace, project and component.`,
		Example: `  # Create a deployment track with specific parameters
  occ create deploymenttrack --name main-track --namespace acme-corp --project online-store \
    --component product-catalog --api-version v1 --auto-deploy true`,
	}

	ListDeploymentTrack = Command{
		Use:     "deploymenttrack [name]",
		Aliases: []string{"deptrack", "deptracks"},
		Short:   "List deployment tracks",
		Long:    `List deployment tracks in a namespace, project and component.`,
		Example: `  # List all deployment tracks
  occ get deploymenttrack --namespace acme-corp --project online-store --component product-catalog

  # List specific deployment track
  occ get deploymenttrack main-track --namespace acme-corp --project online-store --component product-catalog

  # Output deployment tracks in YAML format
  occ get deploymenttrack -o yaml`,
	}

	ListEnvironment = Command{
		Use:     "environment [name]",
		Aliases: []string{"env", "environments", "envs"},
		Short:   "List environments",
		Long:    `List all environments or a specific environment in a namespace.`,
		Example: `  # List all environments in a namespace
  occ get environment --namespace acme-corp

  # List a specific environment
  occ get environment development --namespace acme-corp

  # Output environments in YAML format
  occ get environment --namespace acme-corp -o yaml`,
	}

	CreateDataPlane = Command{
		Use:     "dataplane",
		Aliases: []string{"dp", "dataplanes"},
		Short:   "Create a data plane",
		Long:    `Create a data plane in the specified namespace.`,
		Example: `  # Create a data plane with specific parameters
  occ create dataplane --name primary-dataplane --namespace acme-corp --cluster-name k8s-cluster-01 \
    --connection-config kubeconfig --enable-cilium --enable-scale-to-zero --gateway-type envoy \
    --public-virtual-host api.example.com`,
	}

	ListDataPlane = Command{
		Use:     "dataplane [name]",
		Aliases: []string{"dp", "dataplanes"},
		Short:   "List data planes",
		Long:    `List all data planes or a specific data plane in a namespace.`,
		Example: `  # List all data planes
  occ get dataplane --namespace acme-corp

  # List a specific data plane
  occ get dataplane primary-dataplane --namespace acme-corp

  # Output data plane details in YAML format
  occ get dataplane --namespace acme-corp -o yaml`,
	}

	ListEndpoint = Command{
		Use:     "endpoint [name]",
		Aliases: []string{"ep", "endpoints"},
		Short:   "List endpoints",
		Long:    `List all endpoints in a namespace, project, component, and environment.`,
		Example: `  # List all endpoints
  occ get endpoint --namespace acme-corp --project online-store --component product-catalog \
  --environment dev

  # List a specific endpoint
  occ get endpoint product-ep --namespace acme-corp --project online-store --component product-catalog \
   --environment dev

  # Output endpoint details in YAML format
  occ get endpoint --namespace acme-corp --project online-store --component product-catalog \
  --environment development -o yaml`,
	}

	CreateEnvironment = Command{
		Use:     "environment",
		Aliases: []string{"env", "environments"},
		Short:   "Create an environment",
		Long:    `Create an environment in the specified namespace.`,
		Example: `  # Create a development environment
  occ create environment --name dev --namespace acme-corp --dataplane-ref primary-dataplane --dns-prefix dev

  # Create a production environment
  occ create environment --name production --namespace acme-corp --dataplane-ref primary-dataplane \
    --dns-prefix prod --production`,
	}

	CreateDeployableArtifact = Command{
		Use:     "deployableartifact",
		Aliases: []string{"da", "artifact"},
		Short:   "Create a deployable artifact",
		Long:    `Create a deployable artifact in the specified namespace, project and component.`,
		Example: `  # Create a deployable artifact from a build
  occ create deployableartifact --name product-catalog-artifact --namespace acme-corp \
    --project online-store --component product-catalog --build product-catalog-build-01

  # Create a deployable artifact from an image
  occ create deployableartifact --name product-catalog-artifact --namespace acme-corp \
    --project online-store --component product-catalog --from-image-ref product-catalog:latest`,
	}

	CreateDeploymentPipeline = Command{
		Use:     "deploymentpipeline",
		Aliases: []string{"deppipe", "deppipes", "deploymentpipelines"},
		Short:   "Create a deployment pipeline",
		Long:    `Create a deployment pipeline in the specified namespace.`,
		Example: `  # Create a deployment pipeline with specific parameters
  occ create deploymentpipeline --name dev-stage-prod --namespace acme-corp \
   --environment-order "development,staging,production"`,
	}

	ListDeploymentPipeline = Command{
		Use:     "deploymentpipeline [name]",
		Aliases: []string{"deppipe", "deppipes", "deploymentpipelines"},
		Short:   "List deployment pipelines",
		Long:    `List all deployment pipelines or a specific deployment pipeline in a namespace.`,
		Example: `  # List all deployment pipelines
  occ get deploymentpipeline --namespace acme-corp

  # List a specific deployment pipeline
  occ get deploymentpipeline default --namespace acme-corp

  # Output deployment pipeline details in YAML format
  occ get deploymentpipeline --namespace acme-corp -o yaml`,
	}

	ListConfigurationGroup = Command{
		Use:     "configurationgroup [name]",
		Aliases: []string{"cg", "configurationgroup"},
		Short:   "List configuration groups",
		Long:    `List all configuration groups or a specific configuration group in a namespace.`,
		Example: `  # List all configuration groups
  occ get configurationgroup --namespace acme-corp

  # List a specific configuration group
  occ get configurationgroup config-group-1 --namespace acme-corp

  # Output configuration group details in YAML format
  occ get configurationgroup --namespace acme-corp -o yaml`,
	}

	// ------------------------------------------------------------------------
	// Config Command Definitions
	// ------------------------------------------------------------------------

	// ConfigRoot holds usage and help texts for "config" command.
	ConfigRoot = Command{
		Use:   "config",
		Short: "Manage Choreo configuration contexts",
		Long: "Manage configuration contexts that store default values (e.g., namespace, project, component) " +
			"for occ commands.",
		Example: fmt.Sprintf(`  # List all stored configuration contexts
  %[1]s config get-contexts

  # Set or update a configuration context
  %[1]s config set-context --name acme-corp-context --namespace acme-corp

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
		Long:  "Configure a context by specifying values for namespace, project, component, build, environment, etc.",
		Example: fmt.Sprintf(`  # Set a configuration context named acme-corp-context
  %[1]s config set-context acme-corp-context --namespace acme-corp \
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

	// ConfigSetControlPlane holds usage and help texts for "config set-control-plane" command.
	ConfigSetControlPlane = Command{
		Use:   "set-control-plane",
		Short: "Configure OpenChoreo API server connection",
		Long:  "Set the OpenChoreo API server endpoint and authentication details for remote connections.",
		Example: fmt.Sprintf(`  # Set remote control plane endpoint
  %[1]s config set-control-plane --endpoint https://api.choreo.example.com --token <your-token>

  # Set local control plane (for development)
  %[1]s config set-control-plane --endpoint http://localhost:8080`, messages.DefaultCLIName),
	}

	// ------------------------------------------------------------------------
	// Component Release Commands (File-System Mode)
	// ------------------------------------------------------------------------

	ComponentReleaseRoot = Command{
		Use:   "component-release",
		Short: "Manage component releases",
		Long:  "Commands for managing component releases in file-system mode",
	}

	ComponentReleaseGenerate = Command{
		Use:   "generate",
		Short: "Generate component releases",
		Long:  "Generate ComponentRelease resources from Component, Workload, ComponentType, and Trait definitions",
		Example: fmt.Sprintf(`  # Generate releases for all components
  %[1]s component-release generate --all

  # Generate releases for all components in a specific project
  %[1]s component-release generate --project demo-project

  # Generate release for a specific component (requires --project)
  %[1]s component-release generate --project demo-project --component greeter-service

  # Dry run (preview without writing)
  %[1]s component-release generate --all --dry-run

  # Custom output path
  %[1]s component-release generate --all --output-path /custom/path`, messages.DefaultCLIName),
	}

	// ------------------------------------------------------------------------
	// Release Binding Commands (File-System Mode)
	// ------------------------------------------------------------------------

	ReleaseBindingRoot = Command{
		Use:   "release-binding",
		Short: "Manage release bindings",
		Long:  "Commands for managing release bindings in file-system mode",
	}

	ReleaseBindingGenerate = Command{
		Use:   "generate",
		Short: "Generate release bindings for components",
		Long:  "Generate ReleaseBinding resources that bind component releases to environments",
		Example: fmt.Sprintf(`  # Generate bindings for all components in development environment
  %[1]s release-binding generate --target-env development --use-pipeline default-pipeline --all

  # Generate bindings for all components in a specific project
  %[1]s release-binding generate --target-env staging --use-pipeline default-pipeline --project demo-project

  # Generate binding for a specific component
  %[1]s release-binding generate --target-env production --use-pipeline default-pipeline \
    --project demo-project --component greeter-service

  # Generate binding with explicit component release
  %[1]s release-binding generate --target-env production --use-pipeline default-pipeline \
    --project demo-project --component greeter-service --component-release greeter-service-20251222-3

  # Dry run (preview without writing)
  %[1]s release-binding generate --target-env development --use-pipeline default-pipeline --all --dry-run

  # Custom output path
  %[1]s release-binding generate --target-env development --use-pipeline default-pipeline --all \
    --output-path /custom/path`, messages.DefaultCLIName),
	}

	// ------------------------------------------------------------------------
	// Flag Descriptions (Used in config commands)
	// ------------------------------------------------------------------------

	// FlagContextNameDesc is used for the --name flag.
	FlagContextNameDesc = "Name of the configuration context to create, update, or use"

	// FlagNamespaceDesc is used for the --namespace flag.
	FlagNamespaceDesc = "Namespace name stored in this configuration context"

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
		Short: "Delete OpenChoreo resources by file names",
		Long:  "Delete resources in OpenChoreo platform such as namespaces, projects, components, etc.",
		Example: `  # Delete resources from a YAML file
  occ delete -f resources.yaml`,
	}
)
