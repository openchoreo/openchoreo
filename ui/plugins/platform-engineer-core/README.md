# Platform Engineer Core Plugin

This is the frontend plugin for the Platform Engineer Core, which provides a comprehensive view of all environments across the platform for platform engineers.

## Features

- View all environments across organizations in a card-based layout
- Filter environments by organization
- See environment status, deployments, and endpoints
- Responsive design with modern UI components

## Installation

1. Install the plugin package:

```bash
yarn add @openchoreo/backstage-plugin-platform-engineer-core
```

2. Add the plugin to your Backstage app in `packages/app/src/App.tsx`:

```typescript
import { PlatformEngineerViewPage } from '@openchoreo/backstage-plugin-platform-engineer-core';

// In your app routes
<Route path="/platform-engineer-view" element={<PlatformEngineerViewPage />} />;
```

3. Add a navigation item in your sidebar (optional):

```typescript
// In packages/app/src/components/Root/Root.tsx
import { PlatformEngineerViewIcon } from '@openchoreo/backstage-plugin-platform-engineer-core';

<SidebarItem
  icon={PlatformEngineerViewIcon}
  to="platform-engineer-view"
  text="Platform View"
/>;
```

## Configuration

The plugin uses the same OpenChoreo configuration as other plugins:

```yaml
openchoreo:
  baseUrl: 'https://your-openchoreo-instance.com'
  token: 'your-api-token' # optional
```

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
