# 快速開始：Go Greeter Service + OpenTelemetry

## 🎯 目標

讓 Go Greeter Service 將 traces 傳送到 ClickStack OTEL Collector。

## 📋 前置條件

- OpenChoreo 集群已部署
- ClickStack OTEL Collector 正在運行
  ```bash
  kubectl get pods -n openchoreo-observability-plane -l app.kubernetes.io/component=clickstack-collector
  ```

## 🚀 快速部署

### 步驟 1: 更新 Workload 環境變數

在你的 `greeter-service.yaml` 中添加：

```yaml
env:
  # OTEL Collector 端點
  - key: OTEL_EXPORTER_OTLP_ENDPOINT
    value: "clickstack-collector.openchoreo-observability-plane.svc.cluster.local:4317"
  
  # 服務名稱
  - key: OTEL_SERVICE_NAME
    value: "greeter-service"
  
  # 版本和環境
  - key: SERVICE_VERSION
    value: "1.0.0"
  - key: DEPLOYMENT_ENV
    value: "production"
```

### 步驟 2: 在 Go 代碼中集成 OTEL SDK

#### 安裝依賴

```bash
go get go.opentelemetry.io/otel@v1.21.0
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc@v1.21.0
go get go.opentelemetry.io/otel/sdk@v1.21.0
go get go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@v0.46.1
```

#### 初始化 Tracer (main.go)

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
    ctx := context.Background()
    
    // 初始化 tracer
    tp, err := initTracer(ctx)
    if err != nil {
        log.Fatal(err)
    }
    defer tp.Shutdown(ctx)
    
    // 啟動 HTTP server...
}
```

完整範例參考: `main.go`

#### 包裝 HTTP Handlers

```go
import "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

mux.Handle("/greeter/greet", otelhttp.NewHandler(
    http.HandlerFunc(greetHandler),
    "greet",
))
```

### 步驟 3: 部署並測試

```bash
# 部署服務
kubectl apply -f workload-with-otel.yaml

# 等待 Pod 就緒
kubectl wait --for=condition=ready pod -l openchoreo.dev/component=greeter-service -n default --timeout=120s

# Port-forward gateway
kubectl port-forward -n openchoreo-data-plane svc/gateway-external 8443:443 &

# 發送測試請求
curl -k "https://localhost:8443/greeter/greet?name=Alice"
```

### 步驟 4: 驗證 Traces

```bash
# 進入 ClickHouse
kubectl exec -n openchoreo-observability-plane clickhouse-0 -it -- \
  clickhouse-client --user default --password clickstack-change-me --database observability

# 查詢 traces
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
LIMIT 5;
```

## 📊 OTEL Collector 端點

| 訪問方式 | gRPC | HTTP |
|----------|------|------|
| **Cluster 內部** (推薦) | `clickstack-collector.openchoreo-observability-plane.svc.cluster.local:4317` | `:4318` |
| **NodePort** (測試) | `172.19.0.2:30317` | `:30318` |

## 🔍 故障排查

### Traces 沒有出現？

1. **檢查 Collector 狀態**
   ```bash
   kubectl get pods -n openchoreo-observability-plane -l app.kubernetes.io/component=clickstack-collector
   ```

2. **查看 Collector 日誌**
   ```bash
   kubectl logs -n openchoreo-observability-plane -l app.kubernetes.io/component=clickstack-collector --tail=50
   ```

3. **檢查服務日誌**
   ```bash
   kubectl logs -l openchoreo.dev/component=greeter-service
   ```

4. **測試連接**
   ```bash
   kubectl run test-pod --image=busybox --rm -it -- \
     nc -zv clickstack-collector.openchoreo-observability-plane.svc.cluster.local 4317
   ```

### 環境變數未設置？

確認 Pod 中的環境變數：
```bash
kubectl exec <pod-name> -- env | grep OTEL
```

## 📚 相關文檔

- 📖 [完整集成指南](../../go-greeter-otel-integration.md)
- 💻 [範例代碼](./main.go)
- 🐳 [Dockerfile](./Dockerfile)
- ⚙️ [Workload 配置](./workload-with-otel.yaml)

## 🎓 下一步

- [ ] 添加數據庫查詢 tracing
- [ ] 實現跨服務追蹤
- [ ] 配置自定義採樣策略
- [ ] 集成 metrics 和 logs
- [ ] 部署 HyperDX UI 查看 traces

## 💡 關鍵要點

1. ✅ 使用 `otelhttp` 自動包裝 HTTP handlers
2. ✅ 在 Kubernetes 中使用 Cluster 內部端點
3. ✅ 設置有意義的 service name 和 version
4. ✅ 添加自定義 attributes 提升可觀測性
5. ✅ 記得在應用關閉時調用 `tp.Shutdown()`
