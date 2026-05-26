// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

const (
	clusterDataPlane   = "e2e-shared"
	openChoreoAPIVer   = "openchoreo.dev/v1alpha1"
	kubernetesAPIVerV1 = "v1"

	projectName = "alerts-proj"
	envDev      = "development"
	envStaging  = "staging"

	componentMetric        = "alert-metric-svc"
	componentLog           = "alert-log-svc"
	componentBuildLogs     = "alert-build-svc"
	releaseBindingSuffix   = "-" + envDev
	servicePort            = 9090
	notificationChannel    = "webhook-notification-channel-development"
	imageGreeter           = "ghcr.io/openchoreo/samples/greeter-service:latest"
	curlImage              = "curlimages/curl:8.10.1"
	curlPodLabel           = "app=alerts-tester"
	curlContainer          = "tester"
	alertRuleMetric        = "alert-metric-cpu"
	alertRuleLog           = "alert-log-error"
	alertReceiverNamespace = "e2e-alert-webhook"
)

var alertRunID = fmt.Sprintf("%d", time.Now().UnixNano())

var cpNs = fmt.Sprintf("e2e-alerts-%s", alertRunID)

func mustYAMLDocs(objects ...any) string {
	docs := make([]string, 0, len(objects))
	for _, obj := range objects {
		data, err := yaml.Marshal(obj)
		if err != nil {
			panic(fmt.Sprintf("failed to marshal yaml document: %v", err))
		}
		docs = append(docs, strings.TrimSpace(string(data)))
	}
	return strings.Join(docs, "\n---\n")
}

func cpNamespaceYAML() string {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{
			Name: cpNs,
			Labels: map[string]string{
				"openchoreo.dev/control-plane": "true",
			},
		},
	}
	return mustYAMLDocs(ns)
}

func platformResourcesYAML() string {
	pipeline := &openchoreov1alpha1.DeploymentPipeline{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "DeploymentPipeline"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": "default"},
		},
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{{
				SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: envDev},
				TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
					{Name: envStaging},
				},
			}},
		},
	}
	envs := make([]any, 0, 2)
	for _, name := range []string{envDev, envStaging} {
		envs = append(envs, &openchoreov1alpha1.Environment{
			TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Environment"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: cpNs,
				Labels:    map[string]string{"openchoreo.dev/name": name},
			},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindClusterDataPlane,
					Name: clusterDataPlane,
				},
			},
		})
	}
	proj := &openchoreov1alpha1.Project{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Project"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      projectName,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": projectName},
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"},
		},
	}
	docs := []any{pipeline}
	docs = append(docs, envs...)
	docs = append(docs, proj)
	return mustYAMLDocs(docs...)
}

// notificationChannelYAML applies a webhook-type ObservabilityAlertsNotificationChannel
// targeting the in-cluster webhook receiver. The receiver namespace is fixed
// so the URL stays stable across spec runs.
func notificationChannelYAML() string {
	body := map[string]any{
		"apiVersion": openChoreoAPIVer,
		"kind":       "ObservabilityAlertsNotificationChannel",
		"metadata": map[string]any{
			"name":      notificationChannel,
			"namespace": cpNs,
		},
		"spec": map[string]any{
			"environment":  envDev,
			"isEnvDefault": true,
			"type":         "webhook",
			"webhookConfig": map[string]any{
				"url": framework.WebhookReceiverURL(alertReceiverNamespace),
				"headers": map[string]any{
					"Content-Type": map[string]any{
						"value": "application/json",
					},
				},
			},
		},
	}
	return mustYAMLDocs(body)
}

