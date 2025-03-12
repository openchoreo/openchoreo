# Open Source Choreo IDP Samples

This directory contains some sample implementations to help you understand, get started and utilize Open Source Choreo IDP effectively.

## Categories
We have broken down these samples to represent the tasks for each user category. 
- **Platform Configuration** - Targetted at Platform Engineers who would want to customize the Open Source Choreo IDP according to an organization's needs.
- **Developer Applications** - For developers who would want to deploy different types of applications (web applications, services and tasks) in different languages.

## Platform Configuration Samples
These samples provide the required configurations for setting up Choreo platform resources. These are the Choreo abstractions that help you configure foundational elements like data planes, deployment-pipelines, environments, and projects to match what is required in your organization.

For more detailed information on the Choreo abstractions, refer to the [Open Source Choreo Abstractions Document](https://github.com/choreo-idp/choreo/tree/main/docs).

#### Default Resources
When you set up Open Source Choreo, some default resources are automatically created in order to get you started fast. On a new Open Source Choreo setup you would already have a default organization, a default dataplane, three default environments that are linked using a default deployment-pipeline and a default project to which developers can add their various application components.

However, if you wish to experiment and change this default setup to better suit what your organization requires, the samples in this folder are designed to help you do just that.

## Developer Application Samples
The samples in this category are for developers who would want to deploy a particular type of application which is written in a specific language. 

An application is abstracted as a Component within a given Project. Open Source Choreo supports Services, APIs, Web Applications and Tasks. The language used to develop the component is abstracted into a BuildPack in Open Source Choreo. This opens up many options such as Ballerina, Go, Node, Python etc. for developer applications.

## How to Use These Samples

Each sample includes:
- Description of the sample use case
- Source code link
- Required Custom Resources 
- Instructions on running the sample 
- Commands and instructions to verify the sample

To run a sample:
1. Navigate to the specific sample directory
2. Read the sample-specific README and follow the instructions 


## Available Samples
#### Platform Configuration
- [Create a new organization](https://github.com/choreo-idp/choreo/tree/samples/samples/platform-configuration/organization) - Brief description
- [Add a new environment to an organization](https://github.com/choreo-idp/choreo/tree/samples/samples/platform-configuration/environment) - Brief description

#### Developer Applications
- [Deploy a Ballerina Service](https://github.com/choreo-idp/choreo/tree/samples/samples/applications/languages/ballerina) - Brief description
- [Deploy a Go Service](https://github.com/choreo-idp/choreo/tree/samples/samples/applications/languages/go) - Brief description