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
kubectl port-forward -n openchoreo-data-plane svc/gateway-external 8443:443 &
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

> [!NOTE]
> You can see that each request to `/books` takes about 5 seconds due to the intentional delay. We will use this delay to help trigger the circuit breaker in the next test.

### Test 2: Trigger Circuit Breaker

Generate load to trigger the circuit breaker and capture detailed metrics for verification:

```bash
#!/bin/bash

# Circuit Breaker Load Test with Controlled Load Ramping
echo "Starting circuit breaker load test with response tracking..."

# Configuration
MAX_CONCURRENT=20        # Maximum concurrent requests
RAMP_UP_DURATION=30     # Seconds to ramp up to max load
TEST_DURATION=60        # Total test duration in seconds
PROGRESS_INTERVAL=5     # Progress reporting interval

# Get endpoint
endpoint="$(kubectl get servicebinding reading-list-service-circuit-breaker -o jsonpath='{.status.endpoints[0].public.uri}')/books"
if [[ -z "${endpoint}" ]]; then
  echo "Failed to resolve service endpoint." >&2
  exit 1
fi

echo "Testing endpoint: $endpoint"
echo "Test configuration:"
echo "  Max concurrent requests: $MAX_CONCURRENT"
echo "  Ramp-up duration: ${RAMP_UP_DURATION}s"
echo "  Total test duration: ${TEST_DURATION}s"
echo ""

# Initialize results file
results_file="circuit_breaker_results_$(date +%Y%m%d_%H%M%S).csv"
echo "Timestamp,ResponseCode,ResponseTime,ConcurrentLevel" > "$results_file"

# Function to make a single request and log result
make_request() {
    local concurrent_level=$1
    local start_time end_time response_time response_code timestamp
    
    start_time=$(date +%s.%N)
    response_code=$(curl -k -s -w "%{http_code}" -o /dev/null "$endpoint" 2>/dev/null || echo "000")
    end_time=$(date +%s.%N)
    response_time=$(echo "$end_time - $start_time" | bc -l 2>/dev/null || echo "0")
    timestamp=$(date +%s)
    
    # Use a temporary file to avoid write conflicts
    echo "$timestamp,$response_code,$response_time,$concurrent_level" >> "${results_file}.tmp.$$"
}

# Function to run requests at a specific concurrency level
run_concurrent_requests() {
    local concurrent_level=$1
    local pids=()
    
    for ((i=1; i<=concurrent_level; i++)); do
        make_request "$concurrent_level" &
        pids+=($!)
    done
    
    # Wait for all requests to complete
    for pid in "${pids[@]}"; do
        wait "$pid" 2>/dev/null
    done
    
    # Merge temporary files
    if ls "${results_file}.tmp."* >/dev/null 2>&1; then
        cat "${results_file}.tmp."* >> "$results_file"
        rm -f "${results_file}.tmp."*
    fi
}

echo "Starting load test..."
test_start_time=$(date +%s)
last_progress_time=$test_start_time

while true; do
    current_time=$(date +%s)
    elapsed=$((current_time - test_start_time))
    
    # Check if test duration is complete
    if [ $elapsed -ge $TEST_DURATION ]; then
        break
    fi
    
    # Calculate current concurrency level based on ramp-up
    if [ $elapsed -le $RAMP_UP_DURATION ]; then
        # Linear ramp-up
        concurrent_level=$(echo "$MAX_CONCURRENT * $elapsed / $RAMP_UP_DURATION" | bc -l | cut -d. -f1)
        concurrent_level=$((concurrent_level > 0 ? concurrent_level : 1))
    else
        # Maintain max concurrency
        concurrent_level=$MAX_CONCURRENT
    fi
    
    # Progress reporting
    if [ $((current_time - last_progress_time)) -ge $PROGRESS_INTERVAL ]; then
        echo "Progress: ${elapsed}s/${TEST_DURATION}s - Current concurrency: $concurrent_level"
        last_progress_time=$current_time
        
        # Show real-time stats
        if [ -f "$results_file" ]; then
            total_requests=$(tail -n +2 "$results_file" | wc -l | tr -d ' ')
            success_count=$(grep ',200,' "$results_file" 2>/dev/null | wc -l | tr -d ' ')
            circuit_breaker_count=$(grep ',503,' "$results_file" 2>/dev/null | wc -l | tr -d ' ')
            echo "  Requests so far: $total_requests (Success: $success_count, Circuit breaker: $circuit_breaker_count)"
        fi
        echo ""
    fi
    
    # Run requests at current concurrency level
    run_concurrent_requests "$concurrent_level"
    
    # Small delay to prevent overwhelming the system
    sleep 0.1
done

echo "Load test completed! Results saved to $results_file"
echo ""
echo "=== Final Test Summary ==="
if [ -f "$results_file" ]; then
    total_requests=$(tail -n +2 "$results_file" | wc -l | tr -d ' ')
    success_count=$(grep ',200,' "$results_file" 2>/dev/null | wc -l | tr -d ' ')
    circuit_breaker_count=$(grep ',503,' "$results_file" 2>/dev/null | wc -l | tr -d ' ')
    error_count=$(tail -n +2 "$results_file" | grep -v ',200,' | grep -v ',503,' | wc -l | tr -d ' ')
    
    echo "Total requests: $total_requests"
    echo "Successful responses (200): $success_count"
    echo "Circuit breaker responses (503): $circuit_breaker_count"
    echo "Other errors: $error_count"
    
    if [ "$success_count" -gt 0 ] && [ "$circuit_breaker_count" -gt 0 ]; then
        echo ""
        echo "✅ Circuit breaker appears to be working!"
        echo "   Both successful requests and circuit breaker activations were observed."
    elif [ "$circuit_breaker_count" -gt 0 ]; then
        echo ""
        echo "⚠️  Circuit breaker activated but no successful requests observed."
        echo "   This might indicate the circuit breaker threshold is too low."
    elif [ "$success_count" -gt 0 ]; then
        echo ""
        echo "⚠️  All requests succeeded - circuit breaker may not have activated."
        echo "   Consider increasing load or checking circuit breaker configuration."
    else
        echo ""
        echo "❌ No successful requests - there might be a service issue."
    fi
    
    echo ""
    echo "Run the analysis script below for detailed results."
fi
```

