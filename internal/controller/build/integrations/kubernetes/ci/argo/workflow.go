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

package argo

import (
	"encoding/base64"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations/kubernetes"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations/kubernetes/ci"
	dpkubernetes "github.com/choreo-idp/choreo/internal/dataplane/kubernetes"
	argoproj "github.com/choreo-idp/choreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	"github.com/choreo-idp/choreo/internal/ptr"
)

func makeArgoWorkflow(buildCtx *integrations.BuildContext) *argoproj.Workflow {
	workflow := argoproj.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeWorkflowName(buildCtx),
			Namespace: kubernetes.MakeNamespaceName(buildCtx),
			Labels: map[string]string{
				dpkubernetes.LabelKeyManagedBy: dpkubernetes.LabelBuildControllerCreated,
			},
		},
		Spec: makeWorkflowSpec(buildCtx.Build, buildCtx.Component.Spec.Source.GitRepository.URL),
	}
	return &workflow
}

func makeWorkflowSpec(buildObj *choreov1.Build, repo string) argoproj.WorkflowSpec {
	hostPathType := corev1.HostPathDirectoryOrCreate
	return argoproj.WorkflowSpec{
		ServiceAccountName: makeServiceAccountName(),
		Entrypoint:         "build-workflow",
		Templates: []argoproj.Template{
			{
				Name: "build-workflow",
				Steps: []argoproj.ParallelSteps{
					{
						Steps: []argoproj.WorkflowStep{
							{Name: string(integrations.CloneStep), Template: string(integrations.CloneStep)},
						},
					},
					{
						Steps: []argoproj.WorkflowStep{
							{
								Name:     string(integrations.BuildStep),
								Template: string(integrations.BuildStep),
								Arguments: argoproj.Arguments{
									Parameters: []argoproj.Parameter{
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
						Steps: []argoproj.WorkflowStep{
							{
								Name:     string(integrations.PushStep),
								Template: string(integrations.PushStep),
								Arguments: argoproj.Arguments{
									Parameters: []argoproj.Parameter{
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
			makeCloneStep(buildObj, repo),
			makeBuildStep(buildObj),
			makePushStep(buildObj),
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
		TTLStrategy: &argoproj.TTLStrategy{
			SecondsAfterFailure: ptr.Int32(3600),
			SecondsAfterSuccess: ptr.Int32(3600),
		},
	}
}

func makeCloneStep(buildObj *choreov1.Build, repo string) argoproj.Template {
	branch := ""
	gitRevision := ""
	if buildObj.Spec.Branch != "" {
		branch = buildObj.Spec.Branch
	} else if buildObj.Spec.GitRevision != "" {
		gitRevision = buildObj.Spec.GitRevision[:8]
	} else {
		branch = "main"
	}
	return argoproj.Template{
		Name: string(integrations.CloneStep),
		Metadata: argoproj.Metadata{
			Labels: map[string]string{
				"step":     string(integrations.CloneStep),
				"workflow": buildObj.ObjectMeta.Name,
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
		Outputs: argoproj.Outputs{
			Parameters: []argoproj.Parameter{
				{
					Name: "git-revision",
					ValueFrom: &argoproj.ValueFrom{
						Path: "/tmp/git-revision.txt",
					},
				},
			},
		},
	}
}

func makeBuildStep(buildObj *choreov1.Build) argoproj.Template {
	return argoproj.Template{
		Name: string(integrations.BuildStep),
		Inputs: argoproj.Inputs{
			Parameters: []argoproj.Parameter{
				{
					Name: "git-revision",
				},
			},
		},
		Metadata: argoproj.Metadata{
			Labels: map[string]string{
				"step":     string(integrations.BuildStep),
				"workflow": buildObj.ObjectMeta.Name,
			},
		},
		Container: &corev1.Container{
			Image: "ghcr.io/choreo-idp/podman-runner:v1.0",
			SecurityContext: &corev1.SecurityContext{
				Privileged: ptr.Bool(true),
			},
			Command: []string{"sh", "-c"},
			Args:    generateBuildArgs(buildObj, ci.ConstructImageNameWithTag(buildObj)),
			VolumeMounts: []corev1.VolumeMount{
				{Name: "workspace", MountPath: "/mnt/vol"},
				{Name: "podman-cache", MountPath: "/shared/podman/cache"},
			},
		},
	}
}

func makePushStep(buildObj *choreov1.Build) argoproj.Template {
	return argoproj.Template{
		Name: string(integrations.PushStep),
		Inputs: argoproj.Inputs{
			Parameters: []argoproj.Parameter{
				{
					Name: "git-revision",
				},
			},
		},
		Metadata: argoproj.Metadata{
			Labels: map[string]string{
				"step":     string(integrations.PushStep),
				"workflow": buildObj.ObjectMeta.Name,
			},
		},
		Container: &corev1.Container{
			Image: "ghcr.io/choreo-idp/podman-runner:v1.0",
			SecurityContext: &corev1.SecurityContext{
				Privileged: ptr.Bool(true),
			},
			Command: []string{"sh", "-c"},
			Args: []string{
				generatePushImageScript(ci.ConstructImageNameWithTag(buildObj)),
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "workspace", MountPath: "/mnt/vol"},
			},
		},
		Outputs: argoproj.Outputs{
			Parameters: []argoproj.Parameter{
				{
					Name: "image",
					ValueFrom: &argoproj.ValueFrom{
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

func getDockerContext(buildObj *choreov1.Build) string {
	if buildObj.Spec.BuildConfiguration.Docker.Context != "" {
		return buildObj.Spec.BuildConfiguration.Docker.Context
	}
	return buildObj.Spec.Path
}

func getDockerfilePath(buildObj *choreov1.Build) string {
	if buildObj.Spec.BuildConfiguration.Docker.DockerfilePath != "" {
		return buildObj.Spec.BuildConfiguration.Docker.DockerfilePath
	}
	return "Dockerfile"
}

func getLanguageVersion(buildObj *choreov1.Build) string {
	version := buildObj.Spec.BuildConfiguration.Buildpack.Version
	if version == "" {
		return ""
	}
	switch buildObj.Spec.BuildConfiguration.Buildpack.Name {
	case choreov1.BuildpackGo:
		return fmt.Sprintf("--env GOOGLE_GO_VERSION=%q", version)
	case choreov1.BuildpackNodeJS:
		return fmt.Sprintf("--env GOOGLE_NODEJS_VERSION=%s", version)
	case choreov1.BuildpackPython:
		return fmt.Sprintf("--env GOOGLE_PYTHON_VERSION=%q", version)
	case choreov1.BuildpackPHP:
		// Handled separately by generating composer.json
		return ""
	default:
		// For BuildpackRuby and BuildpackJava
		return fmt.Sprintf("--env GOOGLE_RUNTIME_VERSION=%s", version)
	}
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

func generateBuildArgs(buildObj *choreov1.Build, imageName string) []string {
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

	if buildObj.Spec.BuildConfiguration.Buildpack != nil {
		if buildObj.Spec.BuildConfiguration.Buildpack.Name == choreov1.BuildpackReact {
			buildScript = makeReactBuildScript(buildObj.Spec.BuildConfiguration.Buildpack.Version, buildObj.Spec.Path, imageName)
		} else if buildObj.Spec.BuildConfiguration.Buildpack.Name == choreov1.BuildpackBallerina {
			buildScript = makeBuildpackBuildScript(buildObj, imageName, true)
		} else {
			buildScript = makeBuildpackBuildScript(buildObj, imageName, false)
		}
	} else {
		buildScript = makeDockerfileBuildScript(buildObj, imageName)
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

func makeBuildpackBuildScript(buildObj *choreov1.Build, imageName string, isBallerina bool) string {
	baseScript := `
podman system service --time=0 &
until podman info --format '{{.Host.RemoteSocket.Exists}}' 2>/dev/null | grep -q "true"; do
  sleep 1
done`
	if isBallerina {
		return baseScript + makeBallerinaBuildScript(imageName, buildObj.Spec.Path)
	}
	return baseScript + makeGoogleBuildpackBuildScript(imageName, buildObj)
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

/usr/local/bin/pack build %s-{{inputs.parameters.git-revision}} --builder=ghcr.io/choreo-idp/buildpack/ballerina:18 \
--docker-host=inherit --path=/mnt/vol/source%s --volume "/mnt/vol":/app/generated-artifacts:rw --pull-policy if-not-present

podman save -o /mnt/vol/app-image.tar %s-{{inputs.parameters.git-revision}}`,
		makeBuilderCacheScript("ghcr.io/choreo-idp/buildpack/ballerina:18", "/shared/podman/cache/ballerina-builder.tar"),
		imageName, path, imageName)
}

func makePHPVersionSetup(buildObj *choreov1.Build) string {
	if buildObj.Spec.BuildConfiguration.Buildpack.Name == choreov1.BuildpackPHP {
		buildPath := fmt.Sprintf("/mnt/vol/source%s", buildObj.Spec.Path)
		version := buildObj.Spec.BuildConfiguration.Buildpack.Version
		return fmt.Sprintf(`
apk add --no-cache jq

if [ -f %s/composer.json ]; then
    if jq -e '.require' %s/composer.json > /dev/null; then
        jq '.require["php"] = "%s"' %s/composer.json > %s/composer.json.tmp && mv %s/composer.json.tmp %s/composer.json
    else
        echo '{"require": {"php": "%s"}}' > %s/composer.json
    fi
else
    echo '{"require": {"php": "%s"}}' > %s/composer.json
fi`, buildPath, buildPath, version, buildPath, buildPath, buildPath, buildPath, version, buildPath, version, buildPath)
	}
	return ""
}

func makeGoogleBuildpackBuildScript(imageName string, buildObj *choreov1.Build) string {
	phpVersionSetup := makePHPVersionSetup(buildObj)

	return fmt.Sprintf(`
%s

%s

%s

/usr/local/bin/pack build %s-{{inputs.parameters.git-revision}} --builder=gcr.io/buildpacks/builder:google-22 \
--docker-host=inherit --path=/mnt/vol/source%s --pull-policy if-not-present %s

podman save -o /mnt/vol/app-image.tar %s-{{inputs.parameters.git-revision}}`,
		phpVersionSetup,
		makeBuilderCacheScript("gcr.io/buildpacks/builder:google-22", "/shared/podman/cache/google-builder.tar"),
		makeBuilderCacheScript("gcr.io/buildpacks/google-22/run:latest", "/shared/podman/cache/google-run.tar"),
		imageName, buildObj.Spec.Path, getLanguageVersion(buildObj), imageName)
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
