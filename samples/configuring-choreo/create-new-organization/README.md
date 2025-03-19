# Create a new Organization
This sample demonstrates how to create a new organization in Choreo.

An organization serves as the top-level grouping for related projects and is essential for managing applications effectively.

To start creating applications within an organization, you need the following resources:
- Environments
- Data Plane
- Deployment Pipeline
- Project

## Deploy in Choreo
Use the following command to create a new organization called `ACME`. This will create only tbe organization. 

```bash
choreoctl apply -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/configuring-choreo/create-new-organization/organization.yaml
``` 

You will see the following output.
```bash
organization.core.choreo.dev/acme created
```

If you want to create the organization along with all the necessary resources, use the following command:

```bash
choreoctl apply -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/configuring-choreo/create-new-organization/complete-organization.yaml
``` 

You will see the following output.
```bash
organization.core.choreo.dev/acme created
dataplane.core.choreo.dev/dp-local created
deploymentpipeline.core.choreo.dev/default-pipeline created
environment.core.choreo.dev/development created
project.core.choreo.dev/customer-portal created
```

## Clean Up

To remove all deployed resources, use the following command.

```shell
choreoctl delete -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/configuring-choreo/create-new-organization/complete-organization.yaml
```
