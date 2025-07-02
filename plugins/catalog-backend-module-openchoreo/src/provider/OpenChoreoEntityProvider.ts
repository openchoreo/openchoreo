import {
  EntityProvider,
  EntityProviderConnection,
} from '@backstage/plugin-catalog-node';
import { Entity } from '@backstage/catalog-model';
import { SchedulerServiceTaskRunner } from '@backstage/backend-plugin-api';
import { Config } from '@backstage/config';
import { LoggerService } from '@backstage/backend-plugin-api';
import { createOpenChoreoApiClient, OpenChoreoApiClient, ModelsProject } from '@internal/plugin-openchoreo-api';

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
      this.logger.info('Fetching projects from OpenChoreo API');
      
      // Example: Get projects for a specific organization
      // In a real implementation, you might want to iterate over multiple orgs
      // or get this from configuration
      const orgName = 'default-org'; // This could come from config
      const projects = await this.client.getAllProjects(orgName);

      this.logger.info(`Fetched ${projects[0]} projects from OpenChoreo API for organization: ${orgName}`);
      
      this.logger.info(`Found ${projects.length} projects from OpenChoreo`);

      const entities: Entity[] = projects.map(project => 
        this.translateProjectToEntity(project, orgName)
      );

      await this.connection.applyMutation({
        type: 'full',
        entities: entities.map(entity => ({
          entity,
          locationKey: `provider:${this.getProviderName()}`,
        })),
      });

      this.logger.info(`Successfully processed ${entities.length} entities`);
    } catch (error) {
      this.logger.error(`Failed to run OpenChoreoEntityProvider: ${error}`);
    }
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
        title: project.name,
        description: project.name,
        namespace: orgName,
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
}