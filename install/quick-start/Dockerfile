FROM alpine:3.21

# Version arguments for reproducibility and easy updates
ARG KUBECTL_VERSION=v1.32.10
ARG HELM_VERSION=v3.19.2
ARG K3D_VERSION=v5.8.3

# Inject the target os and architecture (https://docs.docker.com/reference/dockerfile/#automatic-platform-args-in-the-global-scope)
ARG TARGETARCH
ARG TARGETOS

# Install base packages including troubleshooting tools
# Core utilities, network debugging tools, system monitoring, file editing, user management, and additional utilities
RUN apk add --no-cache \
  bash curl socat git jq yq bash-completion docker-cli openssl \
  bind-tools netcat-openbsd tcpdump \
  htop procps \
  vim \
  sudo \
  wget ca-certificates && \
  mkdir -p /etc/bash_completion.d

# Install kubectl
RUN curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" > /tmp/kubectl && \
  install -o root -g root -m 0755 /tmp/kubectl /usr/local/bin/kubectl && \
  kubectl completion bash > /etc/bash_completion.d/kubectl && \
  rm /tmp/kubectl

# Install helm using official script with version flag
RUN curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | \
  DESIRED_VERSION=${HELM_VERSION} bash && \
  helm completion bash > /etc/bash_completion.d/helm

# Install k3d using official script with version tag
RUN curl -fsSL https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | \
  TAG=${K3D_VERSION} bash && \
  k3d completion bash > /etc/bash_completion.d/k3d

# Copy the built choreoctl binary from the builder stage
COPY bin/dist/${TARGETOS}/${TARGETARCH}/choreoctl /usr/local/bin/choreoctl

# Generate choreoctl bash completion
RUN /usr/local/bin/choreoctl completion bash > /etc/bash_completion.d/choreoctl

# Create openchoreo user with sudo privileges
ARG USER_HOME=/home/openchoreo
RUN adduser -D -h ${USER_HOME} -s /bin/bash openchoreo && \
    echo "openchoreo ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/openchoreo && \
    chmod 0440 /etc/sudoers.d/openchoreo

# Copy bash configuration and welcome message with correct ownership
COPY --chown=openchoreo:openchoreo install/quick-start/.bash_profile ${USER_HOME}/.bash_profile
COPY --chown=openchoreo:openchoreo install/quick-start/.bashrc ${USER_HOME}/.bashrc
COPY --chown=openchoreo:openchoreo install/quick-start/.welcome ${USER_HOME}/.welcome

# Copy installation scripts and samples to user's home directory with correct ownership

# Hidden files
COPY --chown=openchoreo:openchoreo install/quick-start/.entrypoint.sh ${USER_HOME}/.entrypoint.sh
COPY --chown=openchoreo:openchoreo install/quick-start/.config.sh ${USER_HOME}/.config.sh
COPY --chown=openchoreo:openchoreo install/k3d/single-cluster/config.yaml ${USER_HOME}/.k3d-config.yaml

# User facing files
COPY --chown=openchoreo:openchoreo install/quick-start/install.sh ${USER_HOME}/install.sh
COPY --chown=openchoreo:openchoreo install/quick-start/uninstall.sh ${USER_HOME}/uninstall.sh
COPY --chown=openchoreo:openchoreo install/quick-start/.helpers.sh ${USER_HOME}/.helpers.sh
COPY --chown=openchoreo:openchoreo install/quick-start/check-status.sh ${USER_HOME}/check-status.sh
COPY --chown=openchoreo:openchoreo install/quick-start/validate-installation.sh ${USER_HOME}/validate-installation.sh
COPY --chown=openchoreo:openchoreo install/quick-start/deploy-react-starter.sh ${USER_HOME}/deploy-react-starter.sh
COPY --chown=openchoreo:openchoreo install/quick-start/deploy-gcp-demo.sh ${USER_HOME}/deploy-gcp-demo.sh
COPY --chown=openchoreo:openchoreo install/quick-start/.values-cp.yaml ${USER_HOME}/.values-cp.yaml
COPY --chown=openchoreo:openchoreo install/quick-start/.values-dp.yaml ${USER_HOME}/.values-dp.yaml
COPY --chown=openchoreo:openchoreo install/add-data-plane.sh ${USER_HOME}/add-data-plane.sh
COPY --chown=openchoreo:openchoreo install/add-build-plane.sh ${USER_HOME}/add-build-plane.sh
COPY --chown=openchoreo:openchoreo samples/ ${USER_HOME}/samples

WORKDIR ${USER_HOME}

# Setting openchoreo user will be done in entrypoint after docker socket permissions are handled
ENTRYPOINT ["/home/openchoreo/.entrypoint.sh"]
