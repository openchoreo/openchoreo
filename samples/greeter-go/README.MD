# Quick Start Guide: Securing and Accessing a Choreo API Service

This guide walks you through configuring JWT authentication for your API service using Asgardeo as the identity provider, then accessing the service through Choreo's gateway.

## 1. Configure JWKS for Authentication

First, patch your development environment to use a remote JWKS endpoint from Asgardeo:

```bash
kubectl patch environments.core.choreo.dev development --type=merge -p '{"spec":{"gateway":{"security":{"remoteJwks":{"uri":"https://api.asgardeo.io/t/<organization_name>/oauth2/jwks"}}}}}'
```

## 2. Update Deployable Artifact Security Scheme

Set the security scheme for your API to OAuth:

```bash
# Update your deployment configuration to use OAuth
# Add this to your configuration file or apply via kubectl
configuration.EndpointTemplates.apiSettings.security.securitySchemes[0] = oauth
```

## 3. Expose the API Gateway Locally

Forward the Choreo gateway service to access it locally:

```bash
kubectl port-forward -n choreo-system svc/<gateway_service_name> 8888:443 &
```

## 4. Obtain an Access Token

Request an access token from Asgardeo using client credentials:

```bash
curl -X POST https://api.asgardeo.io/t/<organization_name>/oauth2/token \
  -d "client_id=v<client_id>&client_secret=<client_secret>&grant_type=client_credentials"
```

Save the returned access token for the next step.

## 5. Invoke the Protected Service

Call your API with the Bearer token:

```bash
curl -k https://localhost:8888/default-project/greeting-service-go/greeter/greet \
  -H "host:development.apis.choreo.local" \
  -H "Authorization: Bearer <token>"
```

