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
        echo "SUCCESS: Circuit breaker appears to be working!"
        echo "   Both successful requests and circuit breaker activations were observed."
    elif [ "$circuit_breaker_count" -gt 0 ]; then
        echo ""
        echo "WARNING: Circuit breaker activated but no successful requests observed."
        echo "   This might indicate the circuit breaker threshold is too low."
    elif [ "$success_count" -gt 0 ]; then
        echo ""
        echo "WARNING: All requests succeeded - circuit breaker may not have activated."
        echo "   Consider increasing load or checking circuit breaker configuration."
    else
        echo ""
        echo "ERROR: No successful requests - there might be a service issue."
    fi

    echo ""
    echo "Run the analysis script below for detailed results."
fi
