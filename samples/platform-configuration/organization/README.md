# Create a new Organization
This sample demonstrates how to create a new organization in Choreo. It creates all other necessary resources such as 
environments, dataplanes, and deployment tracks.

Note: If you only need the Organization resource, apply only that resource.

## Deploy in Choreo
Use the following command to create the new organization called `ACME`.

```bash
choreoctl apply -f samples/platform-configuration/organization/organization.yaml
``` 

```bash
organization.core.choreo.dev/acme created
dataplane.core.choreo.dev/dp-local created
deploymentpipeline.core.choreo.dev/default-pipeline created
environment.core.choreo.dev/development created
```