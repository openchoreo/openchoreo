import { LoggerService } from '@backstage/backend-plugin-api';
import {
  DefaultApiClient,
  ModelsEnvironment,
  ModelsDataPlane,
  BindingResponse,
} from '@openchoreo/backstage-plugin-api';
import {
  PlatformEnvironmentService,
  Environment,
  DataPlane,
  DataPlaneWithEnvironments,
} from '../types';
// import { DefaultApiClient } from '../api';
// import { ModelsEnvironment, ModelsDataPlane, BindingResponse } from '../models';

/**
 * Service for managing platform-wide environment information.
 * This service provides a platform engineer's view of all environments across organizations.
 */
export class PlatformEnvironmentInfoService
  implements PlatformEnvironmentService
{
  private readonly logger: LoggerService;
  private readonly defaultClient: DefaultApiClient;

  public constructor(logger: LoggerService, baseUrl: string, _token?: string) {
    this.logger = logger;
    this.defaultClient = new DefaultApiClient(baseUrl, {});
  }

  static create(
    logger: LoggerService,
    baseUrl: string,
    token?: string,
  ): PlatformEnvironmentInfoService {
    return new PlatformEnvironmentInfoService(logger, baseUrl, token);
  }

  /**
   * Fetches all environments across all organizations.
   * This provides a platform-wide view for platform engineers.
   */
  async fetchAllEnvironments(): Promise<Environment[]> {
    const startTime = Date.now();
    try {
      this.logger.info('Starting platform-wide environment fetch');

      // For now, we'll fetch environments from a default organization
      // In a real implementation, you might need to fetch from multiple organizations
      // or have a platform-wide API endpoint
      const environmentsResponse = await this.defaultClient.environmentsGet({
        orgName: 'default', // This should be configurable or fetched from a platform API
      });
      console.log('environmentsResponse', environmentsResponse);

      if (!environmentsResponse.ok) {
        this.logger.error('Failed to fetch platform environments');
        return [];
      }

      const environmentsData = await environmentsResponse.json();
      if (!environmentsData.success || !environmentsData.data?.items) {
        this.logger.warn('No environments found in platform API response');
        return [];
      }

      const environments = environmentsData.data.items as ModelsEnvironment[];
      console.log('environments', environments);
      const result = this.transformEnvironmentData(environments, 'default');

      const totalTime = Date.now() - startTime;
      this.logger.info(
        `Platform environment fetch completed: ${result.length} environments found (${totalTime}ms)`,
      );

      return result;
    } catch (error: unknown) {
      const totalTime = Date.now() - startTime;
      this.logger.error(
        `Error fetching platform environments (${totalTime}ms):`,
        error as Error,
      );
      return [];
    }
  }

  /**
   * Fetches environments for a specific organization.
   */
  async fetchEnvironmentsByOrganization(
    organizationName: string,
  ): Promise<Environment[]> {
    const startTime = Date.now();
    try {
      this.logger.info(
        `Starting environment fetch for organization: ${organizationName}`,
      );

      const environmentsResponse = await this.defaultClient.environmentsGet({
        orgName: organizationName,
      });

      if (!environmentsResponse.ok) {
        this.logger.error(
          `Failed to fetch environments for organization ${organizationName}`,
        );
        return [];
      }

      const environmentsData = await environmentsResponse.json();
      if (!environmentsData.success || !environmentsData.data?.items) {
        this.logger.warn(
          `No environments found for organization ${organizationName}`,
        );
        return [];
      }

      const environments = environmentsData.data.items as ModelsEnvironment[];
      const result = this.transformEnvironmentData(
        environments,
        organizationName,
      );

      const totalTime = Date.now() - startTime;
      this.logger.info(
        `Environment fetch completed for ${organizationName}: ${result.length} environments found (${totalTime}ms)`,
      );

      return result;
    } catch (error: unknown) {
      const totalTime = Date.now() - startTime;
      this.logger.error(
        `Error fetching environments for organization ${organizationName} (${totalTime}ms):`,
        error as Error,
      );
      return [];
    }
  }

  /**
   * Fetches all dataplanes across all organizations.
   * This provides a platform-wide view for platform engineers.
   */
  async fetchAllDataplanes(): Promise<DataPlane[]> {
    const startTime = Date.now();
    try {
      this.logger.info('Starting platform-wide dataplane fetch');

      // For now, we'll fetch dataplanes from a default organization
      // In a real implementation, you might need to fetch from multiple organizations
      // or have a platform-wide API endpoint
      const dataplanesResponse = await this.defaultClient.dataplanesGet({
        orgName: 'default', // This should be configurable or fetched from a platform API
      });

      if (!dataplanesResponse.ok) {
        this.logger.error('Failed to fetch platform dataplanes');
        return [];
      }

      const dataplanesData = await dataplanesResponse.json();
      if (!dataplanesData.success || !dataplanesData.data?.items) {
        this.logger.warn('No dataplanes found in platform API response');
        return [];
      }

      const dataplanes = dataplanesData.data.items as ModelsDataPlane[];
      console.log('dataplanes', dataplanes);
      const result = this.transformDataPlaneData(dataplanes, 'default');

      const totalTime = Date.now() - startTime;
      this.logger.info(
        `Platform dataplane fetch completed: ${result.length} dataplanes found (${totalTime}ms)`,
      );

      return result;
    } catch (error: unknown) {
      const totalTime = Date.now() - startTime;
      this.logger.error(
        `Error fetching platform dataplanes (${totalTime}ms):`,
        error as Error,
      );
      return [];
    }
  }

  /**
   * Fetches dataplanes for a specific organization.
   */
  async fetchDataplanesByOrganization(
    organizationName: string,
  ): Promise<DataPlane[]> {
    const startTime = Date.now();
    try {
      this.logger.info(
        `Starting dataplane fetch for organization: ${organizationName}`,
      );

      const dataplanesResponse = await this.defaultClient.dataplanesGet({
        orgName: organizationName,
      });

      if (!dataplanesResponse.ok) {
        this.logger.error(
          `Failed to fetch dataplanes for organization ${organizationName}`,
        );
        return [];
      }

      const dataplanesData = await dataplanesResponse.json();
      if (!dataplanesData.success || !dataplanesData.data?.items) {
        this.logger.warn(
          `No dataplanes found for organization ${organizationName}`,
        );
        return [];
      }

      const dataplanes = dataplanesData.data.items as ModelsDataPlane[];
      console.log('dataplanes', dataplanes);
      const result = this.transformDataPlaneData(dataplanes, organizationName);

      const totalTime = Date.now() - startTime;
      this.logger.info(
        `Dataplane fetch completed for ${organizationName}: ${result.length} dataplanes found (${totalTime}ms)`,
      );

      return result;
    } catch (error: unknown) {
      const totalTime = Date.now() - startTime;
      this.logger.error(
        `Error fetching dataplanes for organization ${organizationName} (${totalTime}ms):`,
        error as Error,
      );
      return [];
    }
  }

  /**
   * Fetches all dataplanes with their associated environments
   */
  async fetchDataplanesWithEnvironments(): Promise<
    DataPlaneWithEnvironments[]
  > {
    const startTime = Date.now();
    try {
      this.logger.info('Starting dataplanes with environments fetch');

      // Fetch both dataplanes and environments in parallel
      const [dataplanes, environments] = await Promise.all([
        this.fetchAllDataplanes(),
        this.fetchAllEnvironments(),
      ]);
      console.log('dataplanes', dataplanes);
      console.log('environments', environments);

      // Group environments by dataPlaneRef
      const environmentsByDataPlane = new Map<string, Environment[]>();
      environments.forEach(env => {
        const dataPlaneRef = env.dataPlaneRef;
        if (!environmentsByDataPlane.has(dataPlaneRef)) {
          environmentsByDataPlane.set(dataPlaneRef, []);
        }
        environmentsByDataPlane.get(dataPlaneRef)!.push(env);
      });

      // Create DataPlaneWithEnvironments objects
      const dataplanesWithEnvironments: DataPlaneWithEnvironments[] =
        dataplanes.map(dataplane => ({
          ...dataplane,
          environments: environmentsByDataPlane.get(dataplane.name) || [],
        }));

      const totalTime = Date.now() - startTime;
      this.logger.info(
        `Dataplanes with environments fetch completed: ${dataplanesWithEnvironments.length} dataplanes with ${environments.length} total environments (${totalTime}ms)`,
      );

      return dataplanesWithEnvironments;
    } catch (error: unknown) {
      const totalTime = Date.now() - startTime;
      this.logger.error(
        `Error fetching dataplanes with environments (${totalTime}ms):`,
        error as Error,
      );
      return [];
    }
  }

  /**
   * Fetches all dataplanes with their associated environments and component counts
   */
  async fetchDataplanesWithEnvironmentsAndComponentCounts(): Promise<
    DataPlaneWithEnvironments[]
  > {
    const startTime = Date.now();
    try {
      this.logger.info(
        'Starting dataplanes with environments and component counts fetch',
      );

      // First get dataplanes with environments
      const dataplanesWithEnvironments =
        await this.fetchDataplanesWithEnvironments();

      // For each environment, we need to count components
      // Note: This is a simplified approach. In a real implementation, you might want to:
      // 1. Get all components from the catalog API
      // 2. For each component, check its bindings to see which environments it's deployed to
      // 3. Count components per environment

      // For now, we'll add a placeholder count and log that this needs catalog integration
      const enrichedDataplanes = dataplanesWithEnvironments.map(dataplane => ({
        ...dataplane,
        environments: dataplane.environments.map(env => ({
          ...env,
          componentCount: 0, // Placeholder - will be populated by frontend using catalog API
        })),
      }));

      const totalTime = Date.now() - startTime;
      this.logger.info(
        `Dataplanes with environments and component counts fetch completed: ${enrichedDataplanes.length} dataplanes (${totalTime}ms)`,
      );

      return enrichedDataplanes;
    } catch (error: unknown) {
      const totalTime = Date.now() - startTime;
      this.logger.error(
        `Error fetching dataplanes with environments and component counts (${totalTime}ms):`,
        error as Error,
      );
      return [];
    }
  }

  /**
   * Fetches component counts per environment using bindings API
   */
  async fetchComponentCountsPerEnvironment(
    components: Array<{
      orgName: string;
      projectName: string;
      componentName: string;
    }>,
  ): Promise<Map<string, number>> {
    const startTime = Date.now();
    const componentCountsByEnvironment = new Map<string, number>();

    try {
      this.logger.info(
        `Starting component counts fetch for ${components.length} components`,
      );

      // Process components in parallel with some concurrency control
      const batchSize = 10; // Process 10 components at a time to avoid overwhelming the API

      for (let i = 0; i < components.length; i += batchSize) {
        const batch = components.slice(i, i + batchSize);

        const batchPromises = batch.map(async component => {
          try {
            // Get bindings for this component
            const bindingsResponse = await this.defaultClient.bindingsGet({
              orgName: component.orgName,
              projectName: component.projectName,
              componentName: component.componentName,
            });

            if (bindingsResponse.ok) {
              const bindingsData = await bindingsResponse.json();
              if (bindingsData.success && bindingsData.data?.items) {
                // Count environments where this component is deployed
                bindingsData.data.items.forEach((binding: BindingResponse) => {
                  const envName = binding.environment;
                  if (envName) {
                    const currentCount =
                      componentCountsByEnvironment.get(envName) || 0;
                    componentCountsByEnvironment.set(envName, currentCount + 1);
                  }
                });
              }
            }
          } catch (error) {
            this.logger.warn(
              `Failed to fetch bindings for component ${component.orgName}/${component.projectName}/${component.componentName}:`,
              error instanceof Error ? error : new Error(String(error)),
            );
          }
        });

        // Wait for this batch to complete before processing the next
        await Promise.all(batchPromises);
      }

      const totalTime = Date.now() - startTime;
      this.logger.info(
        `Component counts fetch completed: Found deployments in ${componentCountsByEnvironment.size} environments (${totalTime}ms)`,
      );

      return componentCountsByEnvironment;
    } catch (error: unknown) {
      const totalTime = Date.now() - startTime;
      this.logger.error(
        `Error fetching component counts (${totalTime}ms):`,
        error as Error,
      );
      return componentCountsByEnvironment;
    }
  }

  /**
   * Fetches count of distinct components that have at least one binding (deployment)
   */
  async fetchDistinctDeployedComponentsCount(
    components: Array<{
      orgName: string;
      projectName: string;
      componentName: string;
    }>,
  ): Promise<number> {
    const startTime = Date.now();
    const deployedComponents = new Set<string>();

    try {
      this.logger.info(
        `Starting distinct deployed components count for ${components.length} components`,
      );

      // Process components in parallel with some concurrency control
      const batchSize = 10; // Process 10 components at a time to avoid overwhelming the API

      for (let i = 0; i < components.length; i += batchSize) {
        const batch = components.slice(i, i + batchSize);

        const batchPromises = batch.map(async component => {
          try {
            // Get bindings for this component
            const bindingsResponse = await this.defaultClient.bindingsGet({
              orgName: component.orgName,
              projectName: component.projectName,
              componentName: component.componentName,
            });

            if (bindingsResponse.ok) {
              const bindingsData = await bindingsResponse.json();
              if (
                bindingsData.success &&
                bindingsData.data?.items &&
                bindingsData.data.items.length > 0
              ) {
                // If component has at least one binding, count it as deployed
                const componentKey = `${component.orgName}/${component.projectName}/${component.componentName}`;
                deployedComponents.add(componentKey);
              }
            }
          } catch (error) {
            this.logger.warn(
              `Failed to fetch bindings for component ${component.orgName}/${component.projectName}/${component.componentName}:`,
              error instanceof Error ? error : new Error(String(error)),
            );
          }
        });

        // Wait for this batch to complete before processing the next
        await Promise.all(batchPromises);
      }

      const totalTime = Date.now() - startTime;
      this.logger.info(
        `Distinct deployed components count completed: Found ${deployedComponents.size} deployed components (${totalTime}ms)`,
      );

      return deployedComponents.size;
    } catch (error: unknown) {
      const totalTime = Date.now() - startTime;
      this.logger.error(
        `Error fetching distinct deployed components count (${totalTime}ms):`,
        error as Error,
      );
      return 0;
    }
  }

  private transformEnvironmentData(
    environmentData: ModelsEnvironment[],
    organizationName: string,
  ): Environment[] {
    return environmentData.map(env => {
      const transformedEnv: Environment = {
        name: env.name,
        namespace: env.namespace,
        displayName: env.displayName,
        description: env.description,
        organization: organizationName,
        dataPlaneRef: env.dataPlaneRef,
        isProduction: env.isProduction,
        dnsPrefix: env.dnsPrefix,
        createdAt: env.createdAt,
        status: env.status,
      };

      return transformedEnv;
    });
  }

  private transformDataPlaneData(
    dataplaneData: ModelsDataPlane[],
    organizationName: string,
  ): DataPlane[] {
    return dataplaneData.map(dp => {
      const transformedDataPlane: DataPlane = {
        name: dp.name,
        namespace: dp.namespace,
        displayName: dp.displayName,
        description: dp.description,
        organization: organizationName,
        registryPrefix: dp.registryPrefix,
        registrySecretRef: dp.registrySecretRef,
        kubernetesClusterName: dp.kubernetesClusterName,
        apiServerURL: dp.apiServerURL,
        publicVirtualHost: dp.publicVirtualHost,
        organizationVirtualHost: dp.organizationVirtualHost,
        observerURL: dp.observerURL,
        observerUsername: dp.observerUsername,
        createdAt: dp.createdAt,
        status: dp.status,
      };

      return transformedDataPlane;
    });
  }
}
