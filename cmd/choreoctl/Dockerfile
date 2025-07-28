# Build stage
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

WORKDIR /app

COPY ../../go.mod ../../go.sum ./
RUN go mod download

COPY ../.. .

ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o choreoctl cmd/choreoctl/main.go

# Runtime stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates

# Add non-root user
RUN adduser -D choreoctl

COPY --from=builder /app/choreoctl /usr/local/bin/choreoctl
RUN chmod +x /usr/local/bin/choreoctl

USER choreoctl

ENTRYPOINT ["/usr/local/bin/choreoctl"]
