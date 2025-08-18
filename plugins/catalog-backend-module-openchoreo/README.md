# catalog-backend-module-openchoreo

This is the OpenChoreo backend module for the Backstage catalog plugin.

## Installation

Add the module to your Backstage backend:

```bash
yarn add @internal/plugin-catalog-backend-module-openchoreo
```

## Configuration

Add the OpenChoreo configuration to your `app-config.yaml`:

```yaml
openchoreo:
  baseUrl: http://localhost:8080/api/v1
  token: ${OPENCHOREO_TOKEN} # optional: for authentication
```

## Usage

Register the module in your backend:

```typescript
// packages/backend/src/index.ts
import { createBackend } from '@backstage/backend-defaults';

const backend = createBackend();

// ... other plugins

// Add the OpenChoreo catalog module
backend.add(import('@internal/plugin-catalog-backend-module-openchoreo'));

backend.start();
```

## Features

- **Entity Provider**: Automatically discovers and ingests projects from OpenChoreo API as Backstage System entities
- **Scheduled Updates**: Runs every 30 minutes to keep entities in sync
- **Configuration-based**: Uses Backstage configuration system for API connection details

## Entity Mapping

The module translates OpenChoreo projects to Backstage entities as follows:

- **OpenChoreo Project** → **Backstage System**
- Project metadata becomes system metadata
- Projects are tagged with `openchoreo` and `project`
- Entities are annotated with OpenChoreo-specific information

## Development

To work on this module:

```bash
# Start in development mode
yarn start

# Run tests
yarn test

# Build for production
yarn build
```
