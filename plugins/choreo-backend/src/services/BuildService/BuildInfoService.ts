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

  async triggerBuild(orgName: string, projectName: string, componentName: string, commit?: string): Promise<ModelsBuild> {
    this.logger.info(`Triggering build for component: ${componentName} in project: ${projectName}, organization: ${orgName}${commit ? ` with commit: ${commit}` : ''}`);
    
    try {
      const client = new OpenChoreoApiClient(this.baseUrl, '', this.logger);
      const build = await client.triggerBuild(orgName, projectName, componentName, commit);
      
      this.logger.info(`Successfully triggered build for component: ${componentName}, build name: ${build.name}`);
      return build;
    } catch (error) {
      this.logger.error(`Failed to trigger build for component ${componentName}: ${error}`);
      throw error;
    }
  }
}
