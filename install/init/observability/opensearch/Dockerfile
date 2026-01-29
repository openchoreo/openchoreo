FROM alpine:3.22@sha256:55ae5d250caebc548793f321534bc6a8ef1d116f334f18f4ada1b2daad3251b2

RUN apk upgrade && \
    apk add bash curl jq && \
    addgroup -g 10500 opensearch && \
    adduser -D -u 10500 -G opensearch opensearch

USER opensearch

COPY --chown=opensearch --chmod=0540 install/init/observability/opensearch/setup-opensearch-cluster.sh setup-opensearch-cluster.sh

CMD ["bash", "setup-opensearch-cluster.sh"]
