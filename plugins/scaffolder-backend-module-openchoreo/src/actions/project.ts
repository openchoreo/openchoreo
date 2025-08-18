import { createTemplateAction } from '@backstage/plugin-scaffolder-node';
import { OpenChoreoApiClient } from '@internal/plugin-openchoreo-api';
import { Config } from '@backstage/config';

export const createProjectAction = (config: Config) => {
  return createTemplateAction<{
    orgName: string;
    projectName: string;
    displayName?: string;
    description?: string;
    deploymentPipeline?: string;
  }>({
    id: 'openchoreo:project:create',
    description: 'Create OpenChoreo Project',
    schema: {
      input: {
        required: ['orgName', 'projectName'],
        type: 'object',
        properties: {
          orgName: {
            type: 'string',
            title: 'Organization Name',
            description:
              'The name of the organization to create the project in',
          },
          projectName: {
            type: 'string',
            title: 'Project Name',
            description: 'The name of the project to create',
          },
          displayName: {
            type: 'string',
            title: 'Display Name',
            description: 'The display name of the project',
          },
          description: {
            type: 'string',
            title: 'Description',
            description: 'The description of the project',
          },
          deploymentPipeline: {
            type: 'string',
            title: 'Deployment Pipeline',
            description: 'The deployment pipeline for the project',
          },
        },
      },
      output: {
        type: 'object',
        properties: {
          projectName: {
            type: 'string',
            title: 'Project Name',
            description: 'The name of the created project',
          },
          organizationName: {
            type: 'string',
            title: 'Organization Name',
            description: 'The organization where the project was created',
          },
        },
      },
    },
    async handler(ctx) {
      ctx.logger.info(
        `Creating project with parameters: ${JSON.stringify(ctx.input)}`,
      );

      // Extract organization name from domain format (e.g., "domain:default/default-org" -> "default-org")
      const extractOrgName = (fullOrgName: string): string => {
        const parts = fullOrgName.split('/');
        return parts[parts.length - 1];
      };

      const orgName = extractOrgName(ctx.input.orgName);
      ctx.logger.info(
        `Extracted organization name: ${orgName} from ${ctx.input.orgName}`,
      );

      // Get the base URL from configuration
      const baseUrl = config.getString('openchoreo.baseUrl');

      // Create a new instance of the OpenChoreoApiClient
      const client = new OpenChoreoApiClient(baseUrl, '', ctx.logger);

      try {
        const response = await client.createProject(orgName, {
          name: ctx.input.projectName,
          displayName: ctx.input.displayName,
          description: ctx.input.description,
          deploymentPipeline: ctx.input.deploymentPipeline,
        });

        ctx.logger.info(
          `Project created successfully: ${JSON.stringify(response)}`,
        );

        // Set outputs for the scaffolder
        ctx.output('projectName', response.name);
        ctx.output('organizationName', orgName);
      } catch (error) {
        ctx.logger.error(`Error creating project: ${error}`);
        throw new Error(`Failed to create project: ${error}`);
      }
    },
  });
};
