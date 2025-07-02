/**
 * This is a copy of the DiscoveryApi, to avoid importing core-plugin-api.
 */
export type DiscoveryApi = {
  getBaseUrl(pluginId: string): Promise<string>;
};
