# Go Greeter Service - OpenTelemetry Traces 集成指南

## 概述

本指南說明如何配置 Go Greeter Service 將 traces 傳送到 ClickStack OpenTelemetry Collector。

## OTEL Collector 端點信息

### Cluster 內部訪問（推薦）
- **gRPC**: `clickstack-collector.openchoreo-observability-plane.svc.cluster.local:4317`
- **HTTP**: `clickstack-collector.openchoreo-observability-plane.svc.cluster.local:4318`

### NodePort 訪問（開發/測試）
- **gRPC**: `172.19.0.2:30317`
- **HTTP**: `172.19.0.2:30318`

---

## 實現步驟

### 1. Go 依賴套件

在 `go.mod` 中添加以下依賴：

```go
require (
    go.opentelemetry.io/otel v1.21.0
    go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.21.0
    go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.21.0
    go.opentelemetry.io/otel/sdk v1.21.0
    go.opentelemetry.io/otel/trace v1.21.0
    go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.46.1
    google.golang.org/grpc v1.59.0
)
```

### 2. 初始化 OpenTelemetry

創建 `tracing.go` 文件：

```go
package main

import (
    "context"
    "log"
    "os"
    "time"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/propagation"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

// InitTracer 初始化 OpenTelemetry tracer
func InitTracer(ctx context.Context) (*sdktrace.TracerProvider, error) {
    // 從環境變數獲取配置
    endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
    if endpoint == "" {
        endpoint = "clickstack-collector.openchoreo-observability-plane.svc.cluster.local:4317"
    }

    serviceName := os.Getenv("OTEL_SERVICE_NAME")
    if serviceName == "" {
        serviceName = "greeter-service"
    }

    // 創建 OTLP gRPC exporter
    exporter, err := otlptracegrpc.New(ctx,
        otlptracegrpc.WithEndpoint(endpoint),
        otlptracegrpc.WithInsecure(), // 使用非加密連接
        otlptracegrpc.WithDialOption(grpc.WithBlock()),
        otlptracegrpc.WithTimeout(5*time.Second),
    )
    if err != nil {
        return nil, err
    }

    // 創建 resource（服務元數據）
    res, err := resource.New(ctx,
        resource.WithAttributes(
            semconv.ServiceNameKey.String(serviceName),
            semconv.ServiceVersionKey.String(os.Getenv("SERVICE_VERSION")),
            semconv.DeploymentEnvironmentKey.String(os.Getenv("DEPLOYMENT_ENV")),
        ),
        resource.WithFromEnv(), // 自動從環境變數獲取 resource attributes
        resource.WithProcess(),
        resource.WithOS(),
        resource.WithContainer(),
        resource.WithHost(),
    )
    if err != nil {
        return nil, err
    }

    // 創建 TracerProvider
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter,
            sdktrace.WithMaxExportBatchSize(512),
            sdktrace.WithBatchTimeout(5*time.Second),
        ),
        sdktrace.WithResource(res),
        sdktrace.WithSampler(sdktrace.AlwaysSample()), // 開發環境全量採樣
    )

    // 設置全局 TracerProvider
    otel.SetTracerProvider(tp)

    // 設置全局 Propagator（用於跨服務追蹤）
    otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
        propagation.TraceContext{},
        propagation.Baggage{},
    ))

    log.Printf("OpenTelemetry initialized: endpoint=%s, service=%s", endpoint, serviceName)
    return tp, nil
}

// ShutdownTracer 優雅關閉 tracer
func ShutdownTracer(ctx context.Context, tp *sdktrace.TracerProvider) error {
    if err := tp.Shutdown(ctx); err != nil {
        log.Printf("Error shutting down tracer provider: %v", err)
        return err
    }
    return nil
}
```

### 3. 在 HTTP Server 中集成

修改 `main.go`：

