FROM alpine:3.21

# Version arguments for reproducibility and easy updates
ARG KUBECTL_VERSION=v1.32.10
ARG HELM_VERSION=v3.19.2
ARG K3D_VERSION=v5.8.3

# Inject the target os and architecture (https://docs.docker.com/reference/dockerfile/#automatic-platform-args-in-the-global-scope)
ARG TARGETARCH
ARG TARGETOS

# Install base packages and create user
ARG USER_HOME=/home/openchoreo
RUN apk add --no-cache \
  bash curl socat jq yq bash-completion docker-cli openssl \
  bind-tools netcat-openbsd tcpdump \
  htop procps \
  sudo \
  wget ca-certificates

# Create user and add sudo privileges
RUN mkdir -p /etc/bash_completion.d && \
  adduser -D -h ${USER_HOME} -s /bin/bash openchoreo && \
  echo "openchoreo ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/openchoreo && \
  chmod 0440 /etc/sudoers.d/openchoreo

# Install kubectl, helm, and k3d tools
RUN curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" > /tmp/kubectl && \
  install -o root -g root -m 0755 /tmp/kubectl /usr/local/bin/kubectl && \
  kubectl completion bash > /etc/bash_completion.d/kubectl && \
  rm /tmp/kubectl && \
  curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | \
    DESIRED_VERSION=${HELM_VERSION} bash && \
  helm completion bash > /etc/bash_completion.d/helm && \
  curl -fsSL https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | \
    TAG=${K3D_VERSION} bash && \
  k3d completion bash > /etc/bash_completion.d/k3d

# Copy the built choreoctl binary from the builder stage
COPY bin/dist/${TARGETOS}/${TARGETARCH}/choreoctl /usr/local/bin/choreoctl

# Generate choreoctl bash completion
RUN /usr/local/bin/choreoctl completion bash > /etc/bash_completion.d/choreoctl

# Copy all configuration files, scripts, and samples in single layer
COPY --chown=openchoreo:openchoreo \
  install/quick-start/.bash_profile \
  install/quick-start/.bashrc \
  install/quick-start/.welcome \
  install/quick-start/.entrypoint.sh \
  install/quick-start/.config.sh \
  install/k3d/single-cluster/config.yaml \
  install/quick-start/install.sh \
  install/quick-start/uninstall.sh \
  install/quick-start/.helpers.sh \
  install/quick-start/check-status.sh \
  install/quick-start/validate-installation.sh \
  install/quick-start/deploy-react-starter.sh \
  install/quick-start/deploy-gcp-demo.sh \
  install/quick-start/build-deploy-greeter.sh \
  install/quick-start/.values-cp.yaml \
  install/quick-start/.values-dp.yaml \
  install/quick-start/.values-bp.yaml \
  install/k3d/preload-images.sh \
  install/add-data-plane.sh \
  install/add-build-plane.sh \
  samples/ \
  ${USER_HOME}/

WORKDIR ${USER_HOME}

# Setting openchoreo user will be done in entrypoint after docker socket permissions are handled
ENTRYPOINT ["/home/openchoreo/.entrypoint.sh"]
