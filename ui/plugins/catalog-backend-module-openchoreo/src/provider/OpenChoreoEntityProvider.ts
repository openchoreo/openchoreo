import {
  EntityProvider,
  EntityProviderConnection,
} from '@backstage/plugin-catalog-node';
import { Entity } from '@backstage/catalog-model';
import { SchedulerServiceTaskRunner } from '@backstage/backend-plugin-api';
import { Config } from '@backstage/config';
import { LoggerService } from '@backstage/backend-plugin-api';
import {
  createOpenChoreoApiClient,
  OpenChoreoApiClient,
  ModelsProject,
  ModelsOrganization,
  ModelsComponent,
  ModelsEnvironment,
  ModelsCompleteComponent,
  WorkloadEndpoint,
} from '@openchoreo/backstage-plugin-api';
import {
  CHOREO_ANNOTATIONS,
  CHOREO_LABELS,
} from '@openchoreo/backstage-plugin-api';
import { EnvironmentEntityV1alpha1 } from '../kinds';

/**
 * Provides entities from OpenChoreo API
 */
export class OpenChoreoEntityProvider implements EntityProvider {
  private readonly taskRunner: SchedulerServiceTaskRunner;
  private connection?: EntityProviderConnection;
  private readonly logger: LoggerService;
  private readonly client: OpenChoreoApiClient;
  private readonly defaultOwner: string;

  constructor(
    taskRunner: SchedulerServiceTaskRunner,
    logger: LoggerService,
    config: Config,
  ) {
    this.taskRunner = taskRunner;
    this.logger = logger;
    this.client = createOpenChoreoApiClient(config, logger);
    // Default owner for all entities - configurable via app-config.yaml
    this.defaultOwner =
      config.getOptionalString('openchoreo.defaultOwner') || 'developers';
  }

