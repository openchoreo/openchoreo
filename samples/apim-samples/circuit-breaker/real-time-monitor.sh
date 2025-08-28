#!/bin/bash

# Real-time Circuit Breaker Monitoring Script
# Usage: Run this in a separate terminal while the load test is running

echo "Starting real-time circuit breaker monitoring..."
echo "This script will monitor:"
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
  echo "WARNING: Could not resolve service endpoint. Monitoring will be limited."
  endpoint="https://localhost:8443/default/$SERVICE_NAME/api/v1/reading-list/books"
  echo "Using fallback endpoint: $endpoint"
fi

echo "Monitoring endpoint: $endpoint"
echo ""

# Function to check gateway logs
monitor_gateway_logs() {
  echo "=== Gateway Logs (Circuit Breaker Events) ==="
  kubectl logs -l app=choreo-external-gateway -n $NAMESPACE --tail=10 2>/dev/null | \
    grep -E "(overflow|circuit|503|upstream_reset)" || echo "No circuit breaker events detected yet"
  echo ""
}

# Function to test service responsiveness
test_service_response() {
  echo "=== Service Response Test ==="
  local start_time end_time response_time response_code

  start_time=$(date +%s.%N)
  response_code=$(curl -k -s -w "%{http_code}" -o /dev/null "$endpoint" 2>/dev/null || echo "000")
  end_time=$(date +%s.%N)
  response_time=$(echo "$end_time - $start_time" | bc -l 2>/dev/null || echo "0")

  printf "Response: %s | Time: %.3fs" "$response_code" "$response_time"

  case $response_code in
    200) echo " SUCCESS (Success)" ;;
    503) echo " CB (Circuit Breaker)" ;;
    000) echo " ERROR (Connection Failed)" ;;
    *) echo " WARNING (Other Error)" ;;
  esac
  echo ""
}

# Function to monitor load test results
monitor_load_test_results() {
  echo "=== Load Test Progress ==="
  local latest_results=$(ls -t circuit_breaker_results_*.csv 2>/dev/null | head -n1)

  if [[ -n "$latest_results" && -f "$latest_results" ]]; then
    local total_requests=$(tail -n +2 "$latest_results" | wc -l | tr -d ' ')
    local success_count=$(grep ',200,' "$latest_results" 2>/dev/null | wc -l | tr -d ' ')
    local circuit_breaker_count=$(grep ',503,' "$latest_results" 2>/dev/null | wc -l | tr -d ' ')

    if [[ $total_requests -gt 0 ]]; then
      local success_rate=$(( success_count * 100 / total_requests ))
      local cb_rate=$(( circuit_breaker_count * 100 / total_requests ))

      echo "Total requests: $total_requests"
      echo "Success: $success_count (${success_rate}%)"
      echo "Circuit breaker: $circuit_breaker_count (${cb_rate}%)"

      # Show trend (last 10 requests)
      echo ""
      echo "Recent request pattern (last 10):"
      tail -10 "$latest_results" | cut -d',' -f2 | tr '\n' ' ' | sed 's/200/OK/g' | sed 's/503/CB/g'
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
  echo "=== Cluster Resource Monitoring ==="

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
echo "Starting continuous monitoring (Press Ctrl+C to stop)..."
echo "===================================================================================="

trap 'echo ""; echo "Monitoring stopped"; exit 0' INT

monitor_counter=0
while true; do
  clear
  echo "Circuit Breaker Real-time Monitor - $(date)"
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
