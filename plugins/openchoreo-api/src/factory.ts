import { Config } from '@backstage/config';
import { LoggerService } from '@backstage/backend-plugin-api';
import { OpenChoreoApiClient } from './client';
import { readOpenChoreoConfigFromConfig } from './config';

/**
 * Create an OpenChoreoApiClient from Backstage configuration
 * @public
 */
export function createOpenChoreoApiClient(
  config: Config,
  logger?: LoggerService,
): OpenChoreoApiClient {
  const openChoreoConfig = readOpenChoreoConfigFromConfig(config);

  return new OpenChoreoApiClient(
    openChoreoConfig.baseUrl,
    openChoreoConfig.token,
    logger,
  );
}

/**
 * Create an OpenChoreoApiClient with custom configuration
 * @public
 */
export function createOpenChoreoApiClientFromOptions(options: {
  baseUrl: string;
  token?: string;
  logger?: LoggerService;
}): OpenChoreoApiClient {
  return new OpenChoreoApiClient(
    options.baseUrl,
    options.token,
    options.logger,
  );
}
