import { FetchApi } from '../types/fetch';
import crossFetch from 'cross-fetch';
import * as parser from 'uri-template';
import {
  ModelsProject,
  ModelsOrganization,
  ModelsComponent,
  ModelsEnvironment,
  ModelsBuildTemplate,
  ModelsBuild,
  RequestOptions,
  ProjectsGetRequest,
  OrganizationsGetRequest,
  ComponentsGetRequest,
  EnvironmentsGetRequest,
  BuildTemplatesGetRequest,
  BuildsGetRequest,
  BuildsTriggerRequest,
  ProjectsPostRequest,
  ComponentsPostRequest,
  TypedResponse,
  OpenChoreoApiResponse,
  OpenChoreoApiSingleResponse,
  ComponentGetRequest,
  ModelsCompleteComponent,
} from '../models';

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

  /**
   * Retrieves all Organization CRs from all namespaces
   * List all organizations
   */
  public async organizationsGet(
    _request: OrganizationsGetRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<OpenChoreoApiResponse<ModelsOrganization>>> {
    const uri = `/orgs`;

    return await this.fetchApi.fetch(`${this.baseUrl}${uri}`, {
      headers: {
        'Content-Type': 'application/json',
        ...(options?.token && { Authorization: `Bearer ${options?.token}` }),
      },
      method: 'GET',
    });
  }

  /**
   * Retrieves all environments for an organization
   * List all environments of an organization
   */
  public async environmentsGet(
    request: EnvironmentsGetRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<OpenChoreoApiResponse<ModelsEnvironment>>> {
    const uriTemplate = `/orgs/{orgName}/environments`;

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

  /**
   * Retrieves all Component CRs from a project
   * List all components of a project
   */
  public async componentsGet(
    request: ComponentsGetRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<OpenChoreoApiResponse<ModelsComponent>>> {
    const uriTemplate = `/orgs/{orgName}/projects/{projectName}/components`;

    const uri = parser.parse(uriTemplate).expand({
      orgName: request.orgName,
      projectName: request.projectName,
    });

    return await this.fetchApi.fetch(`${this.baseUrl}${uri}`, {
      headers: {
        'Content-Type': 'application/json',
        ...(options?.token && { Authorization: `Bearer ${options?.token}` }),
      },
      method: 'GET',
    });
  }

  /**
   * Retrieves all Component CRs from a project
   * List all components of a project
   */
  public async componentGet(
    request: ComponentGetRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<OpenChoreoApiResponse<ModelsCompleteComponent>>> {
    const uriTemplate = `/orgs/{orgName}/projects/{projectName}/components/{componentName}?include=type,workload`;

    const uri = parser.parse(uriTemplate).expand({
      orgName: request.orgName,
      projectName: request.projectName,
      componentName: request.componentName,
    });

    return await this.fetchApi.fetch(`${this.baseUrl}${uri}`, {
      headers: {
        'Content-Type': 'application/json',
        ...(options?.token && { Authorization: `Bearer ${options?.token}` }),
      },
      method: 'GET',
    });
  }

  /**
   * Creates a new project in the specified organization
   * Create a new project
   */
  public async projectsPost(
    request: ProjectsPostRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<OpenChoreoApiSingleResponse<ModelsProject>>> {
    const uriTemplate = `/orgs/{orgName}/projects`;

    const uri = parser.parse(uriTemplate).expand({
      orgName: request.orgName,
    });

    const body = {
      name: request.name,
      ...(request.displayName && { displayName: request.displayName }),
      ...(request.description && { description: request.description }),
      ...(request.deploymentPipeline && {
        deploymentPipeline: request.deploymentPipeline,
      }),
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
   * Retrieves all build templates for an organization
   * List all build templates of an organization
   */
  public async buildTemplatesGet(
    request: BuildTemplatesGetRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<OpenChoreoApiResponse<ModelsBuildTemplate>>> {
    const uriTemplate = `/orgs/{orgName}/build-templates`;

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

  /**
   * Retrieves all builds for a component
   * List all builds of a component
   */
  public async buildsGet(
    request: BuildsGetRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<OpenChoreoApiResponse<ModelsBuild>>> {
    const uriTemplate = `/orgs/{orgName}/projects/{projectName}/components/{componentName}/builds`;

    const uri = parser.parse(uriTemplate).expand({
      orgName: request.orgName,
      projectName: request.projectName,
      componentName: request.componentName,
    });

    return await this.fetchApi.fetch(`${this.baseUrl}${uri}`, {
      headers: {
        'Content-Type': 'application/json',
        ...(options?.token && { Authorization: `Bearer ${options?.token}` }),
      },
      method: 'GET',
    });
  }

  /**
   * Triggers a new build for a component
   * Trigger a build for a component
   */
  public async buildsPost(
    request: BuildsTriggerRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<OpenChoreoApiSingleResponse<ModelsBuild>>> {
    const uriTemplate = `/orgs/{orgName}/projects/{projectName}/components/{componentName}/builds`;

    let uri = parser.parse(uriTemplate).expand({
      orgName: request.orgName,
      projectName: request.projectName,
      componentName: request.componentName,
    });

    if (request.commit) {
      uri += `?commit=${encodeURIComponent(request.commit)}`;
    }

    return await this.fetchApi.fetch(`${this.baseUrl}${uri}`, {
      headers: {
        'Content-Type': 'application/json',
        ...(options?.token && { Authorization: `Bearer ${options?.token}` }),
      },
      method: 'POST',
    });
  }

  /**
   * Creates a new component in the specified project
   * Create a new component
   */
  public async componentsPost(
    request: ComponentsPostRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<OpenChoreoApiSingleResponse<ModelsComponent>>> {
    const uriTemplate = `/orgs/{orgName}/projects/{projectName}/components`;

    const uri = parser.parse(uriTemplate).expand({
      orgName: request.orgName,
      projectName: request.projectName,
    });

    const body = {
      name: request.name,
      type: request.type,
      ...(request.displayName && { displayName: request.displayName }),
      ...(request.description && { description: request.description }),
      ...(request.buildConfig && { buildConfig: request.buildConfig }),
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
