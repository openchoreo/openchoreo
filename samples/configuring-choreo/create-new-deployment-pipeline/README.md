# Create a new Deployment Pipeline
This sample demonstrates how to create a new Deployment Pipeline in Choreo. 

The [DeploymentPipeline](../../../docs/resource-kind-reference-guide.md#deploymentpipeline) resource kind represents an ordered set of environments that a deployment will go through to reach a critical environment. 

In this sample the new deployment pipeline facilitates promoting from development to staging, and then from staging to production environments which were created earlier.

> [!Note] 
> In case you have not created a new organization name "acme", please run the following command before creating the deployment pipeline.

```bash
choreoctl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/configuring-choreo/create-new-organization/organization.yaml
```

## Deploy in Choreo
Use the following command to create the new deployment pipeline.

```bash
choreoctl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/configuring-choreo/create-new-deployment-pipeline/deployment-pipeline.yaml
``` 

You will see the following output.

```bash
deploymentpipeline.openchoreo.dev/pipeline-dev-stage-prod created
```
