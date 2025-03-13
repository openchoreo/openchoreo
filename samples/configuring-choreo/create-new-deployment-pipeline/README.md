# Create a new Deployment Pipeline
This sample demonstrates how to create a new Deployment Pipeline in Choreo. 

The DeploymentPipeline resource kind represents an ordered set of environments that a deployment will go through to reach a critical environment. 

In this sample the new deployment pipeline facilitates promoting from development to staging, and then from staging to production environments which were created earlier.

## Deploy in Choreo
Use the following command to create the new deployment pipeline.

```bash
choreoctl apply -f samples/platform-configuration/deployment-pipeline/deployment-pipeline.yaml
``` 

```bash
deploymentpipeline.core.choreo.dev/pipeline-dev-stage-prod created
```
