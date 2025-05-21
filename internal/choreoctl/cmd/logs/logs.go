/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package logs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/kinds"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type LogsImpl struct{}

func NewLogsImpl() *LogsImpl {
	return &LogsImpl{}
}

func (i *LogsImpl) GetLogs(params api.LogParams) error {
	if params.Interactive {
		return getLogsInteractive()
	}
	return handleLogs(params)
}

func handleLogs(params api.LogParams) error {
	if err := validation.ValidateParams(validation.CmdLogs, validation.ResourceLogs, params); err != nil {
		return err
	}

	// If TailLines is not set, provide a default value
	if params.TailLines <= 0 {
		params.TailLines = 100
	}

	switch params.Type {
	case "build":
		return getBuildLogs(params)
	case "deployment":
		return getDeploymentLogs(params)
	default:
		return fmt.Errorf("log type '%s' not supported", params.Type)
	}
}

func getBuildLogs(params api.LogParams) error {
	if params.Organization == "" || params.Build == "" {
		return fmt.Errorf("organization and build name are required for build logs")
	}

	buildRes, err := kinds.NewBuildResource(
		constants.BuildV1Config,
		params.Organization,
		params.Project,
		params.Component,
		params.DeploymentTrack,
	)
	if err != nil {
		return fmt.Errorf("failed to create Build resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name: params.Build,
	}

	// Get all builds matching the filter
	builds, err := buildRes.List()
	if err != nil {
		return fmt.Errorf("failed to list builds: %w", err)
	}

	// Filter by name if needed
	if filter.Name != "" {
		filtered, err := resources.FilterByName(builds, filter.Name)
		if err != nil {
			return fmt.Errorf("build '%s' not found: %w", params.Build, err)
		}
		builds = filtered
	}

	if len(builds) == 0 {
		return fmt.Errorf("build '%s' not found", params.Build)
	}
	if len(builds) > 1 {
		return fmt.Errorf("multiple builds found with name '%s'", params.Build)
	}

	fmt.Print("\nFetching build logs...\n")
	buildWrapper := builds[0]

	// Get the Kubernetes name directly from the wrapper
	buildK8sName := buildWrapper.GetKubernetesName()
	buildNamespace := fmt.Sprintf("choreo-ci-%s", params.Organization)

	// Get k8s client
	k8sClient, err := resources.GetClient()
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Get all pods in the namespace with workflow label matching build's k8s name
	pods := &corev1.PodList{}
	if err := k8sClient.List(context.Background(), pods,
		client.InNamespace(buildNamespace),
		client.MatchingLabels{"workflow": dpkubernetes.GenerateK8sNameWithLengthLimit(63, buildK8sName)}); err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no build pods found for build '%s'", params.Build)
	}

	// Sort pods by creation timestamp to show logs in order
	sort.Slice(pods.Items, func(i, j int) bool {
		return pods.Items[i].CreationTimestamp.Before(&pods.Items[j].CreationTimestamp)
	})

	// Get logs for each pod
	for _, pod := range pods.Items {
		step := pod.Labels["step"]
		fmt.Printf("\n=== Build %s ===\n", step)

		// Pass tailLines parameter properly
		tailLinesPtr := &params.TailLines
		logs, err := GetPodLogs(pod.Name, buildNamespace, "main", params.Follow, tailLinesPtr)
		if err != nil {
			return fmt.Errorf("failed to get logs for pod %s: %w", pod.Name, err)
		}
		fmt.Println(logs)
	}
	return nil
}