  getProviderName(): string {
    return 'OpenChoreoEntityProvider';
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
      this.logger.info(
        'Fetching organizations and projects from OpenChoreo API',
      );

      // First, get all organizations
      const organizations = await this.client.getAllOrganizations();
      this.logger.info(
        `Found ${organizations.length} organizations from OpenChoreo`,
      );

      const allEntities: Entity[] = [];

      // Create Domain entities for each organization
      const domainEntities: Entity[] = organizations.map(org =>
        this.translateOrganizationToDomain(org),
      );
      allEntities.push(...domainEntities);

      // Get environments for each organization and create Environment entities
      for (const org of organizations) {
        try {
          const environments = await this.client.getAllEnvironments(org.name);
          this.logger.info(
            `Found ${environments.length} environments in organization: ${org.name}`,
          );

          const environmentEntities: Entity[] = environments.map(environment =>
            this.translateEnvironmentToEntity(environment, org.name),
          );
          allEntities.push(...environmentEntities);
        } catch (error) {
          this.logger.warn(
            `Failed to fetch environments for organization ${org.name}: ${error}`,
          );
        }
      }

      // Get projects for each organization and create System entities
      for (const org of organizations) {
        try {
          const projects = await this.client.getAllProjects(org.name);
          this.logger.info(
            `Found ${projects.length} projects in organization: ${org.name}`,
          );

          const systemEntities: Entity[] = projects.map(project =>
            this.translateProjectToEntity(project, org.name),
          );
          allEntities.push(...systemEntities);

          // Get components for each project and create Component entities
          for (const project of projects) {
            try {
              const components = await this.client.getAllComponents(
                org.name,
                project.name,
              );
              this.logger.info(
                `Found ${components.length} components in project: ${project.name}`,
              );

              for (const component of components) {
                // If the component is a Service, fetch complete details and create both component and API entities
                if (component.type === 'Service') {
                  try {
                    const completeComponent = await this.client.getComponent(
                      org.name,
                      project.name,
                      component.name,
                    );

                    // Create component entity with providesApis
                    const componentEntity =
                      this.translateServiceComponentToEntity(
                        completeComponent,
                        org.name,
                        project.name,
                      );
                    allEntities.push(componentEntity);

                    // Create API entities if endpoints exist
                    if (completeComponent.workload?.endpoints) {
                      const apiEntities = this.createApiEntitiesFromWorkload(
                        completeComponent,
                        org.name,
                        project.name,
                      );
                      allEntities.push(...apiEntities);
                    }
                  } catch (error) {
                    this.logger.warn(
                      `Failed to fetch complete component details for ${component.name}: ${error}`,
                    );
                    // Fallback to basic component entity
                    const componentEntity = this.translateComponentToEntity(
                      component,
                      org.name,
                      project.name,
                    );
                    allEntities.push(componentEntity);
                  }
                } else {
                  // Create basic component entity for non-Service components
                  const componentEntity = this.translateComponentToEntity(
                    component,
                    org.name,
                    project.name,
                  );
                  allEntities.push(componentEntity);
                }
              }
            } catch (error) {
              this.logger.warn(
                `Failed to fetch components for project ${project.name} in organization ${org.name}: ${error}`,
              );
            }
          }
        } catch (error) {
          this.logger.warn(
            `Failed to fetch projects for organization ${org.name}: ${error}`,
          );
        }
      }

      await this.connection.applyMutation({
        type: 'full',
        entities: allEntities.map(entity => ({
          entity,
          locationKey: `provider:${this.getProviderName()}`,
        })),
      });

      const systemCount = allEntities.filter(e => e.kind === 'System').length;
      const componentCount = allEntities.filter(
        e => e.kind === 'Component',
      ).length;
      const apiCount = allEntities.filter(e => e.kind === 'API').length;
      const environmentCount = allEntities.filter(
        e => e.kind === 'Environment',
      ).length;
      this.logger.info(
        `Successfully processed ${allEntities.length} entities (${domainEntities.length} domains, ${systemCount} systems, ${componentCount} components, ${apiCount} apis, ${environmentCount} environments)`,
      );
    } catch (error) {
      this.logger.error(`Failed to run OpenChoreoEntityProvider: ${error}`);
    }
  }

  /**
   * Translates a ModelsOrganization from OpenChoreo API to a Backstage Domain entity
   */
  private translateOrganizationToDomain(
    organization: ModelsOrganization,
  ): Entity {
    const domainEntity: Entity = {
      apiVersion: 'backstage.io/v1alpha1',
      kind: 'Domain',
      metadata: {
        name: organization.name,
        title: organization.displayName || organization.name,
        description: organization.description || organization.name,
        // namespace: 'default',
        tags: ['openchoreo', 'organization', 'domain'],
        annotations: {
          'backstage.io/managed-by-location': `provider:${this.getProviderName()}`,
          'backstage.io/managed-by-origin-location': `provider:${this.getProviderName()}`,
          [CHOREO_ANNOTATIONS.ORGANIZATION]: organization.name,
          [CHOREO_ANNOTATIONS.NAMESPACE]: organization.namespace,
          [CHOREO_ANNOTATIONS.CREATED_AT]: organization.createdAt,
          [CHOREO_ANNOTATIONS.STATUS]: organization.status,
        },
        labels: {
          [CHOREO_LABELS.MANAGED]: 'true',
        },
      },
      spec: {
        owner: this.defaultOwner,
      },
    };

    return domainEntity;
  }

  /**
   * Translates a ModelsProject from OpenChoreo API to a Backstage System entity
   */
  private translateProjectToEntity(
    project: ModelsProject,
    orgName: string,
  ): Entity {
    const systemEntity: Entity = {
      apiVersion: 'backstage.io/v1alpha1',
      kind: 'System',
      metadata: {
        name: project.name,
        title: project.displayName || project.name,
        description: project.description || project.name,
        // namespace: orgName,
        tags: ['openchoreo', 'project'],
        annotations: {
          'backstage.io/managed-by-location': `provider:${this.getProviderName()}`,
          'backstage.io/managed-by-origin-location': `provider:${this.getProviderName()}`,
          [CHOREO_ANNOTATIONS.PROJECT_ID]: project.name,
          [CHOREO_ANNOTATIONS.ORGANIZATION]: orgName,
        },
        labels: {
          'openchoreo.io/managed': 'true',
          // ...project.metadata?.labels,
        },
      },
      spec: {
        owner: this.defaultOwner,
        domain: orgName,
      },
    };

    return systemEntity;
  }

  /**
   * Translates a ModelsEnvironment from OpenChoreo API to a Backstage Environment entity
   */
  private translateEnvironmentToEntity(
    environment: ModelsEnvironment,
    orgName: string,
  ): EnvironmentEntityV1alpha1 {
    const environmentEntity: EnvironmentEntityV1alpha1 = {
      apiVersion: 'backstage.io/v1alpha1',
      kind: 'Environment',
      metadata: {
        name: environment.name,
        title: environment.displayName || environment.name,
        description:
          environment.description || `${environment.name} environment`,
        tags: [
          'openchoreo',
          'environment',
          environment.isProduction ? 'production' : 'non-production',
        ],
        annotations: {
          'backstage.io/managed-by-location': `provider:${this.getProviderName()}`,
          'backstage.io/managed-by-origin-location': `provider:${this.getProviderName()}`,
          [CHOREO_ANNOTATIONS.ENVIRONMENT]: environment.name,
          [CHOREO_ANNOTATIONS.ORGANIZATION]: orgName,
          [CHOREO_ANNOTATIONS.NAMESPACE]: environment.namespace,
          [CHOREO_ANNOTATIONS.CREATED_AT]: environment.createdAt,
          [CHOREO_ANNOTATIONS.STATUS]: environment.status,
          'openchoreo.io/data-plane-ref': environment.dataPlaneRef,
          'openchoreo.io/dns-prefix': environment.dnsPrefix,
          'openchoreo.io/is-production': environment.isProduction.toString(),
        },
        labels: {
          [CHOREO_LABELS.MANAGED]: 'true',
          'openchoreo.io/environment-type': environment.isProduction
            ? 'production'
            : 'non-production',
        },
      },
      spec: {
        type: environment.isProduction ? 'production' : 'non-production',
        owner: 'guests', // This could be configured or mapped from environment metadata
        domain: orgName, // Link to the parent domain (organization)
        isProduction: environment.isProduction,
        dataPlaneRef: environment.dataPlaneRef,
        dnsPrefix: environment.dnsPrefix,
      },
    };

    return environmentEntity;
  }

  /**
   * Translates a ModelsComponent from OpenChoreo API to a Backstage Component entity
   */
  private translateComponentToEntity(
    component: ModelsComponent,
    orgName: string,
    projectName: string,
    providesApis?: string[],
  ): Entity {
    let backstageComponentType: string = component.type.toLowerCase(); // e.g., 'service', 'webapp', etc.
    if (component.type === 'WebApplication') {
      backstageComponentType = 'website';
    }

    const componentEntity: Entity = {
      apiVersion: 'backstage.io/v1alpha1',
      kind: 'Component',
      metadata: {
        name: component.name,
        title: component.name,
        description: component.description || component.name,
        // namespace: orgName,
        tags: ['openchoreo', 'component', component.type.toLowerCase()],
        annotations: {
          'backstage.io/managed-by-location': `provider:${this.getProviderName()}`,
          'backstage.io/managed-by-origin-location': `provider:${this.getProviderName()}`,
          [CHOREO_ANNOTATIONS.COMPONENT]: component.name,
          [CHOREO_ANNOTATIONS.COMPONENT_TYPE]: component.type,
          [CHOREO_ANNOTATIONS.PROJECT]: projectName,
          [CHOREO_ANNOTATIONS.ORGANIZATION]: orgName,
          [CHOREO_ANNOTATIONS.CREATED_AT]: component.createdAt,
          [CHOREO_ANNOTATIONS.STATUS]: component.status,
          ...(component.repositoryUrl && {
            'backstage.io/source-location': `url:${component.repositoryUrl}`,
          }),
          ...(component.branch && {
            [CHOREO_ANNOTATIONS.BRANCH]: component.branch,
          }),
        },
        labels: {
          [CHOREO_LABELS.MANAGED]: 'true',
        },
      },
      spec: {
        type: backstageComponentType,
        lifecycle: component.status.toLowerCase(), // Map status to lifecycle
        owner: this.defaultOwner,
        system: projectName, // Link to the parent system (project)
        ...(providesApis && providesApis.length > 0 && { providesApis }),
      },
    };

    return componentEntity;
  }

  /**
   * Translates a ModelsCompleteComponent (Service) to a Backstage Component entity with providesApis
   */
  private translateServiceComponentToEntity(
    completeComponent: ModelsCompleteComponent,
    orgName: string,
    projectName: string,
  ): Entity {
    // Generate API names for providesApis
    const providesApis: string[] = [];
    if (completeComponent.workload?.endpoints) {
      Object.keys(completeComponent.workload.endpoints).forEach(
        endpointName => {
          providesApis.push(`${completeComponent.name}-${endpointName}`);
        },
      );
    }

    // Reuse the base translateComponentToEntity method
    return this.translateComponentToEntity(
      completeComponent,
      orgName,
      projectName,
      providesApis,
    );
  }

  /**
   * Creates API entities from a Service component's workload endpoints
   */
  private createApiEntitiesFromWorkload(
    completeComponent: ModelsCompleteComponent,
    orgName: string,
    projectName: string,
  ): Entity[] {
    const apiEntities: Entity[] = [];

    if (!completeComponent.workload?.endpoints) {
      return apiEntities;
    }

    Object.entries(completeComponent.workload.endpoints).forEach(
      ([endpointName, endpoint]) => {
        const apiEntity: Entity = {
          apiVersion: 'backstage.io/v1alpha1',
          kind: 'API',
          metadata: {
            name: `${completeComponent.name}-${endpointName}`,
            title: `${completeComponent.name} ${endpointName} API`,
            description: `${endpoint.type} endpoint for ${completeComponent.name} service on port ${endpoint.port}`,
            tags: ['openchoreo', 'api', endpoint.type.toLowerCase()],
            annotations: {
              'backstage.io/managed-by-location': `provider:${this.getProviderName()}`,
              'backstage.io/managed-by-origin-location': `provider:${this.getProviderName()}`,
              [CHOREO_ANNOTATIONS.COMPONENT]: completeComponent.name,
              [CHOREO_ANNOTATIONS.ENDPOINT_NAME]: endpointName,
              [CHOREO_ANNOTATIONS.ENDPOINT_TYPE]: endpoint.type,
              [CHOREO_ANNOTATIONS.ENDPOINT_PORT]: endpoint.port.toString(),
              [CHOREO_ANNOTATIONS.PROJECT]: projectName,
              [CHOREO_ANNOTATIONS.ORGANIZATION]: orgName,
            },
            labels: {
              'openchoreo.io/managed': 'true',
            },
          },
          spec: {
            type: this.mapWorkloadEndpointTypeToBackstageType(endpoint.type),
            lifecycle: 'production',
            owner: this.defaultOwner,
            system: projectName,
            definition: this.createApiDefinitionFromWorkloadEndpoint(endpoint),
          },
        };

        apiEntities.push(apiEntity);
      },
    );

    return apiEntities;
  }

  /**
   * Maps WorkloadEndpoint type to Backstage API spec type
   */
  private mapWorkloadEndpointTypeToBackstageType(workloadType: string): string {
    switch (workloadType) {
      case 'REST':
      case 'HTTP':
        return 'openapi';
      case 'GraphQL':
        return 'graphql';
      case 'gRPC':
        return 'grpc';
      case 'Websocket':
        return 'asyncapi';
      case 'TCP':
      case 'UDP':
        return 'openapi'; // Default to openapi for TCP/UDP
      default:
        return 'openapi';
    }
  }

  /**
   * Creates API definition from WorkloadEndpoint
   */
  private createApiDefinitionFromWorkloadEndpoint(
    endpoint: WorkloadEndpoint,
  ): string {
    if (endpoint.schema?.content) {
      return endpoint.schema.content;
    }
    return 'No schema available';

    //   // Create a basic definition based on endpoint type
    //   if (endpoint.type === 'REST' || endpoint.type === 'HTTP') {
    //     const definition = {
    //       openapi: '3.0.0',
    //       info: {
    //         title: `${endpointName} API`,
    //         version: '1.0.0',
    //         description: `${endpoint.type} API endpoint on port ${endpoint.port}`,
    //       },
    //       servers: [
    //         {
    //           url: `http://localhost:${endpoint.port}`,
    //           description: `${endpoint.type} server`,
    //         },
    //       ],
    //       paths: {
    //         '/': {
    //           get: {
    //             summary: `${endpoint.type} endpoint`,
    //             description: `${endpoint.type} endpoint on port ${endpoint.port}`,
    //             responses: {
    //               '200': {
    //                 description: 'Successful response',
    //               },
    //             },
    //           },
    //         },
    //       },
    //     };
    //     return JSON.stringify(definition, null, 2);
    //   }

    //   if (endpoint.type === 'GraphQL') {
    //     const definition = {
    //       graphql: '1.0.0',
    //       info: {
    //         title: `${endpointName} GraphQL API`,
    //         version: '1.0.0',
    //         description: `GraphQL API endpoint on port ${endpoint.port}`,
    //       },
    //       servers: [
    //         {
    //           url: `http://localhost:${endpoint.port}/graphql`,
    //           description: 'GraphQL server',
    //         },
    //       ],
    //     };
    //     return JSON.stringify(definition, null, 2);
    //   }

    //   // Default minimal definition
    //   const definition = {
    //     info: {
    //       title: `${endpointName} API`,
    //       version: '1.0.0',
    //       description: `${endpoint.type} endpoint on port ${endpoint.port}`,
    //     },
    //     type: endpoint.type,
    //     port: endpoint.port,
    //   };
    //   return JSON.stringify(definition, null, 2);
  }
}