```go
package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

func main() {
    ctx := context.Background()

    // 初始化 OpenTelemetry
    tp, err := InitTracer(ctx)
    if err != nil {
        log.Fatalf("Failed to initialize tracer: %v", err)
    }
    defer func() {
        shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        if err := ShutdownTracer(shutdownCtx, tp); err != nil {
            log.Printf("Error shutting down tracer: %v", err)
        }
    }()

    // 獲取 tracer 實例
    tracer = otel.Tracer("greeter-service")

    // 創建 HTTP 路由
    mux := http.NewServeMux()

    // 使用 otelhttp 包裝 handler，自動創建 spans
    mux.Handle("/greeter/greet", otelhttp.NewHandler(
        http.HandlerFunc(greetHandler),
        "greet",
    ))

    // 啟動 HTTP Server
    port := os.Getenv("PORT")
    if port == "" {
        port = "9090"
    }

    server := &http.Server{
        Addr:    ":" + port,
        Handler: mux,
    }

    // 優雅關閉
    go func() {
        sigCh := make(chan os.Signal, 1)
        signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
        <-sigCh
        log.Println("Shutting down server...")
        shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()
        if err := server.Shutdown(shutdownCtx); err != nil {
            log.Printf("Server shutdown error: %v", err)
        }
    }()

    log.Printf("Starting server on port %s", port)
    if err := server.ListenAndServe(); err != http.ErrServerClosed {
        log.Fatalf("Server failed: %v", err)
    }
}

// greetHandler 處理問候請求
func greetHandler(w http.ResponseWriter, r *http.Request) {
    // 從 context 中獲取 span（由 otelhttp 自動創建）
    ctx := r.Context()
    span := trace.SpanFromContext(ctx)

    // 添加自定義 attributes
    name := r.URL.Query().Get("name")
    if name == "" {
        name = "World"
    }
    span.SetAttributes(attribute.String("user.name", name))

    // 創建子 span 示範
    _, childSpan := tracer.Start(ctx, "generate-greeting")
    greeting := generateGreeting(name)
    childSpan.SetAttributes(attribute.String("greeting.message", greeting))
    childSpan.End()

    // 返回響應
    w.Header().Set("Content-Type", "text/plain")
    w.WriteHeader(http.StatusOK)
    w.Write([]byte(greeting))
}

func generateGreeting(name string) string {
    return "Hello, " + name + "!"
}
```

### 4. 更新 Workload YAML 配置

修改 `greeter-service.yaml`，添加必要的環境變數：

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: greeter-service
spec:
  owner:
    componentName: greeter-service
    projectName: default
  containers:
    main:
      image: ghcr.io/openchoreo/samples/greeter-service:latest
      command:
        - ./go-greeter
      args:
        - --port
        - "9090"
      env:
        # === 基本配置 ===
        - key: LOG_LEVEL
          value: info
        - key: PORT
          value: "9090"

        # === OpenTelemetry 配置 ===
        - key: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "clickstack-collector.openchoreo-observability-plane.svc.cluster.local:4317"

        - key: OTEL_SERVICE_NAME
          value: "greeter-service"

        - key: SERVICE_VERSION
          value: "1.0.0"

        - key: DEPLOYMENT_ENV
          value: "development"

        # OpenTelemetry Resource Attributes (自動注入 K8s 元數據)
        - key: OTEL_RESOURCE_ATTRIBUTES
          value: "service.namespace=default,deployment.environment=dev"

        # === GitHub 配置（如果需要）===
        - key: GITHUB_REPOSITORY
          valueFrom:
            configurationGroupRef:
              key: repository
              name: github
        - key: GITHUB_TOKEN
          valueFrom:
            configurationGroupRef:
              key: pat
              name: github

  endpoints:
    greeter-api:
      type: REST
      port: 9090
```

---

## 驗證 Traces

### 1. 部署服務

```bash
kubectl apply -f samples/from-image/go-greeter-service/greeter-service.yaml
```

### 2. 發送測試請求

```bash
# Port-forward gateway
kubectl port-forward -n openchoreo-data-plane svc/gateway-external 8443:443 &

