import { FetchApi } from '../types/fetch';
import crossFetch from 'cross-fetch';
import * as parser from 'uri-template';
import {
  RequestOptions,
  TypedResponse,
  ComponentLogsPostRequest,
  ComponentBuildLogsPostRequest,
  RuntimeLogsResponse,
} from '../models';

/**
 * API client for observability-related operations
 * @public
 */
export class ObservabilityApiClient {
  private readonly baseUrl: string;
  private readonly fetchApi: FetchApi;

  constructor(
    baseUrl: string,
    options: {
      fetchApi?: { fetch: typeof fetch };
    },
  ) {
    this.baseUrl = baseUrl;
    this.fetchApi = options.fetchApi || { fetch: crossFetch };
  }

  /**
   * Fetches runtime logs for a specific component
   * @param request - The request parameters for fetching logs
   * @param options - Optional request options including authentication
   * @returns Promise containing the logs response
   */
  public async componentRuntimeLogsPost(
    request: ComponentLogsPostRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<RuntimeLogsResponse>> {
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

    return await this.fetchApi.fetch(`${this.baseUrl}${uri}`, {
      headers: {
        'Content-Type': 'application/json',
        ...(options?.token && { Authorization: `Bearer ${options?.token}` }),
      },
      method: 'POST',
      body: JSON.stringify(body),
    });
  }

  /**
   * Fetches build logs for a specific component
   * @param request - The request parameters for fetching build logs
   * @param options - Optional request options including authentication
   * @returns Promise containing the logs response
   */
  public async componentBuildLogsPost(
    request: ComponentBuildLogsPostRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<RuntimeLogsResponse>> {
    const uriTemplate = `/api/logs/component/{componentId}`;

    const uri = parser.parse(uriTemplate).expand({
      componentId: request.componentId,
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

    return await this.fetchApi.fetch(`${this.baseUrl}${uri}`, {
      headers: {
        'Content-Type': 'application/json',
        ...(options?.token && { Authorization: `Bearer ${options?.token}` }),
      },
      method: 'POST',
      body: JSON.stringify(body),
    });
  }
}
