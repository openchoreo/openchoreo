# syntax=docker/dockerfile:1.6
# Minimal busybox-based HTTP echo container. Used by the
# workload-auto-generation e2e spec — there is intentionally NO workload.yaml
# alongside this Dockerfile so the build pipeline must auto-generate a Workload.
FROM docker.io/library/busybox:1.36@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662
COPY --chown=10001:10001 server.sh /server.sh
RUN chmod 0555 /server.sh
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/server.sh"]
