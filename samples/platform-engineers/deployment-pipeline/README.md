# Create a new Deployment Pipeline
This sample demonstrates how to create a new Deployment Pipeline in Choreo. Deployment pipelines help manage the promotion 
of your application across different environments. This deployment pipeline facilitates promoting from development to staging, 
and then from staging to production environments.

## Deploy in Choreo
Use the following command to create the new deployment pipeline.

```bash
kubectl apply -f samples/platform-engineers/deployment-pipeline/deployment-pipeline.yaml
``` 

```bash
deploymentpipeline.core.choreo.dev/pipeline-dev-stage-prod created
```