### Verify Circuit Breaker Behavior

After running the load test, analyze the results to prove the circuit breaker is working:

```bash
#!/bin/bash

# Circuit Breaker Results Analysis Script
# Usage: ./analyze_results.sh [results_file.csv]

results_file="${1:-circuit_breaker_results_*.csv}"

# Find the most recent results file if pattern is used
if [[ "$results_file" == *"*"* ]]; then
  results_file=$(ls -t circuit_breaker_results_*.csv 2>/dev/null | head -n1)
fi

if [[ ! -f "$results_file" ]]; then
  echo "❌ No results file found. Please run the load test first."
  exit 1
fi

echo "📊 Analyzing circuit breaker results from: $results_file"
echo ""

# Basic statistics
total_requests=$(tail -n +2 "$results_file" | wc -l | tr -d ' ')
success_count=$(grep ',200,' "$results_file" 2>/dev/null | wc -l | tr -d ' ')
circuit_breaker_count=$(grep ',503,' "$results_file" 2>/dev/null | wc -l | tr -d ' ')
error_count=$(tail -n +2 "$results_file" | grep -v ',200,' | grep -v ',503,' | wc -l | tr -d ' ')

echo "=== 📈 Overall Statistics ==="
echo "Total requests: $total_requests"
echo "Successful responses (200): $success_count ($(( success_count * 100 / total_requests ))%)"
echo "Circuit breaker responses (503): $circuit_breaker_count ($(( circuit_breaker_count * 100 / total_requests ))%)"
echo "Other errors: $error_count ($(( error_count * 100 / total_requests ))%)"
echo ""

# Response time analysis
echo "=== ⏱️ Response Time Analysis ==="
if command -v awk >/dev/null 2>&1; then
  success_avg_time=$(grep ',200,' "$results_file" | awk -F',' '{sum+=$3; count++} END {if(count>0) printf "%.3f", sum/count; else print "N/A"}')
  circuit_breaker_avg_time=$(grep ',503,' "$results_file" | awk -F',' '{sum+=$3; count++} END {if(count>0) printf "%.3f", sum/count; else print "N/A"}')
  
  echo "Average response time for successful requests (200): ${success_avg_time}s"
  echo "Average response time for circuit breaker responses (503): ${circuit_breaker_avg_time}s"
  
  # Calculate speed difference
  if [[ "$success_avg_time" != "N/A" && "$circuit_breaker_avg_time" != "N/A" ]]; then
    speed_improvement=$(awk "BEGIN {printf \"%.1f\", $success_avg_time/$circuit_breaker_avg_time}")
    echo "🚀 Circuit breaker responses are ${speed_improvement}x faster (fail-fast behavior)"
  fi
else
  echo "awk not available - skipping response time analysis"
fi
echo ""

# Timeline analysis
echo "=== 📅 Response Timeline Analysis ==="
echo "Timestamp (seconds) | Response Code | Response Time | Concurrency"
echo "-------------------+---------------+---------------+------------"

tail -n +2 "$results_file" | head -20 | while IFS=',' read timestamp code time concurrent; do
  # Convert timestamp to relative time (first request = 0)
  if [[ -z "$first_timestamp" ]]; then
    first_timestamp=$timestamp
  fi
  relative_time=$((timestamp - first_timestamp))
  
  printf "%18d | %13s | %11.3fs | %10s\n" "$relative_time" "$code" "$time" "$concurrent"
done

if [[ $total_requests -gt 20 ]]; then
  echo "... (showing first 20 requests, total: $total_requests)"
fi
echo ""

# Concurrency level analysis
echo "=== 🔄 Concurrency Level Impact ==="
if command -v awk >/dev/null 2>&1; then
  echo "Concurrency | Total | Success | Circuit Breaker | Success Rate"
  echo "------------|-------|---------|-----------------|-------------"
  
  tail -n +2 "$results_file" | awk -F',' '
  {
    concurrency = $4
    total[concurrency]++
    if ($2 == "200") success[concurrency]++
    if ($2 == "503") cb[concurrency]++
  }
  END {
    for (c in total) {
      success_rate = (success[c] > 0) ? int(success[c] * 100 / total[c]) : 0
      printf "%10s | %5d | %7d | %14d | %10s%%\n", c, total[c], success[c]+0, cb[c]+0, success_rate
    }
  }' | sort -n
else
  echo "awk not available - skipping concurrency analysis"
fi
echo ""

# Circuit breaker effectiveness assessment
echo "=== 🛡️ Circuit Breaker Effectiveness Assessment ==="
if [[ "$success_count" -gt 0 && "$circuit_breaker_count" -gt 0 ]]; then
  echo "✅ Circuit breaker is working correctly!"
  echo "   📊 Evidence:"
  echo "   • Mixed responses (success + circuit breaker) observed"
  echo "   • Circuit breaker activated under load"
  echo "   • Fast-fail behavior protecting upstream service"
  
  # Calculate protection level
  protection_rate=$(( circuit_breaker_count * 100 / (success_count + circuit_breaker_count) ))
  echo "   • Protection rate: ${protection_rate}% of overload requests blocked"
  
elif [[ "$circuit_breaker_count" -gt 0 ]]; then
  echo "⚠️  Circuit breaker activated but no successful requests"
  echo "   🔍 Possible issues:"
  echo "   • Circuit breaker threshold might be too low"
  echo "   • Service might be completely unavailable"
  echo "   • Configuration issue with the service"
  
elif [[ "$success_count" -gt 0 ]]; then
  echo "⚠️  No circuit breaker activation observed"
  echo "   🔍 Possible reasons:"
  echo "   • Load was insufficient to trigger circuit breaker"
  echo "   • Circuit breaker thresholds are too high"
  echo "   • Circuit breaker might not be properly configured"
  echo "   💡 Try increasing MAX_CONCURRENT in the load test"
  
else
  echo "❌ No successful requests - service appears to be down"
  echo "   🔍 Check:"
  echo "   • Service deployment status"
  echo "   • Network connectivity"
  echo "   • Service binding configuration"
fi

echo ""
echo "=== 💡 Recommendations ==="
if [[ "$success_count" -gt 0 && "$circuit_breaker_count" -gt 0 ]]; then
  echo "• Circuit breaker is functioning well"
  echo "• Monitor service logs to ensure upstream protection"
  echo "• Consider adjusting thresholds based on service capacity"
elif [[ "$circuit_breaker_count" -eq 0 ]]; then
  echo "• Increase load test concurrency (MAX_CONCURRENT parameter)"
  echo "• Verify circuit breaker configuration in APIClass"
  echo "• Check if circuit breaker is enabled in the gateway"
else
  echo "• Investigate service health and availability"
  echo "• Review circuit breaker threshold configuration"
  echo "• Check service binding and gateway configuration"
fi

echo ""
echo "📁 Full results available in: $results_file"
echo "🔍 Use 'tail -f $results_file' to see raw data"
```

