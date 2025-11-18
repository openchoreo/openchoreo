# Go Greeter Service - OpenTelemetry 架構

## 📐 整體架構

```
┌─────────────────────────────────────────────────────────────────┐
│                     OpenChoreo 集群                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │         openchoreo-data-plane namespace                   │  │
│  │                                                            │  │
│  │  ┌──────────────────────────────────────┐                │  │
│  │  │  Go Greeter Service Pod              │                │  │
│  │  │  ┌────────────────────────────────┐  │                │  │
│  │  │  │  main.go                       │  │                │  │
│  │  │  │  ┌──────────────────────────┐  │  │                │  │
│  │  │  │  │ HTTP Server              │  │  │                │  │
│  │  │  │  │  + otelhttp wrapper      │  │  │                │  │
│  │  │  │  └──────────────────────────┘  │  │                │  │
│  │  │  │                                 │  │                │  │
│  │  │  │  ┌──────────────────────────┐  │  │                │  │
│  │  │  │  │ OTEL SDK                 │  │  │                │  │
│  │  │  │  │  - TracerProvider        │  │  │                │  │
│  │  │  │  │  - OTLP Exporter (gRPC)  │  │  │                │  │
│  │  │  │  │  - Resource Detector     │  │  │                │  │
│  │  │  │  └──────────────────────────┘  │  │                │  │
│  │  │  └────────────────────────────────┘  │                │  │
│  │  │           │                           │                │  │
│  │  │           │ Traces (gRPC/4317)        │                │  │
│  │  │           ▼                           │                │  │
│  │  └───────────────────────────────────────┘                │  │
│  │                                                            │  │
│  └──────────────────────────────────────────────────────────┘  │
│                      │                                          │
│                      │                                          │
│  ┌───────────────────▼──────────────────────────────────────┐  │
│  │   openchoreo-observability-plane namespace               │  │
│  │                                                            │  │
│  │  ┌──────────────────────────────────────┐                │  │
│  │  │  ClickStack OTEL Collector           │                │  │
│  │  │  ┌────────────────────────────────┐  │                │  │
│  │  │  │ Receivers:                     │  │                │  │
│  │  │  │  - OTLP/gRPC  :4317           │  │                │  │
│  │  │  │  - OTLP/HTTP  :4318           │  │                │  │
│  │  │  │  - Prometheus :9090           │  │                │  │
│  │  │  └────────────────────────────────┘  │                │  │
│  │  │                                       │                │  │
│  │  │  ┌────────────────────────────────┐  │                │  │
│  │  │  │ Processors:                    │  │                │  │
│  │  │  │  - k8sattributes              │  │                │  │
│  │  │  │  - resourcedetection          │  │                │  │
│  │  │  │  - batch                      │  │                │  │
│  │  │  │  - filter/exclude-clickstack  │  │                │  │
│  │  │  └────────────────────────────────┘  │                │  │
│  │  │                                       │                │  │
│  │  │  ┌────────────────────────────────┐  │                │  │
│  │  │  │ Exporters:                     │  │                │  │
│  │  │  │  - ClickHouse                  │  │                │  │
│  │  │  │  - Logging (debug)             │  │                │  │
│  │  │  └────────────────────────────────┘  │                │  │
│  │  └───────────────┬───────────────────────┘                │  │
│  │                  │                                         │  │
│  │                  │ SQL INSERT                              │  │
│  │                  ▼                                         │  │
│  │  ┌──────────────────────────────────────┐                │  │
│  │  │  ClickHouse                          │                │  │
│  │  │  ┌────────────────────────────────┐  │                │  │
│  │  │  │ Database: observability        │  │                │  │
│  │  │  │                                 │  │                │  │
│  │  │  │ Tables:                        │  │                │  │
│  │  │  │  - otel_logs                   │  │                │  │
│  │  │  │  - otel_traces ◄───────────    │  │                │  │
│  │  │  │  - otel_metrics_*              │  │                │  │
│  │  │  └────────────────────────────────┘  │                │  │
│  │  └──────────────────────────────────────┘                │  │
│  │                                                            │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## 🔄 Trace 數據流

### 1. Request 進入 (User → Service)

```
curl /greeter/greet?name=Alice
    │
    ▼
Gateway (Envoy)
    │
    ▼
Greeter Service Pod
```

### 2. Span 創建 (Service 內部)

```go
// Root span (自動由 otelhttp 創建)
greet
  ├─ Attributes:
  │   └─ user.name: "Alice"
  │   └─ http.method: "GET"
  │   └─ http.path: "/greeter/greet"
  │
  └─ Child spans (手動創建)
      ├─ generate-greeting
      │   └─ format-greeting
      └─ ...
```

### 3. Trace 導出 (Service → Collector)

```
OTEL SDK (in service)
    │
    │ OTLP/gRPC (port 4317)
    │ Batch: 512 spans / 5s
    │
    ▼
clickstack-collector.openchoreo-observability-plane.svc.cluster.local:4317
```

### 4. 處理和豐富 (Collector)

```
Receiver (OTLP)
    │
    ▼
Processor (k8sattributes)
    │ 添加: pod name, namespace, labels
    │       openchoreo.organization
    │       openchoreo.project
    │       openchoreo.component
    ▼
