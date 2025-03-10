/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package build

import (
	"encoding/base64"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	dpkubernetes "github.com/choreo-idp/choreo/internal/dataplane/kubernetes"
	argo "github.com/choreo-idp/choreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	"github.com/choreo-idp/choreo/internal/ptr"
)

func makeArgoWorkflow(build *choreov1.Build, repo string, buildNamespace string) *argo.Workflow {
	workflow := argo.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dpkubernetes.GenerateK8sNameWithLengthLimit(63, build.ObjectMeta.Name),
			Namespace: buildNamespace,
		},
		Spec: makeWorkflowSpec(build, repo),
	}
	return &workflow
}

func makeWorkflowSpec(build *choreov1.Build, repo string) argo.WorkflowSpec {
	hostPathType := corev1.HostPathDirectoryOrCreate
	return argo.WorkflowSpec{
		ServiceAccountName: "argo-workflow-sa",
		Entrypoint:         "build-workflow",
		Templates: []argo.Template{
			{
				Name: "build-workflow",
				Steps: []argo.ParallelSteps{
					{
						Steps: []argo.WorkflowStep{
							{Name: string(CloneStep), Template: string(CloneStep)},
						},
					},
					{
						Steps: []argo.WorkflowStep{
							{
								Name:     string(BuildStep),
								Template: string(BuildStep),
								Arguments: argo.Arguments{
									Parameters: []argo.Parameter{
										{
											Name:  "git-revision",
											Value: ptr.String("{{steps.clone-step.outputs.parameters.git-revision}}"),
										},
									},
								},
							},
						},
					},
					{
						Steps: []argo.WorkflowStep{
							{
								Name:     string(PushStep),
								Template: string(PushStep),
								Arguments: argo.Arguments{
									Parameters: []argo.Parameter{
										{
											Name:  "git-revision",
											Value: ptr.String("{{steps.clone-step.outputs.parameters.git-revision}}"),
										},
									},
								},
							},
						},
					},
				},
			},
			makeCloneStep(build, repo),
			makeBuildStep(build),
			makePushStep(build),
		},
		VolumeClaimTemplates: makePersistentVolumeClaim(),
		Affinity:             makeNodeAffinity(),
		Volumes: []corev1.Volume{
			{
				Name: "podman-cache",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/shared/podman/cache",
						Type: &hostPathType,
					},
				},
			},
		},
		TTLStrategy: &argo.TTLStrategy{
			SecondsAfterFailure: ptr.Int32(3600),
			SecondsAfterSuccess: ptr.Int32(3600),
		},
	}
}

func makeCloneStep(build *choreov1.Build, repo string) argo.Template {
	branch := ""
	gitRevision := ""
	if build.Spec.Branch != "" {
		branch = build.Spec.Branch
	} else if build.Spec.GitRevision != "" {
		gitRevision = build.Spec.GitRevision[:8]
	} else {
		branch = "main"
	}
	return argo.Template{
		Name: string(CloneStep),
		Metadata: argo.Metadata{
			Labels: map[string]string{
				"step":     string(CloneStep),
				"workflow": build.ObjectMeta.Name,
			},
		},
		Container: &corev1.Container{
			Image:   "alpine/git",
			Command: []string{"sh", "-c"},
			Args:    generateCloneArgs(repo, branch, gitRevision),
			VolumeMounts: []corev1.VolumeMount{
				{Name: "workspace", MountPath: "/mnt/vol"},
			},
		},
		Outputs: argo.Outputs{
			Parameters: []argo.Parameter{
				{
					Name: "git-revision",
					ValueFrom: &argo.ValueFrom{
						Path: "/tmp/git-revision.txt",
					},
				},
			},
		},
	}
}

