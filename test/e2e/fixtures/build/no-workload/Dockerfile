# syntax=docker/dockerfile:1.6
# Minimal busybox-based HTTP echo container. Used by the
# workload-auto-generation e2e spec — there is intentionally NO workload.yaml
# alongside this Dockerfile so the build pipeline must auto-generate a Workload.
FROM busybox:1.36
COPY server.sh /server.sh
RUN chmod +x /server.sh
EXPOSE 8080
ENTRYPOINT ["/server.sh"]
