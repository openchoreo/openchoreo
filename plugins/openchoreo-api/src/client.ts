import { DefaultApiClient } from './api';
import {
  ModelsProject,
  ModelsOrganization,
  ModelsComponent,
  ModelsBuildTemplate,
  ModelsBuild,
  OpenChoreoApiResponse,
  OpenChoreoApiSingleResponse,
  BuildConfig,
  BindingResponse,
  DeploymentPipelineResponse,
  ModelsCompleteComponent,
  ModelsWorkload,
} from './models';
import { LoggerService } from '@backstage/backend-plugin-api';

export class OpenChoreoApiClient {
  private client: DefaultApiClient;
  private token?: string;
  private logger?: LoggerService;

  constructor(baseUrl: string, token?: string, logger?: LoggerService) {
    this.token = token;
    this.logger = logger;
    this.client = new DefaultApiClient(baseUrl, {});
  }

  async getAllProjects(orgName: string): Promise<ModelsProject[]> {
    this.logger?.info(`Fetching projects for organization: ${orgName}`);

    try {
      const response = await this.client.projectsGet(
        { orgName },
        { token: this.token },
      );

      const apiResponse: OpenChoreoApiResponse<ModelsProject> =
        await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);

      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      const projects = apiResponse.data.items;
      this.logger?.info(
        `Successfully fetched ${projects.length} projects for org: ${orgName} (total: ${apiResponse.data.totalCount})`,
      );

      return projects;
    } catch (error) {
      this.logger?.error(
        `Failed to fetch projects for org ${orgName}: ${error}`,
      );
      throw error;
    }
  }

  async getAllOrganizations(): Promise<ModelsOrganization[]> {
    this.logger?.info('Fetching all organizations');

    try {
      const response = await this.client.organizationsGet(
        {},
        { token: this.token },
      );

      const apiResponse: OpenChoreoApiResponse<ModelsOrganization> =
        await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);

      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      const organizations = apiResponse.data.items;
      this.logger?.info(
        `Successfully fetched ${organizations.length} organizations (total: ${apiResponse.data.totalCount})`,
      );

      return organizations;
    } catch (error) {
      this.logger?.error(`Failed to fetch organizations: ${error}`);
      throw error;
    }
  }

  async getAllComponents(
    orgName: string,
    projectName: string,
  ): Promise<ModelsComponent[]> {
    this.logger?.info(
      `Fetching components for project: ${projectName} in organization: ${orgName}`,
    );

    try {
      const response = await this.client.componentsGet(
        { orgName, projectName },
        { token: this.token },
      );

      const apiResponse: OpenChoreoApiResponse<ModelsComponent> =
        await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);

      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      const components = apiResponse.data.items;
      this.logger?.info(
        `Successfully fetched ${components.length} components for project: ${projectName} in org: ${orgName} (total: ${apiResponse.data.totalCount})`,
      );

      return components;
    } catch (error) {
      this.logger?.error(
        `Failed to fetch components for project ${projectName} in org ${orgName}: ${error}`,
      );
      throw error;
    }
  }

  async createProject(
    orgName: string,
    projectData: {
      name: string;
      displayName?: string;
      description?: string;
      deploymentPipeline?: string;
    },
  ): Promise<ModelsProject> {
    this.logger?.info(
      `Creating project: ${projectData.name} in organization: ${orgName}`,
    );

    try {
      const response = await this.client.projectsPost(
        {
          orgName,
          name: projectData.name,
          displayName: projectData.displayName,
          description: projectData.description,
          deploymentPipeline: projectData.deploymentPipeline,
        },
        { token: this.token },
      );

      const apiResponse: OpenChoreoApiSingleResponse<ModelsProject> =
        await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);

      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      const project = apiResponse.data;
      this.logger?.info(
        `Successfully created project: ${project.name} in org: ${orgName}`,
      );

      return project;
    } catch (error) {
      this.logger?.error(
        `Failed to create project ${projectData.name} in org ${orgName}: ${error}`,
      );
      throw error;
    }
  }

  async createComponent(
    orgName: string,
    projectName: string,
    componentData: {
      name: string;
      displayName?: string;
      description?: string;
      type: string;
      buildConfig?: BuildConfig;
    },
  ): Promise<ModelsComponent> {
    this.logger?.info(
      `Creating component: ${componentData.name} in project: ${projectName}, organization: ${orgName}`,
    );

    try {
      const response = await this.client.componentsPost(
        {
          orgName,
          projectName,
          name: componentData.name,
          displayName: componentData.displayName,
          description: componentData.description,
          type: componentData.type,
          buildConfig: componentData.buildConfig,
        },
        { token: this.token },
      );

      const apiResponse: OpenChoreoApiSingleResponse<ModelsComponent> =
        await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);

      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      const component = apiResponse.data;
      this.logger?.info(
        `Successfully created component: ${component.name} in project: ${projectName}, org: ${orgName}`,
      );

      return component;
    } catch (error) {
      this.logger?.error(
        `Failed to create component ${componentData.name} in project ${projectName}, org ${orgName}: ${error}`,
      );
      throw error;
    }
  }

  async getAllBuildTemplates(orgName: string): Promise<ModelsBuildTemplate[]> {
    this.logger?.info(`Fetching build templates for organization: ${orgName}`);

    try {
      const response = await this.client.buildTemplatesGet(
        { orgName },
        { token: this.token },
      );

      const apiResponse: OpenChoreoApiResponse<ModelsBuildTemplate> =
        await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);

      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      const buildTemplates = apiResponse.data?.items || [];
      this.logger?.info(
        `Successfully fetched ${buildTemplates.length} build templates for org: ${orgName}`,
      );

      return buildTemplates;
    } catch (error) {
      this.logger?.error(
        `Failed to fetch build templates for org ${orgName}: ${error}`,
      );
      throw error;
    }
  }

  async getAllBuilds(
    orgName: string,
    projectName: string,
    componentName: string,
  ): Promise<ModelsBuild[]> {
    this.logger?.info(
      `Fetching builds for component: ${componentName} in project: ${projectName}, organization: ${orgName}`,
    );

    try {
      const response = await this.client.buildsGet(
        { orgName, projectName, componentName },
        { token: this.token },
      );

      const apiResponse: OpenChoreoApiResponse<ModelsBuild> =
        await response.json();
      this.logger?.info(`API response: ${JSON.stringify(apiResponse)}`);

      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      if (!apiResponse.data.items) {
        this.logger?.info(`No builds found for component: ${componentName}`);
        return [];
      }

      const builds = apiResponse.data.items;
      this.logger?.info(
        `Successfully fetched ${builds.length} builds for component: ${componentName} (total: ${apiResponse.data.totalCount})`,
      );

      return builds;
    } catch (error) {
      this.logger?.error(
        `Failed to fetch builds for component ${componentName}: ${error}`,
      );
      throw error;
    }
  }

  async triggerBuild(
    orgName: string,
    projectName: string,
    componentName: string,
    commit?: string,
  ): Promise<ModelsBuild> {
    this.logger?.info(
      `Triggering build for component: ${componentName} in project: ${projectName}, organization: ${orgName}${
        commit ? ` with commit: ${commit}` : ''
      }`,
    );

    try {
      const response = await this.client.buildsPost(
        { orgName, projectName, componentName, commit },
        { token: this.token },
      );

      const apiResponse: OpenChoreoApiSingleResponse<ModelsBuild> =
        await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);

      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      if (!apiResponse.data) {
        throw new Error('No build data returned');
      }

      this.logger?.info(
        `Successfully triggered build for component: ${componentName}, build name: ${apiResponse.data.name}`,
      );

      return apiResponse.data;
    } catch (error) {
      this.logger?.error(
        `Failed to trigger build for component ${componentName}: ${error}`,
      );
      throw error;
    }
  }

  async getComponent(
    orgName: string,
    projectName: string,
    componentName: string,
  ): Promise<ModelsCompleteComponent> {
    this.logger?.info(
      `Fetching component details for: ${componentName} in project: ${projectName}, organization: ${orgName}`,
    );

    try {
      const response = await this.client.componentGet({
        orgName,
        projectName,
        componentName,
      });

      const apiResponse = await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);

      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      const component = apiResponse.data;
      this.logger?.info(
        `Successfully fetched component details for: ${componentName}`,
      );
      return component;
    } catch (error) {
      this.logger?.error(
        `Failed to fetch component details for ${componentName}: ${error}`,
      );
      throw error;
    }
  }

  async getComponentBindings(
    orgName: string,
    projectName: string,
    componentName: string,
    environments?: string[],
  ): Promise<BindingResponse[]> {
    this.logger?.info(
      `Fetching bindings for component: ${componentName} in project: ${projectName}, organization: ${orgName}${
        environments?.length
          ? ` for environments: ${environments.join(', ')}`
          : ''
      }`,
    );

    try {
      const response = await this.client.bindingsGet(
        {
          orgName,
          projectName,
          componentName,
          environment: environments,
        },
        { token: this.token },
      );

      const apiResponse: OpenChoreoApiResponse<BindingResponse> =
        await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);

      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      // Extract bindings from paginated data structure
      const bindings = apiResponse.data?.items || [];

      this.logger?.info(
        `Successfully fetched ${bindings.length} bindings for component: ${componentName}`,
      );

      return bindings;
    } catch (error) {
      this.logger?.error(
        `Failed to fetch bindings for component ${componentName}: ${error}`,
      );
      throw error;
    }
  }

  async getProjectDeploymentPipeline(
    orgName: string,
    projectName: string,
  ): Promise<DeploymentPipelineResponse> {
    this.logger?.info(
      `Fetching deployment pipeline for project: ${projectName} in organization: ${orgName}`,
    );

    try {
      const response = await this.client.projectDeploymentPipelineGet(
        { orgName, projectName },
        { token: this.token },
      );

      const apiResponse: OpenChoreoApiSingleResponse<DeploymentPipelineResponse> =
        await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);

      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      const deploymentPipeline = apiResponse.data;
      this.logger?.info(
        `Successfully fetched deployment pipeline: ${deploymentPipeline.name} for project: ${projectName} in org: ${orgName}`,
      );

      return deploymentPipeline;
    } catch (error) {
      this.logger?.error(
        `Failed to fetch deployment pipeline for project ${projectName} in org ${orgName}: ${error}`,
      );
      throw error;
    }
  }

  async promoteComponent(
    orgName: string,
    projectName: string,
    componentName: string,
    sourceEnvironment: string,
    targetEnvironment: string,
  ): Promise<BindingResponse[]> {
    this.logger?.info(
      `Promoting component: ${componentName} from ${sourceEnvironment} to ${targetEnvironment} in project: ${projectName}, organization: ${orgName}`,
    );

    try {
      const response = await this.client.componentPromotePost(
        {
          orgName,
          projectName,
          componentName,
          promoteComponentRequest: {
            sourceEnv: sourceEnvironment,
            targetEnv: targetEnvironment,
          },
        },
        { token: this.token },
      );

      const apiResponse: OpenChoreoApiResponse<BindingResponse> =
        await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);

      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      // Extract bindings from paginated data structure
      const bindings = apiResponse.data?.items || [];

      this.logger?.info(
        `Successfully promoted component: ${componentName} from ${sourceEnvironment} to ${targetEnvironment}. Returned ${bindings.length} bindings.`,
      );

      return bindings;
    } catch (error) {
      this.logger?.error(
        `Failed to promote component ${componentName} from ${sourceEnvironment} to ${targetEnvironment}: ${error}`,
      );
      throw error;
    }
  }

  async getWorkload(
    orgName: string,
    projectName: string,
    componentName: string,
  ): Promise<ModelsWorkload> {
    this.logger?.info(
      `Fetching workload for component: ${componentName} in project: ${projectName}, organization: ${orgName}`,
    );

    try {
      const response = await this.client.workloadGet(
        {
          orgName,
          projectName,
          componentName,
        },
        { token: this.token },
      );

      const apiResponse: OpenChoreoApiSingleResponse<ModelsWorkload> =
        await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);

      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      if (!apiResponse.data) {
        throw new Error('No workload data returned');
      }

      this.logger?.info(
        `Successfully fetched workload for component: ${componentName}`,
      );
      return apiResponse.data;
    } catch (error) {
      this.logger?.error(
        `Failed to fetch workload for component ${componentName}: ${error}`,
      );
      throw error;
    }
  }

  async updateWorkload(
    orgName: string,
    projectName: string,
    componentName: string,
    workloadSpec: ModelsWorkload,
  ): Promise<ModelsWorkload> {
    this.logger?.info(
      `Updating workload for component: ${componentName} in project: ${projectName}, organization: ${orgName}`,
    );

    try {
      const response = await this.client.workloadPost(
        {
          orgName,
          projectName,
          componentName,
          workloadSpec,
        },
        { token: this.token },
      );

      const apiResponse: OpenChoreoApiSingleResponse<ModelsWorkload> =
        await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);

      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      if (!apiResponse.data) {
        throw new Error('No workload data returned');
      }

      this.logger?.info(
        `Successfully updated workload for component: ${componentName}`,
      );
      return apiResponse.data;
    } catch (error) {
      this.logger?.error(
        `Failed to update workload for component ${componentName}: ${error}`,
      );
      throw error;
    }
  }

  async updateComponentBinding(
    orgName: string,
    projectName: string,
    componentName: string,
    bindingName: string,
    releaseState: 'Active' | 'Suspend' | 'Undeploy',
  ): Promise<BindingResponse> {
    this.logger?.info(
      `Updating binding: ${bindingName} for component: ${componentName} in project: ${projectName}, organization: ${orgName} to state: ${releaseState}`,
    );

    try {
      const response = await this.client.bindingPatch(
        {
          orgName,
          projectName,
          componentName,
          bindingName,
          updateBindingRequest: {
            releaseState,
          },
        },
        { token: this.token },
      );

      const apiResponse: OpenChoreoApiSingleResponse<BindingResponse> =
        await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);

      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      const binding = apiResponse.data;
      this.logger?.info(
        `Successfully updated binding: ${bindingName} to state: ${releaseState}`,
      );

      return binding;
    } catch (error) {
      this.logger?.error(
        `Failed to update binding ${bindingName} for component ${componentName}: ${error}`,
      );
      throw error;
    }
  }
}
