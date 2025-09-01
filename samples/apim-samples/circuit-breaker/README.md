# Circuit Breaker Service Sample

This sample demonstrates how to deploy a service with circuit breaker capabilities using OpenChoreo's new CRD design. The circuit breaker pattern prevents cascading failures by temporarily blocking requests to an unhealthy upstream service, allowing it time to recover.

## Overview

Circuit breakers help protect your services from being overwhelmed by:
- **Connection Limits**: Limiting concurrent connections to upstream services
- **Request Limits**: Controlling the number of in-flight requests
- **Pending Request Limits**: Managing queued requests and failing fast when queues are full

When thresholds are exceeded, the circuit breaker will:
- Block new connections/requests
- Return HTTP 503 (Service Unavailable) for overflowing requests
- Allow the upstream service time to recover

## Pre-requisites

- A running OpenChoreo Control Plane
- `kubectl` configured to access your cluster
- `curl` for testing API endpoints

## Step 1: Deploy the Service

1. **Review the Service Configuration**

   Examine the service resources that will be deployed:
   ```bash
   cat reading-list-service-with-circuit-breaker.yaml
   ```

2. **Deploy the Reading List Service**

   Apply the service resources:
   ```bash
   kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.3/samples/apim-samples/circuit-breaker/reading-list-service-with-circuit-breaker.yaml
   ```

3. **Verify Service Deployment**

   Check that all resources were created successfully:
   ```bash
   kubectl get component,workload,services.openchoreo.dev reading-list-service-circuit-breaker
   ```

This creates:
- **Component** (`reading-list-service-circuit-breaker`): Component metadata and type definition
- **Workload** (`reading-list-service-circuit-breaker`): Container configuration with reading list API endpoints
- **Service** (`reading-list-service-circuit-breaker`): Runtime service configuration using the `default-with-circuit-breaker` APIClass

## Step 2: Expose the API Gateway

Set up port forwarding to access the service through the gateway:

```bash
kubectl port-forward -n openchoreo-data-plane svc/gateway-external 8443:443 &
```

The service will be available at:
```bash
kubectl get servicebinding reading-list-service-circuit-breaker -o jsonpath='{.status.endpoints[0].public.uri}'
```

## Step 3: Test Circuit Breaker Functionality

> **Note**: The `default-with-circuit-breaker` APIClass used by this example is already configured with circuit breaker settings:
> - `maxConnections`: 50 (Maximum connections to upstream)
> - `maxParallelRequests`: 50 (Maximum concurrent requests)
> - `maxParallelRetries`: 1 (Maximum concurrent retries)
> - `maxPendingRequests`: 20 (Maximum queued requests)

### Test 1: Normal Operation

First, verify the service works under normal conditions:

> [!NOTE]
> This sample uses a special reading list service that includes a 5-second delay on the `/reading-list/books` endpoint. This artificial delay makes it practical to test circuit breaker functionality by creating longer response times and easier triggering of connection limits during load testing.

```bash
# Add a book to the reading list
curl -k -X POST  \
  -H "Content-Type: application/json" \
  -d '{"title":"The Hobbit","author":"J.R.R. Tolkien","status":"to_read"}' \
  "$(kubectl get servicebinding reading-list-service-circuit-breaker -o jsonpath='{.status.endpoints[0].public.uri}')/books"

# Retrieve all books
curl -k "$(kubectl get servicebinding reading-list-service-circuit-breaker -o jsonpath='{.status.endpoints[0].public.uri}')/books"
```

> [!NOTE]
> You can see that each request to `/books` takes about 5 seconds due to the intentional delay. We will use this delay to help trigger the circuit breaker in the next test.

### Test 2: Trigger Circuit Breaker

Run the [generate-load.sh](./generate-load.sh) to generate a load to trigger the circuit breaker and capture detailed metrics for verification:

```bash
./generate-load.sh
```

### Verify Circuit Breaker Behavior

After running the load test, run the [analyze-results.sh](./analyze-results.sh) to analyze the results to prove the circuit breaker is working:

```bash
./analyze-results.sh
```

### Real-time Monitoring During Load Test

While running the load test, run the [real-time-monitor.sh](./real-time-monitor.sh) to monitor the circuit breaker in real-time:

```bash
./real-time-monitor.sh
```

> [!TIP]
> Optional information to help a user be more successful.
> **Usage Instructions:**
> 1. **Terminal 1 - Load Test**: Run the load test script
> 2. **Terminal 2 - Real-time Monitor**: Run the monitoring script
> 3. **Terminal 3 - Gateway Logs** (Optional): 
>  ```bash
>  kubectl logs -l gateway.envoyproxy.io/owning-gateway-name=gateway-external -n openchoreo-data-plane -c envoy -f
>  ```

### Evidence of Working Circuit Breaker

**Key Indicators:**
- ✅ Mix of 200 and 503 responses during load
- ✅ Response times for 503 are very low (fail-fast) compared to normal requests
- ✅ Service logs show stable request rate despite high client load
- ✅ Gateway logs mention circuit breaker activation

### Test 3: Recovery Testing

After the load test completes, wait a moment and test normal operation again:

```bash
# Test that service recovers after load subsides
curl -k "$(kubectl get servicebinding reading-list-service-circuit-breaker -o jsonpath='{.status.endpoints[0].public.uri}')/books"
```

The service should return to normal operation once the circuit breaker allows traffic through again.

## Understanding Circuit Breaker Configuration

The circuit breaker behavior is controlled by the `default-with-circuit-breaker` APIClass, which should include settings like:

```yaml
# Circuit breaker configuration (in APIClass default-with-circuit-breaker)
circuitBreaker:
  enabled: true
  maxConnections: 50           # Maximum connections to upstream (default: 1024)
  maxParallelRequests: 50      # Maximum concurrent requests (default: 1024)
  maxParallelRetries: 1        # Maximum concurrent retries (default: 1024)
  maxPendingRequests: 20       # Maximum queued requests (default: 1024)
```

**Key Parameters (from Envoy Gateway documentation):**
- **maxConnections**: The maximum number of connections that Envoy will establish to the referenced backend (default: 1024)
- **maxParallelRequests**: The maximum number of parallel requests that Envoy will make to the referenced backend (default: 1024)
- **maxParallelRetries**: The maximum number of parallel retries that Envoy will make to the referenced backend (default: 1024)
- **maxPendingRequests**: The maximum number of pending requests that Envoy will queue to the referenced backend (default: 1024)

When these limits are exceeded:
- New connections are blocked
- Excess requests receive HTTP 503 responses
- The upstream service is protected from overload

## Expected Results

During load testing, you should observe:

1. **Initial Success**: First requests succeed normally
2. **Circuit Breaker Activation**: Once limits are reached, you'll see:
   - HTTP 503 responses for excess requests
   - Reduced load on the upstream service
   - Fast failure responses (fail-fast behavior)
3. **Recovery**: After load subsides, normal operation resumes

## Tips for Testing

- Adjust load test parameters (concurrency, test duration) based on your circuit breaker configuration
- Use different endpoints (GET, POST, PUT) to test various operations
- Monitor both client-side responses and server-side logs
- Experiment with different circuit breaker thresholds to understand their impact

## Clean Up

Remove the deployed resources:

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.3/samples/apim-samples/circuit-breaker/reading-list-service-with-circuit-breaker.yaml
```
