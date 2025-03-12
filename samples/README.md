# Open Source Choreo Samples
This directory contains sample implementations to help you understand, configure, and use Open Source Choreo effectively. These samples cover different use cases, from setting up platform configurations to deploying applications in various environments.

## Categories
We have categorized the samples based on what you want to do: 
- **Setting Up & Configuring Open Source Choreo** - Define and customize foundational platform elements such as organizations, environments, and deployment pipelines according to your organization needs.
- **Deploying Applications** - Deploy different types of applications (services, APIs, web apps, tasks) using various programming languages.

## Setting Up & Configuring Open Source Choreo
These samples guide you through configuring platform resources to match your organization's requirements. Open Source Choreo provides abstractions to define:
- Organizations – Manage access and group related applications.
- Environments – Set up Dev, Staging, and Prod environments.
- Data Planes – Define Kubernetes clusters for application deployments.
- Deployment Pipelines – Automate application rollouts.

For more details on these concepts, refer to the [Open Source Choreo Abstractions](https://github.com/choreo-idp/choreo/tree/main/docs) Document.

#### Default Resources
When you set up Open Source Choreo, certain default resources are automatically created to help you get started quickly:
- A default organization
- A default data plane
- Three default environments (Dev, Staging, Prod)
- A default deployment pipeline connecting these environments
- A default project to organize applications

These default configurations provide a quick starting point, but you can modify them based on your organization’s needs. The samples in this section demonstrate how to customize and extend this setup.

## Deploying Applications
These samples help you deploy different types of applications using Open Source Choreo.

### Component Types
- Services – Backend services & APIs.
- Web Applications – Frontend or full-stack applications.
- Tasks – Background jobs or scheduled tasks.

### Supported Languages (via BuildPacks)
Open Source Choreo abstracts the build and deployment process using BuildPacks, enabling developers to deploy applications written in:
- Ballerina
- Go
- Node.js
- Python
- (More languages can be added as extensions.)

Each sample demonstrates how to package, deploy, and manage applications using Open Source Choreo.

## How to Use These Samples
Each sample includes:
- Description of the sample use case
- Source code link
- Required Custom Resources 
- Instructions on running the sample 
- Commands and instructions to verify the sample

To run a sample:
1. Navigate to the specific sample directory
2. Read the sample-specific README.
3. Follow the step-by-step setup instructions. 


## Available Samples
#### Setting Up & Configuring Open Source Choreo
- [Create a new organization](https://github.com/choreo-idp/choreo/tree/samples/samples/platform-configuration/organization)
- [Add a new environment to an organization](https://github.com/choreo-idp/choreo/tree/samples/samples/platform-configuration/environment)

#### Deploying Applications
- [Deploy a Ballerina Service](./samples/applications/languages/ballerina)
- [Deploy a Go Service](./samples/applications/languages/go)