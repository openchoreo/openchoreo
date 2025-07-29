import { LoggerService } from '@backstage/backend-plugin-api';
import {
  OpenChoreoApiClient,
  ModelsBuild,
  ObservabilityApiClient,
  DefaultApiClient,
  ComponentBuildLogsPostRequest,
  RuntimeLogsResponse,
  ObservabilityNotConfiguredError,
} from '@internal/plugin-openchoreo-api';

export class BuildInfoService {
  private logger: LoggerService;
  private observabilityApiClient: ObservabilityApiClient;
  private baseUrl: string;

  constructor(logger: LoggerService, baseUrl: string) {
    this.logger = logger;
    this.baseUrl = baseUrl;
    const defaultApiClient = new DefaultApiClient(baseUrl, {});
    this.observabilityApiClient = new ObservabilityApiClient(
      defaultApiClient,
      {},
    );
  }

  async fetchBuilds(
    orgName: string,
    projectName: string,
    componentName: string,
  ): Promise<ModelsBuild[]> {
    this.logger.info(
      `Fetching builds for component: ${componentName} in project: ${projectName}, organization: ${orgName}`,
    );

    try {
      const client = new OpenChoreoApiClient(this.baseUrl, '', this.logger);
      const builds = await client.getAllBuilds(
        orgName,
        projectName,
        componentName,
      );

      this.logger.info(
        `Successfully fetched ${builds.length} builds for component: ${componentName}`,
      );
      return builds;
    } catch (error) {
      this.logger.error(
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
    this.logger.info(
      `Triggering build for component: ${componentName} in project: ${projectName}, organization: ${orgName}${
        commit ? ` with commit: ${commit}` : ''
      }`,
    );

    try {
      const client = new OpenChoreoApiClient(this.baseUrl, '', this.logger);
      const build = await client.triggerBuild(
        orgName,
        projectName,
        componentName,
        commit,
      );

      this.logger.info(
        `Successfully triggered build for component: ${componentName}, build name: ${build.name}`,
      );
      return build;
    } catch (error) {
      this.logger.error(
        `Failed to trigger build for component ${componentName}: ${error}`,
      );
      throw error;
    }
  }

  async fetchBuildLogs(
    orgName: string,
    projectName: string,
    componentName: string,
    buildId: string,
    buildUuid: string,
    searchPhrase?: string,
    limit?: number,
    sortOrder?: 'asc' | 'desc',
  ): Promise<RuntimeLogsResponse> {
    this.logger.info(
      `Fetching build logs for component: ${componentName}, build: ${buildId}`,
    );

    try {
      const apiRequest: ComponentBuildLogsPostRequest = {
        orgName,
        projectName,
        componentName,
        buildId,
        buildUuid,
        ...(searchPhrase && { searchPhrase }),
        ...(limit && { limit }),
        ...(sortOrder && { sortOrder }),
      };

      this.logger.info(
        `Sending build logs request for component ${componentName} with parameters: ${JSON.stringify(
          apiRequest,
        )}`,
      );

      const response = await this.observabilityApiClient.componentBuildLogsPost(
        apiRequest,
      );

      if (!response.ok) {
        const errorText = await response.text();
        this.logger.error(
          `Failed to fetch build logs for component ${componentName}: ${response.status} ${response.statusText}`,
          { error: errorText },
        );
        throw new Error(
          `Failed to fetch build logs: ${response.status} ${response.statusText}`,
        );
      }

      const logsData = await response.json();

      this.logger.info(
        `Successfully fetched ${
          logsData.logs?.length || 0
        } build logs for component ${componentName}`,
      );

      return {
        logs: logsData.logs || [],
        totalCount: logsData.totalCount || 0,
        tookMs: logsData.tookMs || 0,
      };
    } catch (error: unknown) {
      if (error instanceof ObservabilityNotConfiguredError) {
        this.logger.info(
          `Observability not configured for component ${componentName}`,
        );
        throw error;
      }
      
      this.logger.error(
        `Error fetching build logs for component ${componentName}:`,
        error as Error,
      );
      throw error;
    }
  }
}
