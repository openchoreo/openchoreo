/**
 * Factory functions for creating Thunder IdP API clients
 *
 * @packageDocumentation
 */

import { Config } from '@backstage/config';
import { LoggerService } from '@backstage/backend-plugin-api';
import createClient, { type ClientOptions } from 'openapi-fetch';
import type { paths as UserPaths } from './generated/user/types';
import type { paths as GroupPaths } from './generated/group/types';

/**
 * Configuration options for Thunder IdP API clients
 */
export interface ThunderClientConfig {
  /**
   * Base URL for the Thunder IdP API
   * @example 'https://thunder.example.com:8090'
   */
  baseUrl: string;

  /**
   * Authentication token (Bearer token)
   * If not provided, requests will be made without authentication
   */
  token?: string;

  /**
   * Custom fetch implementation
   * Useful for testing or using a specific fetch polyfill
   */
  fetchApi?: typeof fetch;

  /**
   * Optional logger for debugging
   */
  logger?: LoggerService;
}

/**
 * Creates a Thunder User API client
 *
 * @param config - Configuration options for the client
 * @returns Configured User API client instance
 *
 * @example
 * ```typescript
 * const userClient = createThunderUserClient({
 *   baseUrl: 'https://thunder.example.com:8090',
 *   token: 'your-auth-token'
 * });
 *
 * const { data, error } = await userClient.GET('/users', {
 *   params: { query: { limit: 10 } }
 * });
 * ```
 */
export function createThunderUserClient(config: ThunderClientConfig) {
  const { baseUrl, token, fetchApi, logger } = config;

  logger?.debug(`Creating Thunder User API client with baseUrl: ${baseUrl}`);

  const clientOptions: ClientOptions = {
    baseUrl: baseUrl,
    fetch: fetchApi,
    headers: token
      ? {
          Authorization: `Bearer ${token}`,
        }
      : undefined,
  };

  return createClient<UserPaths>(clientOptions);
}

/**
 * Creates a Thunder Group API client
 *
 * @param config - Configuration options for the client
 * @returns Configured Group API client instance
 *
 * @example
 * ```typescript
 * const groupClient = createThunderGroupClient({
 *   baseUrl: 'https://thunder.example.com:8090',
 *   token: 'your-auth-token'
 * });
 *
 * const { data, error } = await groupClient.GET('/groups', {
 *   params: { query: { limit: 10 } }
 * });
 * ```
 */
export function createThunderGroupClient(config: ThunderClientConfig) {
  const { baseUrl, token, fetchApi, logger } = config;

  logger?.debug(`Creating Thunder Group API client with baseUrl: ${baseUrl}`);

  const clientOptions: ClientOptions = {
    baseUrl: baseUrl,
    fetch: fetchApi,
    headers: token
      ? {
          Authorization: `Bearer ${token}`,
        }
      : undefined,
  };

  return createClient<GroupPaths>(clientOptions);
}

/**
 * Creates Thunder API clients from Backstage configuration
 *
 * @param config - Backstage Config object
 * @param logger - Optional logger service
 * @returns Object containing both user and group API clients
 *
 * @example
 * ```typescript
 * // In your Backstage backend module
 * const clients = createThunderClientsFromConfig(config, logger);
 * const { data: users } = await clients.userClient.GET('/users', {
 *   params: { query: { limit: 10 } }
 * });
 * const { data: groups } = await clients.groupClient.GET('/groups', {
 *   params: { query: { limit: 10 } }
 * });
 * ```
 *
 * @remarks
 * Expects the following configuration in app-config.yaml:
 * ```yaml
 * thunder:
 *   baseUrl: https://thunder.example.com:8090
 *   token: ${THUNDER_TOKEN}
 * ```
 */
export function createThunderClientsFromConfig(
  config: Config,
  logger?: LoggerService
) {
  const baseUrl = config.getString('thunder.baseUrl');
  const token = config.getOptionalString('thunder.token');

  logger?.info('Initializing Thunder IdP API clients');

  const clientConfig: ThunderClientConfig = {
    baseUrl,
    token,
    logger,
  };

  return {
    userClient: createThunderUserClient(clientConfig),
    groupClient: createThunderGroupClient(clientConfig),
  };
}
