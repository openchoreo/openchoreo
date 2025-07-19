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
        { token: this.token }
      );

      // Crete a dummy OpenChoreoApiResponse to match the expected structure
      const apiResponse: OpenChoreoApiResponse<ModelsBuildTemplate> = {
        success: true,
        data: {
          items: [ 
            {
              name: 'default-buildpack-template',
              displayName: 'Default Buildpack Template',
              description: 'A default build template for Buildpack projects',
              version: '1.0.0',
              stack: 'buildpack',
            },
            {
              name: 'default-docker-template',
              displayName: 'Docker Build Template',
              description: 'A build template for Docker projects',
              version: '1.0.0',
              stack: 'docker',
            },
          ],
          totalCount: 0, // Assuming no pagination for simplicity
          page: 1,
          pageSize: 100, // Default page size
        },
      };

      


      // const apiResponse: OpenChoreoApiResponse<ModelsBuildTemplate> = await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);
      
      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      const buildTemplates = apiResponse.data.items;
      this.logger?.info(`Successfully fetched ${buildTemplates.length} build templates for org: ${orgName} (total: ${apiResponse.data.totalCount})`);
      
      return buildTemplates;
    } catch (error) {
      this.logger?.error(`Failed to fetch build templates for org ${orgName}: ${error}`);
      throw error;
    }
  }

  async getAllBuilds(orgName: string, projectName: string, componentName: string): Promise<ModelsBuild[]> {
    this.logger?.info(`Fetching builds for component: ${componentName} in project: ${projectName}, organization: ${orgName}`);
    
    try {
      const response = await this.client.buildsGet(
        { orgName, projectName, componentName },
        { token: this.token }
      );

      const apiResponse: OpenChoreoApiResponse<ModelsBuild> = await response.json();
      this.logger?.info(`API response: ${JSON.stringify(apiResponse)}`);
      
      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      if (!apiResponse.data.items) {
        this.logger?.info(`No builds found for component: ${componentName}`);
        return [];
      }

      const builds = apiResponse.data.items;
      this.logger?.info(`Successfully fetched ${builds.length} builds for component: ${componentName} (total: ${apiResponse.data.totalCount})`);
      
      return builds;
    } catch (error) {
      this.logger?.error(`Failed to fetch builds for component ${componentName}: ${error}`);
      throw error;
    }
  }

  async triggerBuild(orgName: string, projectName: string, componentName: string, commit?: string): Promise<ModelsBuild> {
    this.logger?.info(`Triggering build for component: ${componentName} in project: ${projectName}, organization: ${orgName}${commit ? ` with commit: ${commit}` : ''}`);
    
    try {
      const response = await this.client.buildsPost(
        { orgName, projectName, componentName, commit },
        { token: this.token }
      );

      const apiResponse: OpenChoreoApiSingleResponse<ModelsBuild> = await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);
      
      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      if (!apiResponse.data) {
        throw new Error('No build data returned');
      }

      this.logger?.info(`Successfully triggered build for component: ${componentName}, build name: ${apiResponse.data.name}`);
      
      return apiResponse.data;
    } catch (error) {
      this.logger?.error(`Failed to trigger build for component ${componentName}: ${error}`);
      throw error;
    }
  }
}
