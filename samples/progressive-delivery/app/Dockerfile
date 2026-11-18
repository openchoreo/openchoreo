FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -o /demo-app .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /demo-app /demo-app
EXPOSE 8080
ENTRYPOINT ["/demo-app"]
