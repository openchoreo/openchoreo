/**
 * Thunder IdP API Client Library
 *
 * Auto-generated TypeScript clients for Thunder User and Group Management APIs.
 * This library provides type-safe API clients for interacting with the Thunder Identity Provider.
 *
 * @packageDocumentation
 */

// Export factory functions
export {
  createThunderUserClient,
  createThunderGroupClient,
  createThunderClientsFromConfig,
  type ThunderClientConfig,
} from './factory';

// Export version constant (generated during build)
export { THUNDER_VERSION } from './version';

// Re-export generated types with namespaces to avoid conflicts
export type * as UserAPI from './generated/user/types';
export type * as GroupAPI from './generated/group/types';