func makeBuildStep(build *choreov1.Build) argo.Template {
	return argo.Template{
		Name: string(BuildStep),
		Inputs: argo.Inputs{
			Parameters: []argo.Parameter{
				{
					Name: "git-revision",
				},
			},
		},
		Metadata: argo.Metadata{
			Labels: map[string]string{
				"step":     string(BuildStep),
				"workflow": build.ObjectMeta.Name,
			},
		},
		Container: &corev1.Container{
			Image: "chalindukodikara/podman-runner:1.0",
			SecurityContext: &corev1.SecurityContext{
				Privileged: ptr.Bool(true),
			},
			Command: []string{"sh", "-c"},
			Args:    generateBuildArgs(build, constructImageNameWithTag(build)),
			VolumeMounts: []corev1.VolumeMount{
				{Name: "workspace", MountPath: "/mnt/vol"},
				{Name: "podman-cache", MountPath: "/shared/podman/cache"},
			},
		},
	}
}

func makePushStep(build *choreov1.Build) argo.Template {
	return argo.Template{
		Name: string(PushStep),
		Inputs: argo.Inputs{
			Parameters: []argo.Parameter{
				{
					Name: "git-revision",
				},
			},
		},
		Metadata: argo.Metadata{
			Labels: map[string]string{
				"step":     string(PushStep),
				"workflow": build.ObjectMeta.Name,
			},
		},
		Container: &corev1.Container{
			Image: "chalindukodikara/podman-runner:1.0",
			SecurityContext: &corev1.SecurityContext{
				Privileged: ptr.Bool(true),
			},
			Command: []string{"sh", "-c"},
			Args: []string{
				generatePushImageScript(constructImageNameWithTag(build)),
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "workspace", MountPath: "/mnt/vol"},
			},
		},
		Outputs: argo.Outputs{
			Parameters: []argo.Parameter{
				{
					Name: "image",
					ValueFrom: &argo.ValueFrom{
						Path: "/tmp/image.txt",
					},
				},
			},
		},
	}
}

// TODO: Remove hard coded worker node for node affinity
func makeNodeAffinity() *corev1.Affinity {
	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "kubernetes.io/hostname",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"choreo-worker"},
							},
						},
					},
				},
			},
		},
	}
}

func makePersistentVolumeClaim() []corev1.PersistentVolumeClaim {
	return []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "workspace",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("2Gi"),
					},
				},
			},
		},
	}
}

func getDockerContext(build *choreov1.Build) string {
	if build.Spec.BuildConfiguration.Docker.Context != "" {
		return build.Spec.BuildConfiguration.Docker.Context
	}
	return build.Spec.Path
}

func getDockerfilePath(build *choreov1.Build) string {
	if build.Spec.BuildConfiguration.Docker.DockerfilePath != "" {
		return build.Spec.BuildConfiguration.Docker.DockerfilePath
	}
	return "Dockerfile"
}

func getLanguageVersion(build *choreov1.Build) string {
	if build.Spec.BuildConfiguration.Buildpack.Version == "" {
		return ""
	}
	if build.Spec.BuildConfiguration.Buildpack.Name == choreov1.BuildpackGo {
		return fmt.Sprintf("--env GOOGLE_GO_VERSION=%q", build.Spec.BuildConfiguration.Buildpack.Version)
	} else if build.Spec.BuildConfiguration.Buildpack.Name == choreov1.BuildpackNodeJS {
		return fmt.Sprintf("--env GOOGLE_NODEJS_VERSION=%s", build.Spec.BuildConfiguration.Buildpack.Version)
	} else if build.Spec.BuildConfiguration.Buildpack.Name == choreov1.BuildpackPython {
		return fmt.Sprintf("--env GOOGLE_PYTHON_VERSION=%q", build.Spec.BuildConfiguration.Buildpack.Version)
	} else if build.Spec.BuildConfiguration.Buildpack.Name == choreov1.BuildpackPHP {
		return fmt.Sprintf("--env GOOGLE_COMPOSER_VERSION=%q", build.Spec.BuildConfiguration.Buildpack.Version)
	}
	// BuildpackRuby and BuildpackJava
	return fmt.Sprintf("--env GOOGLE_RUNTIME_VERSION=%s", build.Spec.BuildConfiguration.Buildpack.Version)
}

