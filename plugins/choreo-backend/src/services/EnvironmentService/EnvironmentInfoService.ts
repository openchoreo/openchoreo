import {
  BackstageCredentials,
  LoggerService,
} from '@backstage/backend-plugin-api/*';
import {
  EnvironmentService,
  Environment,
  ObjectToFetch,
  environmentChoreoWorkflowTypes,
} from '../../types';
import { KubernetesBuilder } from '@backstage/plugin-kubernetes-backend';
import { Config } from '@backstage/config';
import { CatalogApi } from '@backstage/catalog-client';
import { PermissionEvaluator } from '@backstage/plugin-permission-common';
import { DiscoveryService } from '@backstage/backend-plugin-api';
import {
  KubernetesFetcher,
  KubernetesClustersSupplier,
} from '@backstage/plugin-kubernetes-node';
import {
  CHOREO_LABELS,
  CHOREO_ANNOTATIONS,
} from '@internal/plugin-choreo/src/constants/labels';

/**
 * Service for managing and retrieving environment-related information for deployments.
 * This service handles fetching deployment status, environment details, and endpoint information
 * from Kubernetes clusters.
 */
export class EnvironmentInfoService implements EnvironmentService {
  private readonly logger: LoggerService;
  private readonly fetcher: KubernetesFetcher;
  private readonly clusterSupplier: KubernetesClustersSupplier;

  private constructor(
    logger: LoggerService,
    fetcher: KubernetesFetcher,
    clusterSupplier: KubernetesClustersSupplier,
  ) {
    this.logger = logger;
    this.fetcher = fetcher;
    this.clusterSupplier = clusterSupplier;
  }

  static async create(
    logger: LoggerService,
    config: Config,
    catalogApi: CatalogApi,
    permissions: PermissionEvaluator,
    discovery: DiscoveryService,
  ): Promise<EnvironmentInfoService> {
    const builder = KubernetesBuilder.createBuilder({
      logger,
      config,
      catalogApi,
      permissions,
      discovery,
    });

    const { fetcher, clusterSupplier } = await builder.build();
    return new EnvironmentInfoService(logger, fetcher, clusterSupplier);
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
   * @throws {Error} When there's an error fetching data from the Kubernetes cluster
   */
  async fetchDeploymentInfo(request: {
    projectName: string;
    componentName: string;
    organizationName: string;
  }): Promise<Environment[]> {
    const credentials: BackstageCredentials = {
      $$type: '@backstage/BackstageCredentials',
      principal: 'anonymous',
    };

    const clusters = await this.clusterSupplier.getClusters({ credentials });

    if (clusters.length === 0) {
      this.logger.warn('No clusters found.');
      return [];
    }

    const objectTypesToFetch: Set<ObjectToFetch> = new Set([
      ...environmentChoreoWorkflowTypes,
    ]);

    const environments: Environment[] = [];

    for (const cluster of clusters) {
      try {
        const fetchedObjects = await this.fetcher.fetchObjectsForService({
          serviceId: cluster.name,
          clusterDetails: cluster,
          credential: {
            type: 'bearer token',
            token: cluster.authMetadata?.serviceAccountToken,
          },
          objectTypesToFetch,
          customResources: [],
        });

        const environmentObjects = fetchedObjects.responses
          .filter(response => response.type === 'customresources')
          .flatMap(response => response.resources)
          .filter(
            // TODO Can filter by namespace instead of label
            resource =>
              resource.kind === 'Environment' &&
              resource.metadata?.labels?.[CHOREO_LABELS.ORGANIZATION] ===
                request.organizationName,
          );

        for (const env of environmentObjects) {
          const deployment = fetchedObjects.responses
            .filter(response => response.type === 'customresources')
            .flatMap(response => response.resources)
            .find(
              resource =>
                resource.kind === 'Deployment' &&
                resource.metadata?.labels?.[CHOREO_LABELS.ENVIRONMENT] ===
                  env.metadata.name &&
                resource.metadata?.labels?.[CHOREO_LABELS.PROJECT] ===
                  request.projectName &&
                resource.metadata?.labels?.[CHOREO_LABELS.COMPONENT] ===
                  request.componentName &&
                resource.metadata?.labels?.[CHOREO_LABELS.ORGANIZATION] ===
                  request.organizationName,
            );

          if (deployment === undefined) {
            // Skip environments without a deployment
            continue;
          }

          const endpoint = fetchedObjects.responses
            .filter(response => (response.type = 'customresources'))
            .flatMap(response => response.resources)
            .find(
              resource =>
                resource.kind === 'Endpoint' &&
                resource.metadata?.labels?.[CHOREO_LABELS.DEPLOYMENT] ===
                  deployment.metadata?.name,
            );

          environments.push({
            name:
              env.metadata.annotations?.[CHOREO_ANNOTATIONS.DISPLAY_NAME] ||
              env.metadata.name,
            deployment: {
              status: deployment?.status?.conditions?.some(
                (c: { type: string; status: string }) =>
                  c.type === 'Ready' && c.status === 'True',
              )
                ? 'success'
                : 'failed',
              lastDeployed:
                deployment?.status?.conditions?.find(
                  (c: { type: string }) => c.type === 'Ready',
                )?.lastTransitionTime || new Date().toISOString(),
            },
            endpoint: {
              url: endpoint.status.address ?? '',
              status: env.status?.conditions?.some(
                (c: { type: string; status: string }) =>
                  c.type === 'Available' && c.status === 'True',
              )
                ? 'active'
                : 'inactive',
            },
          });
        }
      } catch (error: unknown) {
        this.logger.error(
          `Failed to fetch objects for cluster ${cluster.name}:`,
          error as Error,
        );
      }
    }

    return environments;
  }
}
