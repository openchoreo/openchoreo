// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Resource kinds to generate (common Kubernetes resources)
var resourceKinds = []struct {
	apiVersion string
	kind       string
}{
	{"v1", "ConfigMap"},
	{"v1", "Secret"},
	{"v1", "Service"},
	{"v1", "ServiceAccount"},
	{"v1", "PersistentVolumeClaim"},
	{"apps/v1", "Deployment"},
	{"apps/v1", "StatefulSet"},
	{"apps/v1", "DaemonSet"},
	{"batch/v1", "Job"},
	{"batch/v1", "CronJob"},
	{"networking.k8s.io/v1", "Ingress"},
	{"networking.k8s.io/v1", "NetworkPolicy"},
	{"autoscaling/v2", "HorizontalPodAutoscaler"},
	{"policy/v1", "PodDisruptionBudget"},
	{"rbac.authorization.k8s.io/v1", "Role"},
	{"rbac.authorization.k8s.io/v1", "RoleBinding"},
}

// generateTestRepository creates a test repository with the specified number of YAML files
// Each file contains 1-3 resources (like real GitOps repos often have)
func generateTestRepository(t testing.TB, baseDir string, fileCount int) string {
	t.Helper()

	repoDir := filepath.Join(baseDir, fmt.Sprintf("repo-%d", fileCount))
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // Non-cryptographic use for test data

	// Create a realistic directory structure
	namespaces := []string{"default", "kube-system", "monitoring", "logging", "app-prod", "app-staging"}

	filesPerNamespace := fileCount / len(namespaces)
	if filesPerNamespace < 1 {
		filesPerNamespace = 1
	}

	fileIndex := 0
	for _, ns := range namespaces {
		nsDir := filepath.Join(repoDir, "namespaces", ns)
		if err := os.MkdirAll(nsDir, 0755); err != nil {
			t.Fatalf("failed to create namespace dir: %v", err)
		}

		for i := 0; i < filesPerNamespace && fileIndex < fileCount; i++ {
			// Each file has 1-3 resources
			resourcesPerFile := 1 + rng.Intn(3)
			content := generateYAMLFile(rng, ns, fileIndex, resourcesPerFile)

			filePath := filepath.Join(nsDir, fmt.Sprintf("resource-%d.yaml", i))
			if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
				t.Fatalf("failed to write file: %v", err)
			}
			fileIndex++
		}
	}

	// Also create some files in apps/ directory structure (common pattern)
	appsDir := filepath.Join(repoDir, "apps")
	appNames := []string{"frontend", "backend", "api-gateway", "worker", "scheduler"}

	for _, app := range appNames {
		if fileIndex >= fileCount {
			break
		}
		appDir := filepath.Join(appsDir, app)
		if err := os.MkdirAll(appDir, 0755); err != nil {
			t.Fatalf("failed to create app dir: %v", err)
		}

		// Create base, overlays structure (like Kustomize)
		for _, env := range []string{"base", "overlays/dev", "overlays/prod"} {
			if fileIndex >= fileCount {
				break
			}
			envDir := filepath.Join(appDir, env)
			if err := os.MkdirAll(envDir, 0755); err != nil {
				t.Fatalf("failed to create env dir: %v", err)
			}

			resourcesPerFile := 1 + rng.Intn(3)
			content := generateYAMLFile(rng, app, fileIndex, resourcesPerFile)

			filePath := filepath.Join(envDir, "resources.yaml")
			if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
				t.Fatalf("failed to write file: %v", err)
			}
			fileIndex++
		}
	}

	return repoDir
}

// generateYAMLFile creates a multi-document YAML file with random resources
func generateYAMLFile(rng *rand.Rand, namespace string, seed, resourceCount int) string {
	var content string

	for i := 0; i < resourceCount; i++ {
		if i > 0 {
			content += "---\n"
		}

		kindInfo := resourceKinds[rng.Intn(len(resourceKinds))]
		resourceName := fmt.Sprintf("%s-%s-%d-%d", kindInfo.kind, namespace, seed, i)

		content += generateResource(kindInfo.apiVersion, kindInfo.kind, resourceName, namespace, rng)
	}

	return content
}

// generateResource creates a single Kubernetes resource YAML
func generateResource(apiVersion, kind, name, namespace string, rng *rand.Rand) string {
	// Generate labels and annotations for realistic size
	labels := generateLabels(rng, 3+rng.Intn(5))
	annotations := generateAnnotations(rng, 2+rng.Intn(4))

	base := fmt.Sprintf(`apiVersion: %s
kind: %s
metadata:
  name: %s
  namespace: %s
%s%s`, apiVersion, kind, name, namespace, labels, annotations)

	// Add kind-specific spec
	spec := generateSpec(kind, rng)

	return base + spec + "\n"
}

