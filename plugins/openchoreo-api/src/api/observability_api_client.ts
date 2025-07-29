import { FetchApi } from '../types/fetch';
import crossFetch from 'cross-fetch';
import * as parser from 'uri-template';
import {
  RequestOptions,
  TypedResponse,
  ComponentLogsPostRequest,
  ComponentBuildLogsPostRequest,
  RuntimeLogsResponse,
  RuntimeLogsObserverUrlGetRequest,
  ObserverUrlData,
} from '../models';
import { DefaultApiClient } from './default_api_client';

/**
 * API client for observability-related operations
 * Dynamically fetches observer URLs for components and environments
 * @public
 */
export class ObservabilityApiClient {
  private readonly defaultApiClient: DefaultApiClient;
  private readonly fetchApi: FetchApi;
  private readonly observerUrlCache: Map<
    string,
    { url: string; auth: string; timestamp: number }
  > = new Map();
  private readonly cacheExpiryMs = 5 * 60 * 1000; // 5 minutes

  constructor(
    defaultApiClient: DefaultApiClient,
    options: {
      fetchApi?: { fetch: typeof fetch };
    } = {},
  ) {
    this.defaultApiClient = defaultApiClient;
    this.fetchApi = options.fetchApi || { fetch: crossFetch };
  }

  /**
   * Generates a cache key for observer URL caching
   * @private
   */
  private getCacheKey(
    orgName: string,
    projectName: string,
    componentName: string,
    environmentName: string,
  ): string {
    return `${orgName}:${projectName}:${componentName}:${environmentName}`;
  }

  /**
   * Fetches observer URL and connection details for a component environment
   * Implements caching to reduce API calls
   * @private
   */
  private async getObserverUrl(
    orgName: string,
    projectName: string,
    componentName: string,
    environmentName: string,
    options?: RequestOptions,
  ): Promise<{ baseUrl: string; authHeader: string }> {
    const cacheKey = this.getCacheKey(
      orgName,
      projectName,
      componentName,
      environmentName,
    );
    const cached = this.observerUrlCache.get(cacheKey);

    // Check if cached entry is still valid
    if (cached && Date.now() - cached.timestamp < this.cacheExpiryMs) {
      console.debug(`Using cached observer URL for ${cacheKey}`);
      return { baseUrl: cached.url, authHeader: cached.auth };
    }

    console.info(
      `Fetching observer URL for component ${componentName} in environment ${environmentName}`,
    );

    try {
      const request: RuntimeLogsObserverUrlGetRequest = {
        orgName,
        projectName,
        componentName,
        environmentName,
      };

      const response = await this.defaultApiClient.runtimeLogsObserverUrlGet(
        request,
        options,
      );
      const responseData = await response.json();

      if (!responseData.success || !responseData.data) {
        throw new Error(
          `Failed to fetch observer URL: ${JSON.stringify(responseData)}`,
        );
      }

      const observerData: ObserverUrlData = responseData.data;
      const baseUrl = `https://${observerData.observerUrl}`;

      // Create basic auth header
      const credentials = btoa(
        `${observerData.connectionMethod.username}:${observerData.connectionMethod.password}`,
      );
      const authHeader = `Basic ${credentials}`;

      // Cache the result
      this.observerUrlCache.set(cacheKey, {
        url: baseUrl,
        auth: authHeader,
        timestamp: Date.now(),
      });

      console.info(
        `Successfully fetched and cached observer URL for ${cacheKey}`,
      );
      return { baseUrl, authHeader };
    } catch (error) {
      console.error(`Failed to fetch observer URL for ${cacheKey}:`, error);
      throw new Error(
        `Unable to retrieve observer URL for component ${componentName} in environment ${environmentName}: ${error}`,
      );
    }
  }

  /**
   * Fetches runtime logs for a specific component
   * Dynamically resolves observer URL for the component environment
   * @param request - The request parameters for fetching logs
   * @param options - Optional request options including authentication and component context
   * @returns Promise containing the logs response
   */
  public async componentRuntimeLogsPost(
    request: ComponentLogsPostRequest & {
      orgName: string;
      projectName: string;
      componentName: string;
      environmentName: string;
    },
    options?: RequestOptions,
  ): Promise<TypedResponse<RuntimeLogsResponse>> {
    console.info(
      `Fetching runtime logs for component ${request.componentName} in environment ${request.environmentName}`,
    );

    try {
      const { baseUrl, authHeader } = await this.getObserverUrl(
        request.orgName,
        request.projectName,
        request.componentName,
        request.environmentName,
        options,
      );

      const uriTemplate = `/api/logs/component/{componentId}`;

      const uri = parser.parse(uriTemplate).expand({
        componentId: request.componentId,
      });

      const body = {
        environmentId: request.environmentId,
        ...(request.logLevels && { logLevels: request.logLevels }),
        ...(request.startTime && { startTime: request.startTime }),
        ...(request.endTime && { endTime: request.endTime }),
        ...(request.limit !== undefined && { limit: request.limit }),
        ...(request.offset !== undefined && { offset: request.offset }),
      };

      console.debug(`Making runtime logs request to ${baseUrl}${uri}`);

      return await this.fetchApi.fetch(`${baseUrl}${uri}`, {
        headers: {
          'Content-Type': 'application/json',
          Authorization: authHeader,
          ...(options?.token && {
            'X-Original-Authorization': `Bearer ${options?.token}`,
          }),
        },
        method: 'POST',
        body: JSON.stringify(body),
      });
    } catch (error) {
      console.error(
        `Failed to fetch runtime logs for component ${request.componentName}:`,
        error,
      );
      throw error;
    }
  }

  /**
   * Fetches build logs for a specific component
   * Dynamically resolves observer URL for the component environment
   * @param request - The request parameters for fetching build logs
   * @param options - Optional request options including authentication and component context
   * @returns Promise containing the logs response
   */
  public async componentBuildLogsPost(
    request: ComponentBuildLogsPostRequest & {
      orgName: string;
      projectName: string;
      componentName: string;
      environmentName: string;
    },
    options?: RequestOptions,
  ): Promise<TypedResponse<RuntimeLogsResponse>> {
    console.info(
      `Fetching build logs for component ${request.componentName} in environment ${request.environmentName}`,
    );

    try {
      const { baseUrl, authHeader } = await this.getObserverUrl(
        request.orgName,
        request.projectName,
        request.componentName,
        request.environmentName,
        options,
      );

      const uriTemplate = `/api/logs/component/{componentId}`;

      const uri = parser.parse(uriTemplate).expand({
        componentId: request.componentName,
      });

      const body = {
        buildId: request.buildId,
        buildUuid: request.buildUuid,
        logLevels: ['INFO'],
        logType: 'BUILD',
        ...(request.searchPhrase !== undefined && {
          searchPhrase: request.searchPhrase,
        }),
        ...(request.limit !== undefined && { limit: request.limit }),
      };

      console.debug(`Making build logs request to ${baseUrl}${uri}`);

      return await this.fetchApi.fetch(`${baseUrl}${uri}`, {
        headers: {
          'Content-Type': 'application/json',
          Authorization: authHeader,
          ...(options?.token && {
            'X-Original-Authorization': `Bearer ${options?.token}`,
          }),
        },
        method: 'POST',
        body: JSON.stringify(body),
      });
    } catch (error) {
      console.error(
        `Failed to fetch build logs for component ${request.componentName}:`,
        error,
      );
      throw error;
    }
  }
}
