# Reading List Service Sample

This sample demonstrates how to deploy a secure reading list service using OpenChoreo's new CRD design,
showcasing the integration of Component, Workload, Service, and APIClass resources with JWT authentication.

## Overview

This sample deploys a sample reading list service and how it can be secured using the API management capabilities of OpenChoreo.
Here you will be creating a new APIClass resource to configure the API management capabilities.

## Pre-requisites

- Kubernetes cluster with OpenChoreo and the default identity provider installed (Follow our [installation](https://openchoreo.dev/docs/category/installation/) guides for more information)
- The `kubectl` CLI tool installed
- Make sure you have the `jq` command-line JSON processor installed for parsing responses

## Step 1: Deploy the Service (Developer)

1. **Review the Service Configuration**

   Examine the service resources that will be deployed:
   ```bash
   cat reading-list-service-with-jwt-auth.yaml
   ```

2. **Deploy the Reading List Service**

   Apply the service resources:
   ```bash
   kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.3/samples/apim-samples/authentication/reading-list-service-with-jwt-auth.yaml
   ```

3. **Verify Service Deployment**

   Check that all resources were created successfully:
   ```bash
   kubectl get component,workload,services.openchoreo.dev reading-list-service-jwt
   ```

This creates:
- **Component** (`reading-list-service-jwt`): Component metadata and type definition
- **Workload** (`reading-list-service-jwt`): Container configuration with reading list API endpoints
- **Service** (`reading-list-service-jwt`): Runtime service configuration that exposes the API

## Step 2: Expose the API Gateway

Port forward the OpenChoreo gateway service to access it locally:

```bash
kubectl port-forward -n openchoreo-data-plane svc/gateway-external 8443:443 &
```

## Step 3: Test the Secured Service

> [!NOTE]
> **Default Application and APIClass Configuration**
>
> OpenChoreo provides a default application already registered in the identity provider along with a default APIClass configured for authentication.
> This means that tokens generated from the default application can be used to authenticate APIs that utilize the default APIClass,
> simplifying the setup process to demonstrate authentication scenarios.


1. **Test Unauthenticated Access (Should Fail)**

   Try accessing the API without authentication:
   ```bash
   curl -k "$(kubectl get servicebinding reading-list-service-jwt -o jsonpath='{.status.endpoints[0].public.uri}')/books"
   ```

   This should return a 401 Unauthorized error since JWT authentication is required.

2. **Get Access Token**

   Retrieve an access token using the client credentials you configured earlier:
   ```bash
   ACCESS_TOKEN=$(kubectl run curl-pod --rm -i --restart=Never --image=curlimages/curl:latest -- \
     sh -c "curl -s --location 'http://identity-provider.openchoreo-identity-system.svc.cluster.local:8090/oauth2/token' \
     --header 'Content-Type: application/x-www-form-urlencoded' \
     --data 'grant_type=client_credentials&client_id=openchoreo-default-client&client_secret=openchoreo-default-secret' \
     | grep -o '\"access_token\":\"[^\"]*' | cut -d'\"' -f4" 2>/dev/null | head -1)
   ```

3. **Test Authenticated Access**

   Use the access token to make authenticated requests:
   ```bash
   # List all books
   curl -k -H "Authorization: Bearer $ACCESS_TOKEN" \
     "$(kubectl get servicebinding reading-list-service-jwt -o jsonpath='{.status.endpoints[0].public.uri}')/books"
   
   # Add a new book
   curl -k -X POST -H "Authorization: Bearer $ACCESS_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"title":"The Hobbit","author":"J.R.R. Tolkien","status":"to_read"}' \
     "$(kubectl get servicebinding reading-list-service-jwt -o jsonpath='{.status.endpoints[0].public.uri}')/books"
   ```

> [!TIP]
> #### Verification
>
> With proper authentication, you should receive successful responses:
> - GET `/books`: Returns an array of books (initially empty)
> - POST `/books`: Returns the created book object with a generated ID

## Clean Up

Remove all resources:

```bash
# Remove service resources
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.3/samples/apim-samples/authentication/reading-list-service-with-jwt-auth.yaml
```
