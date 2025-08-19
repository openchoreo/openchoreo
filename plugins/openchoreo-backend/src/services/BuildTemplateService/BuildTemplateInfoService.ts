import { LoggerService } from '@backstage/backend-plugin-api';
import {
  OpenChoreoApiClient,
  ModelsBuildTemplate,
} from '@internal/plugin-openchoreo-api';

export class BuildTemplateInfoService {
  private logger: LoggerService;
  private baseUrl: string;

  constructor(logger: LoggerService, baseUrl: string) {
    this.logger = logger;
    this.baseUrl = baseUrl;
  }

  async fetchBuildTemplates(orgName: string): Promise<ModelsBuildTemplate[]> {
    this.logger.info(`Fetching build templates for organization: ${orgName}`);

    try {
      const client = new OpenChoreoApiClient(this.baseUrl, '', this.logger);
      const buildTemplates = await client.getAllBuildTemplates(orgName);

      this.logger.info(
        `Successfully fetched ${buildTemplates.length} build templates for org: ${orgName}`,
      );
      return buildTemplates;
    } catch (error) {
      this.logger.error(
        `Failed to fetch build templates for org ${orgName}: ${error}`,
      );
      throw error;
    }
  }
}
