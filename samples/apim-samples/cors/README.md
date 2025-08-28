# Reading List Service with CORS Sample

This sample demonstrates how to deploy a reading list service with Cross-Origin Resource Sharing (CORS) enabled using OpenChoreo's new CRD design, showcasing the integration of Component, Workload, Service, and APIClass resources with CORS policies.

## Overview

This sample deploys a reading list service and demonstrates how to configure CORS using the API management capabilities of OpenChoreo. The service uses a pre-configured APIClass with CORS policies that allow cross-origin requests from web browsers.

## Pre-requisites

- Kubernetes cluster with OpenChoreo installed
- The `kubectl` CLI tool installed
- A web browser or tools like `curl` that can make cross-origin requests
- Make sure you have the `jq` command-line JSON processor installed for parsing responses

## Step 1: Deploy the Service (Developer)

1. **Review the Service Configuration**
   
   Examine the service resources that will be deployed:
   ```bash
   cat reading-list-service-with-cors.yaml
   ```

2. **Deploy the Reading List Service**
   
   Apply the service resources:
   ```bash
   kubectl apply -f reading-list-service-with-cors.yaml
   ```

3. **Verify Service Deployment**
   
   Check that all resources were created successfully:
   ```bash
   kubectl get component,workload,services.openchoreo.dev reading-list-service-cors
   ```

This creates:
- **Component** (`reading-list-service-cors`): Component metadata and type definition
- **Workload** (`reading-list-service-cors`): Container configuration with reading list API endpoints
- **Service** (`reading-list-service-cors`): Runtime service configuration that uses the `default-with-cors` APIClass

## Step 2: Expose the API Gateway

Port forward the OpenChoreo gateway service to access it locally:

```bash
kubectl port-forward -n openchoreo-data-plane svc/gateway-external 8443:443 &
```

## Step 3: Test CORS Configuration

> [!NOTE]
> **Default APIClass with CORS**
>
> OpenChoreo provides a default APIClass called `default-with-cors` that is configured with CORS policies.
> This APIClass allows cross-origin requests from web browsers, enabling frontend applications to interact with the API.

**Quick Test - Add a Sample Book**

Before diving into CORS-specific testing, let's add a sample book to make our subsequent tests more meaningful:

```bash
curl -k -X POST -H "Content-Type: application/json" \
  -d '{"title":"The Hobbit","author":"J.R.R. Tolkien","status":"to_read"}' \
  "$(kubectl get servicebinding reading-list-service-cors -o jsonpath='{.status.endpoints[0].public.uri}')/books"
```

> [!TIP]
> Adding a sample book first helps demonstrate the API functionality more clearly. When you run GET requests in the following CORS tests, you'll see actual book data in the responses instead of empty arrays, making it easier to verify that the requests are working correctly.

### Method 1: Test with curl (CORS Preflight)

1. **Test Preflight OPTIONS Request**
   
   Test the CORS preflight request that browsers send for certain cross-origin requests:
   ```bash
   curl -k -X OPTIONS -H "Origin: https://example.com" \
     -H "Access-Control-Request-Method: POST" \
     -H "Access-Control-Request-Headers: Content-Type" \
     -v "$(kubectl get servicebinding reading-list-service-cors -o jsonpath='{.status.endpoints[0].public.uri}')/books"
   ```
   
   You should see CORS headers in the response:
   ```
    < access-control-allow-origin: https://example.com
    < access-control-allow-methods: GET, POST, PUT, DELETE
    < access-control-allow-headers: Content-Type, Authorization
    < access-control-max-age: 86400
    < access-control-expose-headers: X-Request-ID
   ```

