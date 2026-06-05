# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import json
import logging
from collections.abc import Awaitable, Callable
from typing import Any

import numpy as np
from langchain.agents.middleware import AgentMiddleware
from langchain.messages import ToolMessage
from langchain.tools.tool_node import ToolCallRequest
from langgraph.types import Command

from src.agent.tool_registry import TOOLS
from src.template_manager import render

logger = logging.getLogger(__name__)


def _process_logs(content: dict[str, Any]) -> str:
    try:
        logs = content.get("logs", [])

        if not logs:
            return "No logs found"

        first_metadata = logs[0].get("metadata", {})

        logs_by_component: dict[str, dict[str, Any]] = {}
        for log in logs:
            log_metadata = log.get("metadata", {})
            component_name = log_metadata.get("componentName", "unknown")

            if component_name not in logs_by_component:
                logs_by_component[component_name] = {
                    "componentName": component_name,
                    "logs": [],
                }
            logs_by_component[component_name]["logs"].append(log)

        context = {
            "namespaceName": first_metadata.get("namespaceName", "N/A"),
            "projectName": first_metadata.get("projectName", "N/A"),
            "environmentName": first_metadata.get("environmentName", "N/A"),
            "components": list(logs_by_component.values()),
        }

        return render("middleware/logs.j2", context)
    except Exception as e:
        logger.error(f"Error processing logs: {e}")
        return json.dumps(content)


def _calculate_metric_stats(values: np.ndarray, timestamps: list[str]) -> dict | None:
    if len(values) == 0:
        return None

    mean = np.mean(values)
    std = np.std(values)

    stats = {
        "mean": float(mean),
        "median": float(np.median(values)),
        "min": float(np.min(values)),
        "max": float(np.max(values)),
        "std_dev": float(std),
        "coefficient_of_variation": float(std / mean) if mean != 0 else 0,
        "p90": float(np.percentile(values, 90)),
        "p95": float(np.percentile(values, 95)),
        "start_time": timestamps[0] if timestamps else None,
        "end_time": timestamps[-1] if timestamps else None,
    }

    return stats


def _detect_anomalies(values: np.ndarray, threshold: float = 3.0) -> dict:
    if len(values) < 2:
        return {"spike_count": 0, "max_spike_magnitude": 0, "largest_drop": 0}

    mean = np.mean(values)
    std = np.std(values)

    # Z-score anomalies
    z_scores = np.abs((values - mean) / std) if std > 0 else np.zeros_like(values)
    anomaly_indices = np.where(z_scores > threshold)[0]

    # Rate of change anomalies (percentage change between consecutive points)
    pct_changes = np.abs(np.diff(values) / (values[:-1] + 1e-10)) * 100
    large_changes = np.where(pct_changes > 50)[0]  # 50% change threshold

    all_anomaly_indices = np.unique(np.concatenate([anomaly_indices, large_changes]))

    # Calculate spike magnitude (max deviation from mean)
    max_spike_magnitude = float(np.max(z_scores)) if len(z_scores) > 0 else 0

    # Largest drop (most negative change)
    actual_changes = np.diff(values)
    largest_drop = float(np.min(actual_changes)) if len(actual_changes) > 0 else 0

    return {
        "spike_count": len(all_anomaly_indices),
        "max_spike_magnitude": max_spike_magnitude,
        "largest_drop": largest_drop,
    }


def _calculate_resource_pressure(
    usage_values: np.ndarray, request_values: np.ndarray, limit_values: np.ndarray
) -> dict | None:
    if len(usage_values) == 0:
        return None

    # Align arrays to minimum length
    usage_to_request = 0
    exceeded_requests = False
    if len(request_values) > 0:
        min_len = min(len(usage_values), len(request_values))
        aligned_usage = usage_values[:min_len]
        aligned_requests = request_values[:min_len]
        usage_to_request = np.mean(aligned_usage / (aligned_requests + 1e-10))
        exceeded_requests = np.any(aligned_usage > aligned_requests)

    usage_to_limit = 0
    exceeded_limits = False
    if len(limit_values) > 0:
        min_len = min(len(usage_values), len(limit_values))
        aligned_usage = usage_values[:min_len]
        aligned_limits = limit_values[:min_len]
        usage_to_limit = np.mean(aligned_usage / (aligned_limits + 1e-10))
        exceeded_limits = np.any(aligned_usage > aligned_limits)

    return {
        "avg_usage_to_request_ratio": float(usage_to_request),
        "avg_usage_to_limit_ratio": float(usage_to_limit),
        "exceeded_requests": bool(exceeded_requests),
        "exceeded_limits": bool(exceeded_limits),
    }


