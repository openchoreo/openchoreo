# Platform Engineer Core Backend Plugin

This is the backend plugin for the Platform Engineer Core, which provides APIs to fetch and manage environment information for platform engineers.

## Features

- Fetch all environments across the platform
- Provide environment details including deployment status and endpoints
- Support for organization-wide environment management

## Installation

1. Install the plugin package:

```bash
yarn add @openchoreo/backstage-plugin-platform-engineer-core-backend
```

2. Add the plugin to your backend in `packages/backend/src/index.ts`:

```typescript
import { platformEngineerCorePlugin } from '@openchoreo/backstage-plugin-platform-engineer-core-backend';

const backend = createBackend();
backend.add(platformEngineerViewPlugin);
```

3. Configure the plugin in your `app-config.yaml`:

```yaml
openchoreo:
  baseUrl: 'https://your-openchoreo-instance.com'
  token: 'your-api-token' # optional
```

## API Endpoints

- `GET /api/platform-engineer-core/environments` - Get all environments
- `GET /api/platform-engineer-core/environments/:orgName` - Get environments for a specific organization

## Development

To start the plugin in development mode:

```bash
yarn start
```

To run tests:

```bash
yarn test
```

To build the plugin:

```bash
yarn build
```
