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
  BindingsGetRequest,
  BindingPatchRequest,
  TypedResponse,
  OpenChoreoApiResponse,
  OpenChoreoApiSingleResponse,
  ComponentGetRequest,
  ModelsCompleteComponent,
  BindingResponse,
  ProjectDeploymentPipelineGetRequest,
  DeploymentPipelineResponse,
  ComponentPromotePostRequest,
  WorkloadGetRequest,
  WorkloadPostRequest,
  ModelsWorkload,
  RuntimeLogsObserverUrlGetRequest,
  BuildObserverUrlGetRequest,
  ObserverUrlData,
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
  ): Promise<
    TypedResponse<OpenChoreoApiSingleResponse<ModelsCompleteComponent>>
  > {
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

  /**
   * Retrieves all bindings for a component
   * List all bindings of a component
   */
  public async bindingsGet(
    request: BindingsGetRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<OpenChoreoApiResponse<BindingResponse>>> {
    const uriTemplate = `/orgs/{orgName}/projects/{projectName}/components/{componentName}/bindings`;

    let uri = parser.parse(uriTemplate).expand({
      orgName: request.orgName,
      projectName: request.projectName,
      componentName: request.componentName,
    });

    // Add environment query parameters if provided
    if (request.environment && request.environment.length > 0) {
      const envParams = request.environment
        .map(env => `environment=${encodeURIComponent(env)}`)
        .join('&');
      uri += `?${envParams}`;
    }

    return await this.fetchApi.fetch(`${this.baseUrl}${uri}`, {
      headers: {
        'Content-Type': 'application/json',
        ...(options?.token && { Authorization: `Bearer ${options?.token}` }),
      },
      method: 'GET',
    });
  }

  /**
   * Retrieves the deployment pipeline for a project
   * Get project deployment pipeline
   */
  public async projectDeploymentPipelineGet(
    request: ProjectDeploymentPipelineGetRequest,
    options?: RequestOptions,
  ): Promise<
    TypedResponse<OpenChoreoApiSingleResponse<DeploymentPipelineResponse>>
  > {
    const uriTemplate = `/orgs/{orgName}/projects/{projectName}/deployment-pipeline`;

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
   * Promotes a component from source environment to target environment
   * Returns the list of BindingResponse for all environments after promotion
   */
  public async componentPromotePost(
    request: ComponentPromotePostRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<OpenChoreoApiResponse<BindingResponse>>> {
    const uriTemplate = `/orgs/{orgName}/projects/{projectName}/components/{componentName}/promote`;

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
      method: 'POST',
      body: JSON.stringify(request.promoteComponentRequest),
    });
  }

  /**
   * Retrieves workload information for a component
   * Get workload configuration
   */
  public async workloadGet(
    request: WorkloadGetRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<OpenChoreoApiSingleResponse<ModelsWorkload>>> {
    const uriTemplate = `/orgs/{orgName}/projects/{projectName}/components/{componentName}/workloads`;

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
   * Updates workload configuration for a component
   * Update workload configuration
   */
  public async workloadPost(
    request: WorkloadPostRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<OpenChoreoApiSingleResponse<ModelsWorkload>>> {
    const uriTemplate = `/orgs/{orgName}/projects/{projectName}/components/{componentName}/workloads`;

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
      method: 'POST',
      body: JSON.stringify(request.workloadSpec),
    });
  }

  /**
   * Retrieves the observer URL for a component in a specific environment
   * Get observer URL for component environment
   */
  public async runtimeLogsObserverUrlGet(
    request: RuntimeLogsObserverUrlGetRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<OpenChoreoApiSingleResponse<ObserverUrlData>>> {
    const uriTemplate = `/orgs/{orgName}/projects/{projectName}/components/{componentName}/environments/{environmentName}/observer-url`;

    const uri = parser.parse(uriTemplate).expand({
      orgName: request.orgName,
      projectName: request.projectName,
      componentName: request.componentName,
      environmentName: request.environmentName,
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
   * Retrieves the build observer URL for a component
   * Get build observer URL for component
   */
  public async buildObserverUrlGet(
    request: BuildObserverUrlGetRequest,
  ): Promise<TypedResponse<OpenChoreoApiSingleResponse<ObserverUrlData>>> {
    const uriTemplate = `/orgs/{orgName}/projects/{projectName}/components/{componentName}/observer-url`;

    const uri = parser.parse(uriTemplate).expand({
      orgName: request.orgName,
      projectName: request.projectName,
      componentName: request.componentName,
    });

    return await this.fetchApi.fetch(`${this.baseUrl}${uri}`, {
      headers: {
        'Content-Type': 'application/json',
      },
      method: 'GET',
    });
  }

  /**
   * Update a component binding's release state
   * Update binding release state
   */
  public async bindingPatch(
    request: BindingPatchRequest,
    options?: RequestOptions,
  ): Promise<TypedResponse<OpenChoreoApiSingleResponse<BindingResponse>>> {
    const uriTemplate = `/orgs/{orgName}/projects/{projectName}/components/{componentName}/bindings/{bindingName}`;

    const uri = parser.parse(uriTemplate).expand({
      orgName: request.orgName,
      projectName: request.projectName,
      componentName: request.componentName,
      bindingName: request.bindingName,
    });

    return await this.fetchApi.fetch(`${this.baseUrl}${uri}`, {
      headers: {
        'Content-Type': 'application/json',
        ...(options?.token && { Authorization: `Bearer ${options?.token}` }),
      },
      method: 'PATCH',
      body: JSON.stringify(request.updateBindingRequest),
    });
  }
}
