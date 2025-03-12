# Open Source Choreo Samples
This directory contains sample implementations to help you understand, configure, and use Open Source Choreo effectively. These samples cover different use cases, from setting up platform configurations to deploying applications in various environments.

## Categories
We have categorized the samples based on what you want to do: 
- **[Configuring Open Source Choreo](./configuring-choreo)** - Define and customize foundational platform elements such as organizations, environments, and deployment pipelines according to your organization needs.
- **[Deploying Applications](./deploying-applications)** - Deploy different types of applications (services, APIs, web apps, tasks) using various programming languages.

## Configuring Open Source Choreo
These samples guide you through configuring platform resources to match your organization's requirements. Open Source Choreo provides abstractions to define:
- [Organizations](./configuring-choreo/organization) – Manage access and group related applications.
- [Environments](./configuring-choreo/environment) – Set up Dev, Staging, and Prod environments.
- [Data Planes](./configuring-choreo/dataplane) – Define Kubernetes clusters for application deployments.
- [Deployment Pipelines](./configuring-choreo/deployment-pipeline) – Automate application rollouts.

For more details on these concepts, refer to the [Open Source Choreo Abstractions](../docs/README.md) Document.

#### Default Resources
When you set up Open Source Choreo, certain default resources are automatically created to help you get started quickly:
- A default organization
- A default data plane
- Three default environments (Dev, Staging, Prod)
- A default deployment pipeline connecting these environments
- A default project to organize applications

These default configurations provide a quick starting point, but you can modify them based on your organization’s needs. The samples in this section demonstrate how to customize and extend this setup.

The following application-related samples uses the default resources created during the installation. 

## Deploying Applications
These samples help you deploy different types of applications using Open Source Choreo.

### Component Types
- [Services](./deploying-applications/build-from-source/reading-list-service) – Backend services & APIs.
- [Web Applications](./deploying-applications/prebuilt-image/react-spa-webapp) – Frontend or full-stack applications.
- [Tasks](./deploying-applications/build-from-source/time-logger-task) – Background jobs or scheduled tasks.

### Supported Languages (via BuildPacks)
Open Source Choreo abstracts the build and deployment process using BuildPacks, enabling developers to deploy applications written in:
- [Ballerina](./deploying-applications/languages/ballerina)
- [Go](./deploying-applications/languages/go)
- Node.js
- Python
- (More languages can be added as extensions.)

### Project Creation
All the above sample components uses the default project. Follow this [sample](./deploying-applications/new-project) to create project if required.   
