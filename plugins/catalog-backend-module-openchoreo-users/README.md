# Thunder User & Group Entity Provider for Backstage Catalog

This Backstage backend module provides a catalog entity provider that syncs users and groups from Thunder IdP into the Backstage catalog.

## Features

- **Full Sync Provider**: Performs complete synchronization of users and groups from Thunder IdP
- **Automatic Pagination**: Handles large user/group datasets by automatically paginating through results
- **Scheduled Updates**: Configurable schedule for periodic synchronization
- **Backstage-compliant Entities**: Transforms Thunder users and groups into valid Backstage User and Group entities

## Installation

This module is already included in your Backstage instance. To enable it, add the module to your backend:

```typescript
// packages/backend/src/index.ts
import { createBackend } from '@backstage/backend-defaults';

const backend = createBackend();

// ... other plugins

// Add the Thunder User & Group Entity Provider
backend.add(
  import(
    '@openchoreo/backstage-plugin-catalog-backend-module-openchoreo-users'
  ),
);

backend.start();
```

## Configuration

Add the following configuration to your `app-config.yaml`:

```yaml
thunder:
  baseUrl: ${THUNDER_BASE_URL} # e.g., https://thunder.example.com:8090
  token: ${THUNDER_TOKEN} # Authentication token for Thunder IdP API
  defaultNamespace: 'default' # Default namespace for User and Group entities (optional)
  schedule:
    frequency: 600 # seconds between runs (default: 600 = 10 minutes)
    timeout: 300 # seconds for timeout (default: 300 = 5 minutes)
```

### Environment Variables

Set the following environment variables:

- `THUNDER_BASE_URL`: The base URL of your Thunder IdP API (e.g., `https://thunder.example.com:8090`)
- `THUNDER_TOKEN`: Authentication token for accessing the Thunder IdP API

## How It Works

### User Synchronization

The provider fetches all users from Thunder IdP and transforms them into Backstage User entities:

- **Username**: Extracted from `attributes.username` or falls back to user ID
- **Display Name**: Constructed from `firstname` and `lastname` attributes
- **Email**: Extracted from `attributes.email`
- **Annotations**: Includes Thunder-specific metadata like user ID, organization unit, and user type

### Group Synchronization

Groups are transformed into Backstage Group entities:

- **Name**: Sanitized group name (lowercase, alphanumeric with hyphens)
- **Members**: Automatically populated with user members from the group
- **Annotations**: Includes Thunder-specific metadata like group ID and organization unit ID

### Entity Naming

All entity names are sanitized to comply with Backstage naming requirements:

- Converted to lowercase
- Only alphanumeric characters, hyphens, and underscores allowed
- Multiple hyphens are collapsed to a single hyphen
- Leading and trailing hyphens are removed

## Entity Structure

### User Entity Example

```yaml
apiVersion: backstage.io/v1alpha1
kind: User
metadata:
  name: john-doe
  title: John Doe
  namespace: default
  annotations:
    backstage.io/managed-by-location: provider:ThunderUserGroupEntityProvider
    thunder.io/user-id: 9a475e1e-b0cb-4b29-8df5-2e5b24fb0ed3
    thunder.io/organization-unit: 456e8400-e29b-41d4-a716-446655440001
    thunder.io/user-type: employee
spec:
  profile:
    displayName: John Doe
    email: john.doe@company.com
  memberOf: []
```

### Group Entity Example

```yaml
apiVersion: backstage.io/v1alpha1
kind: Group
metadata:
  name: engineering-team
  title: Engineering Team
  namespace: default
  annotations:
    backstage.io/managed-by-location: provider:ThunderUserGroupEntityProvider
    thunder.io/group-id: 3fa85f64-5717-4562-b3fc-2c963f66afa6
    thunder.io/organization-unit-id: a839f4bd-39dc-4eaa-b5cc-210d8ecaee87
spec:
  type: team
  profile:
    displayName: Engineering Team
  children: []
  members:
    - john-doe
    - jane-smith
```

## Development

### Building

```bash
yarn workspace @openchoreo/backstage-plugin-catalog-backend-module-openchoreo-users build
```

### Testing

```bash
yarn workspace @openchoreo/backstage-plugin-catalog-backend-module-openchoreo-users test
```

### Linting

```bash
yarn workspace @openchoreo/backstage-plugin-catalog-backend-module-openchoreo-users lint
```

## Troubleshooting

### No users or groups appearing in the catalog

1. Check that the Thunder IdP API is accessible from your Backstage instance
2. Verify that the `THUNDER_BASE_URL` and `THUNDER_TOKEN` environment variables are set correctly
3. Check the Backstage backend logs for any error messages
4. Ensure the token has the necessary permissions to read users and groups

### Entities not updating

1. Check the configured schedule frequency in `app-config.yaml`
2. Verify that the provider is running by checking the logs for "Thunder User & Group Entity Provider registered successfully"
3. Check for any errors in the logs during the sync process

## License

Apache-2.0
