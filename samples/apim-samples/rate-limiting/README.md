# Reading List Service with Rate Limiting Sample

This sample demonstrates how to deploy a reading list service with rate limiting using OpenChoreo's new CRD design, showcasing the integration of Component, Workload, Service, and APIClass resources with rate limiting policies.

## Overview

This sample deploys a reading list service and demonstrates how to configure rate limiting using the API management capabilities of OpenChoreo. The service uses a pre-configured APIClass with rate limiting that allows 10 requests per minute.

## Pre-requisites

- Kubernetes cluster with OpenChoreo installed
- The `kubectl` CLI tool installed
- Make sure you have the `jq` command-line JSON processor installed for parsing responses

## File Structure

```
rate-limiting/
├── reading-list-service-with-rate-limit.yaml  # Developer resources (Component, Workload, Service)
└── README.md                                  # This guide
```

## Step 1: Deploy the Service (Developer)

1. **Review the Service Configuration**
   
   Examine the service resources that will be deployed:
   ```bash
   cat reading-list-service-with-rate-limit.yaml
   ```

2. **Deploy the Reading List Service**
   
   Apply the service resources:
   ```bash
   kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/apim-samples/rate-limiting/reading-list-service-with-rate-limit.yaml
   ```

3. **Verify Service Deployment**
   
   Check that all resources were created successfully:
   ```bash
   kubectl get component,workload,service -l project=default
   ```

This creates:
- **Component** (`reading-list-service-rate-limit`): Component metadata and type definition
- **Workload** (`reading-list-service-rate-limit`): Container configuration with reading list API endpoints
- **Service** (`reading-list-service-rate-limit`): Runtime service configuration that uses the `default-with-rate-limit` APIClass

## Step 2: Expose the API Gateway

Port forward the OpenChoreo gateway service to access it locally:

```bash
kubectl port-forward -n choreo-system svc/choreo-external-gateway 8443:443 &
```

## Step 3: Test the Rate Limiting

> [!NOTE]
> **Default APIClass with Rate Limiting**
>
> OpenChoreo provides a default APIClass called `default-with-rate-limit` that is configured with rate limiting policies.
> This APIClass limits requests to 10 requests per minute window, allowing you to easily demonstrate rate limiting behavior.

1. **Test Normal Access**
   
   Make a few requests to verify the service is working:
   ```bash
   # List all books
   curl -k "https://development.choreoapis.localhost:8443/default/reading-list-service-rate-limit/api/v1/reading-list/books"
   
   # Add a new book
   curl -k -X POST -H "Content-Type: application/json" \
     -d '{"title":"The Hobbit","author":"J.R.R. Tolkien","status":"to_read"}' \
     "https://development.choreoapis.localhost:8443/default/reading-list-service-rate-limit/api/v1/reading-list/books"
   ```

2. **Test Rate Limiting**
   
   Make multiple rapid requests to trigger the rate limit (10 requests per minute):
   ```bash
   # Make 15 rapid requests to exceed the rate limit
   for i in {1..15}; do
     echo "Request $i:"
     curl -k -w "Status: %{http_code}\n" \
       "https://development.choreoapis.localhost:8443/default/reading-list-service-rate-limit/api/v1/reading-list/books"
     echo "---"
   done
   ```

3. **Observe Rate Limiting Behavior**
   
   After the 10th request within a minute, you should start receiving HTTP 429 (Too Many Requests) responses:
   ```
   Status: 429
   local_rate_limitedStatus: 429
   ```

> [!TIP]
> #### Rate Limiting Configuration
>
> The rate limiting is configured in the `default-with-rate-limit` APIClass with:
> - **Requests**: 10 requests allowed
> - **Window**: 1 minute (1m)
> 
> This means each client can make up to 10 requests per minute before being rate limited.

## Step 4: Verify Rate Limit Reset

Wait for about a minute and try making requests again:

```bash
# Wait 60+ seconds, then test again
sleep 65
curl -k "https://development.choreoapis.localhost:8443/default/reading-list-service-rate-limit/api/v1/reading-list/books"
```

The requests should succeed again as the rate limit window has reset.

## Clean Up

Remove all resources:

```bash
# Remove service resources
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/apim-samples/rate-limiting/reading-list-service-with-rate-limit.yaml
```
