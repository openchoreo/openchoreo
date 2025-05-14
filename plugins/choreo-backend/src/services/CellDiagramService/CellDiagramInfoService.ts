import {
  BackstageCredentials,
  LoggerService,
} from '@backstage/backend-plugin-api/*';

import { KubernetesBuilder } from '@backstage/plugin-kubernetes-backend';
import { Config } from '@backstage/config';
import { CatalogApi } from '@backstage/catalog-client';
import { PermissionEvaluator } from '@backstage/plugin-permission-common';
import { DiscoveryService } from '@backstage/backend-plugin-api';
import {
  KubernetesFetcher,
  KubernetesClustersSupplier,
  ObjectToFetch,
  FetchResponseWrapper,
} from '@backstage/plugin-kubernetes-node';
import { Project, Component } from 'choreo-cell-diagram';
import { cellChoreoWorkflowTypes, CellDiagramService } from '../../types';
import {
  CHOREO_LABELS,
  CHOREO_ANNOTATIONS,
} from '@internal/plugin-choreo/src/constants/labels';

/**
 * Service implementation for fetching and managing Cell Diagram information.
 * @implements {CellDiagramService}
 */
export class CellDiagramInfoService implements CellDiagramService {
  private readonly logger: LoggerService;
  private readonly fetcher: KubernetesFetcher;
  private readonly clusterSupplier: KubernetesClustersSupplier;

  /**
   * Private constructor for CellDiagramInfoService.
   * Use the static create method to instantiate.
   * @param {LoggerService} logger - Logger service instance
   * @param {KubernetesFetcher} fetcher - Kubernetes fetcher instance
   * @param {KubernetesClustersSupplier} clusterSupplier - Kubernetes cluster supplier instance
   * @private
   */
  private constructor(
    logger: LoggerService,
    fetcher: KubernetesFetcher,
    clusterSupplier: KubernetesClustersSupplier,
  ) {
    this.logger = logger;
    this.fetcher = fetcher;
    this.clusterSupplier = clusterSupplier;
  }

  /**
   * Creates a new instance of CellDiagramInfoService.
   * @param {LoggerService} logger - Logger service instance
   * @param {Config} config - Backstage configuration
   * @param {CatalogApi} catalogApi - Catalog API instance
   * @param {PermissionEvaluator} permissions - Permission evaluator instance
   * @param {DiscoveryService} discovery - Discovery service instance
   * @returns {Promise<CellDiagramService>} A new instance of CellDiagramInfoService
   * @static
   */
  static async create(
    logger: LoggerService,
    config: Config,
    catalogApi: CatalogApi,
    permissions: PermissionEvaluator,
    discovery: DiscoveryService,
  ): Promise<CellDiagramService> {
    const builder = KubernetesBuilder.createBuilder({
      logger,
      config,
      catalogApi,
      permissions,
      discovery,
    });

    const { fetcher, clusterSupplier } = await builder.build();
    return new CellDiagramInfoService(logger, fetcher, clusterSupplier);
  }

