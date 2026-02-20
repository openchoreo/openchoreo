FROM alpine:3.23@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659

# Version arguments for reproducibility and easy updates
ARG KUBECTL_VERSION=v1.32.10
ARG HELM_VERSION=v3.19.2
ARG K3D_VERSION=v5.8.3

# Inject the target os and architecture
ARG TARGETARCH
ARG TARGETOS
ARG USER_HOME=/home/openchoreo

# Install all base packages
RUN apk add --no-cache \
  bash curl socat git jq yq bash-completion docker-cli openssl \
  bind-tools netcat-openbsd tcpdump \
  htop procps \
  vim \
  sudo \
  wget ca-certificates ncurses && \
  mkdir -p /etc/bash_completion.d

# Install kubectl with checksum verification
RUN curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" > /tmp/kubectl && \
  curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl.sha256" > /tmp/kubectl.sha256 && \
  echo "$(cat /tmp/kubectl.sha256)  /tmp/kubectl" | sha256sum -c - && \
  install -o root -g root -m 0755 /tmp/kubectl /usr/local/bin/kubectl && \
  kubectl completion bash > /etc/bash_completion.d/kubectl && \
  rm /tmp/kubectl /tmp/kubectl.sha256

# Install helm and k3d with checksum verification
RUN curl -fsSL "https://get.helm.sh/helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz" > /tmp/helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz && \
  curl -fsSL "https://get.helm.sh/helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz.sha256sum" > /tmp/helm.sha256sum && \
  cd /tmp && sha256sum -c helm.sha256sum && \
  tar -zxf /tmp/helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz -C /tmp && \
  install -o root -g root -m 0755 /tmp/linux-${TARGETARCH}/helm /usr/local/bin/helm && \
  helm completion bash > /etc/bash_completion.d/helm && \
  rm -rf /tmp/helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz /tmp/helm.sha256sum /tmp/linux-${TARGETARCH} && \
  curl -fsSL "https://github.com/k3d-io/k3d/releases/download/${K3D_VERSION}/k3d-linux-${TARGETARCH}" > /tmp/k3d && \
  curl -fsSL "https://github.com/k3d-io/k3d/releases/download/${K3D_VERSION}/checksums.txt" > /tmp/k3d.checksums && \
  K3D_HASH=$(grep "k3d-linux-${TARGETARCH}" /tmp/k3d.checksums | awk '{print $1}') && \
  test -n "${K3D_HASH}" || { echo "k3d checksum not found for k3d-linux-${TARGETARCH}"; exit 1; } && \
  echo "${K3D_HASH}  /tmp/k3d" | sha256sum -c - && \
  install -o root -g root -m 0755 /tmp/k3d /usr/local/bin/k3d && \
  k3d completion bash > /etc/bash_completion.d/k3d && \
  rm /tmp/k3d /tmp/k3d.checksums

# Copy occ binary
COPY bin/dist/${TARGETOS}/${TARGETARCH}/occ /usr/local/bin/occ

# Create user and generate occ completion
RUN adduser -D -h ${USER_HOME} -s /bin/bash openchoreo && \
    echo "openchoreo ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/openchoreo && \
    chmod 0440 /etc/sudoers.d/openchoreo && \
    /usr/local/bin/occ completion bash > /etc/bash_completion.d/occ

# Copy all configs and scripts in one go
COPY install/ /tmp/install/
COPY samples/ /tmp/samples/

# Move files to final locations and set permissions
RUN cp /tmp/install/quick-start/.bash_profile ${USER_HOME}/.bash_profile && \
    cp /tmp/install/quick-start/.bashrc ${USER_HOME}/.bashrc && \
    cp /tmp/install/quick-start/.welcome ${USER_HOME}/.welcome && \
    cp /tmp/install/quick-start/.entrypoint.sh ${USER_HOME}/.entrypoint.sh && \
    cp /tmp/install/quick-start/.config.sh ${USER_HOME}/.config.sh && \
    cp /tmp/install/k3d/single-cluster/config.yaml ${USER_HOME}/.k3d-config.yaml && \
    cp /tmp/install/quick-start/install.sh ${USER_HOME}/install.sh && \
    cp /tmp/install/quick-start/uninstall.sh ${USER_HOME}/uninstall.sh && \
    cp /tmp/install/quick-start/.helpers.sh ${USER_HOME}/.helpers.sh && \
    cp /tmp/install/quick-start/check-status.sh ${USER_HOME}/check-status.sh && \
    cp /tmp/install/quick-start/validate-installation.sh ${USER_HOME}/validate-installation.sh && \
    cp /tmp/install/quick-start/occ-login.sh ${USER_HOME}/occ-login.sh && \
    cp /tmp/install/quick-start/deploy-react-starter.sh ${USER_HOME}/deploy-react-starter.sh && \
    cp /tmp/install/quick-start/deploy-gcp-demo.sh ${USER_HOME}/deploy-gcp-demo.sh && \
    cp /tmp/install/quick-start/build-deploy-greeter.sh ${USER_HOME}/build-deploy-greeter.sh && \
    cp /tmp/install/quick-start/.values-cp.yaml ${USER_HOME}/.values-cp.yaml && \
    cp /tmp/install/quick-start/.values-dp.yaml ${USER_HOME}/.values-dp.yaml && \
    cp /tmp/install/quick-start/.values-bp.yaml ${USER_HOME}/.values-bp.yaml && \
    cp /tmp/install/quick-start/.values-registry.yaml ${USER_HOME}/.values-registry.yaml && \
    cp /tmp/install/quick-start/.values-op.yaml ${USER_HOME}/.values-op.yaml && \
    cp /tmp/install/k3d/preload-images.sh ${USER_HOME}/.preload-images.sh && \
    mkdir -p ${USER_HOME}/install/k3d/common && \
    cp /tmp/install/k3d/common/values-thunder.yaml ${USER_HOME}/install/k3d/common/values-thunder.yaml && \
    cp /tmp/install/k3d/common/coredns-custom.yaml ${USER_HOME}/install/k3d/common/coredns-custom.yaml && \
    cp /tmp/install/k3d/common/gateway-tmp-volume-patch.json ${USER_HOME}/install/k3d/common/gateway-tmp-volume-patch.json && \
    cp -r /tmp/samples ${USER_HOME}/samples && \
    chown -R openchoreo:openchoreo ${USER_HOME} && \
    rm -rf /tmp/install /tmp/samples && \
    cd ${USER_HOME}

COPY --chown=openchoreo:openchoreo samples/ ${USER_HOME}/samples

# Set working directory
WORKDIR ${USER_HOME}

# Set entrypoint
ENTRYPOINT ["/home/openchoreo/.entrypoint.sh"]
