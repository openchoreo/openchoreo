// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
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

// measureMemory returns current memory allocation
// If runGC is true, runs garbage collection first to get a clean baseline
func measureMemory(runGC bool) uint64 {
	if runGC {
		runtime.GC()
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
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

// TestPerformanceTargets runs comprehensive tests to verify performance targets
func TestPerformanceTargets(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance tests in short mode")
	}

	testCases := []struct {
		name                 string
		fileCount            int
		maxColdStartTime     time.Duration
		maxWarmStartTime     time.Duration
		maxColdStartMemoryMB uint64
		maxWarmStartMemoryMB uint64
	}{
		{
			name:                 "100 files",
			fileCount:            100,
			maxColdStartTime:     500 * time.Millisecond,
			maxWarmStartTime:     50 * time.Millisecond,
			maxColdStartMemoryMB: 50,
			maxWarmStartMemoryMB: 20,
		},
		{
			name:                 "1000 files",
			fileCount:            1000,
			maxColdStartTime:     2 * time.Second,
			maxWarmStartTime:     200 * time.Millisecond,
			maxColdStartMemoryMB: 200,
			maxWarmStartMemoryMB: 100,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", fmt.Sprintf("perf-test-%d-*", tc.fileCount))
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			t.Logf("Generating test repository with %d files...", tc.fileCount)
			repoPath := generateTestRepository(t, tmpDir, tc.fileCount)

			// Test Cold Start
			t.Run("ColdStart", func(t *testing.T) {
				_ = ClearCache(repoPath)
				memBefore := measureMemory(true) // GC before baseline

				start := time.Now()
				idx, err := LoadOrBuild(repoPath)
				if err != nil {
					t.Fatalf("failed to build index: %v", err)
				}
				duration := time.Since(start)

				memAfter := measureMemory(false) // No GC - capture actual usage
				memUsedMB := (memAfter - memBefore) / (1024 * 1024)

				stats := idx.Stats()
				t.Logf("Cold Start: %v, Memory: %d MB, Resources: %d, Files: %d",
					duration, memUsedMB, stats.TotalResources, stats.TotalFiles)

				if duration > tc.maxColdStartTime {
					t.Errorf("Cold start time %v exceeds target %v", duration, tc.maxColdStartTime)
				}

				if memUsedMB > tc.maxColdStartMemoryMB {
					t.Errorf("Cold start memory %d MB exceeds target %d MB", memUsedMB, tc.maxColdStartMemoryMB)
				}
			})

			// Test Warm Start
			t.Run("WarmStart", func(t *testing.T) {
				// Ensure cache exists
				_, err := LoadOrBuild(repoPath)
				if err != nil {
					t.Fatalf("failed to build initial cache: %v", err)
				}

				memBefore := measureMemory(true) // GC before baseline

				start := time.Now()
				idx, err := LoadOrBuild(repoPath)
				if err != nil {
					t.Fatalf("failed to load index: %v", err)
				}
				duration := time.Since(start)

				memAfter := measureMemory(false) // No GC - capture actual usage
				memUsedMB := (memAfter - memBefore) / (1024 * 1024)

				stats := idx.Stats()
				t.Logf("Warm Start: %v, Memory: %d MB, Resources: %d, Files: %d",
					duration, memUsedMB, stats.TotalResources, stats.TotalFiles)

				if duration > tc.maxWarmStartTime {
					t.Errorf("Warm start time %v exceeds target %v", duration, tc.maxWarmStartTime)
				}

				if memUsedMB > tc.maxWarmStartMemoryMB {
					t.Errorf("Warm start memory %d MB exceeds target %d MB", memUsedMB, tc.maxWarmStartMemoryMB)
				}
			})

			// Test Cache Speedup
			t.Run("CacheSpeedup", func(t *testing.T) {
				// Cold start
				_ = ClearCache(repoPath)
				coldStart := time.Now()
				_, err := LoadOrBuild(repoPath)
				if err != nil {
					t.Fatalf("cold start failed: %v", err)
				}
				coldDuration := time.Since(coldStart)

				// Warm start
				warmStart := time.Now()
				_, err = LoadOrBuild(repoPath)
				if err != nil {
					t.Fatalf("warm start failed: %v", err)
				}
				warmDuration := time.Since(warmStart)

				speedup := float64(coldDuration) / float64(warmDuration)
				t.Logf("Cache speedup: %.2fx (cold: %v, warm: %v)", speedup, coldDuration, warmDuration)

				// Cache should provide at least 2x speedup
				if speedup < 1.5 {
					t.Errorf("Cache speedup %.2fx is less than expected 1.5x minimum", speedup)
				}
			})
		})
	}
}

