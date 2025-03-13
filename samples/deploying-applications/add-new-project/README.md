# Create a new Project
This sample demonstrates how to create a new project in Choreo. 

A [Project](../../../docs/resource-kind-reference-guide.md#project) resource kind enforces a grouping for the components within the project.

## Deploy in Choreo
Use the following command to create the new project in the organization you created earlier.

```bash
choreoctl apply -f samples/platform-configuration/project/project.yaml
``` 

You will see the following output.

```bash
project.core.choreo.dev/customer-portal created
```
