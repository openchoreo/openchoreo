# OpenChoreo Concepts

This repository defines Choreo abstractions in the form of Kubernetes CRDs, enabling developers to use these abstractions to create projects, components, builds, deployments, and more. By leveraging these CRDs, developers can declaratively manage their application's lifecycle and infrastructure, ensuring consistency and repeatability across environments.

- **DataPlane**: Represents a Data Plane in Choreo, responsible for maintaining the health status of the data plane and providing data plane information to other resources.
- **Environment**: Represents an environment bound to a specific data plane in Choreo, with a reference to an existing `DataPlane` resource.
- **DeploymentPipeline**: Represents an ordered set of environments that a deployment will go through to reach a critical environment, with a default deployment pipeline for each organization.
- **Project**: Represents a project in Choreo, enforcing a promotion order for the components within the project, with an optional reference to a deployment pipeline.
- **Component**: Represents a deployable unit in Choreo, managing the entire lifecycle of the component from source to deployment, with various deployment architectures.
- **DeploymentTrack**: Represents a deployment path for a component, managing the deployment of the component across environments and handling auto deployment and build management.
- **Build**: Represents a source code to artifact transformation, managed by the deployment track controller, responsible for configuring build parameters and tracking build artifacts.
- **DeployableArtifact**: Represents a build artifact with environment-independent configurations, ready to be deployed to an environment, created by the build controller or manually by the user.
- **Deployment**: Represents a deployment in an environment bound to a deployment track, managing deployment revisions, deploying artifacts, and monitoring deployment status.
- **DeploymentRevision**: Represents a snapshot of the deployment resource at a given time, created by the deployment controller to track deployment history and restore deployment specs during revert operations.
- **Endpoint**: Represents an endpoint exposed by the component, responsible for updating Kubernetes resources, creating managed APIs, and configuring API settings.
- **Secret**: Represents configuration parameters stored in a key vault, used for storing both system secrets and environment-specific secrets, with various secret types like GitHub, Bitbucket, GitLab, and DockerHub.

These abstractions simplify the development and deployment process, allowing developers to focus on writing code while Choreo handles the underlying infrastructure and operational tasks.
