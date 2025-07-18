import { LoggerService } from '@backstage/backend-plugin-api';
import { OpenChoreoApiClient, ModelsBuild } from '@internal/plugin-openchoreo-api';

export class BuildInfoService {
  private logger: LoggerService;
  private baseUrl: string;

  constructor(logger: LoggerService, baseUrl: string) {
    this.logger = logger;
    this.baseUrl = baseUrl;
  }

  async fetchBuilds(orgName: string, projectName: string, componentName: string): Promise<ModelsBuild[]> {
    this.logger.info(`Fetching builds for component: ${componentName} in project: ${projectName}, organization: ${orgName}`);
    
    try {
      const client = new OpenChoreoApiClient(this.baseUrl, '', this.logger);
      const builds = await client.getAllBuilds(orgName, projectName, componentName);
      
      this.logger.info(`Successfully fetched ${builds.length} builds for component: ${componentName}`);
      return builds;
    } catch (error) {
      this.logger.error(`Failed to fetch builds for component ${componentName}: ${error}`);
      throw error;
    }
  }
}