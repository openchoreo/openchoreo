/*
 * Hi!
 *
 * Note that this is an EXAMPLE Backstage backend. Please check the README.
 *
 * Happy hacking!
 */

import { createBackend } from '@backstage/backend-defaults';
import { OpenChoreoDefaultAuthModule } from '@openchoreo/backstage-plugin-auth-backend-module-openchoreo-default';
import { rootHttpRouterServiceFactory } from '@backstage/backend-defaults/rootHttpRouter';

const backend = createBackend();

backend.add(rootHttpRouterServiceFactory());

backend.add(import('@backstage/plugin-app-backend'));
backend.add(import('@backstage/plugin-proxy-backend'));
backend.add(import('@backstage/plugin-scaffolder-backend'));
backend.add(import('@backstage/plugin-scaffolder-backend-module-github'));
backend.add(import('@backstage/plugin-techdocs-backend'));

// auth plugin
backend.add(import('@backstage/plugin-auth-backend'));
// See https://backstage.io/docs/backend-system/building-backends/migrating#the-auth-plugin

// OpenChoreo Default IDP OAuth provider
backend.add(OpenChoreoDefaultAuthModule);

// catalog plugin
backend.add(import('@backstage/plugin-catalog-backend'));
backend.add(
  import('@backstage/plugin-catalog-backend-module-scaffolder-entity-model'),
);

// See https://backstage.io/docs/features/software-catalog/configuration#subscribing-to-catalog-errors
backend.add(import('@backstage/plugin-catalog-backend-module-logs'));

// permission plugin
backend.add(import('@backstage/plugin-permission-backend'));
// See https://backstage.io/docs/permissions/getting-started for how to create your own permission policy
backend.add(
  import('@backstage/plugin-permission-backend-module-allow-all-policy'),
);

// search plugin
backend.add(import('@backstage/plugin-search-backend'));

// search engine
// See https://backstage.io/docs/features/search/search-engines
backend.add(import('@backstage/plugin-search-backend-module-pg'));

// search collators
backend.add(import('@backstage/plugin-search-backend-module-catalog'));
backend.add(import('@backstage/plugin-search-backend-module-techdocs'));

backend.add(import('@openchoreo/backstage-plugin-backend'));
backend.add(import('@openchoreo/backstage-plugin-catalog-backend-module'));
backend.add(import('@openchoreo/backstage-plugin-scaffolder-backend-module'));
backend.add(
  import(
    '@openchoreo/backstage-plugin-catalog-backend-module-openchoreo-users'
  ),
);
backend.add(import('@openchoreo/backstage-plugin-platform-engineer-core-backend'));
// backend.add(import('@openchoreo/backstage-plugin-home-backend'));
backend.start();
