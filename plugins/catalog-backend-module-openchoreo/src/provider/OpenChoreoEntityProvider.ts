import {
  EntityProvider,
  EntityProviderConnection,
} from '@backstage/plugin-catalog-node';
import { Entity } from '@backstage/catalog-model';
import { SchedulerServiceTaskRunner } from '@backstage/backend-plugin-api';
import { Config } from '@backstage/config';
import { LoggerService } from '@backstage/backend-plugin-api';
import { createOpenChoreoApiClient, OpenChoreoApiClient, ModelsProject, ModelsOrganization, ModelsComponent } from '@internal/plugin-openchoreo-api';

/**
 * Provides entities from OpenChoreo API
 */
export class OpenChoreoEntityProvider implements EntityProvider {
  private readonly taskRunner: SchedulerServiceTaskRunner;
  private connection?: EntityProviderConnection;
  private readonly logger: LoggerService;
  private readonly client: OpenChoreoApiClient;

  constructor(
    taskRunner: SchedulerServiceTaskRunner,
    logger: LoggerService,
    config: Config,
  ) {
    this.taskRunner = taskRunner;
    this.logger = logger;
    this.client = createOpenChoreoApiClient(config, logger);
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
      this.logger.info('Fetching organizations and projects from OpenChoreo API');
      
      // First, get all organizations
      const organizations = await this.client.getAllOrganizations();
      this.logger.info(`Found ${organizations.length} organizations from OpenChoreo`);

      const allEntities: Entity[] = [];

      // Create Domain entities for each organization
      const domainEntities: Entity[] = organizations.map(org => 
        this.translateOrganizationToDomain(org)
      );
      allEntities.push(...domainEntities);

      // Get projects for each organization and create System entities
      for (const org of organizations) {
        try {
          const projects = await this.client.getAllProjects(org.name);
          this.logger.info(`Found ${projects.length} projects in organization: ${org.name}`);

          const systemEntities: Entity[] = projects.map(project => 
            this.translateProjectToEntity(project, org.name)
          );
          allEntities.push(...systemEntities);

          // Get components for each project and create Component entities
          for (const project of projects) {
            try {
              const components = await this.client.getAllComponents(org.name, project.name);
              this.logger.info(`Found ${components.length} components in project: ${project.name}`);

              const componentEntities: Entity[] = components.map(component => 
                this.translateComponentToEntity(component, org.name, project.name)
              );
              allEntities.push(...componentEntities);
            } catch (error) {
              this.logger.warn(`Failed to fetch components for project ${project.name} in organization ${org.name}: ${error}`);
            }
          }
        } catch (error) {
          this.logger.warn(`Failed to fetch projects for organization ${org.name}: ${error}`);
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
      const componentCount = allEntities.filter(e => e.kind === 'Component').length;
      this.logger.info(`Successfully processed ${allEntities.length} entities (${domainEntities.length} domains, ${systemCount} systems, ${componentCount} components)`);
    } catch (error) {
      this.logger.error(`Failed to run OpenChoreoEntityProvider: ${error}`);
    }
  }

  /**
   * Translates a ModelsOrganization from OpenChoreo API to a Backstage Domain entity
   */
  private translateOrganizationToDomain(organization: ModelsOrganization): Entity {
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
          'openchoreo.io/organization-id': organization.name,
          'openchoreo.io/namespace': organization.namespace,
          'openchoreo.io/created-at': organization.createdAt,
          'openchoreo.io/status': organization.status,
        },
        labels: {
          'openchoreo.io/managed': 'true',
        },
      },
      spec: {
        owner: 'guests', // This could be configured or mapped from organization metadata
      },
    };

    return domainEntity;
  }

  /**
   * Translates a ModelsProject from OpenChoreo API to a Backstage System entity
   */
  private translateProjectToEntity(project: ModelsProject, orgName: string): Entity {
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
          'openchoreo.io/project-id': project.name,
          'openchoreo.io/organization': orgName,
        },
        labels: {
          'openchoreo.io/managed': 'true',
          // ...project.metadata?.labels,
        },
      },
      spec: {
        owner: 'guests', // This could be mapped from project metadata
        domain: orgName,
      },
    };

    return systemEntity;
  }

  /**
   * Translates a ModelsComponent from OpenChoreo API to a Backstage Component entity
   */
  private translateComponentToEntity(component: ModelsComponent, orgName: string, projectName: string): Entity {

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
          'openchoreo.io/component-id': component.name,
          'openchoreo.io/component-type': component.type,
          'openchoreo.io/project': projectName,
          'openchoreo.io/organization': orgName,
          'openchoreo.io/created-at': component.createdAt,
          'openchoreo.io/status': component.status,
          ...(component.repositoryUrl && { 'backstage.io/source-location': `url:${component.repositoryUrl}` }),
          ...(component.branch && { 'openchoreo.io/branch': component.branch }),
        },
        labels: {
          'openchoreo.io/managed': 'true',
        },
      },
      spec: {
        type: backstageComponentType,
        lifecycle: component.status.toLowerCase(), // Map status to lifecycle
        owner: 'guests', // This could be mapped from component metadata
        system: projectName, // Link to the parent system (project)
      },
    };

    return componentEntity;
  }
}