func generateCloneArgs(repo string, branch string, gitRevision string) []string {
	if branch != "" {
		return []string{
			fmt.Sprintf(`set -e
git clone --single-branch --branch %s --depth 1 %s /mnt/vol/source
cd /mnt/vol/source
COMMIT_SHA=$(git rev-parse HEAD)
echo -n "$COMMIT_SHA" | cut -c1-8 > /tmp/git-revision.txt`, branch, repo),
		}
	}
	return []string{
		fmt.Sprintf(`set -e
git clone --no-checkout --depth 1 %s /mnt/vol/source
cd /mnt/vol/source
git config --global advice.detachedHead false
git fetch --depth 1 origin %s
git checkout %s
echo -n "%s" > /tmp/git-revision.txt`, repo, gitRevision, gitRevision, gitRevision),
	}
}

func generateBuildArgs(build *choreov1.Build, imageName string) []string {
	baseScript := `set -e

mkdir -p /etc/containers
cat <<EOF > /etc/containers/storage.conf
[storage]
driver = "overlay"
runroot = "/run/containers/storage"
graphroot = "/var/lib/containers/storage"
[storage.options.overlay]
mount_program = "/usr/bin/fuse-overlayfs"
EOF`

	var buildScript string

	if build.Spec.BuildConfiguration.Buildpack != nil {
		if build.Spec.BuildConfiguration.Buildpack.Name == choreov1.BuildpackReact {
			buildScript = makeReactBuildScript(build.Spec.BuildConfiguration.Buildpack.Version, build.Spec.Path, imageName)
		} else if build.Spec.BuildConfiguration.Buildpack.Name == choreov1.BuildpackBallerina {
			buildScript = makeBuildpackBuildScript(build, imageName, true)
		} else {
			buildScript = makeBuildpackBuildScript(build, imageName, false)
		}
	} else {
		buildScript = makeDockerfileBuildScript(build, imageName)
	}

	return []string{baseScript + buildScript}
}

func generatePushImageScript(imageName string) string {
	return fmt.Sprintf(`set -e
GIT_REVISION={{inputs.parameters.git-revision}}
mkdir -p /etc/containers
cat <<EOF > /etc/containers/storage.conf
[storage]
driver = "overlay"
runroot = "/run/containers/storage"
graphroot = "/var/lib/containers/storage"
[storage.options.overlay]
mount_program = "/usr/bin/fuse-overlayfs"
EOF

podman load -i /mnt/vol/app-image.tar
podman tag %s-$GIT_REVISION registry.choreo-system:5000/%s-$GIT_REVISION
podman push --tls-verify=false registry.choreo-system:5000/%s-$GIT_REVISION

podman rmi %s-$GIT_REVISION -f
echo -n "%s-$GIT_REVISION" > /tmp/image.txt`, imageName, imageName, imageName, imageName, imageName)
}

func makeDockerfileBuildScript(build *choreov1.Build, imageName string) string {
	return fmt.Sprintf(`
podman build -t %s-{{inputs.parameters.git-revision}} -f /mnt/vol/source%s /mnt/vol/source%s
podman save -o /mnt/vol/app-image.tar %s-{{inputs.parameters.git-revision}}`, imageName, getDockerfilePath(build), getDockerContext(build), imageName)
}

func makeReactBuildScript(nodeVersion, path, imageName string) string {
	targetDir := fmt.Sprintf("/mnt/vol/source%s", path)
	imageReference := fmt.Sprintf("%s-{{inputs.parameters.git-revision}}", imageName)

	return fmt.Sprintf(`
apk add --no-cache coreutils

echo %s | base64 -d > %s/Dockerfile
echo %s | base64 -d > %s/default.conf

DOCKER_BUILDKIT=1 podman build -t %s -f %s/Dockerfile %s

podman save -o /mnt/vol/app-image.tar %s`,
		getDockerfileContent(nodeVersion), targetDir,
		getNginxConfig(), targetDir,
		imageReference, targetDir, targetDir,
		imageReference,
	)
}

