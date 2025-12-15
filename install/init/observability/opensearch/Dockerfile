FROM alpine:3.22

RUN apk upgrade && \
    apk add bash curl jq && \
    addgroup -g 10500 opensearch && \
    adduser -D -u 10500 -G opensearch opensearch

USER opensearch

COPY --chown=opensearch --chmod=0540 install/init/observability/opensearch/setup-opensearch-cluster.sh setup-opensearch-cluster.sh

CMD ["bash", "setup-opensearch-cluster.sh"]
