# OpenChoreo Observability

OpenChoreo provides observability for both developers and platform engineers. This is provided through logs as well as metrics in the openchoreo setup. 

## Setting up Observability Logging

 Observability logs are provided through a combination of fluentbit and open search in the data plane. The option of setting up observability is set to false by default. If you need to setup observability in your data plane, you can execute the following command based on the type of setup you have chosen.

1. In a single cluster setup
```
helm upgrade --install choreo-dataplane oci://ghcr.io/openchoreo/helm-charts/choreo-dataplane \
   --kube-context kind-choreo \
   --namespace "choreo-system" \
   --create-namespace \
   --set certmanager.enabled=false \
   --set observability.logging.enabled=true \
   --set observer.image.tag=latest-dev \
   --timeout 30m
```

2. In a multicluster setup

> [!IMPORTANT]  
> This multi-cluster setup (Control plane + Dataplane with FluentBit/OpenSearch) requires minimum 4 CPU and 8GB memory for stable cluster operation.
```
helm upgrade --install choreo-dataplane oci://ghcr.io/openchoreo/helm-charts/choreo-dataplane \
   --kube-context kind-choreo-dp \
   --namespace "choreo-system" --create-namespace \
   --set observability.logging.enabled=true \
   --timeout 30m
```

## Configuring Observability Logging feature 
By default all logs in the following namespaces are collected and routed to the dashboard.

- choreo-system - this will capture all operational components of the choreo dataplane

- dp* - this will capture all application logs of components deployed in choreo

The configurations for this is in the template files under 

 - `<your_local_openchoreo_repo>/install/helm/choreo-dataplane/templates`,
  
and the values for these templates can be found at 
 - `<your_local_openchoreo_repo>/install/helm/choreo-dataplane/values.yaml`

### Some configurations that could be fine tuned are;

  - Specifying what to include and exclude when collecting logs

    By default all logs in namespaces "choreo-system" and "dp-*". Opensearch and Fluent-bit logs are excluded.

    >     input:
    >     ...
    >     path: "/var/log/containers/*_choreo-system_*.log,/var/log/containers/*_dp-*_*.log"
    >     excludePath: "/var/log/containers/*opensearch-0*_choreo-system_*.log,/var/log/containers/*opensearch-dashboard*_choreo-system_*.log,/var/log/containers/*fluent-bit*_choreo-system_*.log"
    >     ...
 - Specifying where to forward the collected logs to
 
    In the default setup all logs are forwarded to the opensearch node

    >     output:
    >     name: opensearch
    >     match: "kube.*"
    >     host: opensearch
    >     port: 9200
    >     ...

 - Add additional filters for Fluent-bit under the fluent-bit section as a filter in the values file.


 ## Verification of Observability Logging setup
Once the dataplane helm chart has been installed, you can verify whether the necessary componenets are up and running with the following command. 

```
kubectl get pods -n choreo-system
```

You should see pods with names as follows:
```
choreo-system choreo-dataplane-fluent-bit-xxxx    
choreo-system choreo-dataplane-opensearch-0
choreo-system choreo-dataplane-opensearch-dashboard-xxxxx-xxxxx
```  

## Viewing logs on the Opensearch Dashboard
 1. Get the pod name for opensearch dashboard. You can execute the following  and get it. 

    ```
    kubectl get pods -n choreo-system
    ```

 2. Do a port forward for the dashboard as follows
 
    ```
    kubectl port-forward pod/<dashboard-pod-name> 5601:5601 -n choreo-system
    ```
 
 3. Copy the following on your browser as the url

    `http://localhost:5601`

 4. Once opensearch loads, on the home page click on "Discover" under the Opensearch Dashboards menu section. You will see a button indicating "Create Index Patterns" - click this.

 5. On the "Create Index Pattern" page, create an indexing pattern as `kubernetes*` and click on "Next Step" button. Select `@timestamp` from the drop down and click on the "Create Index Pattern" button below.

 6. You will be taken back to the home page - Click on "Discover" again. This will load all the logs onto the dashboard. 

 7. Alternatively, to create the index pattern programatically you can run the following curl command after the port forward
 ```
 curl -X POST "http://localhost:5601/api/saved_objects/index-pattern" \
  -H "Content-Type: application/json" \
  -H "osd-xsrf: true" \
  -u "admin:admin" \
  -d '{
    "attributes": {
      "title": "kubernetes*",
      "timeFieldName": "@timestamp"
    }
  }'
 ```
8. To view application logs, you can use the filter and search capabilities on the "Discover" view 

   For example, after deploying the [Time logger task](./../samples/deploying-applications/build-from-source/time-logger-task/) which logs the current time periodically, you can view the logs as follows:

   1. Get the namespace of your deployed application:

      ```
      kubectl get namespaces
      ```

      Look for a namespace similar to:

      ```
      dp-default-org-default-proje-development-39faf2d8
      ```

   2. In the Opensearch Dashboard "Discover" view, create a filter for this namespace using:

      ```
      kubernetes.namespace_name:dp-default-org-default-proje-development-39faf2d8
      ```

   3. In the search bar, enter `Current time` to filter logs generated by the application.

   4. Refresh or adjust the time range selector in the dashboard to view the latest logs.

9. Use the choreo-observer api to access the logs of a particular component
   
   1. Deploy the [Go greeting serice](../samples/deploying-applications/languages/go/)
   
   2. Do a port forward to the choreo-observer api as below

   ```
   kubectl port-forward svc/choreo-observer 8080:8080 -n choreo-system
   ```

   3. Use CURL to query the service. The example below is to get "ERROR" logs of component "greeting-service-go" in environment "development" for a specific time window 

   ```
   curl -X POST http://localhost:8080/api/logs/component/greeting-service-go \
        -H "Content-Type: application/json" \
        -d '{
          "startTime": "2025-07-02T18:40:00Z",
          "endTime": "2025-07-02T18:50:00Z",
          "environmentId": "development",
          "logLevels": ["ERROR"],
          "limit": 10
        }'
   ```