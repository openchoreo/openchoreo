import { LoggerService } from '@backstage/backend-plugin-api';
import { RuntimeLogsService, RuntimeLogsResponse } from '../../types';
import { DefaultApiClient } from '@internal/plugin-openchoreo-api';

/**
 * Service for fetching runtime logs for components.
 * This service handles fetching runtime logs from the OpenChoreo API.
 */
export class RuntimeLogsInfoService implements RuntimeLogsService {
  private readonly logger: LoggerService;
  private readonly baseUrl: string;

  public constructor(logger: LoggerService, baseUrl: string) {
    this.logger = logger;
    this.baseUrl = baseUrl;
  }

  static create(
    logger: LoggerService,
    baseUrl: string,
  ): RuntimeLogsInfoService {
    return new RuntimeLogsInfoService(logger, baseUrl);
  }

  /**
   * Fetches runtime logs for a specific component.
   * This method retrieves logs based on the provided filters including log levels,
   * time range, and pagination parameters.
   *
   * @param {Object} request - The request parameters
   * @param {string} request.componentId - ID of the component to fetch logs for
   * @param {string} request.environmentId - Environment ID to filter logs
   * @param {string[]} request.logLevels - Optional array of log levels to filter by
   * @param {string} request.startTime - Optional start time for log range
   * @param {string} request.endTime - Optional end time for log range
   * @param {number} request.limit - Optional limit for number of logs (default 50)
   * @param {number} request.offset - Optional offset for pagination (default 0)
   * @returns {Promise<RuntimeLogsResponse>} Response containing logs array, total count, and timing
   * @throws {Error} When there's an error fetching data from the API
   */
  async fetchRuntimeLogs(request: {
    componentId: string;
    environmentId: string;
    logLevels?: string[];
    startTime?: string;
    endTime?: string;
    limit?: number;
    offset?: number;
  }): Promise<RuntimeLogsResponse> {
    try {
      const {
        componentId,
        environmentId,
        logLevels,
        startTime,
        endTime,
        limit = 50,
        offset = 0,
      } = request;

      this.logger.info(
        `Fetching runtime logs for component ${componentId} in environment ${environmentId}`,
      );

      // Prepare the request body
      const requestBody = {
        environmentId,
        ...(logLevels && { logLevels }),
        ...(startTime && { startTime }),
        ...(endTime && { endTime }),
        limit,
        offset,
      };

      // Log the outgoing request for debugging
      this.logger.info(
        `Sending request to ${
          this.baseUrl
        }/api/logs/component/${componentId} with body: ${JSON.stringify(
          requestBody,
        )}`,
      );

      // Make the API call to fetch logs
      const response = await fetch(
        `${this.baseUrl}/api/logs/component/${componentId}`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify(requestBody),
        },
      );

      if (!response.ok) {
        const errorText = await response.text();
        this.logger.error(
          `Failed to fetch runtime logs for component ${componentId}: ${response.status} ${response.statusText}`,
          { error: errorText },
        );
        throw new Error(
          `Failed to fetch runtime logs: ${response.status} ${response.statusText}`,
        );
      }

      const logsData = await response.json();

      this.logger.info(
        `Successfully fetched ${
          logsData.logs?.length || 0
        } runtime logs for component ${componentId}`,
      );

      return {
        logs: logsData.logs || [],
        totalCount: logsData.totalCount || 0,
        tookMs: logsData.tookMs || 0,
      };
    } catch (error: unknown) {
      this.logger.error(
        `Error fetching runtime logs for component ${request.componentId}:`,
        error as Error,
      );
      throw error;
    }
  }
}
