# OpenChoreo Samples
This directory contains sample implementations to help you understand, configure, and use OpenChoreo effectively. These samples cover different use cases, from setting up platform configurations to deploying applications in various environments.

## Categories
We have categorized the samples based on what you might want to do: 
- **[Configuring OpenChoreo](./configuring-choreo)** - Define and customize foundational platform elements such as organizations, environments, and deployment pipelines according to your organization needs.
- **[Deploying Applications](./deploying-applications)** - Deploy different types of applications (services, APIs, web apps, tasks) using various programming languages.


## Configuring OpenChoreo
When you set up OpenChoreo, certain default resources are automatically created to help you get started quickly:
- A default organization
- A default data plane
- Three default environments (Dev, Staging, Prod)
- A default deployment pipeline connecting these environments
- A default project to organize applications

OpenChoreo provides abstractions to define:
- Organizations – Manage access and group related projects.
- Environments – Set up Dev, Staging, and Prod environments.
- Data Planes – Define Kubernetes clusters for application deployments.
- Deployment Pipelines – Automate application rollouts.

For more details on these concepts, refer to the [OpenChoreo Abstractions](../docs/choreo-concepts.md) Document.

These default configurations provide a quick starting point. Once you have done some exploration you can start creating the necessary artifacts to match the needs of your organization. You can;

- Create a new organization 
- Create new environments in the new organization
- Create a new deployment pipeline that will link these new environments
- Update an existing deployment pipeline with a new environment
 

## Deploying Applications
These samples help you deploy different types of applications using OpenChoreo. All samples refer to the default setup.


### Component Types
- [Services](./deploying-applications/build-from-source/reading-list-service) – Backend services & APIs.
- [Web Applications](./deploying-applications/use-prebuilt-image/react-spa-webapp) – Frontend or full-stack applications.
- [Tasks](./deploying-applications/build-from-source/time-logger-task) – Background jobs or scheduled tasks.

### Supported Languages (via BuildPacks)
OpenChoreo abstracts the build and deployment process using BuildPacks, enabling developers to deploy applications written in:
- [Ballerina](./deploying-applications/languages/ballerina)
- [Go](./deploying-applications/languages/go)
- [Node.js](./deploying-applications/languages/node-js)
- [Python](./deploying-applications/languages/python)
- Ruby
- (More languages can be added as extensions.)

### Features
- [Configuration Management](./deploying-applications/use-prebuilt-image/github-issue-reporter-task) – Manage application configurations across environments with Configuration Groups.

### Project Creation
All the above sample components uses the default project. Follow this [sample](./deploying-applications/add-new-project) to create a new project if required.   

Note: In case you need to try these application samples in a new configuration remember to use the new resource names you created while following the "Configuring OpenChoreo" section above.
