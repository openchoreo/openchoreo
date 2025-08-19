# OpenChoreo Backstage Plugins

This repository contains Backstage plugins for integrating with [OpenChoreo](https://openchoreo.dev), providing a developer portal for cloud-native application management, deployment visualization, and observability.

## Features

- **Environment Management**: View and manage application environments and workloads
- **Cell Diagrams**: Visualize system architecture and component relationships
- **Runtime Logs**: Real-time log viewing and filtering capabilities
- **Build Integration**: Track builds and deployment pipelines
- **Scaffolding**: Templates for creating OpenChoreo projects and components
- **Catalog Integration**: Automatic discovery and management of OpenChoreo entities

## Prerequisites

### OpenChoreo Setup

Follow the setup [guide](https://openchoreo.dev/docs/getting-started/single-cluster/)

## Development Setup

### Required Tools

- Node.js 22
- Yarn 4.4.1
- Docker

### 1. Install Dependencies

```bash
yarn install
```

### 2. Environment Variables

Set environment variables

```bash
# Required: OpenChoreo API configuration
export OPENCHOREO_API_URL=http://your-openchoreo-api-url/api/v1
```

### 3. Configuration

The application uses three configuration files:

- `app-config.yaml` - Base configuration with OpenChoreo integration
- `app-config.local.yaml` - Local development overrides
- `app-config.production.yaml` - Production configuration

Key configuration sections in `app-config.yaml`:

```yaml
# OpenChoreo integration
openchoreo:
  baseUrl: ${OPENCHOREO_API_URL}
  token: ${OPENCHOREO_TOKEN} # optional
  schedule:
    frequency: 30 # seconds between catalog syncs
    timeout: 120 # request timeout

# GitHub integration (optional)
integrations:
  github:
    - host: github.com
      token: ${GITHUB_TOKEN}
```

### 4. Start the Application

```bash
# Start both frontend and backend
yarn start

# Or start individual services
yarn build:backend  # Build backend first
yarn start          # Start full application
```

The application will be available at:

- Frontend: http://localhost:3000
- Backend API: http://localhost:7007

### 5. Development Workflow

```bash
# Run tests
yarn test           # Changed files only
yarn test:all       # All tests with coverage

# Code quality
yarn lint           # Lint changed files
yarn lint:all       # Lint all files
yarn fix            # Auto-fix issues

# Build
yarn build:all      # Build all packages
yarn tsc            # TypeScript check
```

## Plugin Development

To develop individual plugins in isolation:

```bash
yarn workspace {plugin-name} start
```

example

```bash
yarn workspace @openchoreo/backstage-plugin-backend start
```

Create new plugins:

```bash
yarn new
```

## Available Plugins

- **`@openchoreo/backstage-plugin`** - Frontend UI components
- **`@openchoreo/backstage-plugin-backend`** - Backend API services  
- **`@openchoreo/backstage-plugin-api`** - Shared API client library
- **`@openchoreo/backstage-plugin-catalog-backend-module`** - Catalog entity provider
- **`@openchoreo/backstage-plugin-scaffolder-backend-module`** - Scaffolder actions

## Installation

The plugins are published to GitHub Packages. To install them in your Backstage application:

```bash
# Configure npm to use GitHub Packages for @openchoreo scope
echo "@openchoreo:registry=https://npm.pkg.github.com" >> .npmrc

# Install the plugins you need
yarn add @openchoreo/backstage-plugin
yarn add @openchoreo/backstage-plugin-backend
yarn add @openchoreo/backstage-plugin-api
```

Note: You'll need a GitHub personal access token with `packages:read` permission to install from GitHub Packages.

## Documentation

- Check individual plugin README files in `plugins/` directory
- Visit [Backstage documentation](https://backstage.io/docs) for general Backstage guidance
