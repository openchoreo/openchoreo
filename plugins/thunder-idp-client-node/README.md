# @openchoreo/backstage-plugin-thunder-idp-client-node

Auto-generated TypeScript API clients for [Thunder Identity Provider](https://github.com/asgardeo/thunder) User and Group Management APIs.

This library provides type-safe, fully typed API clients for interacting with Thunder IdP, built using `openapi-typescript` and `openapi-fetch` for maximum type safety and developer experience.

## Features

- ✨ **Fully Type-Safe**: Generated from OpenAPI specs with complete TypeScript types
- 🔄 **Auto-Regeneration**: Automatically regenerates clients on build
- 📦 **Zero Runtime Dependencies**: Uses native `fetch` API (Node.js 18+)
- 🎯 **Version-Controlled**: Thunder version tracked in `package.json`
- 🔧 **Backstage Integration**: Factory functions for easy Backstage backend integration
- 🚀 **Modern Stack**: Built with `openapi-typescript` and `openapi-fetch`

## Installation

This package is part of the OpenChoreo Backstage plugins monorepo and is installed automatically when you install the workspace dependencies.

```bash
yarn install
```

## Quick Start

### Basic Usage

```typescript
import { createThunderUserClient, createThunderGroupClient } from '@openchoreo/backstage-plugin-thunder-idp-client-node';

// Create API clients
const userClient = createThunderUserClient({
  baseUrl: 'https://thunder.example.com:8090',
  token: 'your-bearer-token'
});

const groupClient = createThunderGroupClient({
  baseUrl: 'https://thunder.example.com:8090',
  token: 'your-bearer-token'
});

// List users with type-safe parameters
const { data: users, error: userError } = await userClient.GET('/users', {
  params: {
    query: {
      limit: 10,
      offset: 0,
      filter: 'username eq "john.doe"'
    }
  }
});

if (userError) {
  console.error('Error fetching users:', userError);
} else {
  console.log('Users:', users);
}

// List groups
const { data: groups, error: groupError } = await groupClient.GET('/groups', {
  params: {
    query: { limit: 10 }
  }
});
```

### Backstage Integration

For Backstage backend modules, use the config-based factory:

```typescript
import { createThunderClientsFromConfig } from '@openchoreo/backstage-plugin-thunder-idp-client-node';
import { LoggerService } from '@backstage/backend-plugin-api';
import { Config } from '@backstage/config';

export function createMyService(config: Config, logger: LoggerService) {
  const { userClient, groupClient } = createThunderClientsFromConfig(config, logger);

  // Use the clients
  const { data: users } = await userClient.GET('/users');
  const { data: groups } = await groupClient.GET('/groups');

  return { users, groups };
}
```

**app-config.yaml**:
```yaml
thunder:
  baseUrl: https://thunder.example.com:8090
  token: ${THUNDER_TOKEN}  # From environment variable
```

## API Clients

This library provides two main API clients:

### User Management API

Interact with Thunder's User Management API:

```typescript
// List users
await userClient.GET('/users', { params: { query: { limit: 10 } } });

// Get user by ID
await userClient.GET('/users/{id}', { params: { path: { id: 'user-uuid' } } });

// Create user
await userClient.POST('/users', {
  body: {
    organizationUnit: 'org-uuid',
    type: 'customer',
    attributes: {
      email: 'user@example.com',
      username: 'john.doe'
    }
  }
});

// Update user
await userClient.PUT('/users/{id}', {
  params: { path: { id: 'user-uuid' } },
  body: { /* updated attributes */ }
});

// Delete user
await userClient.DELETE('/users/{id}', {
  params: { path: { id: 'user-uuid' } }
});

// Get user's groups
await userClient.GET('/users/{id}/groups', {
  params: { path: { id: 'user-uuid' } }
});
```

### Group Management API

Interact with Thunder's Group Management API:

```typescript
// List groups
await groupClient.GET('/groups', { params: { query: { limit: 10 } } });

// Get group by ID
await groupClient.GET('/groups/{id}', { params: { path: { id: 'group-uuid' } } });

// Create group
await groupClient.POST('/groups', {
  body: {
    name: 'Engineering',
    description: 'Engineering team',
    organizationUnitId: 'org-uuid',
    members: [
      { id: 'user-uuid-1', type: 'user' },
      { id: 'user-uuid-2', type: 'user' }
    ]
  }
});

// Update group
await groupClient.PUT('/groups/{id}', {
  params: { path: { id: 'group-uuid' } },
  body: { /* updated fields */ }
});

// Delete group
await groupClient.DELETE('/groups/{id}', {
  params: { path: { id: 'group-uuid' } }
});

// Get group members
await groupClient.GET('/groups/{id}/members', {
  params: { path: { id: 'group-uuid' } }
});
```

## Generating API Clients

### Automatic Generation (Recommended)

Clients are automatically generated before build:

```bash
yarn build
```

This will:
1. Download OpenAPI specs from Thunder repository (using version from `package.json`)
2. Generate TypeScript types
3. Build the package

### Manual Generation

Generate clients manually:

```bash
# Generate using version from package.json
yarn generate:clients

# Clean generated files
yarn clean:generated

# Clean and regenerate
yarn clean:generated && yarn generate:clients
```

### Testing Against Different Versions

Test against a specific Thunder version without modifying `package.json`:

```bash
bash scripts/generate-clients.sh --thunder-version v0.11.0
```

## Upgrading Thunder Version

To upgrade to a new Thunder version:

1. **Update `package.json`**:
   ```json
   {
     "thunderVersion": "v0.11.0"
   }
   ```

2. **Regenerate clients**:
   ```bash
   yarn clean:generated
   yarn generate:clients
   ```

3. **Test the changes**:
   ```bash
   yarn build
   yarn test
   ```

4. **Commit**:
   ```bash
   git add plugins/thunder-idp-client-node/package.json
   git commit -m "chore: upgrade Thunder IdP client to v0.11.0"
   ```

## Configuration Options

### ThunderClientConfig

```typescript
interface ThunderClientConfig {
  baseUrl: string;          // Thunder API base URL
  token?: string;           // Bearer token for authentication
  fetchApi?: typeof fetch;  // Custom fetch implementation (optional)
  logger?: LoggerService;   // Backstage logger (optional)
}
```

## Type Safety

All API endpoints, parameters, request bodies, and response types are fully typed:

```typescript
// ✅ TypeScript will validate paths, parameters, and responses
const { data } = await userClient.GET('/users', {
  params: {
    query: {
      limit: 10,
      offset: 0,
      filter: 'username eq "john.doe"'
    }
  }
});

// ❌ TypeScript will error on invalid paths
const { data } = await userClient.GET('/invalid-path');  // Type error!

// ❌ TypeScript will error on invalid parameters
const { data } = await userClient.GET('/users', {
  params: {
    query: {
      invalidParam: true  // Type error!
    }
  }
});
```

## Error Handling

`openapi-fetch` returns both `data` and `error`, never throws:

```typescript
const { data, error } = await userClient.GET('/users');

if (error) {
  // Handle error (error is typed based on OpenAPI spec)
  console.error('API Error:', error);
  return;
}

// TypeScript knows data is defined here
console.log('Users:', data.users);
```

## Development

### Project Structure

```
plugins/thunder-idp-client-node/
├── src/
│   ├── generated/          # Auto-generated (gitignored)
│   │   ├── user/           # User API types
│   │   │   ├── types.ts
│   │   │   └── index.ts
│   │   └── group/          # Group API types
│   │       ├── types.ts
│   │       └── index.ts
│   ├── factory.ts          # Client factory functions
│   ├── index.ts            # Public API exports
│   └── version.ts          # Thunder version (auto-generated)
├── openapi/                # Downloaded specs (gitignored)
│   ├── user.yaml
│   └── group.yaml
├── scripts/
│   └── generate-clients.sh # Generation script
├── package.json            # Contains thunderVersion field
└── README.md
```

### Scripts

- `yarn generate:clients` - Generate API clients from OpenAPI specs
- `yarn clean:generated` - Remove generated files
- `yarn build` - Build the package (auto-generates clients first)
- `yarn lint` - Lint the code
- `yarn test` - Run tests

## Thunder Version Information

Current Thunder version: Check `thunderVersion` in `package.json`

Generated clients are version-specific to the Thunder release. The version constant is exported:

```typescript
import { THUNDER_VERSION } from '@openchoreo/backstage-plugin-thunder-idp-client-node';

console.log('Using Thunder version:', THUNDER_VERSION);  // e.g., "v0.10.0"
```

## Troubleshooting

### "Cannot find module './generated/user'"

Run the generation script:
```bash
yarn generate:clients
```

### "Failed to download user.yaml"

Check that the Thunder version exists:
```bash
# Check available tags at:
# https://github.com/asgardeo/thunder/tags
```

### Type errors after upgrading Thunder version

Clean and regenerate:
```bash
yarn clean:generated
yarn generate:clients
yarn build
```

## Contributing

This package is part of the OpenChoreo Backstage plugins monorepo. See the main repository README for contribution guidelines.

## License

Apache-2.0

## Links

- [Thunder IdP Repository](https://github.com/asgardeo/thunder)
- [OpenAPI Spec - User API](https://github.com/asgardeo/thunder/blob/main/docs/apis/user.yaml)
- [OpenAPI Spec - Group API](https://github.com/asgardeo/thunder/blob/main/docs/apis/group.yaml)
- [openapi-typescript](https://github.com/drwpow/openapi-typescript)
- [openapi-fetch](https://github.com/drwpow/openapi-typescript/tree/main/packages/openapi-fetch)
