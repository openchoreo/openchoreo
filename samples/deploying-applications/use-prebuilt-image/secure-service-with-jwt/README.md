# Quick Start Guide: Securing and Accessing a Choreo API Service with JWT Authentication

This guide walks you through configuring JWT authentication for your Go greeter API service and accessing the service through Choreo's gateway.

For demonstration purposes, we'll use a sample JWKS endpoint and token from the Envoy Proxy examples. This allows us to focus on the Choreo configuration steps without setting up our own authentication server.

## Pre-requisites

- Basic understanding of JWT authentication
- Kubernetes cluster with Choreo installed
- The `choreoctl` and `kubectl` CLI tools installed

## Update Environment

Patch your development environment to use the example remote JWKS endpoint:

```bash
kubectl -n default-org patch environments.openchoreo.dev development --type=merge -p '{"spec":{"gateway":{"security":{"remoteJwks":{"uri":"https://raw.githubusercontent.com/envoyproxy/gateway/refs/heads/main/examples/kubernetes/jwt/jwks.json"}}}}}'
```

## Deploy Greeter Application

```bash
choreoctl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/deploying-applications/use-prebuilt-image/jwt/greeter-with-jwt.yaml
```

This command deploys the greeter application to your Kubernetes cluster.

## Expose the API Gateway Locally

Port forward the Choreo gateway service to access it locally:

```bash
kubectl port-forward -n choreo-system svc/choreo-external-gateway 8443:443 &
```

## Obtain an Access Token

Define a token:

```bash
export VALID_TOKEN="eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiIsImtpZCI6ImI1MjBiM2MyYzRiZDc1YTEwZTljZWJjOTU3NjkzM2RjIn0.eyJpc3MiOiJodHRwczovL2Zvby5iYXIuY29tIiwic3ViIjoiMTIzNDU2Nzg5MCIsInVzZXIiOnsibmFtZSI6IkpvaG4gRG9lIiwiZW1haWwiOiJqb2huLmRvZUBleGFtcGxlLmNvbSIsInJvbGVzIjpbImFkbWluIiwiZWRpdG9yIl19LCJwcmVtaXVtX3VzZXIiOnRydWUsImlhdCI6MTUxNjIzOTAyMiwic2NvcGUiOiJyZWFkIGFkZCBkZWxldGUgbW9kaWZ5In0.P36iAlmiRCC79OiB3vstF5Q_9OqUYAMGF3a3H492GlojbV6DcuOz8YIEYGsRSWc-BNJaBKlyvUKsKsGVPtYbbF8ajwZTs64wyO-zhd2R8riPkg_HsW7iwGswV12f5iVRpfQ4AG2owmdOToIaoch0aym89He1ZzEjcShr9olgqlAbbmhnk-namd1rP-xpzPnWhhIVI3mCz5hYYgDTMcM7qbokM5FzFttTRXAn5_Luor23U1062Ct_K53QArwxBvwJ-QYiqcBycHf-hh6sMx_941cUswrZucCpa-EwA3piATf9PKAyeeWHfHV9X-y8ipGOFg3mYMMVBuUZ1lBkJCik9f9kboRY6QzpOISARQj9PKMXfxZdIPNuGmA7msSNAXQgqkvbx04jMwb9U7eCEdGZztH4C8LhlRjgj0ZdD7eNbRjeH2F6zrWyMUpGWaWyq6rMuP98W2DWM5ZflK6qvT1c7FuFsWPvWLkgxQwTWQKrHdKwdbsu32Sj8VtUBJ0-ddEb"
```

## Invoke the Protected Service

Then invoke your API with the Bearer token:

```bash
curl -k https://dev.choreoapis.localhost:8443/default-project/greeting-service-image/greeter/greet \
-H "Authorization: Bearer $VALID_TOKEN" -v
```

> [!TIP]
> #### Verification
> 
> You should receive a successful response from your Go greeter service. If you attempt to access the endpoint without a valid token, you'll receive a 401 Unauthorized response.

## Clean up

To remove all deployed resources, use the following command.

```shell
choreoctl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/deploying-applications/use-prebuilt-image/jwt/greeter-with-jwt.yaml
```

> [!NOTE]
> The following configuration is used to enable OAuth in Choreo's gateway.
>
> ```yaml
>  apiSettings:
>    securitySchemes:
>      - oauth
>  networkVisibilities:
>    public:
>      enable: true
> ```
