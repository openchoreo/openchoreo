export interface Config {
  openchoreo?: {
    /**
     * Base URL for the OpenChoreo API
     */
    baseUrl: string;
    /**
     * Optional authentication token for OpenChoreo API
     */
    token?: string;
  };
}