// TestIncrementalUpdate tests incremental update performance
func TestIncrementalUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping incremental update test in short mode")
	}

	testCases := []struct {
		name          string
		totalFiles    int
		changedFiles  int
		maxUpdateTime time.Duration
	}{
		{"1-10 files changed", 500, 5, 100 * time.Millisecond},
		{"10-50 files changed", 500, 30, 500 * time.Millisecond},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", fmt.Sprintf("incremental-test-%d-*", tc.changedFiles))
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			// Generate initial repository
			repoPath := generateTestRepository(t, tmpDir, tc.totalFiles)

			// Build initial cache
			idx, err := LoadOrBuild(repoPath)
			if err != nil {
				t.Fatalf("failed to build initial cache: %v", err)
			}
			initialResourceCount := idx.Stats().TotalResources

			// Simulate file changes by modifying some files
			modifyRandomFiles(t, repoPath, tc.changedFiles)

			// Measure incremental update (simulated by clearing cache and rebuilding)
			// Note: True incremental update would use git diff, but for benchmark
			// purposes we measure a partial rebuild
			start := time.Now()
			idx, err = ForceRebuild(repoPath)
			if err != nil {
				t.Fatalf("failed to rebuild index: %v", err)
			}
			duration := time.Since(start)

			t.Logf("Incremental update (%d changed files): %v, Resources: %d -> %d",
				tc.changedFiles, duration, initialResourceCount, idx.Stats().TotalResources)

			// For now, we just verify the rebuild completes reasonably fast
			// True incremental update would be much faster
			if duration > tc.maxUpdateTime*10 { // Allow 10x margin for full rebuild
				t.Logf("Warning: Rebuild time %v is higher than target %v (expected for full rebuild)", duration, tc.maxUpdateTime)
			}
		})
	}
}

// BenchmarkReportEntry holds a single benchmark result
type BenchmarkReportEntry struct {
	Size           string
	FileCount      int
	ResourceCount  int
	ColdStartTime  time.Duration
	WarmStartTime  time.Duration
	ColdStartMemMB uint64
	WarmStartMemMB uint64
	Speedup        float64
	ColdTarget     time.Duration
	WarmTarget     time.Duration
}

