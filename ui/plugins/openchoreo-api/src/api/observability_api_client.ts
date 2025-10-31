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
  BuildObserverUrlGetRequest,
  ObserverUrlData,
} from '../models';
import { DefaultApiClient } from './default_api_client';

/**
 * Error thrown when observability is not configured for a component
 */
export class ObservabilityNotConfiguredError extends Error {
  constructor(componentName: string) {
    super(`Build logs are not available for component ${componentName}`);
    this.name = 'ObservabilityNotConfiguredError';
  }
}

/**
 * API client for observability-related operations
 * Dynamically fetches observer URLs for components and environments
 * @public
 */
export class ObservabilityApiClient {
  private readonly defaultApiClient: DefaultApiClient;
  private readonly fetchApi: FetchApi;
  private readonly runtimeLogsObserverUrlCache: Map<
    string,
    { url: string; timestamp: number }
  > = new Map();
  private readonly buildLogsObserverUrlCache: Map<
    string,
    { url: string; timestamp: number }
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
   * Generates a cache key for build observer URL caching
   * @private
   */
  private getBuildCacheKey(
    orgName: string,
    projectName: string,
    componentName: string,
  ): string {
    return `${orgName}:${projectName}:${componentName}`;
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
  ): Promise<{ baseUrl: string | undefined; available: boolean }> {
    const cacheKey = this.getCacheKey(
      orgName,
      projectName,
      componentName,
      environmentName,
    );
    const cached = this.runtimeLogsObserverUrlCache.get(cacheKey);

    // Check if cached entry is still valid
    if (cached && Date.now() - cached.timestamp < this.cacheExpiryMs) {
      console.debug(`Using cached observer URL for ${cacheKey}`);
      if (cached.url !== undefined) {
        return { baseUrl: cached.url, available: true };
      }
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
      const baseUrl = observerData.observerUrl;

      if (baseUrl === undefined) {
        return { baseUrl, available: false };
      }

      // Cache the result
      this.runtimeLogsObserverUrlCache.set(cacheKey, {
        url: baseUrl,
        timestamp: Date.now(),
      });

      console.info(
        `Successfully fetched and cached observer URL for ${cacheKey}`,
      );
      return { baseUrl, available: true };
    } catch (error) {
      console.error(`Failed to fetch observer URL for ${cacheKey}:`, error);
      throw new Error(
        `Unable to retrieve observer URL for component ${componentName} in environment ${environmentName}: ${error}`,
      );
    }
  }

  /**
   * Fetches build observer URL and connection details for a component
   * Implements caching to reduce API calls
   * @private
   */
  private async getBuildObserverUrl(
    orgName: string,
    projectName: string,
    componentName: string,
  ): Promise<{ baseUrl: string | undefined; available: boolean }> {
    const cacheKey = this.getBuildCacheKey(orgName, projectName, componentName);
    const cached = this.buildLogsObserverUrlCache.get(cacheKey);

    // Check if cached entry is still valid
    if (cached && Date.now() - cached.timestamp < this.cacheExpiryMs) {
      console.debug(`Using cached build observer URL for ${cacheKey}`);
      if (cached.url !== undefined) {
        return { baseUrl: cached.url, available: true };
      }
    }

    console.info(`Fetching build observer URL for component ${componentName}`);

    try {
      const request: BuildObserverUrlGetRequest = {
        orgName,
        projectName,
        componentName,
      };

      const response = await this.defaultApiClient.buildObserverUrlGet(request);

      let responseData;
      try {
        responseData = await response.json();
      } catch (jsonError) {
        console.error(
          `Failed to parse JSON response from observer URL endpoint:`,
          {
            status: response.status,
            statusText: response.statusText,
            responseBody: responseData,
            jsonError:
              jsonError instanceof Error
                ? jsonError.message
                : String(jsonError),
          },
        );
        throw new Error(
          `Invalid JSON response from observer URL endpoint (status: ${response.status}): ${responseData}`,
        );
      }

      if (!responseData.success || !responseData.data) {
        throw new Error(
          `Failed to fetch build observer URL: ${JSON.stringify(responseData)}`,
        );
      }

      const observerData: ObserverUrlData = responseData.data;
      const baseUrl = observerData.observerUrl;

      if (baseUrl === undefined) {
        return { baseUrl, available: false };
      }

      // Cache the result
      this.buildLogsObserverUrlCache.set(cacheKey, {
        url: baseUrl,
        timestamp: Date.now(),
      });

      console.info(
        `Successfully fetched and cached build observer URL for ${cacheKey}`,
      );
      return { baseUrl, available: true };
    } catch (error) {
      console.error(
        `Failed to fetch build observer URL for ${cacheKey}:`,
        error,
      );
      throw new Error(
        `Unable to retrieve build observer URL for component ${componentName}: ${error}`,
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
    request: ComponentLogsPostRequest,
    orgName: string,
    projectName: string,
    options?: RequestOptions,
  ): Promise<TypedResponse<RuntimeLogsResponse>> {
    console.info(
      `Fetching runtime logs for component ${request.componentId} in environment ${request.componentId}`,
    );

    try {
      const { baseUrl, available } = await this.getObserverUrl(
        orgName,
        projectName,
        request.componentId,
        request.environmentId,
        options,
      );

      if (!available) {
        throw new ObservabilityNotConfiguredError(request.componentId);
      }

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
          ...(options?.token && {
            'X-Original-Authorization': `Bearer ${options?.token}`,
          }),
        },
        method: 'POST',
        body: JSON.stringify(body),
      });
    } catch (error) {
      console.error(
        `Failed to fetch runtime logs for component ${request.componentId}:`,
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
    request: ComponentBuildLogsPostRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<RuntimeLogsResponse>> {
    console.info(`Fetching build logs for component ${request.componentName}`);

    try {
      const { baseUrl, available } = await this.getBuildObserverUrl(
        request.orgName,
        request.projectName,
        request.componentName,
      );

      if (!available) {
        throw new ObservabilityNotConfiguredError(request.componentName);
      }

      const uriTemplate = `/api/logs/component/{componentName}`;

      const uri = parser.parse(uriTemplate).expand({
        componentName: request.componentName,
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
