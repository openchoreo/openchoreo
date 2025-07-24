# Reading List Service Sample

This sample demonstrates how to deploy a secure reading list service using OpenChoreo's new CRD design, 
showcasing the integration of ComponentV2, Workload, Service, and APIClass resources with JWT authentication.

## Overview

This sample deploys a sample reading list service and how it can be secured using the API management capabilities of OpenChoreo.
Here you will be creating a new APIClass resource to configure the API management capabilities.

## Pre-requisites

- Kubernetes cluster with OpenChoreo installed
- OpenChoreo default identity provider installed
- The `kubectl` CLI tool installed
- Make sure you have the `jq` command-line JSON processor installed for parsing responses

## File Structure

```
secure-service-with-jwt/
├── api-class.yaml                 # Platform Engineer resource
├── greeter-service-with-jwt.yaml  # Developer resources (ComponentV2, Workload, Service, API)
└── README.md                      # This guide
```

## Step 1: Deploy the Service (Developer)

1. **Review the Service Configuration**
   
   Examine the service resources that will be deployed:
   ```bash
   cat reading-list-service-with-jwt.yaml
   ```

2. **Deploy the Reading List Service**
   
   Apply the service resources:
   ```bash
   kubectl apply -f reading-list-service-with-jwt.yaml
   ```

3. **Verify Service Deployment**
   
   Check that all resources were created successfully:
   ```bash
   kubectl get componentv2,workload,service -l project=default
   ```

This creates:
- **ComponentV2** (`greeter-service`): Component metadata and type definition
- **Workload** (`reading-list-service`): Container configuration with reading list API endpoints
- **Service** (`greeter-service`): Runtime service configuration that exposes the API

## Step 2: Expose the API Gateway

Port forward the OpenChoreo gateway service to access it locally:

```bash
kubectl port-forward -n choreo-system svc/choreo-external-gateway 8443:443 &
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
   curl -k "https://development.choreoapis.localhost:8443/default/reading-list-service/api/v1/reading-list/books"
   ```
   
   This should return a 401 Unauthorized error since JWT authentication is required.

2. **Get Access Token**
   
   Retrieve an access token using the client credentials you configured earlier:
   ```bash
   ACCESS_TOKEN=$(kubectl run curl-pod --rm -i --restart=Never --image=curlimages/curl:latest -- \
     sh -c "curl -s --location 'http://openchoreo-identity-provider.openchoreo-identity-system:8090/oauth2/token' \
     --header 'Content-Type: application/x-www-form-urlencoded' \
     --data 'grant_type=client_credentials&client_id=reading-list-service-client-001&client_secret=reading-list-service-secret-001&scope=reading-list-permission' \
     | grep -o '\"access_token\":\"[^\"]*' | cut -d'\"' -f4" 2>/dev/null | head -1)
   ```

3. **Test Authenticated Access**
   
   Use the access token to make authenticated requests:
   ```bash
   # List all books
   curl -k -H "Authorization: Bearer $ACCESS_TOKEN" \
     https://development.choreoapis.localhost:8443/default/reading-list-service/api/v1/reading-list/books
   
   # Add a new book
   curl -k -X POST -H "Authorization: Bearer $ACCESS_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"title":"The Hobbit","author":"J.R.R. Tolkien","status":"to_read"}' \
     https://development.choreoapis.localhost:8443/default/reading-list-service/api/v1/reading-list/books
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
kubectl delete -f reading-list-service-with-jwt.yaml

# Remove API resource
kubectl delete api reading-list-api

# Remove API class
kubectl delete -f api-class.yaml

# Remove application from identity provider
curl -X DELETE http://localhost:8090/applications/reading-list-service-client-001
```