2. **Test Actual CORS Request**
   
   Make a cross-origin request with a valid Origin header (one that's allowed in the CORS policy):
   ```bash
   curl -k -H "Origin: https://example.com" \
     -v "$(kubectl get servicebinding reading-list-service-cors -o jsonpath='{.status.endpoints[0].public.uri}')/books"
   ```
   
   You should see the CORS headers in the response:
   ```
   < access-control-allow-origin: https://example.com
   < access-control-expose-headers: X-Request-ID
   ```
   
   **Test with a disallowed origin:**
   ```bash
   curl -k -H "Origin: https://disallowed-site.com" \
     -v "$(kubectl get servicebinding reading-list-service-cors -o jsonpath='{.status.endpoints[0].public.uri}')/books"
   ```
   
   This request should not include CORS headers since `https://disallowed-site.com` is not in the allowed origins list.

### Method 2: Automated Browser Testing with Temporary Container

**Simpler Docker approach (if you have Docker):**

This method creates a temporary web page that automatically tests CORS by making JavaScript fetch requests to your API from a browser. Here's what it does:

1. **Creates an HTML test page** with a button that triggers CORS requests
2. **Serves the page using Docker** on http://localhost:8080 
3. **Simulates a real web application** trying to access your API from a different origin
4. **Shows visual results** - green for success, red for CORS errors
5. **Auto-cleanup** - everything is removed when you stop the container

This approach mimics how a real frontend application would interact with your API and helps you see CORS behavior in action.

Run the following command to create the HTML file and start a Docker container serving it:

```bash
# Create temporary HTML and serve it with Docker
cat > cors-test.html << 'EOF'
<!DOCTYPE html>
<html>
<head>
    <title>CORS Test</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .result { background: #f5f5f5; padding: 20px; margin: 20px 0; border-radius: 5px; }
        .success { border-left: 4px solid #4CAF50; }
        .error { border-left: 4px solid #f44336; }
    </style>
</head>
<body>
    <h1>Reading List CORS Test</h1>
    <button onclick="testCORS()">Test CORS Request</button>
    <div id="result"></div>
    
    <script>
    async function testCORS() {
        const resultDiv = document.getElementById('result');
        try {
            const response = await fetch('https://development.choreoapis.localhost:8443/default/reading-list-service-cors/api/v1/reading-list/books');
            const data = await response.json();
            resultDiv.innerHTML = '<div class="result success"><h3>Success!</h3><pre>' + JSON.stringify(data, null, 2) + '</pre></div>';
        } catch (error) {
            resultDiv.innerHTML = '<div class="result error"><h3>CORS Error:</h3><p>' + error.message + '</p></div>';
        }
    }
    </script>
</body>
</html>
EOF

# Serve with Docker (auto-cleanup when stopped)
echo "Starting web server on http://localhost:8080"
echo "Press Ctrl+C to stop and cleanup..."
docker run --rm -p 8080:80 -v $(pwd)/cors-test.html:/usr/share/nginx/html/index.html:ro nginx:alpine
```

Then access the test page in your browser on [http://localhost:8080](http://localhost:8080).

```bash
# Cleanup
rm cors-test.html
```

> [!NOTE]
> **Expected Behavior:**
>
> When you run this test and click the "Test CORS Request" button, you should observe:
>
> **If CORS is working correctly:**
> - ✅ Green success box appears
> - Shows "Success!" with JSON response data
> - Response includes books from your reading list (including "The Hobbit" if you added it earlier)
>
> **If CORS is blocked:**
> - ❌ Red error box appears  
> - Shows "CORS Error" with message like "Failed to fetch" or "CORS policy blocked"
> - This happens when the web server origin (localhost:8080) is not in the allowed origins list
>
> **Key Point:** Since the Docker container serves from `localhost:8080`, and the CORS policy only allows `https://example.com` and `https://app.example.com`, you will likely see a CORS error. This demonstrates that the CORS policy is working correctly by blocking unauthorized origins.
>
> To test with an allowed origin, you would need to modify your `/etc/hosts` file to map `example.com` to `127.0.0.1` and serve over HTTPS, or update the APIClass configuration to include `http://localhost:8080` as an allowed origin.


## Step 3: Test API Functionality

Once CORS is working, test the actual API functionality:

```bash
# List all books
curl -k "$(kubectl get servicebinding reading-list-service-cors -o jsonpath='{.status.endpoints[0].public.uri}')/books"

# Add a new book
curl -k -X POST -H "Content-Type: application/json" \
  -d '{"title":"The Lord of the Rings","author":"J.R.R. Tolkien","status":"to_read"}' \
  "$(kubectl get servicebinding reading-list-service-cors -o jsonpath='{.status.endpoints[0].public.uri}')/books"

# Get a specific book (replace {id} with actual book ID from previous response)
curl -k "$(kubectl get servicebinding reading-list-service-cors -o jsonpath='{.status.endpoints[0].public.uri}')/books/{id}"
```

> [!TIP]
> #### CORS Configuration
>
> The CORS configuration in the `default-with-cors` APIClass includes:
> - **Access-Control-Allow-Origin**: `https://example.com, https://app.example.com` (specific allowed origins)
> - **Access-Control-Allow-Methods**: `GET, POST, PUT, DELETE` (no OPTIONS explicitly listed)
> - **Access-Control-Allow-Headers**: `Content-Type, Authorization` (specific headers only)
> - **Access-Control-Expose-Headers**: `X-Request-ID` (headers exposed to client)
> - **Access-Control-Max-Age**: `86400` (24 hours preflight cache)
>
> This configuration is more restrictive and secure than allowing all origins. Only the specified origins can make cross-origin requests to this API.

## Troubleshooting CORS Issues

1. **Browser Developer Tools**: Check the Network tab for CORS-related errors
2. **Preflight Requests**: Ensure OPTIONS requests are returning proper CORS headers
3. **Origin Header**: Verify that the Origin header is being sent with cross-origin requests
4. **HTTPS**: Some browsers require HTTPS for certain CORS scenarios

## Clean Up

Remove all resources:

```bash
# Remove service resources
kubectl delete -f reading-list-service-with-cors.yaml
```
