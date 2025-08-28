#!/bin/bash

# Circuit Breaker Results Analysis Script
# Usage: ./analyze_results.sh [results_file.csv]

results_file="${1:-circuit_breaker_results_*.csv}"

# Find the most recent results file if pattern is used
if [[ "$results_file" == *"*"* ]]; then
  results_file=$(ls -t circuit_breaker_results_*.csv 2>/dev/null | head -n1)
fi

if [[ ! -f "$results_file" ]]; then
  echo "ERROR: No results file found. Please run the load test first."
  exit 1
fi

echo "Analyzing circuit breaker results from: $results_file"
echo ""

# Basic statistics
total_requests=$(tail -n +2 "$results_file" | wc -l | tr -d ' ')
success_count=$(grep ',200,' "$results_file" 2>/dev/null | wc -l | tr -d ' ')
circuit_breaker_count=$(grep ',503,' "$results_file" 2>/dev/null | wc -l | tr -d ' ')
error_count=$(tail -n +2 "$results_file" | grep -v ',200,' | grep -v ',503,' | wc -l | tr -d ' ')

echo "=== Overall Statistics ==="
echo "Total requests: $total_requests"
echo "Successful responses (200): $success_count ($(( success_count * 100 / total_requests ))%)"
echo "Circuit breaker responses (503): $circuit_breaker_count ($(( circuit_breaker_count * 100 / total_requests ))%)"
echo "Other errors: $error_count ($(( error_count * 100 / total_requests ))%)"
echo ""

# Response time analysis
echo "=== Response Time Analysis ==="
if command -v awk >/dev/null 2>&1; then
  success_avg_time=$(grep ',200,' "$results_file" | awk -F',' '{sum+=$3; count++} END {if(count>0) printf "%.3f", sum/count; else print "N/A"}')
  circuit_breaker_avg_time=$(grep ',503,' "$results_file" | awk -F',' '{sum+=$3; count++} END {if(count>0) printf "%.3f", sum/count; else print "N/A"}')

  echo "Average response time for successful requests (200): ${success_avg_time}s"
  echo "Average response time for circuit breaker responses (503): ${circuit_breaker_avg_time}s"

  # Calculate speed difference
  if [[ "$success_avg_time" != "N/A" && "$circuit_breaker_avg_time" != "N/A" ]]; then
    speed_improvement=$(awk "BEGIN {printf \"%.1f\", $success_avg_time/$circuit_breaker_avg_time}")
    echo "Circuit breaker responses are ${speed_improvement}x faster (fail-fast behavior)"
  fi
else
  echo "awk not available - skipping response time analysis"
fi
echo ""

# Timeline analysis
echo "=== Response Timeline Analysis ==="
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
echo "=== Concurrency Level Impact ==="
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
echo "=== Circuit Breaker Effectiveness Assessment ==="
if [[ "$success_count" -gt 0 && "$circuit_breaker_count" -gt 0 ]]; then
  echo "SUCCESS: Circuit breaker is working correctly!"
  echo "   Evidence:"
  echo "   • Mixed responses (success + circuit breaker) observed"
  echo "   • Circuit breaker activated under load"
  echo "   • Fast-fail behavior protecting upstream service"

  # Calculate protection level
  protection_rate=$(( circuit_breaker_count * 100 / (success_count + circuit_breaker_count) ))
  echo "   • Protection rate: ${protection_rate}% of overload requests blocked"

elif [[ "$circuit_breaker_count" -gt 0 ]]; then
  echo "WARNING: Circuit breaker activated but no successful requests"
  echo "   Possible issues:"
  echo "   • Circuit breaker threshold might be too low"
  echo "   • Service might be completely unavailable"
  echo "   • Configuration issue with the service"

elif [[ "$success_count" -gt 0 ]]; then
  echo "WARNING: No circuit breaker activation observed"
  echo "   Possible reasons:"
  echo "   • Load was insufficient to trigger circuit breaker"
  echo "   • Circuit breaker thresholds are too high"
  echo "   • Circuit breaker might not be properly configured"
  echo "   TIP: Try increasing MAX_CONCURRENT in the load test"

else
  echo "ERROR: No successful requests - service appears to be down"
  echo "   Check:"
  echo "   • Service deployment status"
  echo "   • Network connectivity"
  echo "   • Service binding configuration"
fi

echo ""
echo "=== Recommendations ==="
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
echo "Full results available in: $results_file"
echo "Use 'tail -f $results_file' to see raw data"
