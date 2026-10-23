FROM alpine:3.22@sha256:2e0b81d60597f449baa56dc25d355367869d2dca4cd57e866f1e2b52af3eb55f

RUN apk upgrade && \
    apk add bash curl jq && \
    addgroup -g 10500 opensearch && \
    adduser -D -u 10500 -G opensearch opensearch

USER opensearch

COPY --chown=opensearch --chmod=0540 install/init/observability/opensearch/setup-opensearch-cluster.sh setup-opensearch-cluster.sh

CMD ["bash", "setup-opensearch-cluster.sh"]
