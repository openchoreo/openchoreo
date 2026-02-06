# AI Assistant Configuration for OpenChoreo MCP Server

This guide shows how to configure different AI assistants to connect to the OpenChoreo MCP server.

## Prerequisites

Before configuring your AI assistant:

1. **Running OpenChoreo instance** - Complete the [Quick Start Guide](https://openchoreo.dev/docs/getting-started/quick-start-guide/)
2. **MCP Server Endpoint** - Know your OpenChoreo API endpoint (e.g., `http://api.openchoreo.localhost:8080/mcp`)
3. **Authentication Token** - Obtain a JWT token for authenticating with the MCP server
4. **AI Assistant Installed** - Have one of the supported AI assistants installed

## MCP Server Configuration for AI assistants

The OpenChoreo MCP server exposes an HTTP endpoint that AI assistants connect to. You'll need:

- **Endpoint URL**: Typically `http://api.openchoreo.localhost:8080/mcp` (adjust based on your setup)
- **Authentication**: Bearer token in the `Authorization` header
- **Transport**: HTTP-based MCP transport (most assistants support this)

## Obtaining an Authentication Token

OpenChoreo uses an identity provider for authentication. The default identity provider is **Asgardeo Thunder** with the token endpoint at `http://thunder.openchoreo.localhost:8080/oauth2/token`.

Follow these steps to obtain an authentication token if you're using the default identity provider:

### Step 1: Get the Application ID

1. Open your browser and navigate to http://thunder.openchoreo.localhost:8080/develop
   > **Note**: Default credentials are `admin` / `admin`

2. Open the `Sample App` application
3. Copy the **Application ID**

### Step 2: Obtain an Admin Token

Replace `<application_id>` with your Sample App ID and run:

```bash
ADMIN_TOKEN_RESPONSE=$(curl -k -s -X POST 'http://thunder.openchoreo.localhost:8080/flow/execute' \
  -H 'Content-Type: application/json' \
  -d '{
    "applicationId": "<application_id>",
    "flowType": "AUTHENTICATION",
    "inputs": {
      "username": "admin",
      "password": "admin",
      "requested_permissions": "system"
    }
  }')

ADMIN_TOKEN=$(echo $ADMIN_TOKEN_RESPONSE | jq -r '.assertion')
```

### Step 3: Create an OAuth2 Application

Create a new OAuth2 application with client credentials grant:

```bash
curl -L 'http://thunder.openchoreo.localhost:8080/applications' \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "name": "MCP client",
    "description": "MCP client application to use openchoreo MCP server",
    "inbound_auth_config": [
        {
            "type": "oauth2",
            "config": {
                "client_id": "mcp_client",
                "client_secret": "mcp_client_secret",
                "grant_types": [
                    "client_credentials"
                ],
                "token_endpoint_auth_method": "client_secret_basic",
                "token": {
                    "issuer": "thunder",
                    "access_token": {
                        "validity_period": 3600
                    }
                }
            }
        }
    ]
}'
```

### Step 4: Obtain Your Token

Use the client credentials to get an access token:

```bash
curl -L 'http://thunder.openchoreo.localhost:8080/oauth2/token' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -u 'mcp_client:mcp_client_secret' \
  -d 'grant_type=client_credentials'
```

The response will contain an `access_token` that you can use as the Bearer token for authenticating with the OpenChoreo MCP server.
