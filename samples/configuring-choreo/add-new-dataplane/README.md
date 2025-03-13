# Create a new DataPlane
This sample demonstrates how to create a new dataPlane in Choreo. The DataPlane resource kind represents a Private Data Plane in Choreo. The controller of this resource kind is responsible for keeping the health status of the data plane

Note: Please note that this is currently for reference purposes only. At the moment, Choreo does not support adding external data planes, 
but this feature is on the roadmap for future releases.

## Deploy in Choreo
Use the following command to create the new dataPlane.

```bash
choreoctl apply -f samples/platform-configuration/dataplane/dataplane.yaml
``` 

```bash
dataplane.core.choreo.dev/us-east-1 created
```
