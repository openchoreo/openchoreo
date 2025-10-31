export interface Environment {
  name: string;
  namespace: string;
  displayName: string;
  description: string;
  organization: string;
  dataPlaneRef: string;
  isProduction: boolean;
  dnsPrefix: string;
  createdAt: string;
  status: string;
  componentCount?: number;
}

export interface DataPlane {
  name: string;
  namespace?: string;
  displayName?: string;
  description?: string;
  organization: string;
  registryPrefix?: string;
  registrySecretRef?: string;
  kubernetesClusterName?: string;
  apiServerURL?: string;
  publicVirtualHost?: string;
  organizationVirtualHost?: string;
  observerURL?: string;
  observerUsername?: string;
  createdAt?: string;
  status?: string;
}

export interface DataPlaneWithEnvironments extends DataPlane {
  environments: Environment[];
}

export interface PlatformEnvironmentService {
  /**
   * Fetches all environments across all organizations
   */
  fetchAllEnvironments(): Promise<Environment[]>;

  /**
   * Fetches environments for a specific organization
   */
  fetchEnvironmentsByOrganization(
    organizationName: string,
  ): Promise<Environment[]>;

  /**
   * Fetches all dataplanes across all organizations
   */
  fetchAllDataplanes(): Promise<DataPlane[]>;

  /**
   * Fetches dataplanes for a specific organization
   */
  fetchDataplanesByOrganization(organizationName: string): Promise<DataPlane[]>;

  /**
   * Fetches all dataplanes with their associated environments
   */
  fetchDataplanesWithEnvironments(): Promise<DataPlaneWithEnvironments[]>;

  /**
   * Fetches all dataplanes with their associated environments and component counts
   */
  fetchDataplanesWithEnvironmentsAndComponentCounts(): Promise<
    DataPlaneWithEnvironments[]
  >;

  /**
   * Fetches component counts per environment using bindings API
   */
  fetchComponentCountsPerEnvironment(
    components: Array<{
      orgName: string;
      projectName: string;
      componentName: string;
    }>,
  ): Promise<Map<string, number>>;

  /**
   * Fetches count of distinct components that have at least one binding (deployment)
   */
  fetchDistinctDeployedComponentsCount(
    components: Array<{
      orgName: string;
      projectName: string;
      componentName: string;
    }>,
  ): Promise<number>;
}
