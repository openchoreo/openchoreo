

import { DefaultApiClient } from './api';
import { ModelsProject, OpenChoreoApiResponse } from './models';
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
}