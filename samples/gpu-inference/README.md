# GPU Model Inference with Ollama

This sample deploys an Ollama server as an OpenChoreo component, asks Kubernetes for one NVIDIA GPU, persists downloaded models, and calls two small language models through Ollama's HTTP API.

## Prerequisites

- An OpenChoreo installation with the `deployment/service`-style APIs available and the default `development` environment and `default` project.
- A Data Plane Kubernetes cluster with an NVIDIA GPU worker node.
- NVIDIA GPU support installed in that cluster. The [NVIDIA GPU Operator](https://github.com/NVIDIA/gpu-operator) can install the drivers, container toolkit, and device plugin. Follow NVIDIA's [installation guide](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/getting-started.html) for the cluster and driver configuration. Check the GPU Operator's supported Kubernetes versions and hardware for your host before installing it.
- A default StorageClass, or a storage class configured for the `model-cache` trait.

For a compatible cluster where the Operator should install the NVIDIA drivers, the documented Helm installation is:

```bash
helm upgrade --install gpu-operator \
  oci://nvcr.io/nvidia/cloud-native-charts/gpu-operator \
  --version v26.7.0 \
  --namespace gpu-operator \
  --create-namespace \
  --wait
```

If the GPU drivers are already installed on the worker nodes, follow NVIDIA's guide for the `driver.enabled=false` configuration. GPU Operator installation is cluster-wide and requires privileged node access; confirm the host OS, kernel, Kubernetes distribution, and driver state before applying it.

After installing GPU support, confirm a GPU node advertises `nvidia.com/gpu` and has the `nvidia.com/gpu.present=true` label. The sample uses that label for platform-controlled placement. If your GPU nodes use a different label or are tainted, update the platform-owned `gpu-placement` ReleaseBinding configuration and add a matching toleration policy to the Trait before deployment.

## Deploy the sample

Apply the ComponentType, Trait, Component, Workload, and ReleaseBinding:

```bash
kubectl apply --server-side -f samples/gpu-inference/gpu-inference.yaml
```

Wait for the deployment and inspect the allocated resources:

```bash
kubectl get releasebinding ollama-model-server-development -o yaml
kubectl get pods -A -l openchoreo.dev/component=ollama-model-server -o wide
kubectl describe pod -n <workload-namespace> <ollama-pod>
```

The pod should be scheduled on a node that has GPU capacity. Its container limit requests one `nvidia.com/gpu`. The platform-owned Trait controls the node selector; application teams do not choose the GPU node label in their Workload.

## Download and call two small models

Forward the internal Ollama service to the local machine:

```bash
kubectl port-forward -n <workload-namespace> service/ollama-model-server 11434:11434
```

In another terminal, download two small instruction models. Model downloads are stored in the component's persistent volume:

```bash
kubectl exec -n <workload-namespace> deploy/ollama-model-server -- ollama pull qwen2.5:0.5b-instruct
kubectl exec -n <workload-namespace> deploy/ollama-model-server -- ollama pull smollm2:360m-instruct-q4_K_M
```

Ask each model a question through Ollama's chat API:

```bash
curl --fail-with-body http://localhost:11434/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"model":"qwen2.5:0.5b-instruct","stream":false,"messages":[{"role":"user","content":"Reply with exactly: qwen is running"}]}'

curl --fail-with-body http://localhost:11434/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"model":"smollm2:360m-instruct-q4_K_M","stream":false,"messages":[{"role":"user","content":"Reply with exactly: smollm is running"}]}'
```

Both model tags are listed in the [Ollama model library](https://ollama.com/library). The listed model artifacts are a few hundred megabytes each; runtime memory also depends on context size and concurrent requests.

After making a request, check the loaded model's processor allocation:

```bash
kubectl exec -n <workload-namespace> deploy/ollama-model-server -- ollama ps
```

The `PROCESSOR` column should show GPU use. Also inspect the NVIDIA device plugin and Ollama pod logs if the model runs on CPU or inference fails.

## Cleanup

```bash
kubectl delete -f samples/gpu-inference/gpu-inference.yaml
```

Check whether the generated PVC remains after deleting the component; if it does, remove it and its backing volume separately to reclaim model storage.

This sample validates the OpenChoreo-to-Ollama serving path. Connecting Agent Manager to the internal Ollama endpoint is a follow-up integration step.