# 發送請求
curl -k "$(kubectl get servicebinding greeter-service -o jsonpath='{.status.endpoints[0].public.uri}')/greet?name=Alice"
```

### 3. 檢查 ClickHouse 中的 Traces

```bash
# 進入 ClickHouse
kubectl exec -n openchoreo-observability-plane clickhouse-0 -it -- clickhouse-client \
  --user default --password clickstack-change-me --database observability

# 查詢最近的 traces
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

# 查看特定 trace 的所有 spans
SELECT
    SpanName,
    ParentSpanId,
    Duration,
    SpanAttributes
FROM otel_traces
WHERE TraceId = '<your-trace-id>'
ORDER BY Timestamp;
```

### 4. 使用 HyperDX UI 查看（如果已部署）

訪問 HyperDX UI：
```bash
# 獲取 NodePort
kubectl get svc -n openchoreo-observability-plane clickstack-hyperdx

# 訪問 http://172.19.0.2:30580
```

---

## 環境變數說明

| 變數名 | 說明 | 預設值 | 必填 |
|--------|------|--------|------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTEL Collector gRPC 端點 | `clickstack-collector...` | ✅ |
| `OTEL_SERVICE_NAME` | 服務名稱 | `greeter-service` | ✅ |
| `SERVICE_VERSION` | 服務版本 | - | ❌ |
| `DEPLOYMENT_ENV` | 部署環境 | - | ❌ |
| `OTEL_RESOURCE_ATTRIBUTES` | 額外的 resource attributes | - | ❌ |

---

## 進階配置

### 1. HTTP 協議（替代 gRPC）

```go
import "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"

exporter, err := otlptracehttp.New(ctx,
    otlptracehttp.WithEndpoint("clickstack-collector.openchoreo-observability-plane.svc.cluster.local:4318"),
    otlptracehttp.WithInsecure(),
)
```

### 2. 自定義採樣策略

```go
// 基於機率採樣（50%）
tp := sdktrace.NewTracerProvider(
    sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.5)),
    // ... 其他配置
)

// 基於父 span 決策
tp := sdktrace.NewTracerProvider(
    sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.5))),
    // ... 其他配置
)
```

### 3. 添加數據庫查詢 Tracing

```go
import "go.opentelemetry.io/contrib/instrumentation/database/sql/otelsql"

db, err := otelsql.Open("postgres", dataSourceName,
    otelsql.WithAttributes(
        semconv.DBSystemPostgreSQL,
    ),
)
```

### 4. 添加 HTTP Client Tracing

```go
import "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

client := &http.Client{
    Transport: otelhttp.NewTransport(http.DefaultTransport),
}
```

---

## 常見問題

### Q1: Traces 沒有出現在 ClickHouse 中？

**檢查步驟**：
1. 確認 OTEL Collector Pod 正在運行
2. 檢查 Collector 日誌是否有錯誤
3. 驗證端點連接性
4. 確認環境變數正確設置

```bash
# 檢查 Collector 狀態
kubectl get pods -n openchoreo-observability-plane -l app.kubernetes.io/component=clickstack-collector

# 查看 Collector 日誌
kubectl logs -n openchoreo-observability-plane -l app.kubernetes.io/component=clickstack-collector --tail=50

# 測試連接
kubectl run test-pod --image=busybox --rm -it -- nc -zv clickstack-collector.openchoreo-observability-plane.svc.cluster.local 4317
```

### Q2: 如何減少 Traces 的存儲成本？

1. 調整採樣率
2. 設置 TTL（已在 Collector 中配置為 30 天）
3. 過濾不重要的 spans

### Q3: 跨服務追蹤如何工作？

OpenTelemetry 使用 W3C Trace Context 傳播追蹤資訊。確保：
- 所有服務都使用 `otelhttp` 或類似的 instrumentation
- 使用相同的 Propagator 配置

---

## 參考資源

- [OpenTelemetry Go SDK](https://github.com/open-telemetry/opentelemetry-go)
- [OTLP Exporter](https://github.com/open-telemetry/opentelemetry-go/tree/main/exporters/otlp)
- [Go HTTP Instrumentation](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/net/http)
- [ClickHouse OTEL Schema](https://clickhouse.com/docs/en/operations/opentelemetry)
