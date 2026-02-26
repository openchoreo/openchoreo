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
		Use:   "create",
		Short: "Create a workload from a descriptor file",
		Long: fmt.Sprintf(`Create a workload from a workload descriptor file.

The workload descriptor (workload.yaml) should be located alongside your source code
and describes the endpoints and configuration for your workload.

Examples:
  # Create workload from descriptor
  %[1]s workload create workload.yaml --namespace acme-corp --project online-store \
    --component product-catalog --image myimage:latest

  # Create workload and save to file
  %[1]s workload create workload.yaml -o acme-corp -p online-store -c product-catalog \
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
		Use:   "scaffold",
		Short: "Scaffold a Component YAML from ComponentType and Traits",
		Long: fmt.Sprintf(`Generate a Component YAML file based on a ComponentType definition.

The command fetches the ComponentType and any specified Traits from the cluster,
applies default values, and generates a YAML file with required fields as
placeholders and optional fields as commented examples.

The --namespace and --project flags can be omitted if set in the current context.

Examples:
  # Scaffold a basic component
  %[1]s component scaffold --name my-app --type deployment/web-app

  # Scaffold with traits
  %[1]s component scaffold --name my-app --type deployment/web-app --traits storage,ingress

  # Scaffold with workflow
  %[1]s component scaffold --name my-app --type deployment/web-app --workflow docker-build

  # Output to file
  %[1]s component scaffold --name my-app --type deployment/web-app -o my-app.yaml`, messages.DefaultCLIName),
	}

	ListNamespace = Command{
		Use:   "list",
		Short: "List namespaces",
		Long:  `List all namespaces.`,
		Example: `  # List all namespaces
  occ namespace list`,
	}

	GetNamespace = Command{
		Use:   "get [NAMESPACE_NAME]",
		Short: "Get a namespace",
		Long:  `Get a namespace and display its details in YAML format.`,
		Example: `  # Get a namespace
  occ namespace get acme-corp`,
	}

	DeleteNamespace = Command{
		Use:   "delete [NAMESPACE_NAME]",
		Short: "Delete a namespace",
		Long:  `Delete a namespace by name.`,
		Example: `  # Delete a namespace
  occ namespace delete acme-corp`,
	}

	ListProject = Command{
		Use:   "list",
		Short: "List projects",
		Long:  `List all projects in a namespace.`,
		Example: `  # List all projects in a namespace
  occ project list --namespace acme-corp`,
	}

	GetProject = Command{
		Use:   "get [PROJECT_NAME]",
		Short: "Get a project",
		Long:  `Get a project and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get a project
  %[1]s project get my-project --namespace acme-corp`, messages.DefaultCLIName),
	}

	DeleteProject = Command{
		Use:   "delete [PROJECT_NAME]",
		Short: "Delete a project",
		Long:  `Delete a project by name.`,
		Example: fmt.Sprintf(`  # Delete a project
  %[1]s project delete my-project --namespace acme-corp`, messages.DefaultCLIName),
	}

	ListComponent = Command{
		Use:   "list",
		Short: "List components",
		Long:  `List all components in a project.`,
		Example: `  # List all components in a project
  occ component list --namespace acme-corp --project online-store`,
	}

	GetComponent = Command{
		Use:   "get [COMPONENT_NAME]",
		Short: "Get a component",
		Long:  `Get a component and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get a component
  %[1]s component get my-component --namespace acme-corp`, messages.DefaultCLIName),
	}

	DeleteComponent = Command{
		Use:   "delete [COMPONENT_NAME]",
		Short: "Delete a component",
		Long:  `Delete a component by name.`,
		Example: fmt.Sprintf(`  # Delete a component
  %[1]s component delete my-component --namespace acme-corp`, messages.DefaultCLIName),
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
		Use:   "list",
		Short: "List environments",
		Long:  `List all environments in a namespace.`,
		Example: `  # List all environments in a namespace
  occ environment list --namespace acme-corp`,
	}

	GetEnvironment = Command{
		Use:   "get [ENVIRONMENT_NAME]",
		Short: "Get an environment",
		Long:  `Get an environment and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get an environment
  %[1]s environment get dev --namespace acme-corp`, messages.DefaultCLIName),
	}

	DeleteEnvironment = Command{
		Use:   "delete [ENVIRONMENT_NAME]",
		Short: "Delete an environment",
		Long:  `Delete an environment by name.`,
		Example: fmt.Sprintf(`  # Delete an environment
  %[1]s environment delete dev --namespace acme-corp`, messages.DefaultCLIName),
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
		Use:   "list",
		Short: "List data planes",
		Long:  `List all data planes in a namespace.`,
		Example: `  # List all data planes in a namespace
  occ dataplane list --namespace acme-corp`,
	}

	GetDataPlane = Command{
		Use:   "get [DATAPLANE_NAME]",
		Short: "Get a data plane",
		Long:  `Get a data plane and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get a data plane
  %[1]s dataplane get primary-dataplane --namespace acme-corp`, messages.DefaultCLIName),
	}

	DeleteDataPlane = Command{
		Use:   "delete [DATAPLANE_NAME]",
		Short: "Delete a data plane",
		Long:  `Delete a data plane by name.`,
		Example: fmt.Sprintf(`  # Delete a data plane
  %[1]s dataplane delete primary-dataplane --namespace acme-corp`, messages.DefaultCLIName),
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

	DeploymentPipelineRoot = Command{
		Use:     "deploymentpipeline",
		Aliases: []string{"deppipe", "deppipes", "deploymentpipelines"},
		Short:   "Manage deployment pipelines",
		Long:    `Manage deployment pipelines for OpenChoreo.`,
	}

	ListDeploymentPipelineDirect = Command{
		Use:   "list",
		Short: "List deployment pipelines",
		Long:  `List all deployment pipelines in a namespace.`,
		Example: `  # List all deployment pipelines in a namespace
  occ deploymentpipeline list --namespace acme-corp`,
	}

	GetDeploymentPipeline = Command{
		Use:   "get [DEPLOYMENT_PIPELINE_NAME]",
		Short: "Get a deployment pipeline",
		Long:  `Get a deployment pipeline and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get a deployment pipeline
  %[1]s deploymentpipeline get my-pipeline --namespace acme-corp`, messages.DefaultCLIName),
	}

	DeleteDeploymentPipeline = Command{
		Use:   "delete [DEPLOYMENT_PIPELINE_NAME]",
		Short: "Delete a deployment pipeline",
		Long:  `Delete a deployment pipeline by name.`,
		Example: fmt.Sprintf(`  # Delete a deployment pipeline
  %[1]s deploymentpipeline delete my-pipeline --namespace acme-corp`, messages.DefaultCLIName),
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

	ListBuildPlane = Command{
		Use:   "list",
		Short: "List build planes",
		Long:  `List all build planes in a namespace.`,
		Example: `  # List all build planes in a namespace
  occ buildplane list --namespace acme-corp`,
	}

	GetBuildPlane = Command{
		Use:   "get [BUILDPLANE_NAME]",
		Short: "Get a build plane",
		Long:  `Get a build plane and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get a build plane
  %[1]s buildplane get primary-buildplane --namespace acme-corp`, messages.DefaultCLIName),
	}

	DeleteBuildPlane = Command{
		Use:   "delete [BUILDPLANE_NAME]",
		Short: "Delete a build plane",
		Long:  `Delete a build plane by name.`,
		Example: fmt.Sprintf(`  # Delete a build plane
  %[1]s buildplane delete primary-buildplane --namespace acme-corp`, messages.DefaultCLIName),
	}

	ListObservabilityPlane = Command{
		Use:   "list",
		Short: "List observability planes",
		Long:  `List all observability planes in a namespace.`,
		Example: `  # List all observability planes in a namespace
  occ observabilityplane list --namespace acme-corp`,
	}

	GetObservabilityPlane = Command{
		Use:   "get [OBSERVABILITYPLANE_NAME]",
		Short: "Get an observability plane",
		Long:  `Get an observability plane and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get an observability plane
  %[1]s observabilityplane get primary-observabilityplane --namespace acme-corp`, messages.DefaultCLIName),
	}

	DeleteObservabilityPlane = Command{
		Use:   "delete [OBSERVABILITYPLANE_NAME]",
		Short: "Delete an observability plane",
		Long:  `Delete an observability plane by name.`,
		Example: fmt.Sprintf(`  # Delete an observability plane
  %[1]s observabilityplane delete primary-observabilityplane --namespace acme-corp`, messages.DefaultCLIName),
	}

	ListComponentType = Command{
		Use:   "list",
		Short: "List component types",
		Long:  `List all component types available in a namespace.`,
		Example: `  # List all component types in a namespace
  occ componenttype list --namespace acme-corp`,
	}

	GetComponentType = Command{
		Use:   "get [COMPONENT_TYPE_NAME]",
		Short: "Get a component type",
		Long:  `Get a component type and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get a component type
  %[1]s componenttype get web-app --namespace acme-corp`, messages.DefaultCLIName),
	}

	DeleteComponentType = Command{
		Use:   "delete [COMPONENT_TYPE_NAME]",
		Short: "Delete a component type",
		Long:  `Delete a component type by name.`,
		Example: fmt.Sprintf(`  # Delete a component type
  %[1]s componenttype delete web-app --namespace acme-corp`, messages.DefaultCLIName),
	}

	ListTrait = Command{
		Use:   "list",
		Short: "List traits",
		Long:  `List all traits available in a namespace.`,
		Example: `  # List all traits in a namespace
  occ trait list --namespace acme-corp`,
	}

	GetTrait = Command{
		Use:   "get [TRAIT_NAME]",
		Short: "Get a trait",
		Long:  `Get a trait and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get a trait
  %[1]s trait get ingress --namespace acme-corp`, messages.DefaultCLIName),
	}

	DeleteTrait = Command{
		Use:   "delete [TRAIT_NAME]",
		Short: "Delete a trait",
		Long:  `Delete a trait by name.`,
		Example: fmt.Sprintf(`  # Delete a trait
  %[1]s trait delete ingress --namespace acme-corp`, messages.DefaultCLIName),
	}

	ClusterComponentType = Command{
		Use:     "clustercomponenttype",
		Aliases: []string{"cct", "clustercomponenttypes"},
		Short:   "Manage cluster component types",
		Long:    `Manage cluster-scoped component types for OpenChoreo.`,
	}

	ListClusterComponentType = Command{
		Use:   "list",
		Short: "List cluster component types",
		Long:  `List all cluster-scoped component types available across the cluster.`,
		Example: `  # List all cluster component types
  occ clustercomponenttype list`,
	}

	GetClusterComponentType = Command{
		Use:   "get [CLUSTER_COMPONENT_TYPE_NAME]",
		Short: "Get a cluster component type",
		Long:  `Get a cluster component type and display its details in YAML format.`,
		Example: `  # Get a cluster component type
  occ clustercomponenttype get web-app`,
	}

	DeleteClusterComponentType = Command{
		Use:   "delete [CLUSTER_COMPONENT_TYPE_NAME]",
		Short: "Delete a cluster component type",
		Long:  `Delete a cluster component type by name.`,
		Example: `  # Delete a cluster component type
  occ clustercomponenttype delete web-app`,
	}

	ClusterTrait = Command{
		Use:     "clustertrait",
		Aliases: []string{"clustertraits"},
		Short:   "Manage cluster traits",
		Long:    `Manage cluster-scoped traits for OpenChoreo.`,
	}

	ListClusterTrait = Command{
		Use:   "list",
		Short: "List cluster traits",
		Long:  `List all cluster-scoped traits available across the cluster.`,
		Example: `  # List all cluster traits
  occ clustertrait list`,
	}

	GetClusterTrait = Command{
		Use:   "get [CLUSTER_TRAIT_NAME]",
		Short: "Get a cluster trait",
		Long:  `Get a cluster trait and display its details in YAML format.`,
		Example: `  # Get a cluster trait
  occ clustertrait get ingress`,
	}

	DeleteClusterTrait = Command{
		Use:   "delete [CLUSTER_TRAIT_NAME]",
		Short: "Delete a cluster trait",
		Long:  `Delete a cluster trait by name.`,
		Example: `  # Delete a cluster trait
  occ clustertrait delete ingress`,
	}

	ListWorkflow = Command{
		Use:   "list",
		Short: "List workflows",
		Long:  `List all workflows available in a namespace.`,
		Example: `  # List all workflows in a namespace
  occ workflow list --namespace acme-corp`,
	}

	StartWorkflow = Command{
		Use:   "start WORKFLOW_NAME",
		Short: "Start a workflow run",
		Long:  `Start a new workflow run with optional parameters.`,
		Example: `  # Start a workflow
  occ workflow start database-migration --namespace acme-corp

  # Start with parameters
  occ workflow start migration --namespace acme --set version=v2 --set dry_run=false`,
	}

	ListSecretReference = Command{
		Use:   "list",
		Short: "List secret references",
		Long:  `List all secret references in a namespace.`,
		Example: `  # List all secret references in a namespace
  occ secretreference list --namespace acme-corp`,
	}

	GetSecretReference = Command{
		Use:   "get [SECRET_REFERENCE_NAME]",
		Short: "Get a secret reference",
		Long:  `Get a secret reference and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get a secret reference
  %[1]s secretreference get my-secret --namespace acme-corp`, messages.DefaultCLIName),
	}

	DeleteSecretReference = Command{
		Use:   "delete [SECRET_REFERENCE_NAME]",
		Short: "Delete a secret reference",
		Long:  `Delete a secret reference by name.`,
		Example: fmt.Sprintf(`  # Delete a secret reference
  %[1]s secretreference delete my-secret --namespace acme-corp`, messages.DefaultCLIName),
	}

	AuthzClusterRole = Command{
		Use:     "authzclusterrole",
		Aliases: []string{"authzclusterroles", "cr"},
		Short:   "Manage authz cluster roles",
		Long:    `Manage cluster-scoped authorization roles for OpenChoreo.`,
	}

	ListAuthzClusterRole = Command{
		Use:   "list",
		Short: "List authz cluster roles",
		Long:  `List all cluster-scoped authorization roles.`,
		Example: `  # List all authz cluster roles
  occ authzclusterrole list`,
	}

	GetAuthzClusterRole = Command{
		Use:   "get [NAME]",
		Short: "Get an authz cluster role",
		Long:  `Get an authz cluster role and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get an authz cluster role
  %[1]s authzclusterrole get my-role`, messages.DefaultCLIName),
	}

	DeleteAuthzClusterRole = Command{
		Use:   "delete [NAME]",
		Short: "Delete an authz cluster role",
		Long:  `Delete a cluster-scoped authorization role by name.`,
		Example: fmt.Sprintf(`  # Delete an authz cluster role
  %[1]s authzclusterrole delete my-role`, messages.DefaultCLIName),
	}

	AuthzClusterRoleBinding = Command{
		Use:     "authzclusterrolebinding",
		Aliases: []string{"authzclusterrolebindings", "crb"},
		Short:   "Manage authz cluster role bindings",
		Long:    `Manage cluster-scoped authorization role bindings for OpenChoreo.`,
	}

	ListAuthzClusterRoleBinding = Command{
		Use:   "list",
		Short: "List authz cluster role bindings",
		Long:  `List all cluster-scoped authorization role bindings.`,
		Example: `  # List all authz cluster role bindings
  occ authzclusterrolebinding list`,
	}

	GetAuthzClusterRoleBinding = Command{
		Use:   "get [NAME]",
		Short: "Get an authz cluster role binding",
		Long:  `Get an authz cluster role binding and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get an authz cluster role binding
  %[1]s authzclusterrolebinding get my-binding`, messages.DefaultCLIName),
	}

	DeleteAuthzClusterRoleBinding = Command{
		Use:   "delete [NAME]",
		Short: "Delete an authz cluster role binding",
		Long:  `Delete a cluster-scoped authorization role binding by name.`,
		Example: fmt.Sprintf(`  # Delete an authz cluster role binding
  %[1]s authzclusterrolebinding delete my-binding`, messages.DefaultCLIName),
	}

	AuthzRole = Command{
		Use:     "authzrole",
		Aliases: []string{"authzroles"},
		Short:   "Manage authz roles",
		Long:    `Manage namespace-scoped authorization roles for OpenChoreo.`,
	}

	ListAuthzRole = Command{
		Use:   "list",
		Short: "List authz roles",
		Long:  `List all authz roles in a namespace.`,
		Example: `  # List all authz roles in a namespace
  occ authzrole list --namespace acme-corp`,
	}

	GetAuthzRole = Command{
		Use:   "get [NAME]",
		Short: "Get an authz role",
		Long:  `Get an authz role and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get an authz role
  %[1]s authzrole get my-role --namespace acme-corp`, messages.DefaultCLIName),
	}

	DeleteAuthzRole = Command{
		Use:   "delete [NAME]",
		Short: "Delete an authz role",
		Long:  `Delete an authorization role by name.`,
		Example: fmt.Sprintf(`  # Delete an authz role
  %[1]s authzrole delete my-role --namespace acme-corp`, messages.DefaultCLIName),
	}

	AuthzRoleBinding = Command{
		Use:     "authzrolebinding",
		Aliases: []string{"authzrolebindings", "rb"},
		Short:   "Manage authz role bindings",
		Long:    `Manage namespace-scoped authorization role bindings for OpenChoreo.`,
	}

	ListAuthzRoleBinding = Command{
		Use:   "list",
		Short: "List authz role bindings",
		Long:  `List all authz role bindings in a namespace.`,
		Example: `  # List all authz role bindings in a namespace
  occ authzrolebinding list --namespace acme-corp`,
	}

	GetAuthzRoleBinding = Command{
		Use:   "get [NAME]",
		Short: "Get an authz role binding",
		Long:  `Get an authz role binding and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get an authz role binding
  %[1]s authzrolebinding get my-binding --namespace acme-corp`, messages.DefaultCLIName),
	}

	DeleteAuthzRoleBinding = Command{
		Use:   "delete [NAME]",
		Short: "Delete an authz role binding",
		Long:  `Delete an authorization role binding by name.`,
		Example: fmt.Sprintf(`  # Delete an authz role binding
  %[1]s authzrolebinding delete my-binding --namespace acme-corp`, messages.DefaultCLIName),
	}

	// Resource root commands

	BuildPlane = Command{
		Use:     "buildplane",
		Aliases: []string{"bp", "buildplanes"},
		Short:   "Manage build planes",
		Long:    `Manage build planes for OpenChoreo.`,
	}

	ObservabilityPlane = Command{
		Use:     "observabilityplane",
		Aliases: []string{"op", "observabilityplanes"},
		Short:   "Manage observability planes",
		Long:    `Manage observability planes for OpenChoreo.`,
	}

	ComponentType = Command{
		Use:     "componenttype",
		Aliases: []string{"ct", "componenttypes"},
		Short:   "Manage component types",
		Long:    `Manage component types for OpenChoreo.`,
	}

	Trait = Command{
		Use:     "trait",
		Aliases: []string{"traits"},
		Short:   "Manage traits",
		Long:    `Manage traits for OpenChoreo.`,
	}

	Workflow = Command{
		Use:     "workflow",
		Aliases: []string{"wf", "workflows"},
		Short:   "Manage workflows",
		Long:    `Manage workflows for OpenChoreo.`,
	}

	SecretReference = Command{
		Use:     "secretreference",
		Aliases: []string{"sr", "secretreferences", "secret-ref"},
		Short:   "Manage secret references",
		Long:    `Manage secret references for OpenChoreo.`,
	}

	Namespace = Command{
		Use:     "namespace",
		Aliases: []string{"ns", "namespaces"},
		Short:   "Manage namespaces",
		Long:    `Manage namespaces for OpenChoreo.`,
	}

	Project = Command{
		Use:     "project",
		Aliases: []string{"proj", "projects"},
		Short:   "Manage projects",
		Long:    `Manage projects for OpenChoreo.`,
	}

	Component = Command{
		Use:     "component",
		Aliases: []string{"comp", "components"},
		Short:   "Manage components",
		Long:    `Manage components for OpenChoreo.`,
	}

	DeployComponent = Command{
		Use:   "deploy [COMPONENT_NAME]",
		Short: "Deploy or promote a component",
		Long:  "Deploy a component release to the root environment or promote to the next environment in the pipeline.",
		Example: fmt.Sprintf(`  # Deploy latest release to root environment
  %[1]s component deploy api-service --namespace acme-corp --project online-store

  # Deploy specific release
  %[1]s component deploy api-service --release api-service-20260126-143022-1

  # Promote to next environment
  %[1]s component deploy api-service --to staging

  # Deploy with overrides
  %[1]s component deploy api-service --set componentTypeEnvOverrides.replicas=3`, messages.DefaultCLIName),
	}

	Environment = Command{
		Use:     "environment",
		Aliases: []string{"env", "environments", "envs"},
		Short:   "Manage environments",
		Long:    `Manage environments for OpenChoreo.`,
	}

	DataPlane = Command{
		Use:     "dataplane",
		Aliases: []string{"dp", "dataplanes"},
		Short:   "Manage data planes",
		Long:    `Manage data planes for OpenChoreo.`,
	}

	Workload = Command{
		Use:     "workload",
		Aliases: []string{"wl", "workloads"},
		Short:   "Manage workloads",
		Long:    `Manage workloads for OpenChoreo.`,
	}

	ListWorkload = Command{
		Use:   "list",
		Short: "List workloads",
		Long:  `List all workloads in a namespace.`,
		Example: `  # List all workloads in a namespace
  occ workload list --namespace acme-corp`,
	}

	GetWorkload = Command{
		Use:   "get [WORKLOAD_NAME]",
		Short: "Get a workload",
		Long:  `Get a workload and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get a workload
  %[1]s workload get my-workload --namespace acme-corp`, messages.DefaultCLIName),
	}

	DeleteWorkload = Command{
		Use:   "delete [WORKLOAD_NAME]",
		Short: "Delete a workload",
		Long:  `Delete a workload by name.`,
		Example: fmt.Sprintf(`  # Delete a workload
  %[1]s workload delete my-workload --namespace acme-corp`, messages.DefaultCLIName),
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
		Example: fmt.Sprintf(`  # Add a new configuration context
  %[1]s config context add my-context --controlplane cp1 --credentials cred1

  # List all configuration contexts
  %[1]s config context list

  # Switch to a configuration context
  %[1]s config context use my-context

  # Update a configuration context
  %[1]s config context update my-context --namespace acme-corp --project online-store`, messages.DefaultCLIName),
	}

	// ConfigContext holds usage and help texts for the "config context" parent command.
	ConfigContext = Command{
		Use:   "context",
		Short: "Manage configuration contexts",
		Long:  "Manage configuration contexts that store default values for occ commands.",
	}

	// ConfigContextAdd holds usage and help texts for "config context add" command.
	ConfigContextAdd = Command{
		Use:   "add",
		Short: "Add a new configuration context",
		Long:  "Add a new configuration context with control plane and credentials references.",
		Example: fmt.Sprintf(`  # Add a new context with control plane and credentials
  %[1]s config context add my-context --controlplane cp1 --credentials cred1 --namespace acme-corp`,
			messages.DefaultCLIName),
	}

	// ConfigContextList holds usage and help texts for "config context list" command.
	ConfigContextList = Command{
		Use:   "list",
		Short: "List all configuration contexts",
		Long:  "List all stored configuration contexts, with an asterisk (*) marking the currently active context.",
		Example: fmt.Sprintf(`  # Show all configuration contexts
  %[1]s config context list`, messages.DefaultCLIName),
	}

	// ConfigContextDelete holds usage and help texts for "config context delete" command.
	ConfigContextDelete = Command{
		Use:   "delete",
		Short: "Delete a configuration context",
		Long:  "Delete a configuration context by name.",
		Example: fmt.Sprintf(`  # Delete a configuration context
  %[1]s config context delete my-context`, messages.DefaultCLIName),
	}

	// ConfigContextUpdate holds usage and help texts for "config context update" command.
	ConfigContextUpdate = Command{
		Use:   "update",
		Short: "Update an existing configuration context",
		Long:  "Update fields of an existing configuration context.",
		Example: fmt.Sprintf(`  # Update the namespace and project for a context
  %[1]s config context update my-context --namespace acme-corp --project online-store`,
			messages.DefaultCLIName),
	}

	// ConfigContextUse holds usage and help texts for "config context use" command.
	ConfigContextUse = Command{
		Use:   "use",
		Short: "Switch to a configuration context",
		Long:  "Set the active context, so subsequent commands automatically use its stored values when flags are omitted.",
		Example: fmt.Sprintf(`  # Switch to the configuration context named my-context
  %[1]s config context use my-context`, messages.DefaultCLIName),
	}

	// ConfigControlPlane holds usage and help texts for the "config controlplane" parent command.
	ConfigControlPlane = Command{
		Use:   "controlplane",
		Short: "Manage control plane configurations",
		Long:  "Manage control plane configurations for OpenChoreo API server connections.",
	}

	// ConfigControlPlaneAdd holds usage and help texts for "config controlplane add" command.
	ConfigControlPlaneAdd = Command{
		Use:   "add",
		Short: "Add a new control plane configuration",
		Long:  "Add a new control plane configuration with a URL.",
		Example: fmt.Sprintf(`  # Add a control plane
  %[1]s config controlplane add my-cp --url https://cp.openchoreo.acme.com`, messages.DefaultCLIName),
	}

	// ConfigControlPlaneList holds usage and help texts for "config controlplane list" command.
	ConfigControlPlaneList = Command{
		Use:   "list",
		Short: "List all control plane configurations",
		Long:  "List all stored control plane configurations.",
		Example: fmt.Sprintf(`  # List all control planes
  %[1]s config controlplane list`, messages.DefaultCLIName),
	}

	// ConfigControlPlaneUpdate holds usage and help texts for "config controlplane update" command.
	ConfigControlPlaneUpdate = Command{
		Use:   "update",
		Short: "Update a control plane configuration",
		Long:  "Update the URL of an existing control plane configuration.",
		Example: fmt.Sprintf(`  # Update a control plane URL
  %[1]s config controlplane update my-cp --url https://new-cp.openchoreo.acme.com`, messages.DefaultCLIName),
	}

	// ConfigControlPlaneDelete holds usage and help texts for "config controlplane delete" command.
	ConfigControlPlaneDelete = Command{
		Use:   "delete",
		Short: "Delete a control plane configuration",
		Long:  "Delete a control plane configuration by name.",
		Example: fmt.Sprintf(`  # Delete a control plane
  %[1]s config controlplane delete my-cp`, messages.DefaultCLIName),
	}

	// ConfigCredentials holds usage and help texts for the "config credentials" parent command.
	ConfigCredentials = Command{
		Use:   "credentials",
		Short: "Manage credentials configurations",
		Long:  "Manage credentials configurations for authentication.",
	}

	// ConfigCredentialsAdd holds usage and help texts for "config credentials add" command.
	ConfigCredentialsAdd = Command{
		Use:   "add",
		Short: "Add a new credentials configuration",
		Long:  "Add a new credentials configuration entry.",
		Example: fmt.Sprintf(`  # Add credentials
  %[1]s config credentials add my-creds`, messages.DefaultCLIName),
	}

	// ConfigCredentialsList holds usage and help texts for "config credentials list" command.
	ConfigCredentialsList = Command{
		Use:   "list",
		Short: "List all credentials configurations",
		Long:  "List all stored credentials configurations.",
		Example: fmt.Sprintf(`  # List all credentials
  %[1]s config credentials list`, messages.DefaultCLIName),
	}

	// ConfigCredentialsDelete holds usage and help texts for "config credentials delete" command.
	ConfigCredentialsDelete = Command{
		Use:   "delete",
		Short: "Delete a credentials configuration",
		Long:  "Delete a credentials configuration by name.",
		Example: fmt.Sprintf(`  # Delete credentials
  %[1]s config credentials delete my-creds`, messages.DefaultCLIName),
	}

	// ------------------------------------------------------------------------
	// Component Release Commands (File-System Mode)
	// ------------------------------------------------------------------------

	ComponentReleaseRoot = Command{
		Use:     "componentrelease",
		Aliases: []string{"component-release"},
		Short:   "Manage component releases",
		Long:    "Commands for managing component releases in file-system mode",
	}

	ComponentReleaseGenerate = Command{
		Use:   "generate",
		Short: "Generate component releases",
		Long:  "Generate ComponentRelease resources from Component, Workload, ComponentType, and Trait definitions",
		Example: fmt.Sprintf(`  # Generate releases for all components
  %[1]s componentrelease generate --all

  # Generate releases for all components in a specific project
  %[1]s componentrelease generate --project demo-project

  # Generate release for a specific component (requires --project)
  %[1]s componentrelease generate --project demo-project --component greeter-service

  # Dry run (preview without writing)
  %[1]s componentrelease generate --all --dry-run

  # Custom output path
  %[1]s componentrelease generate --all --output-path /custom/path`, messages.DefaultCLIName),
	}

	// ------------------------------------------------------------------------
	// Release Binding Commands (File-System Mode)
	// ------------------------------------------------------------------------

	ReleaseBindingRoot = Command{
		Use:     "releasebinding",
		Aliases: []string{"release-binding"},
		Short:   "Manage release bindings",
		Long:    "Commands for managing release bindings in file-system mode",
	}

	ReleaseBindingGenerate = Command{
		Use:   "generate",
		Short: "Generate release bindings for components",
		Long:  "Generate ReleaseBinding resources that bind component releases to environments",
		Example: fmt.Sprintf(`  # Generate bindings for all components in development environment
  %[1]s releasebinding generate --target-env development --use-pipeline default-pipeline --all

  # Generate bindings for all components in a specific project
  %[1]s releasebinding generate --target-env staging --use-pipeline default-pipeline --project demo-project

  # Generate binding for a specific component
  %[1]s releasebinding generate --target-env production --use-pipeline default-pipeline \
    --project demo-project --component greeter-service

  # Generate binding with explicit component release
  %[1]s releasebinding generate --target-env production --use-pipeline default-pipeline \
    --project demo-project --component greeter-service --component-release greeter-service-20251222-3

  # Dry run (preview without writing)
  %[1]s releasebinding generate --target-env development --use-pipeline default-pipeline --all --dry-run

  # Custom output path
  %[1]s releasebinding generate --target-env development --use-pipeline default-pipeline --all \
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

	ListComponentRelease = Command{
		Use:   "list",
		Short: "List component releases",
		Long:  `List all component releases for a specific component.`,
		Example: `  # List all component releases for a component
  occ componentrelease list --namespace acme-corp --project online-store --component product-catalog`,
	}

	GetComponentRelease = Command{
		Use:   "get [COMPONENT_RELEASE_NAME]",
		Short: "Get a component release",
		Long:  `Get a component release and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get a component release
  %[1]s componentrelease get my-release --namespace acme-corp`, messages.DefaultCLIName),
	}

	ListReleaseBinding = Command{
		Use:   "list",
		Short: "List release bindings",
		Long:  `List all release bindings for a specific component.`,
		Example: `  # List all release bindings for a component
  occ releasebinding list --namespace acme-corp --project online-store --component product-catalog`,
	}

	GetReleaseBinding = Command{
		Use:   "get [RELEASE_BINDING_NAME]",
		Short: "Get a release binding",
		Long:  `Get a release binding and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get a release binding
  %[1]s releasebinding get my-binding --namespace acme-corp`, messages.DefaultCLIName),
	}

	DeleteReleaseBinding = Command{
		Use:   "delete [RELEASE_BINDING_NAME]",
		Short: "Delete a release binding",
		Long:  `Delete a release binding by name.`,
		Example: fmt.Sprintf(`  # Delete a release binding
  %[1]s releasebinding delete my-binding --namespace acme-corp`, messages.DefaultCLIName),
	}

	// Observability Alerts Notification Channel commands

	ObservabilityAlertsNotificationChannel = Command{
		Use:     "observabilityalertsnotificationchannel",
		Aliases: []string{"oanc", "obsnotifchannel"},
		Short:   "Manage observability alerts notification channels",
		Long:    `Manage observability alerts notification channels for OpenChoreo.`,
	}

	ListObservabilityAlertsNotificationChannel = Command{
		Use:   "list",
		Short: "List observability alerts notification channels",
		Long:  `List all observability alerts notification channels in a namespace.`,
		Example: `  # List all observability alerts notification channels
  occ observabilityalertsnotificationchannel list --namespace acme-corp`,
	}

	GetObservabilityAlertsNotificationChannel = Command{
		Use:   "get [CHANNEL_NAME]",
		Short: "Get an observability alerts notification channel",
		Long:  `Get an observability alerts notification channel and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get an observability alerts notification channel
  %[1]s observabilityalertsnotificationchannel get my-channel --namespace acme-corp`, messages.DefaultCLIName),
	}

	DeleteObservabilityAlertsNotificationChannel = Command{
		Use:   "delete [CHANNEL_NAME]",
		Short: "Delete an observability alerts notification channel",
		Long:  `Delete an observability alerts notification channel by name.`,
		Example: fmt.Sprintf(`  # Delete an observability alerts notification channel
  %[1]s observabilityalertsnotificationchannel delete my-channel --namespace acme-corp`, messages.DefaultCLIName),
	}

	ListWorkflowRun = Command{
		Use:   "list",
		Short: "List workflow runs",
		Long:  `List all workflow runs in a namespace.`,
		Example: `  # List all workflow runs in a namespace
  occ workflowrun list --namespace acme-corp`,
	}

	GetWorkflowRun = Command{
		Use:   "get [WORKFLOW_RUN_NAME]",
		Short: "Get a workflow run",
		Long:  `Get a workflow run and display its details in YAML format.`,
		Example: fmt.Sprintf(`  # Get a workflow run
  %[1]s workflowrun get my-run --namespace acme-corp`, messages.DefaultCLIName),
	}

	WorkflowRun = Command{
		Use:     "workflowrun",
		Aliases: []string{"wr", "workflowruns"},
		Short:   "Manage workflow runs",
		Long:    `Manage workflow runs for OpenChoreo.`,
	}
)
