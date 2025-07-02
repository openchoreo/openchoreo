import { FetchApi } from '../types/fetch';
import crossFetch from 'cross-fetch';
import * as parser from 'uri-template';
import { ModelsProject, RequestOptions, ProjectsGetRequest, TypedResponse, OpenChoreoApiResponse } from '../models';


/**
 * @public
 */
export class DefaultApiClient {
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
   * Retrieves all Project CRs from all namespaces
   * List all projects
   */
  public async projectsGet(
    request: ProjectsGetRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<OpenChoreoApiResponse<ModelsProject>>> {
    const uriTemplate = `/orgs/{orgName}/projects`;

    const uri = parser.parse(uriTemplate).expand({
      orgName: request.orgName,
    });

    return await this.fetchApi.fetch(`${this.baseUrl}${uri}`, {
      headers: {
        'Content-Type': 'application/json',
        ...(options?.token && { Authorization: `Bearer ${options?.token}` }),
      },
      method: 'GET',
    });
  }
}
