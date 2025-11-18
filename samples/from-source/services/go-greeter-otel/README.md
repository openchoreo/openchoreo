# Go Greeter Service with OpenTelemetry

這是一個集成了 OpenTelemetry 的 Go Greeter Service 範例。

## 功能特性

- ✅ 自動 HTTP tracing
- ✅ 自定義 span 創建
- ✅ 跨服務 trace 傳播
- ✅ Kubernetes 元數據自動注入
- ✅ 優雅關閉處理

## 本地運行

### 1. 安裝依賴

```bash
go mod download
```

### 2. 設置環境變數

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="localhost:4317"
export OTEL_SERVICE_NAME="greeter-service"
export SERVICE_VERSION="1.0.0"
export DEPLOYMENT_ENV="local"
export PORT="9090"
```

### 3. 運行服務

```bash
go run main.go
```

### 4. 測試

```bash
# 基本問候
curl http://localhost:9090/greeter/greet

# 帶名字的問候
curl http://localhost:9090/greeter/greet?name=Alice

# 健康檢查
curl http://localhost:9090/greeter/health
```

## Docker 構建

```bash
docker build -t greeter-service:latest .
docker run -p 9090:9090 \
  -e OTEL_EXPORTER_OTLP_ENDPOINT="host.docker.internal:4317" \
  -e OTEL_SERVICE_NAME="greeter-service" \
  greeter-service:latest
```

## Kubernetes 部署

### 1. 標準 Deployment (openchoreo-data-plane)

`deployment.yaml` 內已包含：
- `Secret hyperdx-ingestion`：將 `stringData.api-key` 改為你的 HyperDX Ingestion API Key。
- `Deployment go-greeter`：內建 OTEL 影像與 `HYPERDX_API_KEY`、`OTEL_EXPORTER_OTLP_ENDPOINT` 設定。
- `Service go-greeter`：在 cluster 內以 TCP 9090 暴露服務。

部署方式：
```bash
kubectl apply -f deployment.yaml
```

### 2. OpenChoreo Workload

`workload-with-otel.yaml` 同時建立 Component/Workload/Service：
- 將 `hyperdx-ingestion` Secret 先建立於 `default` namespace。
- 依需要調整 `image` 或 `OTEL_EXPORTER_OTLP_ENDPOINT`。

部署方式：
```bash
kubectl apply -f workload-with-otel.yaml
```

關鍵環境變數：
- `OTEL_EXPORTER_OTLP_ENDPOINT`: HyperDX OTEL Collector 端點
- `OTEL_SERVICE_NAME`: 服務名稱
- `SERVICE_VERSION`: 服務版本
- `DEPLOYMENT_ENV`: 部署環境
- `HYPERDX_API_KEY`: 透過 Secret 注入的 Ingestion API Key

## Trace 範例

每個 HTTP 請求會產生類似以下的 trace 結構：

```
greet (HTTP span)
  └─ generate-greeting
       └─ format-greeting
```

## 響應範例

```json
{
  "message": "Good afternoon, Alice!",
  "timestamp": "2025-11-17T14:30:00Z",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736"
}
```

## 在 ClickHouse 中查詢

```sql
SELECT
    Timestamp,
    TraceId,
    SpanName,
    ServiceName,
    SpanAttributes['user.name'] as UserName,
    Duration
FROM otel_traces
WHERE ServiceName = 'greeter-service'
ORDER BY Timestamp DESC
LIMIT 10;
```
