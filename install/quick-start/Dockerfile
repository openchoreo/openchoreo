FROM alpine:3.21

RUN apk add --no-cache \
  bash curl socat git jq bash-completion docker-cli && \
  mkdir -p /etc/bash_completion.d

# inject the target os and architecture (https://docs.docker.com/reference/dockerfile/#automatic-platform-args-in-the-global-scope)
ARG TARGETARCH
ARG TARGETOS

# install kubectl
RUN curl -fsSL "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/$TARGETARCH/kubectl" > /tmp/kubectl && \
  install -o root -g root -m 0755 /tmp/kubectl /usr/local/bin/kubectl && \
  kubectl completion bash > /etc/bash_completion.d/kubectl && \
  rm /tmp/kubectl

# install helm (https://github.com/helm/helm/releases)
RUN mkdir /tmp/helm && \
  curl -fsSL https://get.helm.sh/helm-v3.17.1-linux-${TARGETARCH}.tar.gz > /tmp/helm/helm.tar.gz && \
  tar -zxvf /tmp/helm/helm.tar.gz -C /tmp/helm && \
  install -o root -g root -m 0755 /tmp/helm/linux-${TARGETARCH}/helm /usr/local/bin/helm && \
  helm completion bash > /etc/bash_completion.d/helm && \
  rm -rf /tmp/helm

# install kind https://kind.sigs.k8s.io/docs/user/quick-start/#installing-from-release-binaries
RUN curl -fsSL https://kind.sigs.k8s.io/dl/v0.27.0/kind-linux-${TARGETARCH} > /tmp/kind && \
  install -o root -g root -m 0755 /tmp/kind /usr/local/bin/kind && \
  rm /tmp/kind

# install terraform (https://github.com/hashicorp/terraform/releases)
RUN mkdir /tmp/terraform && \
  curl -fsSL https://releases.hashicorp.com/terraform/1.11.0/terraform_1.11.0_linux_${TARGETARCH}.zip > /tmp/terraform/terraform.zip && \
  unzip /tmp/terraform/terraform.zip -d /tmp/terraform && \
  install -o root -g root -m 0755 /tmp/terraform/terraform /usr/local/bin/terraform && \
  rm -rf /tmp/terraform

ENV KUBECONFIG="/state/kube/config-internal.yaml"

COPY install/quick-start /app

COPY install/check-status.sh /app/check-status.sh

COPY samples/deploying-applications/use-prebuilt-image/react-spa-webapp/react-starter.yaml /app/react-starter.yaml

COPY samples/ /app/samples

# Copy the built choreoctl binary from the builder stage
COPY bin/dist/${TARGETOS}/${TARGETARCH}/choreoctl /usr/local/bin/choreoctl

ENV PATH="/usr/local/bin:${PATH}"

WORKDIR /app

ENTRYPOINT ["/app/startup.sh"]
