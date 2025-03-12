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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations"
	argo "github.com/choreo-idp/choreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	"github.com/choreo-idp/choreo/internal/ptr"
)

var _ = Describe("Argo Workflow Generation", func() {
	var (
		buildCtx *integrations.BuildContext
	)

	BeforeEach(func() {
		buildCtx = newTestBuildContext()
	})

	Context("Make clone step", func() {
		DescribeTable("should generate the correct git clone arguments",
			func(repo string, branch string, gitRevision string, expected []string) {
				result := generateCloneArgs(repo, branch, gitRevision)
				Expect(result).To(Equal(expected))
			},
			Entry("when branch is provided", "https://github.com/example/repo.git", "main", "",
				[]string{
					`set -e
git clone --single-branch --branch main --depth 1 https://github.com/example/repo.git /mnt/vol/source
cd /mnt/vol/source
COMMIT_SHA=$(git rev-parse HEAD)
echo -n "$COMMIT_SHA" | cut -c1-8 > /tmp/git-revision.txt`,
				}),
			Entry("when branch is empty and git revision is provided", "https://github.com/example/repo.git", "", "abcdef12",
				[]string{
					`set -e
git clone --no-checkout --depth 1 https://github.com/example/repo.git /mnt/vol/source
cd /mnt/vol/source
git config --global advice.detachedHead false
git fetch --depth 1 origin abcdef12
git checkout abcdef12
echo -n "abcdef12" > /tmp/git-revision.txt`,
				}),
		)

		It("should generate a valid clone step template", func() {
			buildCtx = newDockerBasedBuildCtx(buildCtx)
			template := makeCloneStep(buildCtx.Build, buildCtx.Component.Spec.Source.GitRepository.URL)

			Expect(template.Name).To(Equal(string(integrations.CloneStep)))
			Expect(template.Metadata.Labels).To(HaveKeyWithValue("step", string(integrations.CloneStep)))
			Expect(template.Metadata.Labels).To(HaveKeyWithValue("workflow", "test-build"))
			Expect(template.Container).NotTo(BeNil())
			Expect(template.Container.Image).To(Equal("alpine/git"))
			Expect(template.Container.Command).To(Equal([]string{"sh", "-c"}))
			Expect(template.Container.Args).NotTo(BeEmpty())
			Expect(template.Container.VolumeMounts).To(ContainElement(corev1.VolumeMount{Name: "workspace", MountPath: "/mnt/vol"}))
			Expect(template.Outputs.Parameters).To(ContainElement(argo.Parameter{
				Name: "git-revision",
				ValueFrom: &argo.ValueFrom{
					Path: "/tmp/git-revision.txt",
				},
			}))
		})
	})
	Context("Make build step", func() {
		It("should return a valid base64-encoded Dockerfile", func() {
			nodeVersion := "18"
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

			expectedBase64 := base64.StdEncoding.EncodeToString([]byte(dockerfile))
			generatedBase64 := getDockerfileContent(nodeVersion)

			Expect(generatedBase64).To(Equal(expectedBase64))
		})

		It("should return a valid base64-encoded Nginx configuration", func() {
			nginxConfig := `server {
  listen 80;
  location / {
    root   /usr/share/nginx/html;
    index  index.html index.htm;
    try_files $uri $uri/ /index.html; 
  }
} `

			expectedBase64 := base64.StdEncoding.EncodeToString([]byte(nginxConfig))
			generatedBase64 := getNginxConfig()

			Expect(generatedBase64).To(Equal(expectedBase64))
		})

		DescribeTable("should return the correct Dockerfile path",
			func(dockerfilePath string, expected string) {
				// Create the Build object with the specified Dockerfile path
				build := &choreov1.Build{
					Spec: choreov1.BuildSpec{
						BuildConfiguration: choreov1.BuildConfiguration{
							Docker: &choreov1.DockerConfiguration{
								DockerfilePath: dockerfilePath,
							},
						},
					},
				}
				result := getDockerfilePath(build)
				Expect(result).To(Equal(expected))
			},
			Entry("when Dockerfile path is provided", "path/to/Dockerfile", "path/to/Dockerfile"),
			Entry("when Dockerfile path is empty", "", "Dockerfile"),
		)

		It("should generate correct docker build script", func() {
			buildCtx = newDockerBasedBuildCtx(buildCtx)
			script := makeDockerfileBuildScript(buildCtx.Build, imageName())

			expectedScript := fmt.Sprintf(`
podman build -t %s-{{inputs.parameters.git-revision}} -f /mnt/vol/source%s /mnt/vol/source%s
podman save -o /mnt/vol/app-image.tar %s-{{inputs.parameters.git-revision}}`, imageName(), "/time-logger/Dockerfile", "/time-logger", imageName())

			Expect(script).To(Equal(expectedScript))
		})

		It("should generate correct react build script", func() {
			nodeVersion := "18.x.x"
			path := "/my-app"
			imageName := "org-project-component:main-asad87s"

			expectedTargetDir := fmt.Sprintf("/mnt/vol/source%s", path)
			expectedImageReference := fmt.Sprintf("%s-{{inputs.parameters.git-revision}}", imageName)

			expectedScript := fmt.Sprintf(`
apk add --no-cache coreutils

echo %s | base64 -d > %s/Dockerfile
echo %s | base64 -d > %s/default.conf

DOCKER_BUILDKIT=1 podman build -t %s -f %s/Dockerfile %s

podman save -o /mnt/vol/app-image.tar %s`,
				getDockerfileContent(nodeVersion), expectedTargetDir,
				getNginxConfig(), expectedTargetDir,
				expectedImageReference, expectedTargetDir, expectedTargetDir,
				expectedImageReference,
			)

			generatedScript := makeReactBuildScript(nodeVersion, path, imageName)

			Expect(generatedScript).To(Equal(expectedScript))
		})

		It("should generate correct ballerina build script", func() {
			path := "/ballerina-time-logger"

			expectedCacheScript := `
if [[ ! -f "/shared/podman/cache/ballerina-builder.tar" ]]; then
  podman pull chalindukodikara/choreo-buildpack:ballerina-builder
  podman save -o /shared/podman/cache/ballerina-builder.tar chalindukodikara/choreo-buildpack:ballerina-builder
else
  if ! podman load -i /shared/podman/cache/ballerina-builder.tar; then
    podman pull chalindukodikara/choreo-buildpack:ballerina-builder
    podman save -o /shared/podman/cache/ballerina-builder.tar chalindukodikara/choreo-buildpack:ballerina-builder
  fi
fi`

			script := makeBallerinaBuildScript(imageName(), path)

			expectedScript := fmt.Sprintf(`
%s

/usr/local/bin/pack build %s-{{inputs.parameters.git-revision}} --builder=chalindukodikara/choreo-buildpack:ballerina-builder \
--docker-host=inherit --path=/mnt/vol/source%s --volume "/mnt/vol":/app/generated-artifacts:rw --pull-policy if-not-present

podman save -o /mnt/vol/app-image.tar %s-{{inputs.parameters.git-revision}}`, expectedCacheScript, imageName(), path, imageName())

			Expect(script).To(Equal(expectedScript))
		})

		DescribeTable("should return correct environment version flag",
			func(buildpackName choreov1.BuildpackName, version string, expected string) {
				build := &choreov1.Build{
					Spec: choreov1.BuildSpec{
						BuildConfiguration: choreov1.BuildConfiguration{
							Buildpack: &choreov1.BuildpackConfiguration{
								Name:    buildpackName,
								Version: version,
							},
						},
					},
				}

				result := getLanguageVersion(build)
				Expect(result).To(Equal(expected))
			},
			Entry("when the buildpack is Go", choreov1.BuildpackGo, "1.x", "--env GOOGLE_GO_VERSION=\"1.x\""),
			Entry("when the buildpack is NodeJS", choreov1.BuildpackNodeJS, "18.x.x", "--env GOOGLE_NODEJS_VERSION=18.x.x"),
			Entry("when the buildpack is Python", choreov1.BuildpackPython, "3.10", "--env GOOGLE_PYTHON_VERSION=\"3.10\""),
			Entry("when the buildpack is PHP", choreov1.BuildpackPHP, "8.1.x", "--env GOOGLE_COMPOSER_VERSION=\"8.1.x\""),
			Entry("when the buildpack is Ruby", choreov1.BuildpackRuby, "3.1.x", "--env GOOGLE_RUNTIME_VERSION=3.1.x"),
			Entry("when the version is empty", choreov1.BuildpackGo, "", ""),
		)

		It("should generate the correct google buildpack build script", func() {
			cacheScript1 := `
if [[ ! -f "/shared/podman/cache/google-builder.tar" ]]; then
  podman pull gcr.io/buildpacks/builder:google-22
  podman save -o /shared/podman/cache/google-builder.tar gcr.io/buildpacks/builder:google-22
else
  if ! podman load -i /shared/podman/cache/google-builder.tar; then
    podman pull gcr.io/buildpacks/builder:google-22
    podman save -o /shared/podman/cache/google-builder.tar gcr.io/buildpacks/builder:google-22
  fi
fi`

			cacheScript2 := `
if [[ ! -f "/shared/podman/cache/google-run.tar" ]]; then
  podman pull gcr.io/buildpacks/google-22/run:latest
  podman save -o /shared/podman/cache/google-run.tar gcr.io/buildpacks/google-22/run:latest
else
  if ! podman load -i /shared/podman/cache/google-run.tar; then
    podman pull gcr.io/buildpacks/google-22/run:latest
    podman save -o /shared/podman/cache/google-run.tar gcr.io/buildpacks/google-22/run:latest
  fi
fi`

			expectedScript := fmt.Sprintf(`
%s

%s

/usr/local/bin/pack build %s-{{inputs.parameters.git-revision}} --builder=gcr.io/buildpacks/builder:google-22 \
--docker-host=inherit --path=/mnt/vol/source/time-logger --pull-policy if-not-present --env GOOGLE_GO_VERSION="1.x"

podman save -o /mnt/vol/app-image.tar %s-{{inputs.parameters.git-revision}}`,
				cacheScript1, cacheScript2, imageName(), imageName())

			build := &choreov1.Build{
				Spec: choreov1.BuildSpec{
					Path: "/time-logger",
					BuildConfiguration: choreov1.BuildConfiguration{
						Buildpack: &choreov1.BuildpackConfiguration{
							Name:    choreov1.BuildpackGo,
							Version: choreov1.SupportedVersions[choreov1.BuildpackGo][0],
						},
					},
				},
			}

			result := makeGoogleBuildpackBuildScript(imageName(), build)

			Expect(result).To(Equal(expectedScript))
		})

		It("should start daemon for buildpacks", func() {
			buildCtx = newBuildpackBasedBuildCtx(buildCtx)
			buildScript := makeBuildpackBuildScript(buildCtx.Build, imageName(), false)

			Expect(buildScript).To(ContainSubstring(`podman system service --time=0 &
until podman info --format '{{.Host.RemoteSocket.Exists}}' 2>/dev/null | grep -q "true"; do
  sleep 1
done`))
		})

		It("should generate correct podman configurations", func() {
			buildCtx = newBuildpackBasedBuildCtx(buildCtx)
			buildStepArgs := generateBuildArgs(buildCtx.Build, imageName())

			joinedArgs := strings.Join(buildStepArgs, "\n")

			Expect(joinedArgs).To(ContainSubstring(`mkdir -p /etc/containers
cat <<EOF > /etc/containers/storage.conf
[storage]
driver = "overlay"
runroot = "/run/containers/storage"
graphroot = "/var/lib/containers/storage"
[storage.options.overlay]
mount_program = "/usr/bin/fuse-overlayfs"
EOF`))
		})

	})

	Context("Make push step", func() {
		It("should generate the correct image push script", func() {
			expectedScript := fmt.Sprintf(`set -e
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
echo -n "%s-$GIT_REVISION" > /tmp/image.txt`, imageName(), imageName(), imageName(), imageName(), imageName())

			generatedScript := generatePushImageScript(imageName())

			Expect(generatedScript).To(Equal(expectedScript))
		})

		It("should generate the correct image push script", func() {
			buildCtx = newBuildpackBasedBuildCtx(buildCtx)
			expectedScript := generatePushImageScript(imageName())
			pushStep := makePushStep(buildCtx.Build)
			Expect(pushStep.Name).To(Equal(string(integrations.PushStep)))
			Expect(pushStep.Inputs.Parameters).To(HaveLen(1))
			Expect(pushStep.Inputs.Parameters[0].Name).To(Equal("git-revision"))
			Expect(pushStep.Metadata.Labels).To(HaveKeyWithValue("step", string(integrations.PushStep)))
			Expect(pushStep.Metadata.Labels).To(HaveKeyWithValue("workflow", buildCtx.Build.ObjectMeta.Name))
			Expect(pushStep.Container.Image).To(Equal("chalindukodikara/podman-runner:1.0"))

			isPrivileged := true
			Expect(pushStep.Container.SecurityContext.Privileged).To(Equal(&isPrivileged))
			Expect(pushStep.Container.Command).To(Equal([]string{"sh", "-c"}))
			Expect(pushStep.Container.Args).To(Equal([]string{expectedScript}))

			Expect(pushStep.Container.VolumeMounts).To(HaveLen(1))
			Expect(pushStep.Container.VolumeMounts[0].Name).To(Equal("workspace"))
			Expect(pushStep.Container.VolumeMounts[0].MountPath).To(Equal("/mnt/vol"))

			Expect(pushStep.Outputs.Parameters).To(HaveLen(1))
			Expect(pushStep.Outputs.Parameters[0].Name).To(Equal("image"))
			Expect(pushStep.Outputs.Parameters[0].ValueFrom.Path).To(Equal("/tmp/image.txt"))
		})
	})

	Context("Make argo workflow", func() {
		It("should generate correct PersistentVolumeClaim", func() {
			pvc := makePersistentVolumeClaim()
			Expect(pvc).To(HaveLen(1))
			Expect(pvc[0].ObjectMeta.Name).To(Equal("workspace"))
			Expect(pvc[0].Spec.AccessModes).To(HaveLen(1))
			Expect(pvc[0].Spec.AccessModes[0]).To(Equal(corev1.ReadWriteOnce))
			Expect(pvc[0].Spec.Resources.Requests).To(HaveKeyWithValue(corev1.ResourceStorage, resource.MustParse("2Gi")))
		})

		It("should generate the correct Workflow spec", func() {
			buildCtx = newBuildpackBasedBuildCtx(buildCtx)
			workflowSpec := makeWorkflowSpec(buildCtx.Build, buildCtx.Component.Spec.Source.GitRepository.URL)

			Expect(workflowSpec.ServiceAccountName).To(Equal("workflow-sa"))
			Expect(workflowSpec.Entrypoint).To(Equal("build-workflow"))
			Expect(workflowSpec.Templates).To(HaveLen(4))

			buildWorkflowTemplate := workflowSpec.Templates[0]
			Expect(buildWorkflowTemplate.Name).To(Equal("build-workflow"))
			Expect(buildWorkflowTemplate.Steps).To(HaveLen(3))

			cloneStep := buildWorkflowTemplate.Steps[0].Steps[0]
			Expect(cloneStep.Name).To(Equal(string(integrations.CloneStep)))
			Expect(cloneStep.Template).To(Equal(string(integrations.CloneStep)))

			buildStep := buildWorkflowTemplate.Steps[1].Steps[0]
			Expect(buildStep.Name).To(Equal(string(integrations.BuildStep)))
			Expect(buildStep.Template).To(Equal(string(integrations.BuildStep)))

			pushStep := buildWorkflowTemplate.Steps[2].Steps[0]
			Expect(pushStep.Name).To(Equal(string(integrations.PushStep)))
			Expect(pushStep.Template).To(Equal(string(integrations.PushStep)))

			Expect(workflowSpec.VolumeClaimTemplates).To(HaveLen(1))
			Expect(workflowSpec.VolumeClaimTemplates[0].ObjectMeta.Name).To(Equal("workspace"))
			Expect(workflowSpec.VolumeClaimTemplates[0].Spec.Resources.Requests).To(HaveKeyWithValue(corev1.ResourceStorage, resource.MustParse("2Gi")))

			Expect(workflowSpec.Volumes).To(HaveLen(1))
			podmanCacheVolume := workflowSpec.Volumes[0]
			Expect(podmanCacheVolume.Name).To(Equal("podman-cache"))
			Expect(podmanCacheVolume.VolumeSource.HostPath.Path).To(Equal("/shared/podman/cache"))

			Expect(workflowSpec.TTLStrategy).NotTo(BeNil())
			Expect(workflowSpec.TTLStrategy.SecondsAfterFailure).To(Equal(ptr.Int32(3600)))
			Expect(workflowSpec.TTLStrategy.SecondsAfterSuccess).To(Equal(ptr.Int32(3600)))
		})

		It("should generate the workflow in correct namespace", func() {
			buildCtx = newBuildpackBasedBuildCtx(buildCtx)
			workflow := makeArgoWorkflow(buildCtx)

			Expect(workflow.ObjectMeta.Name).To(Equal(buildCtx.Build.Name + "-c9f6181a"))
			Expect(workflow.ObjectMeta.Namespace).To(Equal("choreo-ci-" + buildCtx.Build.Labels["core.choreo.dev/organization"]))
		})

		It("should limit workflow name to 63 characters", func() {
			buildCtx = newBuildpackBasedBuildCtx(buildCtx)
			buildCtx.Build.Name = "test-build-name-having-113-characters-test-build-name-having-113-characters-test-build-name-having-113-characters"
			workflow := makeArgoWorkflow(buildCtx)

			Expect(workflow.ObjectMeta.Name).To(Equal(buildCtx.Build.Name[:54] + "-41c7560f"))
			Expect(workflow.ObjectMeta.Namespace).To(Equal("choreo-ci-" + buildCtx.Build.Labels["core.choreo.dev/organization"]))
		})

	})
})