### Real-time Monitoring During Load Test

While running the load test, monitor the circuit breaker in real-time:

```bash
#!/bin/bash

# Real-time Circuit Breaker Monitoring Script
# Usage: Run this in a separate terminal while the load test is running

echo "🔍 Starting real-time circuit breaker monitoring..."
echo "📊 This script will monitor:"
echo "   • Gateway logs for circuit breaker events"
echo "   • Service response codes and times"
echo "   • Load test results in real-time"
echo ""

# Configuration
SERVICE_NAME="reading-list-service-circuit-breaker"
NAMESPACE="openchoreo-data-plane"
MONITORING_INTERVAL=6  # seconds

# Get service endpoint
endpoint="$(kubectl get servicebinding $SERVICE_NAME -o jsonpath='{.status.endpoints[0].public.uri}' 2>/dev/null)/books"

if [[ -z "${endpoint}" || "$endpoint" == "/books" ]]; then
  echo "⚠️  Could not resolve service endpoint. Monitoring will be limited."
  endpoint="https://localhost:8443/default/$SERVICE_NAME/api/v1/reading-list/books"
  echo "Using fallback endpoint: $endpoint"
fi

echo "📡 Monitoring endpoint: $endpoint"
echo ""

# Function to check gateway logs
monitor_gateway_logs() {
  echo "=== 🚪 Gateway Logs (Circuit Breaker Events) ==="
  kubectl logs -l app=choreo-external-gateway -n $NAMESPACE --tail=10 2>/dev/null | \
    grep -E "(overflow|circuit|503|upstream_reset)" || echo "No circuit breaker events detected yet"
  echo ""
}

# Function to test service responsiveness
test_service_response() {
  echo "=== 🌐 Service Response Test ==="
  local start_time end_time response_time response_code
  
  start_time=$(date +%s.%N)
  response_code=$(curl -k -s -w "%{http_code}" -o /dev/null "$endpoint" 2>/dev/null || echo "000")
  end_time=$(date +%s.%N)
  response_time=$(echo "$end_time - $start_time" | bc -l 2>/dev/null || echo "0")
  
  printf "Response: %s | Time: %.3fs" "$response_code" "$response_time"
  
  case $response_code in
    200) echo " ✅ (Success)" ;;
    503) echo " 🔴 (Circuit Breaker)" ;;
    000) echo " ❌ (Connection Failed)" ;;
    *) echo " ⚠️  (Other Error)" ;;
  esac
  echo ""
}

# Function to monitor load test results
monitor_load_test_results() {
  echo "=== 📈 Load Test Progress ==="
  local latest_results=$(ls -t circuit_breaker_results_*.csv 2>/dev/null | head -n1)
  
  if [[ -n "$latest_results" && -f "$latest_results" ]]; then
    local total_requests=$(tail -n +2 "$latest_results" | wc -l | tr -d ' ')
    local success_count=$(grep ',200,' "$latest_results" 2>/dev/null | wc -l | tr -d ' ')
    local circuit_breaker_count=$(grep ',503,' "$latest_results" 2>/dev/null | wc -l | tr -d ' ')
    
    if [[ $total_requests -gt 0 ]]; then
      local success_rate=$(( success_count * 100 / total_requests ))
      local cb_rate=$(( circuit_breaker_count * 100 / total_requests ))
      
      echo "Total requests: $total_requests"
      echo "✅ Success: $success_count (${success_rate}%)"
      echo "🔴 Circuit breaker: $circuit_breaker_count (${cb_rate}%)"
      
      # Show trend (last 10 requests)
      echo ""
      echo "Recent request pattern (last 10):"
      tail -10 "$latest_results" | cut -d',' -f2 | tr '\n' ' ' | sed 's/200/✅/g' | sed 's/503/🔴/g'
      echo ""
    else
      echo "No requests recorded yet in $latest_results"
    fi
  else
    echo "No load test results file found. Start the load test first."
  fi
  echo ""
}

# Function to show cluster resource usage
monitor_cluster_resources() {
  echo "=== 🖥️ Cluster Resource Monitoring ==="
  
  # Check service pods
  echo "Service pod status:"
  kubectl get pods -l app.kubernetes.io/name=$SERVICE_NAME --no-headers 2>/dev/null | \
    awk '{printf "  Pod: %s | Status: %s | Restarts: %s\n", $1, $3, $4}' || \
    echo "  Service pods not found or not labeled correctly"
  
  # Check gateway pods
  echo ""
  echo "Gateway pod status:"
  kubectl get pods -l app=choreo-external-gateway -n $NAMESPACE --no-headers 2>/dev/null | \
    awk '{printf "  Gateway: %s | Status: %s | Restarts: %s\n", $1, $3, $4}' || \
    echo "  Gateway pods not found"
  echo ""
}

# Main monitoring loop
echo "🚀 Starting continuous monitoring (Press Ctrl+C to stop)..."
echo "===================================================================================="

trap 'echo ""; echo "👋 Monitoring stopped"; exit 0' INT

monitor_counter=0
while true; do
  clear
  echo "🔍 Circuit Breaker Real-time Monitor - $(date)"
  echo "Press Ctrl+C to stop"
  echo "===================================================================================="
  echo ""
  
  # Test service first (quick check)
  test_service_response
  
  # Show load test progress
  monitor_load_test_results
  
  # Every 3rd iteration, show more detailed info
  if (( monitor_counter % 3 == 0 )); then
    monitor_gateway_logs
    monitor_cluster_resources
  fi
  
  echo "Next update in ${MONITORING_INTERVAL}s..."
  sleep $MONITORING_INTERVAL
  ((monitor_counter++))
done
```

**Usage Instructions:**

1. **Terminal 1 - Load Test**: Run the load test script
2. **Terminal 2 - Real-time Monitor**: Run the monitoring script above
3. **Terminal 3 - Gateway Logs** (Optional): 
```bash
kubectl logs -l app=choreo-external-gateway -n openchoreo-data-plane -f | grep -E "(overflow|circuit|503)"
```

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
kubectl delete -f reading-list-service-with-circuit-breaker.yaml
```
