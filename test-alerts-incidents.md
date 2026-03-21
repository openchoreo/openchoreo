# Alert & Incident Test Curls (development environment)

## Cluster resources

| Resource | Name | UID | Project |
|----------|------|-----|---------|
| Project | url-shortener | 25eccd80-3e99-4bf9-96ce-9ba6bd9088b7 | — |
| Component | frontend | 6d918b54-9f50-4eff-8226-83df187ccde0 | url-shortener |
| Component | api-service | 1935a545-856e-4bce-933f-59c0fe3e1742 | url-shortener |
| Component | analytics-service | 60d08490-64cb-4307-b1e0-d2fdfc5dbdd3 | url-shortener |
| Component | postgres | bf85bc08-5afc-456d-9fd0-7ca8670e7f2a | url-shortener |
| Component | redis | 4e0aab93-0a4d-422a-9168-e82fa139ee2e | url-shortener |
| Environment | development | bd6cfee8-354f-4476-8f3e-94a87c114681 | — |
| NotificationChannel | webhook-notification-channel-development | — | — |
| Existing AlertRule | frontend-5xx-log-alert | (in dp-default-url-shortener-development-a16f373a) | url-shortener/frontend |
| Dataplane NS | dp-default-url-shortener-development-a16f373a | — | — |

## Prerequisites

```bash
# Port-forward the internal observer API (webhook endpoint)
kubectl port-forward svc/observer-internal -n openchoreo-observability-plane 8081:8081 &

export BASE_URL="http://observer.openchoreo.localhost:11080"
export INTERNAL_URL="http://localhost:8081"
export TOKEN=$(token)
export DP_NS="dp-default-url-shortener-development-a16f373a"
```

## Step 1: Create ObservabilityAlertRule CRs

The existing `frontend-5xx-log-alert` is already synced. Create additional rules for other url-shortener components in development.

### 1a. High memory — api-service

```bash
kubectl apply -f - <<'EOF'
apiVersion: openchoreo.dev/v1alpha1
kind: ObservabilityAlertRule
metadata:
  name: high-memory-api-service-dev
  namespace: dp-default-url-shortener-development-a16f373a
  labels:
    openchoreo.dev/component-uid: 1935a545-856e-4bce-933f-59c0fe3e1742
    openchoreo.dev/project-uid: 25eccd80-3e99-4bf9-96ce-9ba6bd9088b7
    openchoreo.dev/environment-uid: bd6cfee8-354f-4476-8f3e-94a87c114681
    openchoreo.dev/namespace: default
    openchoreo.dev/project: url-shortener
    openchoreo.dev/component: api-service
    openchoreo.dev/environment: development
spec:
  name: high-memory-api-service-dev
  description: "Memory usage exceeded 90% for api-service in development"
  severity: critical
  enabled: true
  source:
    type: metric
    metric: memory_usage
  condition:
    window: 5m
    interval: 1m
    operator: gt
    threshold: 90
  actions:
    notifications:
      channels:
        - webhook-notification-channel-development
    incident:
      enabled: true
EOF
```

### 1b. High CPU — analytics-service

```bash
kubectl apply -f - <<'EOF'
apiVersion: openchoreo.dev/v1alpha1
kind: ObservabilityAlertRule
metadata:
  name: high-cpu-analytics-dev
  namespace: dp-default-url-shortener-development-a16f373a
  labels:
    openchoreo.dev/component-uid: 60d08490-64cb-4307-b1e0-d2fdfc5dbdd3
    openchoreo.dev/project-uid: 25eccd80-3e99-4bf9-96ce-9ba6bd9088b7
    openchoreo.dev/environment-uid: bd6cfee8-354f-4476-8f3e-94a87c114681
    openchoreo.dev/namespace: default
    openchoreo.dev/project: url-shortener
    openchoreo.dev/component: analytics-service
    openchoreo.dev/environment: development
spec:
  name: high-cpu-analytics-dev
  description: "CPU usage exceeded 85% for analytics-service in development"
  severity: warning
  enabled: true
  source:
    type: metric
    metric: cpu_usage
  condition:
    window: 5m
    interval: 1m
    operator: gt
    threshold: 85
  actions:
    notifications:
      channels:
        - webhook-notification-channel-development
    incident:
      enabled: true
EOF
```

