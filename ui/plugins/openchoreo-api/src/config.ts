import { Config } from '@backstage/config';

/**
 * OpenChoreo configuration schema
 * @public
 */
export interface OpenChoreoConfig {
  baseUrl: string;
  token?: string;
}

/**
 * Read OpenChoreo configuration from Backstage config
 * @public
 */
export function readOpenChoreoConfigFromConfig(
  config: Config,
): OpenChoreoConfig {
  const openChoreoConfig = config.getConfig('openchoreo');

  return {
    baseUrl: openChoreoConfig.getString('baseUrl'),
    token: openChoreoConfig.getOptionalString('token'),
  };
}
