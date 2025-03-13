# Create a new Organization
This sample demonstrates how to create a new organization in Choreo. 

An organization is the overall grouping for related projects. 
An organization to start creating applications needs environments, a dataplane, deployment pipelines and a project. 

## Deploy in Choreo
Use the following command to create the new organization called `ACME`. This will create only an organization. 

```bash
choreoctl apply -f samples/platform-configuration/organization/organization.yaml
``` 

You will see the following output.
```bash
organization.core.choreo.dev/acme created
```

Use the following commaind to create the organization and all the other support resources as well.

```bash
choreoctl apply -f samples/platform-configuration/organization/complete-organization.yaml
``` 

```bash
organization.core.choreo.dev/acme created
dataplane.core.choreo.dev/dp-local created
deploymentpipeline.core.choreo.dev/default-pipeline created
environment.core.choreo.dev/development created
project.core.choreo.dev/customer-portal created
```