Processor (resourcedetection)
    │ 添加: host, OS, container info
    ▼
Processor (batch)
    │ 批次處理: 1000 spans / 10s
    ▼
Exporter (ClickHouse)
```

### 5. 存儲 (Collector → ClickHouse)

```sql
-- ClickHouse 自動創建表結構
INSERT INTO observability.otel_traces (
    Timestamp,
    TraceId,
    SpanId,
    ParentSpanId,
    TraceState,
    SpanName,
    SpanKind,
    ServiceName,
    ResourceAttributes,
    SpanAttributes,
    Duration,
    StatusCode,
    StatusMessage,
    Events,
    Links
) VALUES (...);
```

### 6. 查詢和可視化

```
ClickHouse
    │
    ├─ SQL Query (直接查詢)
    │   └─ kubectl exec clickhouse-0 -- clickhouse-client
    │
    └─ HyperDX UI (可視化)
        └─ http://172.19.0.2:30580
```

## 📊 Trace 數據模型

### Span 結構

```json
{
  "TraceId": "4bf92f3577b34da6a3ce929d0e0e4736",
  "SpanId": "00f067aa0ba902b7",
  "ParentSpanId": "00f067aa0ba902b6",
  "SpanName": "greet",
  "SpanKind": "SPAN_KIND_SERVER",
  "ServiceName": "greeter-service",
  "ResourceAttributes": {
    "service.name": "greeter-service",
    "service.version": "1.0.0",
    "deployment.environment": "production",
    "k8s.pod.name": "greeter-service-abc123",
    "k8s.namespace.name": "default",
    "openchoreo.organization": "default",
    "openchoreo.project": "default",
    "openchoreo.component": "greeter-service"
  },
  "SpanAttributes": {
    "user.name": "Alice",
    "http.method": "GET",
    "http.path": "/greeter/greet",
    "http.status_code": 200
  },
  "Duration": 15000000,  // 15ms in nanoseconds
  "StatusCode": "STATUS_CODE_OK",
  "Events": [],
  "Links": []
}
```

## 🎯 關鍵組件配置

### Go Service 環境變數

| 變數 | 值 | 說明 |
|------|-----|------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `clickstack-collector...4317` | Collector gRPC 端點 |
| `OTEL_SERVICE_NAME` | `greeter-service` | 服務標識 |
| `SERVICE_VERSION` | `1.0.0` | 版本號 |
| `DEPLOYMENT_ENV` | `production` | 環境標識 |

### OTEL Collector 配置

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: "0.0.0.0:4317"
      http:
        endpoint: "0.0.0.0:4318"

processors:
  k8sattributes:
    extract:
      labels:
        - key: openchoreo.dev/organization
        - key: openchoreo.dev/project
        - key: openchoreo.dev/component
  
  batch:
    send_batch_size: 1000
    timeout: 10s

exporters:
  clickhouse:
    endpoint: "http://clickhouse:8123?database=observability"
    database: observability
    traces_table_name: otel_traces
```

## 🔗 網絡連接

### Service → Collector

- **協議**: gRPC (OTLP)
- **端點**: `clickstack-collector.openchoreo-observability-plane.svc.cluster.local:4317`
- **命名空間**: 跨 namespace (data-plane → observability-plane)
- **認證**: 無 (internal cluster traffic)

### Collector → ClickHouse

- **協議**: HTTP (ClickHouse native protocol)
- **端點**: `clickhouse:8123`
- **認證**: `default` / `clickstack-change-me`
- **數據庫**: `observability`

## 📈 性能考量

### Batching

- **Service 端**: OTEL SDK 自動批次 (512 spans / 5s)
- **Collector 端**: 批次處理器 (1000 spans / 10s)
- **好處**: 減少網絡開銷，提高吞吐量

### Sampling

- **當前配置**: `AlwaysSample()` (100% 採樣)
- **建議**: 生產環境使用機率採樣 (例如 10%)
- **配置**:
  ```go
  sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.1))
  ```

### 資源使用

| 組件 | CPU | Memory |
|------|-----|--------|
| Greeter Service | 100m-500m | 128Mi-512Mi |
| OTEL Collector | 500m-2000m | 1Gi-4Gi |
| ClickHouse | 1000m-2000m | 4Gi-8Gi |

## 🎓 最佳實踐

1. ✅ **使用語義化的 Span 名稱**
   - Good: `generate-greeting`, `format-greeting`
   - Bad: `func1`, `operation`

2. ✅ **添加有意義的 Attributes**
   ```go
   span.SetAttributes(
       attribute.String("user.name", name),
       attribute.Int("request.size", len(data)),
   )
   ```

3. ✅ **記錄關鍵事件**
   ```go
   span.AddEvent("cache_hit", trace.WithAttributes(
       attribute.String("cache.key", key),
   ))
   ```

4. ✅ **錯誤處理**
   ```go
   if err != nil {
       span.RecordError(err)
       span.SetStatus(codes.Error, err.Error())
   }
   ```

5. ✅ **Context 傳遞**
   ```go
   ctx, span := tracer.Start(ctx, "operation")
   defer span.End()
   // 將 ctx 傳給下游函數
   doSomething(ctx, ...)
   ```
