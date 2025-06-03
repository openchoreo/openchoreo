# OpenChoreo Observability

OpenChoreo provides observability for both developers and platform engineers. This is provided through logs as well as metrics in the openchoreo setup. 

## Setting up Observability Logging

 Observability logs is provided through a combination of fluentbit and open search in the data plane. The option of setting up observability is set to false by default. If you need to setup observability in your data plane you can do either step below.
 
 1. If the data plane helm has not been installed yet:
```
helm install choreo-dataplane oci://ghcr.io/openchoreo/helm-charts/choreo-dataplane \
            --kube-context kind-choreo --namespace "choreo-system" --create-namespace --timeout 30m \
            --set certmanager.enabled=false --set certmanager.crds.enabled=false \
            --set observability.logging.enabled=true
```
2. If the data plane helm has already been installed:
```
helm upgrade choreo-dataplane oci://ghcr.io/openchoreo/helm-charts/choreo-dataplane \
            --kube-context kind-choreo --namespace "choreo-system" --create-namespace --timeout 30m \
            --set certmanager.enabled=false --set certmanager.crds.enabled=false \
            --set observability.logging.enabled=true
```

Alternatively, if you already have checked out the openchoreo repository you can modify the 
`/openchoreo/github/sk/choreo/install/helm/choreo-dataplane/values.yaml` file as follows 
```
# Customizing overall observability configurations
observability:
  logging:
    enabled: true
```
and run the following command
```
helm install choreo-dataplane . choreo-dataplane \
            --kube-context kind-choreo --namespace "choreo-system" --create-namespace --timeout 30m \
            --set certmanager.enabled=false --set certmanager.crds.enabled=false \
            --set observability.logging.enabled=true
```

## Configuring Observability Logging feature 
By default all logs in the following namespaces are collected and routed to the dashboard.

- choreo-system - this will capture all operanation components of the choreo dataplane

- dp* - this will capture all application logs of components deployed in choreo

The configurations for this is in the template files under 

 - `/openchoreo/github/sk/choreo/install/helm/choreo-dataplane/templates` 
and the values for these templates can be found at 
 - `/openchoreo/github/sk/choreo/install/helm/choreo-dataplane/values.yaml`

### Some configurations that could be fine tuned are;

  - Specifying what to include and exclude when collecting 
By default all logs in namespaces "choreo-system" and "dp-*". Opensearch and Fluent-bit logs are excluded.

>      input:
>     ...
>     path: "/var/log/containers/*_choreo-system_*.log,/var/log/containers/*_dp-*_*.log"
>     excludePath: "/var/log/containers/*opensearch-0*_choreo-system_*.log,/var/log/containers/*opensearch-dashboard*_choreo-system_*.log,/var/log/containers/*fluent-bit*_choreo-system_*.log"
>     ...
 - Specifying where to forward the collected logs to
 In the default setup all logs are forwarded to the opensearch node

>      output:
>     name: opensearch
>     match: "kube.*"
>     host: opensearch
>     port: 9200
>     ...

 - Add additional filters for Fluent-bit under the fluent-bit section as a filter in the values file.
 >      filter:

 ## Verification of Observability Logging setup
Once the dataplane helm chart has been installed you can verify whether the necessary componenets are up and running with the following command. 

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