package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
)

type ContainerSetTemplate struct {
	Containers   []ContainerNode      `json:"containers" protobuf:"bytes,4,rep,name=containers"`
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty" protobuf:"bytes,3,rep,name=volumeMounts"`
	// RetryStrategy describes how to retry container nodes if the container set fails.
	// Note that this works differently from the template-level `retryStrategy` as it is a process-level retry that does not create new Pods or containers.
	RetryStrategy *ContainerSetRetryStrategy `json:"retryStrategy,omitempty" protobuf:"bytes,5,opt,name=retryStrategy"`
}

// ContainerSetRetryStrategy provides controls on how to retry a container set
type ContainerSetRetryStrategy struct {
	// Duration is the time between each retry, examples values are "300ms", "1s" or "5m".
	// Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
	Duration string `json:"duration,omitempty" protobuf:"bytes,1,opt,name=duration"`
	// Retries is the maximum number of retry attempts for each container. It does not include the
	// first, original attempt; the maximum number of total attempts will be `retries + 1`.
	Retries *intstr.IntOrString `json:"retries" protobuf:"bytes,2,rep,name=retries"`
}

type ContainerNode struct {
	corev1.Container `json:",inline" protobuf:"bytes,1,opt,name=container"`
	Dependencies     []string `json:"dependencies,omitempty" protobuf:"bytes,2,rep,name=dependencies"`
}
