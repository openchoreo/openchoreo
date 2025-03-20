# Create a new Environment
This guide demonstrates how to create new environments in Choreo. 

The [Environment](../../../docs/resource-kind-reference-guide.md#environment) resource kind helps organize and manage different stages of the application lifecycle, such as development, testing, and production. The environment is bound to a specific data plane in Choreo. 

We will create three such environments in the new organization created earlier.

> [!Note] 
> In case you have not created a new organization name "acme", please run the following command before creating the new environments.

```bash
choreoctl apply -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/configuring-choreo/create-new-organization/organization.yaml
```

## Deploy in Choreo
Use the following command to create new environments.

```bash
choreoctl apply -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/configuring-choreo/create-new-environments/development-environment.yaml
choreoctl apply -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/configuring-choreo/create-new-environments/staging-environment.yaml
choreoctl apply -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/configuring-choreo/create-new-environments/production-environment.yaml
``` 

You will see the following output

```bash
environment.core.choreo.dev/development created
environment.core.choreo.dev/staging created
environment.core.choreo.dev/production created
```

In case you prefer to have only two environments in your organization e.g. dev and prod you can run just those two commands.