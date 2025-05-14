import { Config, JsonObject } from '@backstage/config';
import { LoggerService } from '@backstage/backend-plugin-api';
import { KubernetesBuilder } from '@backstage/plugin-kubernetes-backend';
import { CatalogApi } from '@backstage/catalog-client';
import { PermissionEvaluator } from '@backstage/plugin-permission-common';
import {
  DiscoveryService,
  BackstageCredentials,
} from '@backstage/backend-plugin-api';
import {
  KubernetesObjectTypes,
  ClusterDetails,
} from '@backstage/plugin-kubernetes-node';
import pluralize from 'pluralize';
import { ANNOTATION_KUBERNETES_AUTH_PROVIDER } from '@backstage/plugin-kubernetes-common';
import { KubernetesResource } from './types';

type ObjectToFetch = {
  group: string;
  apiVersion: string;
  plural: string;
  objectType: KubernetesObjectTypes;
};

// Add new type definitions for auth providers
type AuthProvider = 'serviceAccount' | 'google' | 'aws' | 'azure' | 'oidc';

// Extend ClusterDetails to include authProvider
interface ExtendedClusterDetails extends ClusterDetails {
  authProvider?: AuthProvider;
}

export class KubernetesDataProvider {
  logger: LoggerService;
  config: Config;
  catalogApi: CatalogApi;
  permissions: PermissionEvaluator;
  discovery: DiscoveryService;

  constructor(
    logger: LoggerService,
    config: Config,
    catalogApi: CatalogApi,
    permissions: PermissionEvaluator,
    discovery: DiscoveryService,
  ) {
    this.logger = logger;
    this.config = config;
    this.catalogApi = catalogApi;
    this.permissions = permissions;
    this.discovery = discovery;
  }

  async fetchKubernetesObjects(): Promise<KubernetesResource[]> {
    try {
      const builder = KubernetesBuilder.createBuilder({
        logger: this.logger,
        config: this.config,
        catalogApi: this.catalogApi,
        permissions: this.permissions,
        discovery: this.discovery,
      });

      const { fetcher, clusterSupplier } = await builder.build();

      const credentials: BackstageCredentials = {
        $$type: '@backstage/BackstageCredentials',
        principal: 'anonymous',
      };

      const clusters = await clusterSupplier.getClusters({ credentials });

      if (clusters.length === 0) {
        this.logger.warn('No clusters found.');
        return [];
      }

      const choreoWorkflowTypes: ObjectToFetch[] = [
        {
          group: 'core.choreo.dev',
          apiVersion: 'v1',
          plural: 'projects',
          objectType: 'customresources',
        },
        {
          group: 'core.choreo.dev',
          apiVersion: 'v1',
          plural: 'components',
          objectType: 'customresources',
        },
      ];

      const objectTypesToFetch: Set<ObjectToFetch> = new Set([
        ...choreoWorkflowTypes,
      ]);

      const objectTypeMap = Array.from(objectTypesToFetch).reduce(
        (acc, type) => {
          acc[type.plural] = type;
          return acc;
        },
        {} as Record<string, ObjectToFetch>,
      );

      const excludedNamespaces = new Set(
        this.config.getOptionalStringArray(
          'choreoIngestor.excludedNamespaces',
        ) || ['default', 'kube-public', 'kube-system', 'choreo-system'],
      );

      let allFetchedObjects: KubernetesResource[] = [];

      for (const cluster of clusters as ExtendedClusterDetails[]) {
        // Get the auth provider type from the cluster config
        const authProvider =
          cluster.authMetadata[ANNOTATION_KUBERNETES_AUTH_PROVIDER] ||
          'serviceAccount';

        // Get the auth credentials based on the provider type
        let credential;
        try {
          credential = await this.getAuthCredential(cluster, authProvider);
        } catch (error) {
          if (error instanceof Error) {
            this.logger.error(
              `Failed to get auth credentials for cluster ${cluster.name} with provider ${authProvider}:`,
              error,
            );
          } else {
            this.logger.error(
              `Failed to get auth credentials for cluster ${cluster.name} with provider ${authProvider}:`,
              {
                error: String(error),
              },
            );
          }
          continue;
        }

        try {
          const fetchedObjects = await fetcher.fetchObjectsForService({
            serviceId: cluster.name,
            clusterDetails: cluster,
            credential,
            objectTypesToFetch,
            customResources: [],
          });
          const filteredObjects = fetchedObjects.responses.flatMap(response =>
            response.resources
              .filter(resource => {
                return !excludedNamespaces.has(resource.metadata.namespace);
              })
              .map(async resource => {
                let type = response.type as string;
                if (response.type === 'customresources') {
                  type = pluralize(resource.kind.toLowerCase());
                }
                const objectType = objectTypeMap[type];
                if (
                  objectType.group === null ||
                  objectType.apiVersion === null
                ) {
                  return {};
                }
                return {
                  ...resource,
                  apiVersion: `${objectType.group}/${objectType.apiVersion}`,
                  kind: objectType.plural?.slice(0, -1),
                  clusterName: cluster.name,
                };
              }),
          );

          allFetchedObjects = allFetchedObjects.concat(
            await Promise.all(filteredObjects),
          );
        } catch (clusterError) {
          if (clusterError instanceof Error) {
            this.logger.error(
              `Failed to fetch objects for cluster ${cluster.name}: ${clusterError.message}`,
              clusterError,
            );
          } else {
            this.logger.error(
              `Failed to fetch objects for cluster ${cluster.name}:`,
              {
                error: String(clusterError),
              },
            );
          }
        }
      }

      this.logger.debug(
        `Total fetched Kubernetes objects: ${allFetchedObjects.length}`,
      );
      return allFetchedObjects;
    } catch (error) {
      if (error instanceof Error) {
        this.logger.error('Error fetching Kubernetes objects', error);
      } else if (typeof error === 'object') {
        this.logger.error(
          'Error fetching Kubernetes objects',
          error as JsonObject,
        );
      } else {
        this.logger.error(
          'Unknown error occurred while fetching Kubernetes objects',
          {
            message: String(error),
          },
        );
      }
      return []; // Add this return statement
    }
  }