  /**
   * Fetches project information including its components and their configurations.
   * @param {Object} request - The request object
   * @param {string} request.projectName - Name of the project to fetch
   * @param {string} request.organizationName - Name of the organization the project belongs to
   * @returns {Promise<Project | undefined>} Project information if found, undefined otherwise
   */
  async fetchProjectInfo(request: {
    projectName: string;
    organizationName: string;
  }): Promise<Project | undefined> {
    const credentials: BackstageCredentials = {
      $$type: '@backstage/BackstageCredentials',
      principal: 'anonymous',
    };

    const clusters = await this.clusterSupplier.getClusters({ credentials });

    if (clusters.length === 0) {
      this.logger.warn('No clusters found.');
      return undefined;
    }
    const objectTypesToFetch: Set<ObjectToFetch> = new Set([
      ...cellChoreoWorkflowTypes,
    ]);

    let project: Project;

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

        // Find project

        const projectCrd = fetchedObjects.responses
          .filter(response => response.type === 'customresources')
          .flatMap(response => response.resources)
          .filter(
            // TODO Can filter by namespace instead of label
            resource =>
              resource.kind === 'Project' &&
              resource.metadata?.labels?.[CHOREO_LABELS.ORGANIZATION] ===
                request.organizationName,
          )
          .find(resource => resource.metadata.name === request.projectName);

        if (!projectCrd) {
          continue;
        }

        const componentCrds = fetchedObjects.responses
          .filter(response => response.type === 'customresources')
          .flatMap(response => response.resources)
          .filter(
            resource =>
              resource.kind === 'Component' &&
              resource.metadata?.labels?.[CHOREO_LABELS.PROJECT] ===
                request.projectName &&
              resource.metadata?.labels?.[CHOREO_LABELS.ORGANIZATION] ===
                request.organizationName,
          );

        const components: Component[] = componentCrds.map(component => {
          const endpoint = this.getEndpointForComponent(
            fetchedObjects,
            component.metadata.name,
            request.projectName,
            request.organizationName,
          );
          return {
            id: component.metadata?.uid || component.metadata?.name || '',
            label:
              component.metadata?.annotations?.[
                CHOREO_ANNOTATIONS.DISPLAY_NAME
              ] ||
              component.metadata?.name ||
              '',
            version: component.metadata?.resourceVersion || '1.0.0',
            type: component.spec?.type || 'SERVICE',
            services: {
              [component.metadata?.name || '']: {
                id: component.metadata?.name || '',
                label:
                  component.metadata?.annotations?.[
                    CHOREO_ANNOTATIONS.DISPLAY_NAME
                  ] ||
                  component.metadata?.name ||
                  '',
                type: endpoint.spec.type,
                dependencyIds: [],
                deploymentMetadata: {
                  gateways: {
                    internet: {
                      isExposed: Boolean(
                        endpoint.spec?.networkVisibilities?.public ?? true,
                      ),
                    },
                    intranet: {
                      isExposed: Boolean(
                        endpoint.spec?.networkVisibilities?.organization ??
                          false,
                      ),
                    },
                  },
                },
              },
            },
            connections: [],
          };
        });

        project = {
          id: projectCrd.metadata?.uid || projectCrd.metadata?.name || '',
          name:
            projectCrd.metadata?.annotations?.[
              CHOREO_ANNOTATIONS.DISPLAY_NAME
            ] ||
            projectCrd.metadata?.name ||
            '',
          modelVersion: '1.0.0',
          components,
          connections: [],
          configurations: [],
        };

        return project;
      } catch (error: unknown) {
        this.logger.error(
          `Failed to fetch objects for cluster ${cluster.name}:`,
          error as Error,
        );
      }
    }

    return undefined;
  }

  /**
   * Retrieves the endpoint configuration for a specific component.
   * @param {FetchResponseWrapper} crds - The fetched Custom Resource Definitions
   * @param {string} componentName - Name of the component
   * @param {string} projectName - Name of the project
   * @param {string} organizationName - Name of the organization
   * @returns {any} The endpoint configuration for the component
   * @private
   */
  private getEndpointForComponent(
    crds: FetchResponseWrapper,
    componentName: string,
    projectName: string,
    organizationName: string,
  ): any {
    const endpoint = crds.responses
      .filter(response => response.type === 'customresources')
      .flatMap(response => response.resources)
      .find(
        resource =>
          resource.kind === 'Endpoint' &&
          resource.metadata?.labels?.[CHOREO_LABELS.PROJECT] === projectName &&
          resource.metadata?.labels?.[CHOREO_LABELS.ORGANIZATION] ===
            organizationName &&
          resource.metadata?.labels?.[CHOREO_LABELS.COMPONENT] ===
            componentName, // TODO this will find any endpoint not considering environment
      );

    return endpoint;
  }
}
