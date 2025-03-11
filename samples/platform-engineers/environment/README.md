# Create a new Environment
This guide demonstrates how to create new environments in Choreo. Environments help organize and manage different stages 
of the application lifecycle, such as development, testing, and production.

## Deploy in Choreo
Use the following command to create new environments.

```bash
kubectl apply -f samples/platform-engineers/environment/development-environment.yaml
kubectl apply -f samples/platform-engineers/environment/staging-environment.yaml
kubectl apply -f samples/platform-engineers/environment/production-environment.yaml
``` 

```bash
environment.core.choreo.dev/development created
environment.core.choreo.dev/staging created
environment.core.choreo.dev/production created
```
