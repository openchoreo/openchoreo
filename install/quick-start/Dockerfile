FROM alpine:3.21

RUN apk add --no-cache \
  bash curl socat git jq bash-completion docker-cli openssl && \
  mkdir -p /etc/bash_completion.d

# inject the target os and architecture (https://docs.docker.com/reference/dockerfile/#automatic-platform-args-in-the-global-scope)
ARG TARGETARCH
ARG TARGETOS

# install kubectl
RUN curl -fsSL "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/$TARGETARCH/kubectl" > /tmp/kubectl && \
  install -o root -g root -m 0755 /tmp/kubectl /usr/local/bin/kubectl && \
  kubectl completion bash > /etc/bash_completion.d/kubectl && \
  rm /tmp/kubectl

# install helm - latest stable version (https://github.com/helm/helm/releases)
RUN curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash && \
  helm completion bash > /etc/bash_completion.d/helm

# install k3d - https://k3d.io/stable/#installation
RUN curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash

ENV KUBECONFIG="/state/kube/config-internal.yaml"

COPY install/quick-start /app

COPY install/add-default-dataplane.sh /app/add-default-dataplane.sh

COPY install/add-build-plane.sh /app/add-build-plane.sh

COPY samples/from-image/react-starter-web-app/react-starter.yaml /app/react-starter.yaml

COPY samples/ /app/samples

# Copy the built choreoctl binary from the builder stage
COPY bin/dist/${TARGETOS}/${TARGETARCH}/choreoctl /usr/local/bin/choreoctl

ENV PATH="/usr/local/bin:${PATH}"

# Set custom shell prompt with meaningful hostname
RUN echo 'export PS1="openchoreo-quick-start:\w# "' >> /etc/profile

WORKDIR /app

ENTRYPOINT ["/app/startup.sh"]
