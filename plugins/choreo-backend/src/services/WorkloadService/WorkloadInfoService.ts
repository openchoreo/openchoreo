import { LoggerService } from '@backstage/backend-plugin-api';
import { WorkloadService } from '../../types';
import {
  OpenChoreoApiClient,
  ModelsWorkload,
  DefaultApiClient,
} from '@internal/plugin-openchoreo-api';

/**
 * Service for managing and retrieving workload information.
 * This service handles fetching and applying workload configurations.
 */
export class WorkloadInfoService implements WorkloadService {
  private readonly logger: LoggerService;
  private readonly client: OpenChoreoApiClient;
  private readonly defaultClient: DefaultApiClient;

  public constructor(logger: LoggerService, baseUrl: string, token?: string) {
    this.logger = logger;
    this.client = new OpenChoreoApiClient(baseUrl, token, logger);
    this.defaultClient = new DefaultApiClient(baseUrl, {});
  }

  static create(
    logger: LoggerService,
    baseUrl: string,
    token?: string,
  ): WorkloadInfoService {
    return new WorkloadInfoService(logger, baseUrl, token);
  }

  /**
   * Fetches workload information for a specific component in a project.
   * First tries the dedicated workload endpoint, falls back to component endpoint if needed.
   *
   * @param {Object} request - The request parameters
   * @param {string} request.projectName - Name of the project containing the component
   * @param {string} request.componentName - Name of the component to fetch workload info for
   * @param {string} request.organizationName - Name of the organization owning the project
   * @returns {Promise<ModelsWorkload>} The workload configuration
   * @throws {Error} When there's an error fetching data from the API
   */
  async fetchWorkloadInfo(request: {
    projectName: string;
    componentName: string;
    organizationName: string;
  }): Promise<ModelsWorkload> {
    const { projectName, componentName, organizationName } = request;

    try {
      this.logger.info(
        `Fetching workload info for component: ${componentName} in project: ${projectName}, org: ${organizationName}`,
      );
        const workload = await this.client.getWorkload(
          organizationName,
          projectName,
          componentName,
        );
        return workload;
    } catch (error) {
      this.logger.error(`Failed to fetch workload info: ${error}`);
      throw new Error('Failed to fetch workload info', { cause: error });
    }
  }

  /**
   * Applies workload configuration for a specific component in a project.
   *
   * @param {Object} request - The request parameters
   * @param {string} request.projectName - Name of the project containing the component
   * @param {string} request.componentName - Name of the component to apply workload for
   * @param {string} request.organizationName - Name of the organization owning the project
   * @param {ModelsWorkload} request.workloadSpec - The workload specification to apply
   * @returns {Promise<any>} The result of the apply operation
   * @throws {Error} When there's an error applying the workload
   */
  async applyWorkload(request: {
    projectName: string;
    componentName: string;
    organizationName: string;
    workloadSpec: ModelsWorkload;
  }): Promise<any> {
    const { projectName, componentName, organizationName, workloadSpec } = request;

    try {
      this.logger.info(
        `Applying workload for component: ${componentName} in project: ${projectName}, org: ${organizationName}`,
      );

      // Use the new workload client method
      const result = await this.client.updateWorkload(
        organizationName,
        projectName,
        componentName,
        workloadSpec,
      );

      return result;
    } catch (error) {
      this.logger.error(`Failed to apply workload: ${error}`);
      throw error;
    }
  }
}
