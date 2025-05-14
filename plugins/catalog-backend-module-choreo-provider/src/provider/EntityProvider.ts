import {
  EntityProvider,
  EntityProviderConnection,
} from '@backstage/plugin-catalog-node';
import { Entity } from '@backstage/catalog-model';
import { SchedulerServiceTaskRunner } from '@backstage/backend-plugin-api';
import { KubernetesDataProvider } from './KubernetesDataProvider';
import { Config } from '@backstage/config';
import { CatalogApi } from '@backstage/catalog-client';
import { PermissionEvaluator } from '@backstage/plugin-permission-common';
import { LoggerService, DiscoveryService } from '@backstage/backend-plugin-api';
import { ChoreoPrefix, KubernetesResource } from './types';

export class ChoreoEntityProvider implements EntityProvider {
  private readonly taskRunner: SchedulerServiceTaskRunner;
  private connection?: EntityProviderConnection;
  private readonly logger: LoggerService;
  private readonly config: Config;
  private readonly catalogApi: CatalogApi;
  private readonly permissions: PermissionEvaluator;
  private readonly discovery: DiscoveryService;

  constructor(
    taskRunner: SchedulerServiceTaskRunner,
    logger: LoggerService,
    config: Config,
    catalogApi: CatalogApi,
    permissions: PermissionEvaluator,
    discovery: DiscoveryService,
  ) {
    this.taskRunner = taskRunner;
    this.logger = logger;
    this.config = config;
    this.catalogApi = catalogApi;
    this.permissions = permissions;
    this.discovery = discovery;
  }

  getProviderName(): string {
    return 'ChoreoEntityProvider';
  }

  async connect(connection: EntityProviderConnection): Promise<void> {
    this.connection = connection;
    await this.taskRunner.run({
      id: this.getProviderName(),
      fn: async () => {
        await this.run();
      },
    });
  }

  async run(): Promise<void> {
    if (!this.connection) {
      throw new Error('Connection not initialized');
    }
    try {
      const kubernetesDataProvider = new KubernetesDataProvider(
        this.logger,
        this.config,
        this.catalogApi,
        this.permissions,
        this.discovery,
      );
      if (this.config.getOptionalBoolean('choreoIngestor.enabled')) {
        // Fetch all Kubernetes resources and build a CRD mapping
        const kubernetesData =
          await kubernetesDataProvider.fetchKubernetesObjects();

        const entities: Entity[] = kubernetesData.flatMap(k8s => {
          if (k8s) {
            this.logger.debug(
              `Processing Kubernetes Object: ${JSON.stringify(k8s)}`,
            );
            if (k8s.kind === 'project') {
              return this.translateProjectToEntity(k8s);
            } else if (k8s.kind === 'component') {
              return this.translateComponentToEntity(k8s);
            }
            return [];
          }
          return [];
        });

        await this.connection.applyMutation({
          type: 'full',
          entities: entities.map(entity => ({
            entity,
            locationKey: `provider:${this.getProviderName()}`,
          })),
        });
      } else {
        this.logger.info(`ChoreoEntityProvider Disabled`);
      }
    } catch (error) {
      this.logger.error(`Failed to run ChoreoEntityProvider: ${error}`);
    }
  }

  private translateProjectToEntity(project: KubernetesResource): Entity {
    const defaultAnnotations: Record<string, string> = {
      'backstage.io/managed-by-location': `cluster origin: choreo`,
      'backstage.io/managed-by-origin-location': `cluster origin: choreo`,
    };
    const annotations = project.metadata.annotations || {};
    const labels = project.metadata.labels || {};
    const systemEntity: Entity = {
      apiVersion: 'backstage.io/v1alpha1',
      kind: 'System',
      metadata: {
        name: project.metadata.name,
        description: annotations[`${ChoreoPrefix}description`],
        namespace: project.metadata.namespace,
        tags: [`cluster:${project.clusterName}`, `kind:${project.kind}`],
        annotations: defaultAnnotations,
        labels: labels,
      },
      spec: {
        owner: 'choreo',
        type: 'service',
      },
    };
    return systemEntity;
  }

  private translateComponentToEntity(component: KubernetesResource): Entity {
    const annotations = component.metadata.annotations || {};
    const labels = component.metadata.labels || {};
    const defaultAnnotations: Record<string, string> = {
      'backstage.io/managed-by-location': `cluster origin: choreo`,
      'backstage.io/managed-by-origin-location': `cluster origin: choreo`,
    };
    const componentEntity: Entity = {
      apiVersion: 'backstage.io/v1alpha1',
      kind: 'Component',
      metadata: {
        name: component.metadata.name,
        title: annotations[`${ChoreoPrefix}display-name`],
        description: annotations[`${ChoreoPrefix}description`],
        namespace: component.metadata.namespace,
        tags: [`cluster:${component.clusterName}`, `kind:${component.kind}`],
        annotations: defaultAnnotations,
        labels: labels,
      },
      spec: {
        type: this.componentTypeMapping(component.spec.type.toLowerCase()),
        lifecycle: 'production',
        owner: 'default',
        system: labels[`${ChoreoPrefix}project`],
        // dependsOn: annotations[`${prefix}/dependsOn`]?.split(','),
        // providesApis: annotations[`${prefix}/providesApis`]?.split(','), //TODO How do we map api relationships
        // consumesApis: annotations[`${prefix}/consumesApis`]?.split(','),
      },
    };
    return componentEntity;
  }

  private componentTypeMapping(s: string): string {
    switch (s) {
      case 'service':
        return 'service';
      case 'webapplication':
        return 'website';
      default:
        return s;
    }
  }
}
