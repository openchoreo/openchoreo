import { LoggerService } from '@backstage/backend-plugin-api';
import { EnvironmentService, Environment } from '../../types';
import {
  DefaultApiClient,
  ModelsEnvironment,
} from '@internal/plugin-openchoreo-api';

/**
 * Service for managing and retrieving environment-related information for deployments.
 * This service handles fetching environment details from the OpenChoreo API.
 */
export class EnvironmentInfoService implements EnvironmentService {
  private readonly logger: LoggerService;
  private readonly client: DefaultApiClient;

  public constructor(logger: LoggerService, baseUrl: string) {
    this.logger = logger;
    this.client = new DefaultApiClient(baseUrl, {});
  }

  static create(
    logger: LoggerService,
    baseUrl: string,
  ): EnvironmentInfoService {
    return new EnvironmentInfoService(logger, baseUrl);
  }

  /**
   * Fetches deployment information for a specific component in a project.
   * This method retrieves detailed information about deployments across different environments,
   * including their status, last deployment time, and endpoint details.
   *
   * @param {Object} request - The request parameters
   * @param {string} request.projectName - Name of the project containing the component
   * @param {string} request.componentName - Name of the component to fetch deployment info for
   * @param {string} request.organizationName - Name of the organization owning the project
   * @returns {Promise<Environment[]>} Array of environments with their deployment information
   *                                  Each environment includes:
   *                                  - name: The display name or identifier of the environment
   *                                  - deployment: Object containing deployment status and timestamp
   *                                  - endpoint: Object containing URL and active status
   * @throws {Error} When there's an error fetching data from the API
   */
  async fetchDeploymentInfo(request: {
    projectName: string;
    componentName: string;
    organizationName: string;
  }): Promise<Environment[]> {
    try {
      const response = await this.client.environmentsGet({
        orgName: request.organizationName,
      });

      if (!response.ok) {
        this.logger.error(
          `Failed to fetch environments for organization ${request.organizationName}`,
        );
        return [];
      }

      const environmentsData = await response.json();

      if (!environmentsData.success || !environmentsData.data?.items) {
        this.logger.warn('No environments found in API response');
        return [];
      }

      return this.transformEnvironmentData(environmentsData.data.items);
    } catch (error: unknown) {
      this.logger.error(
        `Error fetching deployment info for ${request.projectName}:`,
        error as Error,
      );
      return [];
    }
  }

  private transformEnvironmentData(
    environmentData: ModelsEnvironment[],
  ): Environment[] {
    return environmentData.map(env => ({
      name: env.displayName || env.name,
      deployment: {
        status: env.status === 'Ready' ? 'success' : 'failed',
        lastDeployed: env.createdAt,
      },
      endpoint: {
        url: `https://${env.dnsPrefix}.example.com`,
        status: env.status === 'Ready' ? 'active' : 'inactive',
      },
    }));
  }
}
