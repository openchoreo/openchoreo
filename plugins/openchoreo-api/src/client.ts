

import { DefaultApiClient } from './api';
import { ModelsProject, ModelsOrganization, ModelsComponent, OpenChoreoApiResponse } from './models';
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
        { token: this.token }
      );

      const apiResponse: OpenChoreoApiResponse<ModelsProject> = await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);
      
      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      const projects = apiResponse.data.items;
      this.logger?.info(`Successfully fetched ${projects.length} projects for org: ${orgName} (total: ${apiResponse.data.totalCount})`);
      
      return projects;
    } catch (error) {
      this.logger?.error(`Failed to fetch projects for org ${orgName}: ${error}`);
      throw error;
    }
  }

  async getAllOrganizations(): Promise<ModelsOrganization[]> {
    this.logger?.info('Fetching all organizations');
    
    try {
      const response = await this.client.organizationsGet(
        {},
        { token: this.token }
      );

      const apiResponse: OpenChoreoApiResponse<ModelsOrganization> = await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);
      
      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      const organizations = apiResponse.data.items;
      this.logger?.info(`Successfully fetched ${organizations.length} organizations (total: ${apiResponse.data.totalCount})`);
      
      return organizations;
    } catch (error) {
      this.logger?.error(`Failed to fetch organizations: ${error}`);
      throw error;
    }
  }

  async getAllComponents(orgName: string, projectName: string): Promise<ModelsComponent[]> {
    this.logger?.info(`Fetching components for project: ${projectName} in organization: ${orgName}`);
    
    try {
      const response = await this.client.componentsGet(
        { orgName, projectName },
        { token: this.token }
      );

      const apiResponse: OpenChoreoApiResponse<ModelsComponent> = await response.json();
      this.logger?.debug(`API response: ${JSON.stringify(apiResponse)}`);
      
      if (!apiResponse.success) {
        throw new Error('API request was not successful');
      }

      const components = apiResponse.data.items;
      this.logger?.info(`Successfully fetched ${components.length} components for project: ${projectName} in org: ${orgName} (total: ${apiResponse.data.totalCount})`);
      
      return components;
    } catch (error) {
      this.logger?.error(`Failed to fetch components for project ${projectName} in org ${orgName}: ${error}`);
      throw error;
    }
  }
}
