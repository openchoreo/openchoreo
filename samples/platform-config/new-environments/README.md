# Create a new Environment
This guide demonstrates how to create new environments in Choreo. 

The Environment resource kind helps organize and manage different stages of the application lifecycle, such as development, testing, and production. The environment is bound to a specific data plane in Choreo. 

We will create four such environments in your namespace.

### Environment Hierarchy

| Environment      | Purpose                                                                 |
|------------------|-------------------------------------------------------------------------|
| 1. development    | Shared development, team-wide feature branches, early integrations      |
| 2. qa             | Functional, regression, integration testing (often automated)           |
| 3. pre-production | Staging environment closely mirrors production (infra, secrets, data)   |
| 4. production     | Live, customer-facing traffic; change-controlled, monitored 

## Deploy in Choreo
Use the following command to create new environments.

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/platform-config/new-environments/development-environment.yaml
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/platform-config/new-environments/qa-environment.yaml
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/platform-config/new-environments/pre-production-environment.yaml
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/platform-config/new-environments/production-environment.yaml
```