func makeBuildpackBuildScript(build *choreov1.Build, imageName string, isBallerina bool) string {
	baseScript := `
podman system service --time=0 &
until podman info --format '{{.Host.RemoteSocket.Exists}}' 2>/dev/null | grep -q "true"; do
  sleep 1
done`
	if isBallerina {
		return baseScript + makeBallerinaBuildScript(imageName, build.Spec.Path)
	}
	return baseScript + makeGoogleBuildpackBuildScript(imageName, build)
}

func makeBuilderCacheScript(image, cachePath string) string {
	return fmt.Sprintf(`
if [[ ! -f "%s" ]]; then
  podman pull %s
  podman save -o %s %s
else
  if ! podman load -i %s; then
    podman pull %s
    podman save -o %s %s
  fi
fi`, cachePath, image, cachePath, image, cachePath, image, cachePath, image)
}

func makeBallerinaBuildScript(imageName, path string) string {
	return fmt.Sprintf(`
%s

/usr/local/bin/pack build %s-{{inputs.parameters.git-revision}} --builder=chalindukodikara/choreo-buildpack:ballerina-builder \
--docker-host=inherit --path=/mnt/vol/source%s --volume "/mnt/vol":/app/generated-artifacts:rw --pull-policy if-not-present

podman save -o /mnt/vol/app-image.tar %s-{{inputs.parameters.git-revision}}`,
		makeBuilderCacheScript("chalindukodikara/choreo-buildpack:ballerina-builder", "/shared/podman/cache/ballerina-builder.tar"),
		imageName, path, imageName)
}

func makeGoogleBuildpackBuildScript(imageName string, build *choreov1.Build) string {
	return fmt.Sprintf(`
%s

%s

/usr/local/bin/pack build %s-{{inputs.parameters.git-revision}} --builder=gcr.io/buildpacks/builder:google-22 \
--docker-host=inherit --path=/mnt/vol/source%s --pull-policy if-not-present %s

podman save -o /mnt/vol/app-image.tar %s-{{inputs.parameters.git-revision}}`,
		makeBuilderCacheScript("gcr.io/buildpacks/builder:google-22", "/shared/podman/cache/google-builder.tar"),
		makeBuilderCacheScript("gcr.io/buildpacks/google-22/run:latest", "/shared/podman/cache/google-run.tar"),
		imageName, build.Spec.Path, getLanguageVersion(build), imageName)
}

func getDockerfileContent(nodeVersion string) string {
	dockerfile := fmt.Sprintf(`
FROM node:%s-alpine as builder

RUN npm install -g pnpm

WORKDIR /app

COPY . .

RUN if [ -f "package-lock.json" ]; then \
    npm ci; \
  elif [ -f "yarn.lock" ]; then \
    yarn install --frozen-lockfile; \
  elif [ -f "pnpm-lock.yaml" ]; then \
    pnpm install --frozen-lockfile; \
  else \
    echo "No valid lock file found"; \
    exit 1; \
  fi

COPY . .

RUN if [ -f "package-lock.json" ]; then \
    npm run build; \
  elif [ -f "yarn.lock" ]; then \
    yarn run build; \
  elif [ -f "pnpm-lock.yaml" ]; then \
    pnpm run build; \
  fi

FROM nginx:alpine3.20

ENV ENABLE_PERMISSIONS=TRUE
ENV DEBUG_PERMISSIONS=TRUE
ENV USER_NGINX=10015
ENV GROUP_NGINX=10015

WORKDIR /usr/share/nginx/html

COPY --from=builder /app/default.conf /etc/nginx/conf.d/default.conf

ARG OUTPUT_DIR=build  # Default output directory for React
COPY --from=builder /app/${OUTPUT_DIR} /usr/share/nginx/html/

RUN chown -R nginx:nginx /usr/share/nginx/html

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]`, nodeVersion)
	return getBase64FromString(dockerfile)
}

func getNginxConfig() string {
	nginxConfig := `server {
  listen 80;
  location / {
    root   /usr/share/nginx/html;
    index  index.html index.htm;
    try_files $uri $uri/ /index.html; 
  }
} `
	return getBase64FromString(nginxConfig)
}

func getBase64FromString(content string) string {
	return base64.StdEncoding.EncodeToString([]byte(content))
}