def _process_metrics(content: dict[str, Any]) -> str:
    try:
        logger.debug("_process_metrics called")

        # Extract time-series data
        metrics_data = {}
        timestamps = {}

        for metric_name in [
            "cpuUsage",
            "cpuRequests",
            "cpuLimits",
            "memoryUsage",
            "memoryRequests",
            "memoryLimits",
        ]:
            if metric_name in content and content[metric_name]:
                values = np.array([point["value"] for point in content[metric_name]])
                times = [point["timestamp"] for point in content[metric_name]]
                metrics_data[metric_name] = values
                timestamps[metric_name] = times

        if not metrics_data:
            return "No metrics data available"

        # Calculate statistics only for usage metrics (not for constant config values)
        stats = {}
        anomalies = {}
        config_values = {}

        for metric_name, values in metrics_data.items():
            if metric_name in ["cpuUsage", "memoryUsage"]:
                # Full statistical analysis for usage metrics
                stats[metric_name] = _calculate_metric_stats(values, timestamps[metric_name])
                anomalies[metric_name] = _detect_anomalies(values)
            elif metric_name in ["cpuRequests", "cpuLimits", "memoryRequests", "memoryLimits"]:
                # Just extract the constant value for config values
                config_values[metric_name] = float(values[0]) if len(values) > 0 else None

        # Calculate resource pressure
        cpu_pressure = None
        memory_pressure = None

        if "cpuUsage" in metrics_data:
            cpu_pressure = _calculate_resource_pressure(
                metrics_data.get("cpuUsage", np.array([])),
                metrics_data.get("cpuRequests", np.array([])),
                metrics_data.get("cpuLimits", np.array([])),
            )

        if "memoryUsage" in metrics_data:
            memory_pressure = _calculate_resource_pressure(
                metrics_data.get("memoryUsage", np.array([])),
                metrics_data.get("memoryRequests", np.array([])),
                metrics_data.get("memoryLimits", np.array([])),
            )

        # Calculate correlations
        correlations = {}
        if "cpuUsage" in metrics_data and "memoryUsage" in metrics_data:
            # Align the arrays (in case they have different lengths)
            min_len = min(len(metrics_data["cpuUsage"]), len(metrics_data["memoryUsage"]))
            cpu_usage = metrics_data["cpuUsage"][:min_len]
            memory = metrics_data["memoryUsage"][:min_len]

            if len(cpu_usage) > 1:
                corr_matrix = np.corrcoef(cpu_usage, memory)
                correlations["cpu_memory"] = float(corr_matrix[0, 1])

        # Prepare context for template
        context = {
            "stats": stats,
            "anomalies": anomalies,
            "config_values": config_values,
            "cpu_pressure": cpu_pressure,
            "memory_pressure": memory_pressure,
            "correlations": correlations,
        }

        return render("middleware/metrics.j2", context)

    except Exception as e:
        logger.error(f"Error processing metrics: {e}", exc_info=True)
        return json.dumps(content)


def _process_traces(content: dict[str, Any]) -> str:
    try:
        traces = content.get("traces", [])

        if not traces:
            return "No traces found"

        context = {
            "traces": traces,
            "total": content.get("total", len(traces)),
        }

        return render("middleware/traces.j2", context)
    except Exception as e:
        logger.error(f"Error processing traces: {e}")
        return json.dumps(content)