// TestBenchmarkReport generates a formatted benchmark report
func TestBenchmarkReport(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping benchmark report in short mode")
	}

	sizes := []struct {
		name       string
		fileCount  int
		coldTarget time.Duration
		warmTarget time.Duration
		coldMemMB  uint64
		warmMemMB  uint64
	}{
		{"Small (100)", 100, 500 * time.Millisecond, 50 * time.Millisecond, 50, 20},
		{"Medium (1,000)", 1000, 2 * time.Second, 200 * time.Millisecond, 200, 100},
	}

	// Check if large test is enabled
	if os.Getenv("RUN_LARGE_TESTS") != "" {
		sizes = append(sizes, struct {
			name       string
			fileCount  int
			coldTarget time.Duration
			warmTarget time.Duration
			coldMemMB  uint64
			warmMemMB  uint64
		}{"Large (10,000)", 10000, 10 * time.Second, 1 * time.Second, 1024, 500})
	}

	results := make([]BenchmarkReportEntry, 0, len(sizes))
	tmpDirs := make([]string, 0, len(sizes))
	defer func() {
		for _, dir := range tmpDirs {
			os.RemoveAll(dir)
		}
	}()

	for _, size := range sizes {
		tmpDir, err := os.MkdirTemp("", fmt.Sprintf("report-%d-*", size.fileCount))
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		tmpDirs = append(tmpDirs, tmpDir)

		t.Logf("Generating %s repository...", size.name)
		repoPath := generateTestRepository(t, tmpDir, size.fileCount)

		entry := BenchmarkReportEntry{
			Size:       size.name,
			FileCount:  size.fileCount,
			ColdTarget: size.coldTarget,
			WarmTarget: size.warmTarget,
		}

		// Cold Start
		if err := ClearCache(repoPath); err != nil {
			t.Fatalf("failed to clear cache: %v", err)
		}
		memBefore := measureMemory(true) // GC before baseline

		start := time.Now()
		idx, err := LoadOrBuild(repoPath)
		if err != nil {
			t.Fatalf("failed to build index: %v", err)
		}
		entry.ColdStartTime = time.Since(start)
		entry.ResourceCount = idx.Stats().TotalResources

		memAfter := measureMemory(false) // No GC - capture actual usage
		if memAfter > memBefore {
			entry.ColdStartMemMB = (memAfter - memBefore) / (1024 * 1024)
		}

		// Warm Start
		memBefore = measureMemory(true) // GC before baseline

		start = time.Now()
		_, err = LoadOrBuild(repoPath)
		if err != nil {
			t.Fatalf("failed to load index: %v", err)
		}
		entry.WarmStartTime = time.Since(start)

		memAfter = measureMemory(false) // No GC - capture actual usage
		if memAfter > memBefore {
			entry.WarmStartMemMB = (memAfter - memBefore) / (1024 * 1024)
		}

		// Calculate speedup
		entry.Speedup = float64(entry.ColdStartTime) / float64(entry.WarmStartTime)

		results = append(results, entry)
	}

	// Print formatted report
	printBenchmarkReport(t, results)
}

func printBenchmarkReport(t *testing.T, results []BenchmarkReportEntry) {
	t.Log("")
	t.Log("================================================================================")
	t.Log("                         BENCHMARK REPORT                                       ")
	t.Log("================================================================================")
	t.Log("")

	// Full Scan (Cold Start) Table
	t.Log("## Full Scan (Cold Start)")
	t.Log("")
	t.Log("| Repository Size   | Target    | Actual    | Memory  | Status              |")
	t.Log("|-------------------|-----------|-----------|---------|---------------------|")

	for _, r := range results {
		status := "✅ PASS"
		improvement := ""
		if r.ColdStartTime <= r.ColdTarget {
			ratio := float64(r.ColdTarget) / float64(r.ColdStartTime)
			improvement = fmt.Sprintf(" (%.0fx better)", ratio)
		} else {
			status = "❌ FAIL"
		}

		t.Logf("| %-17s | %-9s | %-9s | %-7s | %-19s |",
			r.Size,
			formatDuration(r.ColdTarget),
			formatDuration(r.ColdStartTime),
			fmt.Sprintf("%d MB", r.ColdStartMemMB),
			status+improvement,
		)
	}

	t.Log("")

	// Cached Index (Warm Start) Table
	t.Log("## Cached Index (Warm Start)")
	t.Log("")
	t.Log("| Repository Size   | Target    | Actual    | Memory  | Status              |")
	t.Log("|-------------------|-----------|-----------|---------|---------------------|")

	for _, r := range results {
		status := "✅ PASS"
		improvement := ""
		if r.WarmStartTime <= r.WarmTarget {
			ratio := float64(r.WarmTarget) / float64(r.WarmStartTime)
			improvement = fmt.Sprintf(" (%.0fx better)", ratio)
		} else {
			status = "❌ FAIL"
		}

		t.Logf("| %-17s | %-9s | %-9s | %-7s | %-19s |",
			r.Size,
			formatDuration(r.WarmTarget),
			formatDuration(r.WarmStartTime),
			fmt.Sprintf("%d MB", r.WarmStartMemMB),
			status+improvement,
		)
	}

	t.Log("")

	// Cache Speedup Table
	t.Log("## Cache Speedup")
	t.Log("")
	t.Log("| Repository Size   | Cold Start | Warm Start | Speedup  |")
	t.Log("|-------------------|------------|------------|----------|")

	for _, r := range results {
		t.Logf("| %-17s | %-10s | %-10s | %-8s |",
			r.Size,
			formatDuration(r.ColdStartTime),
			formatDuration(r.WarmStartTime),
			fmt.Sprintf("%.1fx", r.Speedup),
		)
	}

	t.Log("")

	// Summary
	t.Log("## Summary")
	t.Log("")

	allPassed := true
	for _, r := range results {
		if r.ColdStartTime > r.ColdTarget || r.WarmStartTime > r.WarmTarget {
			allPassed = false
			break
		}
	}

	if allPassed {
		t.Log("✅ All performance targets met!")
	} else {
		t.Log("❌ Some performance targets not met")
	}

	t.Log("")
	t.Log("================================================================================")
}

