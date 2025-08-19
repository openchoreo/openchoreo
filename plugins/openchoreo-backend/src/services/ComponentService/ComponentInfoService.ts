import { LoggerService } from '@backstage/backend-plugin-api';
import {
  OpenChoreoApiClient,
  ModelsCompleteComponent,
} from '@openchoreo/backstage-plugin-api';

export class ComponentInfoService {
  private logger: LoggerService;
  private baseUrl: string;

  constructor(logger: LoggerService, baseUrl: string) {
    this.logger = logger;
    this.baseUrl = baseUrl;
  }

  async fetchComponentDetails(
    orgName: string,
    projectName: string,
    componentName: string,
  ): Promise<ModelsCompleteComponent> {
    this.logger.info(
      `Fetching component details for: ${componentName} in project: ${projectName}, organization: ${orgName}`,
    );

    try {
      const client = new OpenChoreoApiClient(this.baseUrl, '', this.logger);
      const component = await client.getComponent(
        orgName,
        projectName,
        componentName,
      );

      this.logger.info(
        `Successfully fetched component details for: ${componentName}`,
      );
      return component;
    } catch (error) {
      this.logger.error(
        `Failed to fetch component details for ${componentName}: ${error}`,
      );
      throw error;
    }
  }
}
