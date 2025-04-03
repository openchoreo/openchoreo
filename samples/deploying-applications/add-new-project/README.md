# Create a new Project
This sample demonstrates how to create a new project in Choreo. 

A [Project](../../../docs/resource-kind-reference-guide.md#project) resource kind enforces a grouping for the components within the project.

> [!Note] 
> In case you have not created a new organization name "acme", please run the following command before creating the new project.

```bash
choreoctl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/configuring-choreo/create-new-organization/organization.yaml
```

## Deploy in Choreo
Use the following command to create the new project in the organization you created earlier.

```bash
choreoctl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/deploying-applications/add-new-project/project.yaml
``` 

You will see the following output.

```bash
project.core.choreo.dev/customer-portal created
```