func getDeploymentLogs(params api.LogParams) error {
	if params.Organization == "" || params.Project == "" ||
		params.Component == "" || params.Environment == "" || params.Deployment == "" {
		return fmt.Errorf("organization, project, component, environment and deployment values are required for deployment logs")
	}

	deployRes, err := kinds.NewDeploymentResource(
		constants.DeploymentV1Config,
		params.Organization,
		params.Project,
		params.Component,
		params.Environment,
	)
	if err != nil {
		return fmt.Errorf("failed to create Deployment resource: %w", err)
	}

	filter := &resources.ResourceFilter{
		Name: params.Deployment,
	}

	// Check if deployment exists
	deployments, err := deployRes.List()
	if err != nil {
		return fmt.Errorf("failed to list deployments: %w", err)
	}

	// Filter by name if needed
	if filter.Name != "" {
		filtered, err := resources.FilterByName(deployments, filter.Name)
		if err != nil {
			return fmt.Errorf("deployment '%s' not found: %w", params.Deployment, err)
		}
		deployments = filtered
	}

	if len(deployments) == 0 {
		return fmt.Errorf("deployment '%s' not found", params.Deployment)
	}

	k8sClient, err := resources.GetClient()
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Get all pods with matching deployment labels
	pods := &corev1.PodList{}
	if err := k8sClient.List(context.Background(), pods,
		client.MatchingLabels{
			"organization-name": params.Organization,
			"project-name":      params.Project,
			"component-name":    params.Component,
			"environment-name":  params.Environment,
			"deployment-name":   params.Deployment,
			"belong-to":         "user-workloads",
			"managed-by":        "choreo-deployment-controller",
		}); err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no deployment pods found for component '%s' in environment '%s'",
			params.Component, params.Environment)
	}

	// Sort pods by creation timestamp to show newest first
	sort.Slice(pods.Items, func(i, j int) bool {
		return pods.Items[i].CreationTimestamp.After(pods.Items[j].CreationTimestamp.Time)
	})

	tailLinesPtr := &params.TailLines

	// If following logs, only show the latest pod
	if params.Follow {
		pod := pods.Items[0]
		fmt.Printf("\n=== Pod: %s ===\n", pod.Name)
		logs, err := GetPodLogs(pod.Name, pod.Namespace, "", true, tailLinesPtr)
		if err != nil {
			return fmt.Errorf("failed to get logs for pod %s: %w", pod.Name, err)
		}
		fmt.Println("=======================================")
		fmt.Println(logs)
		return nil
	}

	// Show logs from all pods if not following
	for _, pod := range pods.Items {
		fmt.Printf("\n=== Pod: %s ===\n", pod.Name)
		logs, err := GetPodLogs(pod.Name, pod.Namespace, "", false, tailLinesPtr)
		if err != nil {
			return fmt.Errorf("failed to get logs for pod %s: %w", pod.Name, err)
		}
		fmt.Println(logs)
	}

	return nil
}

func GetPodLogs(podName, namespace, containerName string, follow bool, tailLines *int64) (string, error) {
	k8sClient, err := resources.GetClient()
	if err != nil {
		return "", fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Get pod to verify it exists
	pod := &corev1.Pod{}
	if err := k8sClient.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      podName,
	}, pod); err != nil {
		return "", fmt.Errorf("pod %s not found in namespace %s: %w", podName, namespace, err)
	}

	// Use default container if not specified
	if containerName == "" {
		if len(pod.Spec.Containers) > 0 {
			containerName = pod.Spec.Containers[0].Name
		} else {
			return "", fmt.Errorf("no containers found in pod %s", podName)
		}
	}

	// Get pod logs
	config, err := resources.GetRESTConfig()
	if err != nil {
		return "", fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", fmt.Errorf("failed to create clientset: %w", err)
	}

	// Configure log options
	opts := &corev1.PodLogOptions{
		Container: containerName,
	}

	// Only set TailLines if a positive value is provided
	if tailLines != nil && *tailLines > 0 {
		opts.TailLines = tailLines
	}

	if follow {
		opts.Follow = true
	}

	// Get log stream
	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, opts)
	stream, err := req.Stream(context.Background())
	if err != nil {
		return "", fmt.Errorf("failed to get log stream: %w", err)
	}
	defer stream.Close()

	// Handle log streaming
	if follow {
		_, err = io.Copy(os.Stdout, stream)
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("error streaming logs: %w", err)
		}
		return "", nil
	}

	// Read all logs for non-follow mode
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, stream)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("error reading logs: %w", err)
	}

	return buf.String(), nil
}