  async fetchCRDMapping(): Promise<Record<string, string>> {
    try {
      const builder = KubernetesBuilder.createBuilder({
        logger: this.logger,
        config: this.config,
        catalogApi: this.catalogApi,
        permissions: this.permissions,
        discovery: this.discovery,
      });

      const { fetcher, clusterSupplier } = await builder.build();

      const credentials: BackstageCredentials = {
        $$type: '@backstage/BackstageCredentials',
        principal: 'anonymous',
      };

      const clusters = await clusterSupplier.getClusters({ credentials });

      if (clusters.length === 0) {
        this.logger.warn('No clusters found for CRD mapping.');
        return {};
      }

      const crdMapping: Record<string, string> = {};

      for (const cluster of clusters as ExtendedClusterDetails[]) {
        // Get the auth provider type from the cluster config
        const authProvider =
          cluster.authMetadata[ANNOTATION_KUBERNETES_AUTH_PROVIDER] ||
          'serviceAccount';

        // Get the auth credentials based on the provider type
        let credential;
        try {
          credential = await this.getAuthCredential(cluster, authProvider);
        } catch (error) {
          if (error instanceof Error) {
            this.logger.error(
              `Failed to get auth credentials for cluster ${cluster.name} with provider ${authProvider}:`,
              error,
            );
          } else {
            this.logger.error(
              `Failed to get auth credentials for cluster ${cluster.name} with provider ${authProvider}:`,
              {
                error: String(error),
              },
            );
          }
          continue;
        }

        try {
          const crds = await fetcher.fetchObjectsForService({
            serviceId: cluster.name,
            clusterDetails: cluster,
            credential,
            objectTypesToFetch: new Set([
              {
                group: 'apiextensions.k8s.io',
                apiVersion: 'v1',
                plural: 'customresourcedefinitions',
                objectType: 'customresources' as KubernetesObjectTypes,
              },
            ]),
            customResources: [],
          });

          crds.responses
            .flatMap(response => response.resources)
            .forEach(crd => {
              const kind = crd.spec?.names?.kind;
              const plural = crd.spec?.names?.plural;
              if (kind && plural) {
                crdMapping[kind] = plural;
              }
            });
        } catch (clusterError) {
          if (clusterError instanceof Error) {
            this.logger.error(
              `Failed to fetch objects for cluster ${cluster.name}: ${clusterError.message}`,
              clusterError,
            );
          } else {
            this.logger.error(
              `Failed to fetch objects for cluster ${cluster.name}:`,
              {
                error: String(clusterError),
              },
            );
          }
        }
      }

      return crdMapping;
    } catch (error) {
      if (error instanceof Error) {
        this.logger.error('Error fetching Kubernetes objects', error);
      } else if (typeof error === 'object') {
        this.logger.error(
          'Error fetching Kubernetes objects',
          error as JsonObject,
        );
      } else {
        this.logger.error(
          'Unknown error occurred while fetching Kubernetes objects',
          {
            message: String(error),
          },
        );
      }
      return {};
    }
  }

  private async getAuthCredential(
    cluster: any,
    authProvider: string,
  ): Promise<any> {
    switch (authProvider) {
      case 'serviceAccount': {
        const token = cluster.authMetadata?.serviceAccountToken;
        if (!token) {
          throw new Error(
            'Service account token not found in cluster auth metadata',
          );
        }
        return { type: 'bearer token', token };
      }
      case 'google': {
        // For Google authentication (both client and server-side)
        if (cluster.authMetadata?.google) {
          return {
            type: 'google',
            ...cluster.authMetadata.google,
          };
        }
        throw new Error(
          'Google auth metadata not found in cluster configuration',
        );
      }
      case 'aws': {
        // For AWS authentication
        if (!cluster.authMetadata?.['kubernetes.io/aws-assume-role']) {
          throw new Error('AWS role ARN not found in cluster auth metadata');
        }
        return {
          type: 'aws',
          assumeRole: cluster.authMetadata['kubernetes.io/aws-assume-role'],
          externalId: cluster.authMetadata['kubernetes.io/aws-external-id'],
          clusterAwsId: cluster.authMetadata['kubernetes.io/x-k8s-aws-id'],
        };
      }
      case 'azure': {
        // For Azure authentication (both AKS and server-side)
        return {
          type: 'azure',
          ...cluster.authMetadata?.azure,
        };
      }
      case 'oidc': {
        // For OIDC authentication
        if (!cluster.authMetadata?.oidc) {
          throw new Error(
            'OIDC configuration not found in cluster auth metadata',
          );
        }
        return {
          type: 'oidc',
          ...cluster.authMetadata.oidc,
        };
      }
      default:
        throw new Error(`Unsupported authentication provider: ${authProvider}`);
    }
  }
}
