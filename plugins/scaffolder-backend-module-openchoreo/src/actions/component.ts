import { createTemplateAction } from '@backstage/plugin-scaffolder-node';
import { OpenChoreoApiClient } from '@internal/plugin-openchoreo-api';
import { Config } from '@backstage/config';

export const createComponentAction = (config: Config) => {
  return createTemplateAction<{ 
    orgName: string; 
    projectName: string; 
    componentName: string; 
    displayName?: string; 
    description?: string; 
    componentType: string; 
    dockerImageUrl?: string; 
  }>({
    id: 'openchoreo:component:create',
    description: 'Create OpenChoreo Component',
    schema: {
      input: {
        required: ['orgName', 'projectName', 'componentName', 'componentType'],
        type: 'object',
        properties: {
          orgName: {
            type: 'string',
            title: 'Organization Name',
            description:
              'The name of the organization where the component will be created',
          },
          projectName: {
            type: 'string',
            title: 'Project Name',
            description: 'The name of the project where the component will be created',
          },
          componentName: {
            type: 'string',
            title: 'Component Name',
            description: 'The name of the component to create',
          },
          displayName: {
            type: 'string',
            title: 'Display Name',
            description: 'The display name of the component',
          },
          description: {
            type: 'string',
            title: 'Description',
            description: 'The description of the component',
          },
          componentType: {
            type: 'string',
            title: 'Component Type',
            description: 'The type of the component (e.g., Service, WebApp, ScheduledTask, APIProxy)',
          },
        },
      },
      output: {
        type: 'object',
        properties: {
          componentName: {
            type: 'string',
            title: 'Component Name',
            description: 'The name of the created component',
          },
          projectName: {
            type: 'string',
            title: 'Project Name',
            description: 'The project where the component was created',
          },
          organizationName: {
            type: 'string',
            title: 'Organization Name',
            description: 'The organization where the component was created',
          },
        },
      },
    },
    async handler(ctx) {
      ctx.logger.info(`Creating component with parameters: ${JSON.stringify(ctx.input)}`);

      // Extract organization name from domain format (e.g., "domain:default/default-org" -> "default-org")
      const extractOrgName = (fullOrgName: string): string => {
        const parts = fullOrgName.split('/');
        return parts[parts.length - 1];
      };

      // Extract project name from system format (e.g., "system:default/project-name" -> "project-name")
      const extractProjectName = (fullProjectName: string): string => {
        const parts = fullProjectName.split('/');
        return parts[parts.length - 1];
      };

      const orgName = extractOrgName(ctx.input.orgName);
      const projectName = extractProjectName(ctx.input.projectName);
      
      ctx.logger.info(`Extracted organization name: ${orgName} from ${ctx.input.orgName}`);
      ctx.logger.info(`Extracted project name: ${projectName} from ${ctx.input.projectName}`);

      // Get the base URL from configuration
      const baseUrl = config.getString('openchoreo.baseUrl');
      
      // Create a new instance of the OpenChoreoApiClient
      const client = new OpenChoreoApiClient(baseUrl, '', ctx.logger);
      
      try {
        const response = await client.createComponent(
          orgName,
          projectName,
          {
            name: ctx.input.componentName,
            displayName: ctx.input.displayName,
            description: ctx.input.description,
            type: ctx.input.componentType,
          }
        );

        ctx.logger.info(
          `Component created successfully: ${JSON.stringify(response)}`,
        );
        
        // Set outputs for the scaffolder
        ctx.output('componentName', response.name);
        ctx.output('projectName', projectName);
        ctx.output('organizationName', orgName);
      } catch (error) {
        ctx.logger.error(`Error creating component: ${error}`);
        throw new Error(
          `Failed to create component: ${error}`,
        );
      }
    },
  });
};