// alertComponentYAML returns a service-flavour Component+Workload+
// ReleaseBinding for the alert specs. The ReleaseBinding is needed because
// the alert-rule trait validation rejects rules without an explicit
// notifications channel (the trait CEL falls back to
// `environment.defaultNotificationChannel`, which the e2e environment does
// not surface). Setting
// `releaseBinding.spec.traitEnvironmentConfigs.<ruleName>.actions.notifications.channels`
// pins the channel deterministically.
func alertComponentYAML(componentName, ruleName string, params map[string]any) string {
	comp := map[string]any{
		"apiVersion": openChoreoAPIVer,
		"kind":       "Component",
		"metadata": map[string]any{
			"name":      componentName,
			"namespace": cpNs,
			"labels":    map[string]string{"openchoreo.dev/name": componentName},
		},
		"spec": map[string]any{
			"owner": map[string]any{"projectName": projectName},
			"componentType": map[string]any{
				"kind": "ClusterComponentType",
				"name": "deployment/service",
			},
			"autoDeploy": true,
			"traits": []any{map[string]any{
				"name":         "observability-alert-rule",
				"kind":         "ClusterTrait",
				"instanceName": ruleName,
				"parameters":   params,
			}},
		},
	}
	wl := map[string]any{
		"apiVersion": openChoreoAPIVer,
		"kind":       "Workload",
		"metadata": map[string]any{
			"name":      componentName,
			"namespace": cpNs,
			"labels":    map[string]string{"openchoreo.dev/name": componentName},
		},
		"spec": map[string]any{
			"owner": map[string]any{
				"projectName":   projectName,
				"componentName": componentName,
			},
			"endpoints": map[string]any{
				"http": map[string]any{
					"type":       "HTTP",
					"port":       servicePort,
					"visibility": []string{"project"},
				},
			},
			"container": map[string]any{
				"image": imageGreeter,
				"args":  []string{"--port", strconv.Itoa(servicePort)},
			},
		},
	}
	rb := map[string]any{
		"apiVersion": openChoreoAPIVer,
		"kind":       "ReleaseBinding",
		"metadata": map[string]any{
			"name":      componentName + releaseBindingSuffix,
			"namespace": cpNs,
		},
		"spec": map[string]any{
			"owner": map[string]any{
				"projectName":   projectName,
				"componentName": componentName,
			},
			"environment": envDev,
			"traitEnvironmentConfigs": map[string]any{
				ruleName: map[string]any{
					"enabled": true,
					"actions": map[string]any{
						"notifications": map[string]any{
							"channels": []string{notificationChannel},
						},
					},
				},
			},
		},
	}
	return mustYAMLDocs(comp, wl, rb)
}

// metricAlertParams returns parameter values for a metric-based alert rule
// that mirrors `recommendation-high-cpu-alert` but uses a deliberately low
// CPU threshold (1%) so a tiny pod still trips it without any synthetic
// load. This keeps the spec independent of CPU stress shaping in CI.
func metricAlertParams() map[string]any {
	return map[string]any{
		"description": "e2e metric alert: trigger when CPU > 1% over 1m window",
		"severity":    "warning",
		"source": map[string]any{
			"type":   "metric",
			"metric": "cpu_usage",
		},
		"condition": map[string]any{
			"window":    "1m",
			"interval":  "1m",
			"operator":  "gt",
			"threshold": 1,
		},
	}
}

// logAlertParams returns parameter values for a log-based alert rule that
// mirrors `frontend-rpc-unavailable-error-log-alert`. The query phrase
// matches a stderr line that the greeter's `--port 0` startup deliberately
// emits, so we can trip it just by re-creating the workload with bad args.
func logAlertParams(searchPhrase string) map[string]any {
	return map[string]any{
		"description": "e2e log alert: trigger when an error pattern appears",
		"severity":    "warning",
		"source": map[string]any{
			"type":  "log",
			"query": searchPhrase,
		},
		"condition": map[string]any{
			"window":    "1m",
			"interval":  "1m",
			"operator":  "gt",
			"threshold": 1,
		},
	}
}

// curlPodYAML returns a curl-enabled tester pod for the alerts suite.
// Shares the labelling shape used by the observability suite so the
// framework's exec-by-label helpers behave identically.
func curlPodYAML(dpNamespace string) string {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "alerts-tester",
			Namespace: dpNamespace,
			Labels: map[string]string{
				"app":                       "alerts-tester",
				"openchoreo.dev/project":    projectName,
				"openchoreo.dev/managed-by": "e2e-alerts",
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:    curlContainer,
			Image:   curlImage,
			Command: []string{"sleep", "3600"},
		}}},
	}
	return mustYAMLDocs(pod)
}