func generateLabels(rng *rand.Rand, count int) string {
	if count == 0 {
		return ""
	}

	result := "  labels:\n"
	labelKeys := []string{"app", "component", "version", "environment", "team", "tier", "release", "managed-by"}

	for i := 0; i < count && i < len(labelKeys); i++ {
		result += fmt.Sprintf("    %s: value-%d\n", labelKeys[i], rng.Intn(100))
	}
	return result
}

func generateAnnotations(rng *rand.Rand, count int) string {
	if count == 0 {
		return ""
	}

	result := "  annotations:\n"
	annotationKeys := []string{
		"description",
		"kubectl.kubernetes.io/last-applied-configuration",
		"deployment.kubernetes.io/revision",
		"prometheus.io/scrape",
		"prometheus.io/port",
	}

	for i := 0; i < count && i < len(annotationKeys); i++ {
		result += fmt.Sprintf("    %s: \"value-%d\"\n", annotationKeys[i], rng.Intn(1000))
	}
	return result
}

func generateSpec(kind string, rng *rand.Rand) string {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet":
		return fmt.Sprintf(`spec:
  replicas: %d
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
        - name: main
          image: nginx:1.%d.0
          ports:
            - containerPort: 80
          resources:
            requests:
              cpu: %dm
              memory: %dMi
            limits:
              cpu: %dm
              memory: %dMi
          env:
            - name: ENV_VAR_1
              value: "value1"
            - name: ENV_VAR_2
              value: "value2"
`, rng.Intn(5)+1, rng.Intn(25), rng.Intn(500)+100, rng.Intn(512)+128, rng.Intn(1000)+500, rng.Intn(1024)+512)

	case "Service":
		return fmt.Sprintf(`spec:
  type: ClusterIP
  ports:
    - port: %d
      targetPort: %d
      protocol: TCP
  selector:
    app: myapp
`, 80+rng.Intn(100), 8080+rng.Intn(100))

	case "ConfigMap":
		return fmt.Sprintf(`data:
  config.yaml: |
    setting1: value1
    setting2: value2
    number: %d
  application.properties: |
    app.name=myapp
    app.port=%d
`, rng.Intn(1000), 8080+rng.Intn(100))

	case "Secret":
		return `type: Opaque
stringData:
  username: admin
  password: supersecret
`

	case "Ingress":
		return fmt.Sprintf(`spec:
  rules:
    - host: app-%d.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: my-service
                port:
                  number: 80
`, rng.Intn(100))

	case "Job", "CronJob":
		schedule := ""
		if kind == "CronJob" {
			schedule = fmt.Sprintf(`  schedule: "%d * * * *"\n`, rng.Intn(60))
		}
		return fmt.Sprintf(`spec:
%s  template:
    spec:
      containers:
        - name: job
          image: busybox
          command: ["echo", "hello"]
      restartPolicy: Never
`, schedule)

	default:
		return `spec:
  enabled: true
`
	}
}

// Benchmark tests for different repository sizes

// Cold Start Benchmarks (Full Scan)

func BenchmarkColdStart_100Files(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "benchmark-cold-100-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	repoPath := generateTestRepository(b, tmpDir, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ClearCache(repoPath)
		_, err := LoadOrBuild(repoPath)
		if err != nil {
			b.Fatalf("benchmark failed: %v", err)
		}
	}
}

func BenchmarkColdStart_1000Files(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "benchmark-cold-1000-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	repoPath := generateTestRepository(b, tmpDir, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ClearCache(repoPath)
		_, err := LoadOrBuild(repoPath)
		if err != nil {
			b.Fatalf("benchmark failed: %v", err)
		}
	}
}

// Warm Start Benchmarks (Cache Load)

