# Open Source Choreo Samples
This directory contains sample implementations to help you understand, configure, and use Open Source Choreo effectively. These samples cover different use cases, from setting up platform configurations to deploying applications in various environments.

## Categories
We have categorized the samples based on what you might want to do: 
- **[Configuring Open Source Choreo](./configuring-choreo)** - Define and customize foundational platform elements such as organizations, environments, and deployment pipelines according to your organization needs.
- **[Deploying Applications](./deploying-applications)** - Deploy different types of applications (services, APIs, web apps, tasks) using various programming languages.


## Configuring Open Source Choreo
When you set up Open Source Choreo, certain default resources are automatically created to help you get started quickly:
- A default organization
- A default data plane
- Three default environments (Dev, Staging, Prod)
- A default deployment pipeline connecting these environments
- A default project to organize applications

Open Source Choreo provides abstractions to define:
- Organizations – Manage access and group related [projects](https://github.com/choreo-idp/choreo/blob/main/docs/contributors/resource-kind-reference-guide.md#project).
- [Environments](https://github.com/choreo-idp/choreo/blob/main/docs/contributors/resource-kind-reference-guide.md#environment) – Set up Dev, Staging, and Prod environments.
- [Data Planes](https://github.com/choreo-idp/choreo/blob/main/docs/contributors/resource-kind-reference-guide.md#dataplane) – Define Kubernetes clusters for application deployments.
- [Deployment Pipelines](https://github.com/choreo-idp/choreo/blob/main/docs/contributors/resource-kind-reference-guide.md#deploymentpipeline) – Automate application rollouts.

For more details on these concepts, refer to the [Open Source Choreo Abstractions](../docs/contributors/resource-kind-reference-guide.md) Document.

These default configurations provide a quick starting point. Once you have done some exploration you can start creating the necessary artifacts to match the needs of your organization. You can;

- Create a new organization 
- Add a data plane to your new organization
- Add new environments to the new organization and create a deployment pipeline that will link these 
- Add a new environment to the default-organization and modify the existing deployment pipeline to include it
 

## Deploying Applications
These samples help you deploy different types of applications using Open Source Choreo. All samples refer to the default setup.


### Component Types
- [Services](./deploying-applications/build-from-source/reading-list-service) – Backend services & APIs.
- [Web Applications](./deploying-applications/prebuilt-image/react-spa-webapp) – Frontend or full-stack applications.
- [Tasks](./deploying-applications/build-from-source/time-logger-task) – Background jobs or scheduled tasks.

### Supported Languages (via BuildPacks)
Open Source Choreo abstracts the build and deployment process using BuildPacks, enabling developers to deploy applications written in:
- [Ballerina](./deploying-applications/languages/ballerina)
- [Go](./deploying-applications/languages/go)
- [Node.js](./deploying-applications/languages/node-js)
- [Python](./deploying-applications/languages/python)
- Ruby
- (More languages can be added as extensions.)

### Project Creation
All the above sample components uses the default project. Follow this [sample](./deploying-applications/new-project) to create a new project if required.   

Note: In case you need to try these application samples in a new configuration remember to use the new resource names you created while following the "Configuring Open Source Choreo" section above.