def _build_span_tree(spans: list[dict[str, Any]]) -> list[dict[str, Any]]:
    span_map = {span["spanId"]: span for span in spans}

    root_spans = []
    for span in spans:
        parent_id = span.get("parentSpanId")
        if not parent_id or parent_id not in span_map:
            root_spans.append(span["spanId"])

    result: list[dict[str, Any]] = []

    def walk(span_id: str, depth: int = 0):
        if span_id not in span_map:
            return
        span = span_map[span_id]

        attrs = span.get("attributes", {})
        resource_attrs = span.get("resourceAttributes", {})

        # Pass through all application attributes, dropping only internal/metadata keys
        relevant_attrs = {
            k: v for k, v in attrs.items() if k not in ("data_stream",)
        }

        result.append(
            {
                "spanId": span["spanId"],
                "spanName": span["spanName"],
                "serviceName": resource_attrs.get("service.name", "unknown"),
                "component": resource_attrs.get("openchoreo.dev/component"),
                "project": resource_attrs.get("openchoreo.dev/project"),
                "namespace": resource_attrs.get("openchoreo.dev/namespace"),
                "durationNs": span.get("durationNs", 0),
                "startTime": span.get("startTime"),
                "depth": depth,
                "attributes": relevant_attrs,
            }
        )

        children = [s for s in spans if s.get("parentSpanId") == span_id]
        children.sort(key=lambda s: s.get("startTime", ""))
        for child in children:
            walk(child["spanId"], depth + 1)

    root_spans.sort(key=lambda sid: span_map[sid].get("startTime", ""))
    for root_id in root_spans:
        walk(root_id)

    return result


def _process_trace_spans(content: dict[str, Any]) -> str:
    try:
        spans = content.get("spans", [])

        if not spans:
            return "No spans found"

        span_tree = _build_span_tree(spans)

        context = {
            "spans": span_tree,
            "total": content.get("total", len(spans)),
        }

        return render("middleware/trace_spans.j2", context)
    except Exception as e:
        logger.error(f"Error processing trace spans: {e}")
        return json.dumps(content)


def get_processor(tool_name: str | None) -> Callable[[dict[str, Any]], str]:
    processors: dict[str, Callable[[dict[str, Any]], str]] = {
        TOOLS.QUERY_COMPONENT_LOGS: _process_logs,
        TOOLS.QUERY_RESOURCE_METRICS: _process_metrics,
        TOOLS.QUERY_TRACES: _process_traces,
        TOOLS.QUERY_TRACE_SPANS: _process_trace_spans,
    }
    if tool_name and tool_name in processors:
        return processors[tool_name]
    return lambda content: json.dumps(content)


def _extract_content(content: Any) -> dict[str, Any] | None:
    if isinstance(content, dict):
        return content

    if isinstance(content, list):
        for block in content:
            if isinstance(block, dict) and block.get("type") == "text":
                try:
                    parsed = json.loads(block["text"])
                    if isinstance(parsed, dict):
                        return parsed
                except json.JSONDecodeError, KeyError, TypeError:
                    continue

    return None


def _to_mcp_content(text: str) -> list[dict[str, str]]:
    return [{"type": "text", "text": text}]


class OutputTransformerMiddleware(AgentMiddleware):
    async def awrap_tool_call(
        self,
        request: ToolCallRequest,
        handler: Callable[[ToolCallRequest], Awaitable[ToolMessage | Command]],
    ) -> ToolMessage | Command:
        result = await handler(request)

        if not isinstance(result, ToolMessage):
            return result

        content = _extract_content(result.content)
        if content is None:
            return result

        tool_name = request.tool_call.get("name")
        logger.info(f"OutputTransformerMiddleware processing tool: {tool_name}")

        try:
            processor = get_processor(tool_name)
            processed_text = processor(content)
            processed_content = _to_mcp_content(processed_text)
            logger.debug(f"Processed content length: {len(processed_text)}")
        except Exception as e:
            logger.error(f"Error processing tool output: {e}", exc_info=True)
            return result

        return ToolMessage(  # type: ignore[no-matching-overload]
            content=processed_content,
            tool_call_id=result.tool_call_id,
            name=result.name,
        )