func BenchmarkWarmStart_100Files(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "benchmark-warm-100-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	repoPath := generateTestRepository(b, tmpDir, 100)

	// Build initial cache
	_, err = LoadOrBuild(repoPath)
	if err != nil {
		b.Fatalf("failed to build initial cache: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := LoadOrBuild(repoPath)
		if err != nil {
			b.Fatalf("benchmark failed: %v", err)
		}
	}
}

func BenchmarkWarmStart_1000Files(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "benchmark-warm-1000-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	repoPath := generateTestRepository(b, tmpDir, 1000)

	// Build initial cache
	_, err = LoadOrBuild(repoPath)
	if err != nil {
		b.Fatalf("failed to build initial cache: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := LoadOrBuild(repoPath)
		if err != nil {
			b.Fatalf("benchmark failed: %v", err)
		}
	}
}

// BenchmarkPerformanceTargets_100Files_ColdStart benchmarks cold start with 100 files
func BenchmarkPerformanceTargets_100Files_ColdStart(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "perf-test-100-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	repoPath := generateTestRepository(b, tmpDir, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ClearCache(repoPath)
		_, err := LoadOrBuild(repoPath)
		if err != nil {
			b.Fatalf("failed to build index: %v", err)
		}
	}
}

// BenchmarkPerformanceTargets_100Files_WarmStart benchmarks warm start with 100 files
func BenchmarkPerformanceTargets_100Files_WarmStart(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "perf-test-100-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	repoPath := generateTestRepository(b, tmpDir, 100)

	// Build initial cache
	_, err = LoadOrBuild(repoPath)
	if err != nil {
		b.Fatalf("failed to build initial cache: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := LoadOrBuild(repoPath)
		if err != nil {
			b.Fatalf("failed to load index: %v", err)
		}
	}
}

// BenchmarkPerformanceTargets_1000Files_ColdStart benchmarks cold start with 1000 files
func BenchmarkPerformanceTargets_1000Files_ColdStart(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "perf-test-1000-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	repoPath := generateTestRepository(b, tmpDir, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ClearCache(repoPath)
		_, err := LoadOrBuild(repoPath)
		if err != nil {
			b.Fatalf("failed to build index: %v", err)
		}
	}
}

// BenchmarkPerformanceTargets_1000Files_WarmStart benchmarks warm start with 1000 files
func BenchmarkPerformanceTargets_1000Files_WarmStart(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "perf-test-1000-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	repoPath := generateTestRepository(b, tmpDir, 1000)

	// Build initial cache
	_, err = LoadOrBuild(repoPath)
	if err != nil {
		b.Fatalf("failed to build initial cache: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := LoadOrBuild(repoPath)
		if err != nil {
			b.Fatalf("failed to load index: %v", err)
		}
	}
}

// BenchmarkIncrementalUpdate_500Files_5Changed benchmarks incremental update with 5 changed files
func BenchmarkIncrementalUpdate_500Files_5Changed(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "incremental-test-5-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate initial repository
	repoPath := generateTestRepository(b, tmpDir, 500)

	// Build initial cache
	_, err = LoadOrBuild(repoPath)
	if err != nil {
		b.Fatalf("failed to build initial cache: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Simulate file changes by modifying some files
		modifyRandomFiles(b, repoPath, 5)
		b.StartTimer()

		// Measure incremental update (simulated by rebuilding)
		_, err = ForceRebuild(repoPath)
		if err != nil {
			b.Fatalf("failed to rebuild index: %v", err)
		}
	}
}

// BenchmarkIncrementalUpdate_500Files_30Changed benchmarks incremental update with 30 changed files
func BenchmarkIncrementalUpdate_500Files_30Changed(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "incremental-test-30-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate initial repository
	repoPath := generateTestRepository(b, tmpDir, 500)

	// Build initial cache
	_, err = LoadOrBuild(repoPath)
	if err != nil {
		b.Fatalf("failed to build initial cache: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Simulate file changes by modifying some files
		modifyRandomFiles(b, repoPath, 30)
		b.StartTimer()

		// Measure incremental update (simulated by rebuilding)
		_, err = ForceRebuild(repoPath)
		if err != nil {
			b.Fatalf("failed to rebuild index: %v", err)
		}
	}
}

// modifyRandomFiles modifies random YAML files in the repository
func modifyRandomFiles(t testing.TB, repoPath string, count int) {
	t.Helper()

	var yamlFiles []string
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (filepath.Ext(path) == ".yaml" || filepath.Ext(path) == ".yml") {
			yamlFiles = append(yamlFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk directory: %v", err)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // Non-cryptographic use for test data

	for i := 0; i < count && i < len(yamlFiles); i++ {
		// Pick a random file
		fileIndex := rng.Intn(len(yamlFiles))
		filePath := yamlFiles[fileIndex]

		// Read and modify the file
		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}

		// Append a comment to simulate modification
		newContent := string(content) + fmt.Sprintf("\n# Modified at %v\n", time.Now())

		if err := os.WriteFile(filePath, []byte(newContent), 0600); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		// Remove from list to avoid modifying same file twice
		yamlFiles = append(yamlFiles[:fileIndex], yamlFiles[fileIndex+1:]...)
	}
}

// BenchmarkLargeRepository_10000Files_ColdStart benchmarks cold start with 10,000 files
func BenchmarkLargeRepository_10000Files_ColdStart(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "perf-test-10000-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	repoPath := generateTestRepository(b, tmpDir, 10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ClearCache(repoPath); err != nil {
			b.Fatalf("failed to clear cache: %v", err)
		}

		_, err := LoadOrBuild(repoPath)
		if err != nil {
			b.Fatalf("failed to build index: %v", err)
		}
	}
}

// BenchmarkLargeRepository_10000Files_WarmStart benchmarks warm start with 10,000 files
func BenchmarkLargeRepository_10000Files_WarmStart(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "perf-test-10000-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	repoPath := generateTestRepository(b, tmpDir, 10000)

	// Build initial cache
	_, err = LoadOrBuild(repoPath)
	if err != nil {
		b.Fatalf("failed to build initial cache: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := LoadOrBuild(repoPath)
		if err != nil {
			b.Fatalf("failed to load index: %v", err)
		}
	}
}
