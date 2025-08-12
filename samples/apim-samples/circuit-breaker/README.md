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

## File Structure

```
circuit-breaker/
├── README.md                                          # This guide
└── reading-list-service-with-circuit-breaker.yaml     # All CRD resources
```

## Step 1: Deploy the Service

Apply the sample configuration to deploy a reading list service with circuit breaker protection:

```bash
kubectl apply -f reading-list-service-with-circuit-breaker.yaml
```

This creates:
1. **Component**: Component metadata defining the service type
2. **Workload**: Container configuration with OpenAPI schema and REST endpoints
3. **Service**: Runtime configuration using the `default-with-circuit-breaker` APIClass

> **Note**: The `default-with-circuit-breaker` APIClass should be configured with circuit breaker settings like:
> - `maxPendingRequests`: Maximum pending request queue size (e.g., 0 to disable queuing)
> - `maxParallelRequests`: Maximum concurrent requests (e.g., 10)
> - Connection limits and timeout configurations

## Step 2: Expose the API Gateway

Set up port forwarding to access the service through the gateway:

```bash
kubectl port-forward service/choreo-external-gateway 8443:8443 -n openchoreo-data-plane
```

The service will be available at:
```bash
kubectl get servicebinding reading-list-service-circuit-breaker -o jsonpath='{.status.endpoints[0].public.uri}'
```

## Step 3: Test Circuit Breaker Functionality

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

You should see successful responses with the book data.

### Test 2: Trigger Circuit Breaker

Generate load to trigger the circuit breaker and capture detailed metrics for verification:

```bash
# Load test with detailed response tracking
echo "Starting circuit breaker load test with response tracking..."
echo "Timestamp,ResponseCode,ResponseTime" > circuit_breaker_results.csv

for i in $(seq 1 100); do 
  (for j in $(seq 1 10); do 
    start_time=$(date +%s.%N)
    response_code=$(curl -k -s -w "%{http_code}" -o /dev/null \
      "$(kubectl get servicebinding reading-list-service-circuit-breaker -o jsonpath='{.status.endpoints[0].public.uri}')/books" 2>/dev/null)
    end_time=$(date +%s.%N)
    response_time=$(echo "$end_time - $start_time" | bc -l 2>/dev/null || echo "0")
    timestamp=$(date +%s)
    echo "$timestamp,$response_code,$response_time" >> circuit_breaker_results.csv
  done) &
done; wait

echo "Load test completed. Results saved to circuit_breaker_results.csv"
```

### Verify Circuit Breaker Behavior

After running the load test, analyze the results to prove the circuit breaker is working:

```bash
# Analyze the detailed results (if you used the first approach)
if [ -f circuit_breaker_results.csv ]; then
  echo "=== Circuit Breaker Analysis ==="
  echo "Total requests: $(tail -n +2 circuit_breaker_results.csv | wc -l)"
  echo "Successful responses (200): $(grep ',200,' circuit_breaker_results.csv | wc -l)"
  echo "Circuit breaker responses (503): $(grep ',503,' circuit_breaker_results.csv | wc -l)"
  echo "Other errors: $(tail -n +2 circuit_breaker_results.csv | grep -v ',200,' | grep -v ',503,' | wc -l)"
  
  # Show timeline of responses to demonstrate circuit breaker activation
  echo ""
  echo "=== Response Timeline (First 100 responses) ==="
  echo "Response Code | Response Time"
  cat circuit_breaker_results.csv | while IFS=',' read timestamp code time; do
    printf "%-13s | %.3f seconds\n" "$code" "$time"
  done
fi
```

### Real-time Monitoring During Load Test

While running the load test, monitor the circuit breaker in real-time:

```bash
# Terminal 1: Run the load test (use one of the commands above)

# Terminal 2: Monitor gateway logs for circuit breaker decisions
kubectl logs -l app=choreo-external-gateway -n openchoreo-system -f | grep -i "upstream_reset_before_response_started{overflow}"

# Terminal 3: Monitor real-time response codes
watch -n 1 'curl -k -s -w "Response: %{http_code} | Time: %{time_total}s\n" -o /dev/null https://development.choreoapis.localhost:8443/default/reading-list-service-circuit-breaker/api/v1/reading-list/books'
```

### Evidence of Working Circuit Breaker

A properly functioning circuit breaker should show:

1. **Some Success Phase**: Some requests return HTTP 200
2. **Circuit Breaker Activation**: 
   - HTTP 503 responses start appearing
   - Response times become very fast (fail-fast behavior)
   - Error messages in gateway logs mentioning overflow
3. **Protection Evidence**:
   - Service logs show fewer incoming requests despite continued client attempts
   - Gateway logs show rejected connections
   - Upstream service remains stable under load

**Expected Output Pattern:**
```
Response Code | Response Time
200          | 5.045 seconds
200          | 5.052 seconds  
200          | 5.048 seconds
503          | 0.003 seconds  <- Circuit breaker activated (fast failure)
503          | 0.002 seconds
503          | 0.001 seconds
200          | 5.051 seconds  <- Some requests still succeed
503          | 0.002 seconds
```

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

- Adjust load test parameters (`-n` for total requests, `-c` for concurrency) based on your circuit breaker configuration
- Use different endpoints (GET, POST, PUT) to test various operations
- Monitor both client-side responses and server-side logs
- Experiment with different circuit breaker thresholds to understand their impact

## Clean Up

Remove the deployed resources:

```bash
kubectl delete -f reading-list-service-with-circuit-breaker.yaml
```