### 1c. Error logs — redis

```bash
kubectl apply -f - <<'EOF'
apiVersion: openchoreo.dev/v1alpha1
kind: ObservabilityAlertRule
metadata:
  name: error-log-redis-dev
  namespace: dp-default-url-shortener-development-a16f373a
  labels:
    openchoreo.dev/component-uid: 4e0aab93-0a4d-422a-9168-e82fa139ee2e
    openchoreo.dev/project-uid: 25eccd80-3e99-4bf9-96ce-9ba6bd9088b7
    openchoreo.dev/environment-uid: bd6cfee8-354f-4476-8f3e-94a87c114681
    openchoreo.dev/namespace: default
    openchoreo.dev/project: url-shortener
    openchoreo.dev/component: redis
    openchoreo.dev/environment: development
spec:
  name: error-log-redis-dev
  description: "Error log count exceeded threshold for redis in development"
  severity: warning
  enabled: true
  source:
    type: log
    query: "level:ERROR"
  condition:
    window: 10m
    interval: 1m
    operator: gt
    threshold: 5
  actions:
    notifications:
      channels:
        - webhook-notification-channel-development
    incident:
      enabled: true
EOF
```

### Verify all CRs

```bash
kubectl get observabilityalertrules -n $DP_NS
```

## Step 2: Fire Alert Webhooks

### 2a. 5xx logs — frontend (existing rule)

```bash
curl -s -X POST "$INTERNAL_URL/api/v1alpha1/alerts/webhook" \
  -H "Content-Type: application/json" \
  -d '{
    "ruleName": "frontend-development-def9a783-frontend-5xx-log-alert",
    "ruleNamespace": "'"$DP_NS"'",
    "alertValue": 8,
    "alertTimestamp": "'"$(date -u +%Y-%m-%dT%H:%M:%SZ)"'"
  }' | jq .
```

### 2b. Memory spike — api-service (93.7%)

```bash
curl -s -X POST "$INTERNAL_URL/api/v1alpha1/alerts/webhook" \
  -H "Content-Type: application/json" \
  -d '{
    "ruleName": "high-memory-api-service-dev",
    "ruleNamespace": "'"$DP_NS"'",
    "alertValue": 93.7,
    "alertTimestamp": "'"$(date -u +%Y-%m-%dT%H:%M:%SZ)"'"
  }' | jq .
```

### 2c. CPU spike — analytics-service (91.2%)

```bash
curl -s -X POST "$INTERNAL_URL/api/v1alpha1/alerts/webhook" \
  -H "Content-Type: application/json" \
  -d '{
    "ruleName": "high-cpu-analytics-dev",
    "ruleNamespace": "'"$DP_NS"'",
    "alertValue": 91.2,
    "alertTimestamp": "'"$(date -u +%Y-%m-%dT%H:%M:%SZ)"'"
  }' | jq .
```

### 2d. Error logs — redis (12 errors)

```bash
curl -s -X POST "$INTERNAL_URL/api/v1alpha1/alerts/webhook" \
  -H "Content-Type: application/json" \
  -d '{
    "ruleName": "error-log-redis-dev",
    "ruleNamespace": "'"$DP_NS"'",
    "alertValue": 12,
    "alertTimestamp": "'"$(date -u +%Y-%m-%dT%H:%M:%SZ)"'"
  }' | jq .
```

### 2e. Second memory spike — api-service (97.3%, 5 min later)

```bash
curl -s -X POST "$INTERNAL_URL/api/v1alpha1/alerts/webhook" \
  -H "Content-Type: application/json" \
  -d '{
    "ruleName": "high-memory-api-service-dev",
    "ruleNamespace": "'"$DP_NS"'",
    "alertValue": 97.3,
    "alertTimestamp": "'"$(date -u -v+5M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '+5 minutes' +%Y-%m-%dT%H:%M:%SZ)"'"
  }' | jq .
```

### 2f. Second 5xx spike — frontend (10 min later)