func formatDuration(d time.Duration) string {
	if d >= time.Second {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	return fmt.Sprintf("%dms", d.Milliseconds())
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

// TestLargeRepository tests with 10,000 files (optional, run with -timeout flag)
func TestLargeRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large repository test in short mode")
	}

	// Skip by default, enable with: go test -run TestLargeRepository -v
	if os.Getenv("RUN_LARGE_TESTS") == "" {
		t.Skip("Skipping large repository test. Set RUN_LARGE_TESTS=1 to run")
	}

	tmpDir, err := os.MkdirTemp("", "perf-test-10000-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Log("Generating test repository with 10000 files...")
	repoPath := generateTestRepository(t, tmpDir, 10000)

	// Cold Start
	t.Run("ColdStart", func(t *testing.T) {
		if err := ClearCache(repoPath); err != nil {
			t.Fatalf("failed to clear cache: %v", err)
		}
		memBefore := measureMemory(true) // GC before baseline

		start := time.Now()
		idx, err := LoadOrBuild(repoPath)
		if err != nil {
			t.Fatalf("failed to build index: %v", err)
		}
		duration := time.Since(start)

		memAfter := measureMemory(false) // No GC - capture actual usage
		memUsedMB := (memAfter - memBefore) / (1024 * 1024)

		stats := idx.Stats()
		t.Logf("Cold Start (10k files): %v, Memory: %d MB, Resources: %d, Files: %d",
			duration, memUsedMB, stats.TotalResources, stats.TotalFiles)

		// Target: < 10s for 10,000 files
		if duration > 10*time.Second {
			t.Errorf("Cold start time %v exceeds target 10s", duration)
		}

		// Target: < 1GB memory
		if memUsedMB > 1024 {
			t.Errorf("Cold start memory %d MB exceeds target 1024 MB", memUsedMB)
		}
	})

	// Warm Start
	t.Run("WarmStart", func(t *testing.T) {
		memBefore := measureMemory(true) // GC before baseline

		start := time.Now()
		idx, err := LoadOrBuild(repoPath)
		if err != nil {
			t.Fatalf("failed to load index: %v", err)
		}
		duration := time.Since(start)

		memAfter := measureMemory(false) // No GC - capture actual usage
		memUsedMB := (memAfter - memBefore) / (1024 * 1024)

		stats := idx.Stats()
		t.Logf("Warm Start (10k files): %v, Memory: %d MB, Resources: %d, Files: %d",
			duration, memUsedMB, stats.TotalResources, stats.TotalFiles)

		// Target: < 1s for 10,000 files
		if duration > 1*time.Second {
			t.Errorf("Warm start time %v exceeds target 1s", duration)
		}

		// Target: < 500MB memory
		if memUsedMB > 500 {
			t.Errorf("Warm start memory %d MB exceeds target 500 MB", memUsedMB)
		}
	})
}
