import {
  LoggerService,
} from '@backstage/backend-plugin-api';

// Import types directly to avoid ES module resolution issues
import type { Project, Component } from '@wso2/cell-diagram';
import { CellDiagramService } from '../../types';
import { DefaultApiClient, ModelsCompleteComponent } from '@internal/plugin-openchoreo-api';

// Define ComponentType locally to avoid ES module issues
enum ComponentType {
  SERVICE = "service",
  WEB_APP = "web-app",
  SCHEDULED_TASK = "scheduled-task",
  MANUAL_TASK = "manual-task",
  API_PROXY = "api-proxy",
  WEB_HOOK = "web-hook",
  EVENT_HANDLER = "event-handler",
  TEST = "test",
  EXTERNAL_CONSUMER = "external-consumer",
  SYSTEM_COMPONENT = "system"
}

/**
 * Service implementation for fetching and managing Cell Diagram information.
 * @implements {CellDiagramService}
 */
export class CellDiagramInfoService implements CellDiagramService {
  private readonly logger: LoggerService;
  private readonly client: DefaultApiClient;

  /**
   * Private constructor for CellDiagramInfoService.
   * Use the static create method to instantiate.
   * @param {LoggerService} logger - Logger service instance
   * @param {string} baseUrl - Base url of openchoreo api
   * @private
   */
  public constructor(logger: LoggerService, baseUrl: string) {
    this.client = new DefaultApiClient(baseUrl, {});
    this.logger = logger;
  }

  /**
   * Fetches project information including its components and their configurations.
   * @param {Object} request - The request object
   * @param {string} request.projectName - Name of the project to fetch
   * @param {string} request.orgName - Name of the organization the project belongs to
   * @returns {Promise<Project | undefined>} Project information if found, undefined otherwise
   */
  async fetchProjectInfo({
    projectName,
    orgName,
  }: {
    projectName: string;
    orgName: string;
  }): Promise<Project | undefined> {
    try {
      const response = await this.client.componentsGet({
        orgName,
        projectName,
      });

      if (!response.ok) {
        this.logger.error(
          `Failed to fetch components for project ${projectName}`,
        );
        return undefined;
      }

      const componentsData = await response.json();
      const completeComponents: ModelsCompleteComponent[] = [];

      for (const component of componentsData.data.items) {
        try {
          const componentResponse = await this.client.componentGet({
            orgName,
            projectName,
            componentName: component.name,
          });

          if (componentResponse.ok) {
            const componentData = await componentResponse.json();
            completeComponents.push(componentData.data);
          }
        } catch (error) {
          this.logger.warn(
            `Failed to fetch component ${component.name}: ${error}`,
          );
        }
      }

      const components: Component[] = completeComponents
        .filter(
          component => {
            this.logger.info(JSON.stringify(component, null, 2));
            return component.type === 'Service' || component.type === 'WebApplication';
          }
        )
        .map(component => {
          if (component.type === 'Service') {
            // Extract API information from the Service.apis object
            const apis = component.service?.apis || {};
            const services: { [key: string]: any } = {};

            // Process each API in the Service
            Object.entries(apis).forEach(
              ([apiName, apiConfig]: [string, any]) => {
                const exposeLevels = apiConfig.rest?.exposeLevels || [];
                services[apiName] = {
                  id: component.name || '',
                  label: component.name || '',
                  type: 'SERVICE',
                  dependencyIds: [],
                  deploymentMetadata: {
                    gateways: {
                      internet: {
                        isExposed: exposeLevels.includes('Public'),
                      },
                      intranet: {
                        isExposed: exposeLevels.includes('Organization'),
                      },
                    },
                  },
                };
              },
            );

            return {
              id: component.name || '',
              label: component.name || '',
              version: '1.0.0',
              type: ComponentType.SERVICE,
              services: services,
              connections: [],
            };
          }
          if (component.type === 'WebApplication') {
            return {
              id: component.name || '',
              label: component.name || '',
              version: '1.0.0',
              type: ComponentType.WEB_APP,
              services: {
                main: {
                  id: component.name || '',
                  label: component.name || '',
                  type: 'WebApplication',
                  dependencyIds: [],
                  deploymentMetadata: {
                    gateways: {
                      internet: {
                        isExposed: true,
                      },
                      intranet: {
                        isExposed: false,
                      },
                    },
                  },
                },
              },
              connections: [],
            };
          }
          return null;
        })
        .filter((component): component is Component => component !== null);


      const project: Project = {
        id: projectName,
        name: projectName,
        modelVersion: '1.0.0',
        components: components,
        connections: [],
        configurations: [],
      };

      return project;
    } catch (error: unknown) {
      this.logger.error(
        `Error fetching project info for ${projectName}: ${error}`,
      );
      return undefined;
    }
  }

  /**
   * Static factory method to create a new CellDiagramInfoService instance.
   * @param {LoggerService} logger - Logger service instance
   * @param {string} baseUrl - Base URL of the OpenChoreo API
   * @returns {CellDiagramInfoService} New service instance
   */
  static create(
    logger: LoggerService,
    baseUrl: string,
  ): CellDiagramInfoService {
    return new CellDiagramInfoService(logger, baseUrl);
  }
}
