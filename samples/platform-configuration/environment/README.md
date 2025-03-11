# Create a new Environment
This guide demonstrates how to create new environments in Choreo. Environments help organize and manage different stages 
of the application lifecycle, such as development, testing, and production. The environment is bound to a specific data plane in Choreo.

## Deploy in Choreo
Use the following command to create new environments.

```bash
choreoctl apply -f samples/platform-configuration/environment/development-environment.yaml
choreoctl apply -f samples/platform-configuration/environment/staging-environment.yaml
choreoctl apply -f samples/platform-configuration/environment/production-environment.yaml
``` 

```bash
environment.core.choreo.dev/development created
environment.core.choreo.dev/staging created
environment.core.choreo.dev/production created
```
