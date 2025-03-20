# Update an existing Deployment Pipeline
This sample demonstrates how to update an existing Deployment Pipeline in Choreo. 

The [DeploymentPipeline](../../../docs/resource-kind-reference-guide.md#deploymentpipeline) resource kind represents an ordered set of environments that a deployment will go through to reach a critical environment. 

Currently the default-org has a deployment pipeline which consists of three environments - namely development, staging and production. 

In this sample we first add a new environment named test, and then, update the deployment pipeline.

## Deploy in Choreo
Use the following command to create a new environment.

```bash
choreoctl apply -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/configuring-choreo/update-deployment-pipeline/test-environment.yaml
```
You will see the following output.

```bash
environment.core.choreo.dev/test created
```

Use the following command to update the existing deployment pipeline.

```bash
choreoctl apply -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/configuring-choreo/update-deployment-pipeline/deployment-pipeline.yaml
``` 

You will see the following output.

```bash
deploymentpipeline.core.choreo.dev/pipeline-dev-stage-prod updated
```