```bash
curl -s -X POST "$INTERNAL_URL/api/v1alpha1/alerts/webhook" \
  -H "Content-Type: application/json" \
  -d '{
    "ruleName": "frontend-development-def9a783-frontend-5xx-log-alert",
    "ruleNamespace": "'"$DP_NS"'",
    "alertValue": 15,
    "alertTimestamp": "'"$(date -u -v+10M +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '+10 minutes' +%Y-%m-%dT%H:%M:%SZ)"'"
  }' | jq .
```

## Step 3: Query Alerts

### 3a. All alerts in development

```bash
curl -s -X POST "$BASE_URL/api/v1alpha1/alerts/query" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "startTime": "'"$(date -u -v-1H +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '-1 hour' +%Y-%m-%dT%H:%M:%SZ)"'",
    "endTime": "'"$(date -u -v+1H +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '+1 hour' +%Y-%m-%dT%H:%M:%SZ)"'",
    "searchScope": {
      "namespace": "default",
      "environment": "development"
    },
    "limit": 50,
    "sortOrder": "desc"
  }' | jq .
```

### 3b. Alerts for api-service only

```bash
curl -s -X POST "$BASE_URL/api/v1alpha1/alerts/query" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "startTime": "'"$(date -u -v-1H +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '-1 hour' +%Y-%m-%dT%H:%M:%SZ)"'",
    "endTime": "'"$(date -u -v+1H +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '+1 hour' +%Y-%m-%dT%H:%M:%SZ)"'",
    "searchScope": {
      "namespace": "default",
      "project": "url-shortener",
      "component": "api-service",
      "environment": "development"
    },
    "limit": 20,
    "sortOrder": "desc"
  }' | jq .
```

### 3c. Alerts for frontend only

```bash
curl -s -X POST "$BASE_URL/api/v1alpha1/alerts/query" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "startTime": "'"$(date -u -v-1H +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '-1 hour' +%Y-%m-%dT%H:%M:%SZ)"'",
    "endTime": "'"$(date -u -v+1H +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '+1 hour' +%Y-%m-%dT%H:%M:%SZ)"'",
    "searchScope": {
      "namespace": "default",
      "project": "url-shortener",
      "component": "frontend",
      "environment": "development"
    },
    "sortOrder": "asc"
  }' | jq .
```

## Step 4: Query Incidents

### 4a. All incidents in development

```bash
curl -s -X POST "$BASE_URL/api/v1alpha1/incidents/query" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "startTime": "'"$(date -u -v-1H +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '-1 hour' +%Y-%m-%dT%H:%M:%SZ)"'",
    "endTime": "'"$(date -u -v+1H +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '+1 hour' +%Y-%m-%dT%H:%M:%SZ)"'",
    "searchScope": {
      "namespace": "default",
      "environment": "development"
    },
    "limit": 50,
    "sortOrder": "desc"
  }' | jq .
```

### 4b. Incidents for api-service

```bash
curl -s -X POST "$BASE_URL/api/v1alpha1/incidents/query" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "startTime": "'"$(date -u -v-1H +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '-1 hour' +%Y-%m-%dT%H:%M:%SZ)"'",
    "endTime": "'"$(date -u -v+1H +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '+1 hour' +%Y-%m-%dT%H:%M:%SZ)"'",
    "searchScope": {
      "namespace": "default",
      "project": "url-shortener",
      "component": "api-service",
      "environment": "development"
    },
    "sortOrder": "desc"
  }' | jq .
```

## Step 5: Update an Incident

Use an incident ID from step 4 results:

```bash
export INCIDENT_ID="<incident-id-from-step-4>"

# Acknowledge
curl -s -X PUT "$BASE_URL/api/v1alpha1/incidents/$INCIDENT_ID" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "status": "acknowledged",
    "notes": "Investigating high resource usage"
  }' | jq .

# Resolve
curl -s -X PUT "$BASE_URL/api/v1alpha1/incidents/$INCIDENT_ID" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "status": "resolved",
    "notes": "Scaled up replicas, usage normalized"
  }' | jq .
```

## Cleanup

```bash
kubectl delete observabilityalertrules -n $DP_NS \
  high-memory-api-service-dev \
  high-cpu-analytics-dev \
  error-log-redis-dev
```
