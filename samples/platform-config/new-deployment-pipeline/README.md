# Create a new Deployment Pipeline
This sample demonstrates how to create a new Deployment Pipeline in Choreo. 

The Deployment Pipeline resource kind represents an ordered set of environments that a deployment will go through to reach a critical environment. 

In this sample the new deployment pipeline facilitates promoting from development to qa, qa to pre-production, and then from pre-production to production environments which were created earlier.

## Deploy in Choreo
Use the following command to create the new deployment pipeline.

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/platform-config/new-deployment-pipeline/deployment-pipeline.yaml
```
